package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/version"
)

type SelfUpdate struct {
	cfg config.Config
	log logger.Logger
	cl  *http.Client

	mu          sync.Mutex
	lastVersion string
	lastAttempt time.Time

	policyMu sync.RWMutex
	policy   runtimeAutoUpdateOptions
}

type updateDownloadSource struct {
	URL     string
	Name    string
	UseAuth bool
}

type runtimeAutoUpdateOptions struct {
	enabledSet bool
	enabled    bool
	dirSet     bool
	dir        string
	maxMBSet   bool
	maxMB      int
	timeoutSet bool
	timeout    time.Duration
	retrySet   bool
	retry      time.Duration
	useAuthSet bool
	useAuth    bool
	commandSet bool
	command    string
	workDirSet bool
	workDir    string
}

type effectiveAutoUpdateOptions struct {
	enabled bool
	dir     string
	maxMB   int
	timeout time.Duration
	retry   time.Duration
	useAuth bool
	command string
	workDir string
}

type pendingUpdateState struct {
	TargetVersion    string `json:"target_version"`
	FromVersion      string `json:"from_version"`
	RequestedAtMs    int64  `json:"requested_at_ms"`
	DownloadFile     string `json:"download_file,omitempty"`
	DownloadDir      string `json:"download_dir,omitempty"`
	DownloadSHA256   string `json:"download_sha256,omitempty"`
	DownloadSize     int64  `json:"download_size_bytes,omitempty"`
	LauncherCommand  string `json:"launcher_command,omitempty"`
	LauncherWorkDir  string `json:"launcher_workdir,omitempty"`
	LauncherExitCode *int   `json:"launcher_exit_code,omitempty"`
}

type downloadedArtifact struct {
	FilePath  string
	DirPath   string
	SHA256    string
	SizeBytes int64
	Source    string
}

var updateHandshakeSteps = []string{
	"precheck",
	"download",
	"validation",
	"apply",
	"restart",
	"reconnect",
	"version_confirmed",
}

func NewSelfUpdate(cfg config.Config, log logger.Logger) *SelfUpdate {
	timeout := cfg.AutoUpdateTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &SelfUpdate{
		cfg: cfg,
		log: log,
		cl:  httpx.NewClient(cfg, timeout),
	}
}

func (uc *SelfUpdate) Execute(ctx context.Context, payload *UpdatePayload) error {
	if payload == nil {
		return errors.New("missing update payload")
	}
	opts := uc.effectiveOptions()
	if !opts.enabled {
		uc.log.Info(logger.KV("self update ignored",
			"version", payload.Version,
			"reason", "feature_disabled",
		))
		uc.reportStatusBestEffort(ctx, payload, "skipped", "feature_disabled", "", version.Version, updateStageEvidence("precheck", ""))
		return nil
	}
	if payload.Version == "" || payload.URL == "" {
		uc.reportStatusBestEffort(ctx, payload, "precheck_failed", "invalid_payload", "version/url required", version.Version, updateStageEvidence("precheck", "pacote"))
		return errors.New("invalid update payload: version/url required")
	}
	if !payload.Force && payload.Version == version.Version {
		uc.log.Info(logger.KV("self update skipped",
			"version", payload.Version,
			"reason", "already_current",
		))
		uc.reportStatusBestEffort(ctx, payload, "skipped", "already_current", "", version.Version, updateStageEvidence("precheck", ""))
		return nil
	}
	if uc.shouldSkip(payload.Version, opts.retry) {
		uc.log.Info(logger.KV("self update skipped",
			"version", payload.Version,
			"reason", "cooldown",
		))
		uc.reportStatusBestEffort(ctx, payload, "skipped", "cooldown", "", version.Version, updateStageEvidence("precheck", ""))
		return nil
	}

	uc.reportStatusBestEffort(ctx, payload, "precheck_ok", "", "", version.Version, updateStageEvidence("precheck", ""))
	uc.reportStatusBestEffort(ctx, payload, "download_started", "", "", version.Version, updateStageEvidence("download", ""))

	artifact, err := uc.download(ctx, payload, opts)
	if err != nil {
		uc.reportStatusBestEffort(ctx, payload, "download_failed", "download_failed", err.Error(), version.Version, updateStageEvidence("download", classifyUpdateFailure("download", "download_failed", err)))
		return err
	}

	validationMeta := uc.buildUpdateRuntimeEvidence(artifact, opts, nil)
	addUpdateStageEvidence(validationMeta, "validation", "")
	uc.reportStatusBestEffort(ctx, payload, "validation_ok", "", uc.downloadSummaryMessage(artifact), version.Version, validationMeta)

	if strings.TrimSpace(opts.command) == "" {
		uc.log.Info(logger.KV("self update downloaded",
			"version", payload.Version,
			"file", artifact.FilePath,
			"reason", "command_not_configured",
		))
		downloadMeta := uc.buildUpdateRuntimeEvidence(artifact, opts, nil)
		addUpdateStageEvidence(downloadMeta, "download", "")
		uc.reportStatusBestEffort(
			ctx,
			payload,
			"download_ok",
			"command_not_configured",
			uc.downloadSummaryMessage(artifact),
			version.Version,
			downloadMeta,
		)
		return nil
	}

	if err := uc.savePendingState(opts.dir, payload.Version, version.Version, artifact, opts); err != nil {
		uc.log.Error(logger.KV("self update pending state save failed",
			"version", payload.Version,
			"err", err,
		))
	}
	uc.reportStatusBestEffort(
		ctx,
		payload,
		"apply_started",
		"",
		uc.applySummaryMessage("apply_started", artifact),
		version.Version,
		withUpdateStage(uc.buildUpdateRuntimeEvidence(artifact, opts, nil), "apply", ""),
	)

	launcherExitCode, err := uc.runCommand(ctx, payload, artifact.FilePath, opts)
	if err != nil {
		_ = uc.clearPendingState(opts.dir)
		failureClass := classifyUpdateFailure("apply", "command_failed", err)
		uc.reportStatusBestEffort(
			ctx,
			payload,
			"apply_failed",
			updateFailureReasonCode("command_failed", failureClass),
			err.Error(),
			version.Version,
			withUpdateStage(uc.buildUpdateRuntimeEvidence(artifact, opts, launcherExitCode), "apply", failureClass),
		)
		return err
	}
	if launcherExitCode != nil {
		if err := uc.updatePendingLauncherExitCode(opts.dir, *launcherExitCode); err != nil {
			uc.log.Error(logger.KV("self update pending state exit code save failed",
				"version", payload.Version,
				"err", err,
			))
		}
	}

	uc.log.Info(logger.KV("self update command executed",
		"version", payload.Version,
		"file", artifact.FilePath,
	))
	uc.reportStatusBestEffort(
		ctx,
		payload,
		"apply_dispatched",
		"",
		uc.applySummaryMessage("apply_dispatched", artifact),
		version.Version,
		withUpdateStage(uc.buildUpdateRuntimeEvidence(artifact, opts, launcherExitCode), "restart", ""),
	)
	return nil
}

func (uc *SelfUpdate) shouldSkip(targetVersion string, retry time.Duration) bool {
	if retry <= 0 {
		retry = 30 * time.Minute
	}
	now := time.Now()
	uc.mu.Lock()
	defer uc.mu.Unlock()

	if uc.lastVersion == targetVersion && now.Sub(uc.lastAttempt) < retry {
		return true
	}
	uc.lastVersion = targetVersion
	uc.lastAttempt = now
	return false
}

func (uc *SelfUpdate) download(ctx context.Context, payload *UpdatePayload, opts effectiveAutoUpdateOptions) (downloadedArtifact, error) {
	timeout := opts.timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	dlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	parsed, err := neturl.Parse(payload.URL)
	if err != nil {
		return downloadedArtifact{}, fmt.Errorf("invalid update url: %w", err)
	}

	name := filepath.Base(parsed.Path)
	if name == "" || name == "." || name == "/" {
		name = "aiceberg_agent_update.bin"
	}

	rootDir := opts.dir
	if strings.TrimSpace(rootDir) == "" {
		rootDir = "./data/updates"
	}
	targetDir := filepath.Join(rootDir, sanitizePathSegment(payload.Version))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return downloadedArtifact{}, fmt.Errorf("create update dir: %w", err)
	}
	finalPath := filepath.Join(targetDir, name)
	tmpPath := finalPath + ".part"

	maxMB := opts.maxMB
	if maxMB <= 0 {
		maxMB = 300
	}
	maxBytes := int64(maxMB) * 1024 * 1024
	expectedSHA := normalizeSHA256(payload.SHA256)
	sources := uc.downloadSources(payload.URL, opts.useAuth)
	var errs []string

	for _, source := range sources {
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			return downloadedArtifact{}, fmt.Errorf("cleanup temp update file: %w", err)
		}

		req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, source.URL, nil)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s request: %v", source.Name, err))
			continue
		}
		if source.UseAuth {
			httpx.SetAuth(req, uc.cfg)
		}

		resp, err := uc.cl.Do(req)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s download: %v", source.Name, err))
			continue
		}

		downloadErr := func() error {
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("status %s", resp.Status)
			}

			f, err := os.Create(tmpPath)
			if err != nil {
				return fmt.Errorf("create temp update file: %w", err)
			}
			defer func() { _ = f.Close() }()

			hash := sha256.New()
			limited := io.LimitReader(resp.Body, maxBytes+1)
			written, copyErr := io.Copy(io.MultiWriter(f, hash), limited)
			if copyErr != nil {
				return fmt.Errorf("download body: %w", copyErr)
			}
			if written > maxBytes {
				return fmt.Errorf("update package too large: %d bytes (limit %d)", written, maxBytes)
			}
			if closeErr := f.Close(); closeErr != nil {
				return fmt.Errorf("flush update file: %w", closeErr)
			}

			if expectedSHA != "" {
				got := hex.EncodeToString(hash.Sum(nil))
				if got != expectedSHA {
					return fmt.Errorf("sha256 mismatch: got=%s expected=%s", got, expectedSHA)
				}
			}

			if err := os.Rename(tmpPath, finalPath); err != nil {
				return fmt.Errorf("finalize update file: %w", err)
			}
			return nil
		}()

		if downloadErr == nil {
			gotSHA := normalizeSHA256(expectedSHA)
			if gotSHA == "" {
				hashRaw, hashErr := fileSHA256(finalPath)
				if hashErr != nil {
					errs = append(errs, fmt.Sprintf("%s compute sha: %v", source.Name, hashErr))
					continue
				}
				gotSHA = normalizeSHA256(hashRaw)
			}
			info, statErr := os.Stat(finalPath)
			if statErr != nil {
				errs = append(errs, fmt.Sprintf("%s stat file: %v", source.Name, statErr))
				continue
			}
			uc.log.Info(logger.KV("self update download source",
				"source", source.Name,
			))
			return downloadedArtifact{
				FilePath:  finalPath,
				DirPath:   filepath.Dir(finalPath),
				SHA256:    gotSHA,
				SizeBytes: info.Size(),
				Source:    source.Name,
			}, nil
		}
		errs = append(errs, fmt.Sprintf("%s %v", source.Name, downloadErr))
	}

	if len(errs) == 0 {
		return downloadedArtifact{}, errors.New("download update failed: no source")
	}
	return downloadedArtifact{}, fmt.Errorf("download update failed: %s", strings.Join(errs, " | "))
}

func (uc *SelfUpdate) downloadSources(rawURL string, useAuth bool) []updateDownloadSource {
	if strings.EqualFold(uc.cfg.AgentMode, "relay") && strings.TrimSpace(uc.cfg.HubURL) != "" {
		proxyURL := buildHubUpdateProxyURL(uc.cfg.HubURL, rawURL, useAuth)
		return []updateDownloadSource{{
			URL:     proxyURL,
			Name:    "hub_proxy",
			UseAuth: true,
		}}
	}
	return []updateDownloadSource{{
		URL:     rawURL,
		Name:    "direct",
		UseAuth: useAuth,
	}}
}

func buildHubUpdateProxyURL(hubBaseURL, targetURL string, useAgentAuth bool) string {
	base := strings.TrimRight(strings.TrimSpace(hubBaseURL), "/")
	q := neturl.Values{}
	q.Set("url", targetURL)
	if useAgentAuth {
		q.Set("use_agent_auth", "1")
	}
	return base + "/v1/agent/update/download?" + q.Encode()
}

func (uc *SelfUpdate) runCommand(ctx context.Context, payload *UpdatePayload, filePath string, opts effectiveAutoUpdateOptions) (*int, error) {
	timeout := opts.timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmdLine := strings.TrimSpace(opts.command)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", cmdLine)
	} else {
		cmd = exec.CommandContext(cmdCtx, "/bin/sh", "-c", cmdLine)
	}

	workDir := strings.TrimSpace(opts.workDir)
	if workDir == "" {
		workDir = filepath.Dir(filePath)
	}
	cmd.Dir = workDir

	exePath, _ := os.Executable()
	cmd.Env = append(os.Environ(),
		"AICEBERG_UPDATE_VERSION="+payload.Version,
		"AICEBERG_UPDATE_URL="+payload.URL,
		"AICEBERG_UPDATE_SHA256="+payload.SHA256,
		"AICEBERG_UPDATE_FILE="+filePath,
		"AICEBERG_UPDATE_DIR="+filepath.Dir(filePath),
		"AICEBERG_AGENT_VERSION_CURRENT="+version.Version,
		"AICEBERG_AGENT_BIN="+exePath,
	)

	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(truncateForLog(string(out), 1500))
	if len(out) > 0 {
		uc.log.Info(logger.KV("self update command output",
			"version", payload.Version,
			"output", output,
		))
	}
	exitCode := 0
	if err != nil {
		message := fmt.Sprintf("self update command failed: %v", err)
		if output != "" {
			message += ": " + output
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			return &exitCode, errors.New(message)
		}
		return nil, errors.New(message)
	}
	return &exitCode, nil
}

func (uc *SelfUpdate) ApplyRemoteConfig(payload *AutoUpdatePayload) {
	if payload == nil {
		return
	}
	uc.policyMu.Lock()
	defer uc.policyMu.Unlock()

	// O payload remoto é a fonte de verdade para o override em runtime.
	// Sempre resetamos antes de aplicar para permitir "limpar" campos via backend.
	uc.policy = runtimeAutoUpdateOptions{}

	if payload.Enabled != nil {
		uc.policy.enabledSet = true
		uc.policy.enabled = *payload.Enabled
	}
	if payload.Dir != nil {
		uc.policy.dirSet = true
		uc.policy.dir = strings.TrimSpace(*payload.Dir)
	}
	if payload.MaxMB != nil {
		uc.policy.maxMBSet = true
		uc.policy.maxMB = *payload.MaxMB
	}
	if payload.TimeoutSec != nil {
		uc.policy.timeoutSet = true
		uc.policy.timeout = time.Duration(*payload.TimeoutSec) * time.Second
	}
	if payload.RetryIntervalSec != nil {
		uc.policy.retrySet = true
		uc.policy.retry = time.Duration(*payload.RetryIntervalSec) * time.Second
	}
	if payload.UseAgentAuth != nil {
		uc.policy.useAuthSet = true
		uc.policy.useAuth = *payload.UseAgentAuth
	}
	if payload.Command != nil {
		uc.policy.commandSet = true
		uc.policy.command = strings.TrimSpace(*payload.Command)
	}
	if payload.WorkDir != nil {
		uc.policy.workDirSet = true
		uc.policy.workDir = strings.TrimSpace(*payload.WorkDir)
	}
}

func (uc *SelfUpdate) effectiveOptions() effectiveAutoUpdateOptions {
	opts := effectiveAutoUpdateOptions{
		enabled: uc.cfg.AutoUpdateEnabled,
		dir:     uc.cfg.AutoUpdateDir,
		maxMB:   uc.cfg.AutoUpdateMaxMB,
		timeout: uc.cfg.AutoUpdateTimeout,
		retry:   uc.cfg.AutoUpdateRetryInterval,
		useAuth: uc.cfg.AutoUpdateUseAgentAuth,
		command: uc.cfg.AutoUpdateCommand,
		workDir: uc.cfg.AutoUpdateWorkDir,
	}

	uc.policyMu.RLock()
	defer uc.policyMu.RUnlock()
	if uc.policy.enabledSet {
		opts.enabled = uc.policy.enabled
	}
	if uc.policy.dirSet {
		opts.dir = uc.policy.dir
	}
	if uc.policy.maxMBSet {
		opts.maxMB = uc.policy.maxMB
	}
	if uc.policy.timeoutSet {
		opts.timeout = uc.policy.timeout
	}
	if uc.policy.retrySet {
		opts.retry = uc.policy.retry
	}
	if uc.policy.useAuthSet {
		opts.useAuth = uc.policy.useAuth
	}
	if uc.policy.commandSet {
		opts.command = uc.policy.command
	}
	if uc.policy.workDirSet {
		opts.workDir = uc.policy.workDir
	}
	return opts
}

func (uc *SelfUpdate) Snapshot() map[string]any {
	opts := uc.effectiveOptions()
	out := map[string]any{
		"enabled_effective":  opts.enabled,
		"download_dir":       strings.TrimSpace(opts.dir),
		"max_mb":             opts.maxMB,
		"timeout_sec":        int(opts.timeout.Seconds()),
		"retry_interval_sec": int(opts.retry.Seconds()),
		"use_agent_auth":     opts.useAuth,
		"command":            strings.TrimSpace(opts.command),
		"workdir":            strings.TrimSpace(opts.workDir),
	}
	if st, err := uc.loadPendingState(opts.dir); err == nil && st != nil {
		out["pending_state"] = map[string]any{
			"target_version":      strings.TrimSpace(st.TargetVersion),
			"from_version":        strings.TrimSpace(st.FromVersion),
			"requested_at_ms":     st.RequestedAtMs,
			"download_file":       strings.TrimSpace(st.DownloadFile),
			"download_dir":        strings.TrimSpace(st.DownloadDir),
			"download_sha256":     normalizeSHA256(st.DownloadSHA256),
			"download_size_bytes": st.DownloadSize,
			"launcher_command":    strings.TrimSpace(st.LauncherCommand),
			"launcher_workdir":    strings.TrimSpace(st.LauncherWorkDir),
			"launcher_exit_code":  st.LauncherExitCode,
		}
	} else if err != nil {
		out["pending_state_error"] = err.Error()
	}
	return out
}

func (uc *SelfUpdate) ReportPendingResult(ctx context.Context) error {
	opts := uc.effectiveOptions()
	st, err := uc.loadPendingState(opts.dir)
	if err != nil || st == nil {
		return err
	}

	payload := &UpdatePayload{Version: st.TargetVersion}
	artifact := downloadedArtifact{
		FilePath:  strings.TrimSpace(st.DownloadFile),
		DirPath:   strings.TrimSpace(st.DownloadDir),
		SHA256:    strings.TrimSpace(st.DownloadSHA256),
		SizeBytes: st.DownloadSize,
		Source:    "pending_state",
	}
	reportMeta := uc.buildUpdateRuntimeEvidence(artifact, opts, st.LauncherExitCode)
	reconnectMeta := copyUpdateEvidence(reportMeta)
	addUpdateStageEvidence(reconnectMeta, "reconnect", "")
	if err := uc.reportStatus(
		ctx,
		payload,
		"reconnected",
		"",
		"agente reconectou após restart",
		version.Version,
		reconnectMeta,
	); err != nil {
		return err
	}

	if version.Version == st.TargetVersion {
		confirmedMeta := copyUpdateEvidence(reportMeta)
		addUpdateStageEvidence(confirmedMeta, "version_confirmed", "")
		if err := uc.reportStatus(
			ctx,
			payload,
			"version_confirmed",
			"",
			uc.applySummaryMessage("apply_ok", artifact),
			version.Version,
			confirmedMeta,
		); err != nil {
			return err
		}
		return uc.clearPendingState(opts.dir)
	}

	if err := uc.reportStatus(
		ctx,
		payload,
		"apply_failed",
		"version_mismatch_after_restart",
		fmt.Sprintf("expected target=%s current=%s", st.TargetVersion, version.Version),
		version.Version,
		withUpdateStage(reportMeta, "version_confirmed", "reconexao"),
	); err != nil {
		return err
	}
	return uc.clearPendingState(opts.dir)
}

func (uc *SelfUpdate) reportStatusBestEffort(ctx context.Context, payload *UpdatePayload, status, reasonCode, reasonMessage, currentVersion string, updateMeta map[string]any) {
	if err := uc.reportStatus(ctx, payload, status, reasonCode, reasonMessage, currentVersion, updateMeta); err != nil {
		uc.log.Error(logger.KV("self update report failed",
			"status", status,
			"version", payloadVersion(payload),
			"err", err,
		))
	}
}

func (uc *SelfUpdate) reportStatus(ctx context.Context, payload *UpdatePayload, status, reasonCode, reasonMessage, currentVersion string, updateMeta map[string]any) error {
	if strings.TrimSpace(status) == "" {
		return nil
	}
	body := map[string]any{
		"status":          status,
		"target_version":  payloadVersion(payload),
		"current_version": strings.TrimSpace(currentVersion),
		"reason_code":     strings.TrimSpace(reasonCode),
		"reason_message":  strings.TrimSpace(reasonMessage),
		"ts_unix_ms":      time.Now().UnixMilli(),
	}
	if len(updateMeta) > 0 {
		body["update"] = updateMeta
	}
	enc, err := json.Marshal(body)
	if err != nil {
		return err
	}

	urls := []string{}
	if strings.EqualFold(uc.cfg.AgentMode, "relay") && strings.TrimSpace(uc.cfg.HubURL) != "" {
		urls = append(urls, strings.TrimRight(strings.TrimSpace(uc.cfg.HubURL), "/")+"/v1/agent/update-report")
	} else {
		urls = append(urls, uc.cfg.APIEndpoint("/v1/agent/update-report"))
	}

	var errs []string
	for _, url := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(enc)))
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		httpx.SetAuth(req, uc.cfg)
		resp, err := uc.cl.Do(req)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		errs = append(errs, fmt.Sprintf("%s status=%d", url, resp.StatusCode))
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(errs, " | "))
}

func (uc *SelfUpdate) pendingStatePath(rootDir string) string {
	dir := strings.TrimSpace(rootDir)
	if dir == "" {
		dir = "./data/updates"
	}
	return filepath.Join(dir, ".pending_update.json")
}

func (uc *SelfUpdate) savePendingState(rootDir, targetVersion, fromVersion string, artifact downloadedArtifact, opts effectiveAutoUpdateOptions) error {
	path := uc.pendingStatePath(rootDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data := pendingUpdateState{
		TargetVersion:   strings.TrimSpace(targetVersion),
		FromVersion:     strings.TrimSpace(fromVersion),
		RequestedAtMs:   time.Now().UnixMilli(),
		DownloadFile:    strings.TrimSpace(artifact.FilePath),
		DownloadDir:     strings.TrimSpace(artifact.DirPath),
		DownloadSHA256:  normalizeSHA256(artifact.SHA256),
		DownloadSize:    artifact.SizeBytes,
		LauncherCommand: strings.TrimSpace(opts.command),
		LauncherWorkDir: strings.TrimSpace(opts.workDir),
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (uc *SelfUpdate) loadPendingState(rootDir string) (*pendingUpdateState, error) {
	path := uc.pendingStatePath(rootDir)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out pendingUpdateState
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.TargetVersion) == "" {
		return nil, nil
	}
	return &out, nil
}

func (uc *SelfUpdate) clearPendingState(rootDir string) error {
	path := uc.pendingStatePath(rootDir)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (uc *SelfUpdate) updatePendingLauncherExitCode(rootDir string, exitCode int) error {
	st, err := uc.loadPendingState(rootDir)
	if err != nil || st == nil {
		return err
	}
	st.LauncherExitCode = &exitCode
	path := uc.pendingStatePath(rootDir)
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func payloadVersion(payload *UpdatePayload) string {
	if payload == nil {
		return ""
	}
	return strings.TrimSpace(payload.Version)
}

func (uc *SelfUpdate) buildUpdateRuntimeEvidence(artifact downloadedArtifact, opts effectiveAutoUpdateOptions, launcherExitCode *int) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(artifact.FilePath) != "" {
		out["download_file"] = strings.TrimSpace(artifact.FilePath)
	}
	if strings.TrimSpace(artifact.DirPath) != "" {
		out["download_dir"] = strings.TrimSpace(artifact.DirPath)
	}
	if artifact.SizeBytes > 0 {
		out["download_size_bytes"] = artifact.SizeBytes
	}
	if sha := normalizeSHA256(artifact.SHA256); sha != "" {
		out["download_sha256"] = sha
	}
	if strings.TrimSpace(artifact.Source) != "" {
		out["download_source"] = strings.TrimSpace(artifact.Source)
	}
	if strings.TrimSpace(opts.workDir) != "" {
		out["launcher_workdir"] = strings.TrimSpace(opts.workDir)
	}
	if strings.TrimSpace(opts.command) != "" {
		out["launcher_command"] = strings.TrimSpace(opts.command)
	}
	if launcherExitCode != nil {
		out["launcher_exit_code"] = *launcherExitCode
	}
	return out
}

func updateStageEvidence(stage, failureClass string) map[string]any {
	out := map[string]any{}
	addUpdateStageEvidence(out, stage, failureClass)
	return out
}

func withUpdateStage(meta map[string]any, stage, failureClass string) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	addUpdateStageEvidence(meta, stage, failureClass)
	return meta
}

func addUpdateStageEvidence(meta map[string]any, stage, failureClass string) {
	meta["handshake_steps"] = append([]string(nil), updateHandshakeSteps...)
	if normalized := strings.TrimSpace(stage); normalized != "" {
		meta["stage"] = normalized
	}
	if normalized := strings.TrimSpace(failureClass); normalized != "" {
		meta["failure_class"] = normalized
	}
}

func copyUpdateEvidence(meta map[string]any) map[string]any {
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func classifyUpdateFailure(stage, reasonCode string, err error) string {
	msg := strings.ToLower(strings.TrimSpace(reasonCode + " " + stage))
	if err != nil {
		msg += " " + strings.ToLower(err.Error())
	}
	switch {
	case strings.Contains(msg, "sudo") ||
		strings.Contains(msg, "root") ||
		strings.Contains(msg, "elev") ||
		strings.Contains(msg, "privilege") ||
		strings.Contains(msg, "permiss"):
		return "sudoers"
	case strings.Contains(msg, "download") ||
		strings.Contains(msg, "status ") ||
		strings.Contains(msg, "url") ||
		strings.Contains(msg, "timeout"):
		return "download"
	case strings.Contains(msg, "sha256") ||
		strings.Contains(msg, "too large") ||
		strings.Contains(msg, "payload") ||
		strings.Contains(msg, "pacote") ||
		strings.Contains(msg, "package"):
		return "pacote"
	case strings.Contains(msg, "tar") ||
		strings.Contains(msg, "unzip") ||
		strings.Contains(msg, "expand") ||
		strings.Contains(msg, "formato") ||
		strings.Contains(msg, "binário") ||
		strings.Contains(msg, "binary") ||
		strings.Contains(msg, "agent.exe"):
		return "unpack"
	case strings.Contains(msg, "restart") ||
		strings.Contains(msg, "rein") ||
		strings.Contains(msg, "service") ||
		strings.Contains(msg, "systemctl"):
		return "restart"
	case strings.Contains(msg, "reconnect") ||
		strings.Contains(msg, "reconex") ||
		strings.Contains(msg, "version_mismatch"):
		return "reconexao"
	case strings.Contains(msg, "report"):
		return "report"
	default:
		return "pacote"
	}
}

func updateFailureReasonCode(reasonCode, failureClass string) string {
	reasonCode = strings.TrimSpace(reasonCode)
	failureClass = strings.TrimSpace(failureClass)
	if failureClass == "" {
		return reasonCode
	}
	if reasonCode == "" || reasonCode == "command_failed" || reasonCode == "failed" || reasonCode == "error" {
		return failureClass
	}
	return reasonCode
}

func (uc *SelfUpdate) downloadSummaryMessage(artifact downloadedArtifact) string {
	file := strings.TrimSpace(artifact.FilePath)
	if file == "" {
		return ""
	}
	parts := []string{"downloaded"}
	parts = append(parts, file)
	if artifact.SizeBytes > 0 {
		parts = append(parts, fmt.Sprintf("size=%d", artifact.SizeBytes))
	}
	if sha := normalizeSHA256(artifact.SHA256); sha != "" {
		parts = append(parts, "sha256="+sha)
	}
	return strings.Join(parts, " | ")
}

func (uc *SelfUpdate) applySummaryMessage(status string, artifact downloadedArtifact) string {
	label := strings.TrimSpace(status)
	if label == "" {
		label = "apply"
	}
	file := strings.TrimSpace(artifact.FilePath)
	if file == "" {
		return label
	}
	return fmt.Sprintf("%s | file=%s", label, file)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sanitizePathSegment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "latest"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "latest"
	}
	return out
}

func normalizeSHA256(s string) string {
	normalized := strings.ToLower(strings.TrimSpace(s))
	normalized = strings.TrimPrefix(normalized, "sha256:")
	normalized = strings.TrimSpace(normalized)
	if len(normalized) != 64 {
		return ""
	}
	for _, r := range normalized {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return ""
		}
	}
	return normalized
}

func truncateForLog(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}
