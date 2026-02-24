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
	TargetVersion string `json:"target_version"`
	FromVersion   string `json:"from_version"`
	RequestedAtMs int64  `json:"requested_at_ms"`
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
		uc.reportStatusBestEffort(ctx, payload, "skipped", "feature_disabled", "", version.Version)
		return nil
	}
	if payload.Version == "" || payload.URL == "" {
		return errors.New("invalid update payload: version/url required")
	}
	if !payload.Force && payload.Version == version.Version {
		uc.log.Info(logger.KV("self update skipped",
			"version", payload.Version,
			"reason", "already_current",
		))
		uc.reportStatusBestEffort(ctx, payload, "skipped", "already_current", "", version.Version)
		return nil
	}
	if uc.shouldSkip(payload.Version, opts.retry) {
		uc.log.Info(logger.KV("self update skipped",
			"version", payload.Version,
			"reason", "cooldown",
		))
		uc.reportStatusBestEffort(ctx, payload, "skipped", "cooldown", "", version.Version)
		return nil
	}

	localFile, err := uc.download(ctx, payload, opts)
	if err != nil {
		uc.reportStatusBestEffort(ctx, payload, "download_failed", "download_failed", err.Error(), version.Version)
		return err
	}

	if strings.TrimSpace(opts.command) == "" {
		uc.log.Info(logger.KV("self update downloaded",
			"version", payload.Version,
			"file", localFile,
			"reason", "command_not_configured",
		))
		uc.reportStatusBestEffort(ctx, payload, "download_ok", "command_not_configured", "", version.Version)
		return nil
	}

	if err := uc.savePendingState(opts.dir, payload.Version, version.Version); err != nil {
		uc.log.Error(logger.KV("self update pending state save failed",
			"version", payload.Version,
			"err", err,
		))
	}
	uc.reportStatusBestEffort(ctx, payload, "apply_started", "", "", version.Version)

	if err := uc.runCommand(ctx, payload, localFile, opts); err != nil {
		_ = uc.clearPendingState(opts.dir)
		uc.reportStatusBestEffort(ctx, payload, "apply_failed", "command_failed", err.Error(), version.Version)
		return err
	}

	uc.log.Info(logger.KV("self update command executed",
		"version", payload.Version,
		"file", localFile,
	))
	uc.reportStatusBestEffort(ctx, payload, "apply_dispatched", "", "", version.Version)
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

func (uc *SelfUpdate) download(ctx context.Context, payload *UpdatePayload, opts effectiveAutoUpdateOptions) (string, error) {
	timeout := opts.timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	dlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	parsed, err := neturl.Parse(payload.URL)
	if err != nil {
		return "", fmt.Errorf("invalid update url: %w", err)
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
		return "", fmt.Errorf("create update dir: %w", err)
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
			return "", fmt.Errorf("cleanup temp update file: %w", err)
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
			uc.log.Info(logger.KV("self update download source",
				"source", source.Name,
			))
			return finalPath, nil
		}
		errs = append(errs, fmt.Sprintf("%s %v", source.Name, downloadErr))
	}

	if len(errs) == 0 {
		return "", errors.New("download update failed: no source")
	}
	return "", fmt.Errorf("download update failed: %s", strings.Join(errs, " | "))
}

func (uc *SelfUpdate) downloadSources(rawURL string, useAuth bool) []updateDownloadSource {
	sources := []updateDownloadSource{{
		URL:     rawURL,
		Name:    "direct",
		UseAuth: useAuth,
	}}
	if strings.EqualFold(uc.cfg.AgentMode, "relay") && strings.TrimSpace(uc.cfg.HubURL) != "" {
		proxyURL := buildHubUpdateProxyURL(uc.cfg.HubURL, rawURL, useAuth)
		sources = append([]updateDownloadSource{{
			URL:     proxyURL,
			Name:    "hub_proxy",
			UseAuth: true,
		}}, sources...)
	}
	return sources
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

func (uc *SelfUpdate) runCommand(ctx context.Context, payload *UpdatePayload, filePath string, opts effectiveAutoUpdateOptions) error {
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
	if len(out) > 0 {
		uc.log.Info(logger.KV("self update command output",
			"version", payload.Version,
			"output", truncateForLog(string(out), 1500),
		))
	}
	if err != nil {
		return fmt.Errorf("self update command failed: %w", err)
	}
	return nil
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

func (uc *SelfUpdate) ReportPendingResult(ctx context.Context) error {
	opts := uc.effectiveOptions()
	st, err := uc.loadPendingState(opts.dir)
	if err != nil || st == nil {
		return err
	}

	payload := &UpdatePayload{Version: st.TargetVersion}
	if version.Version == st.TargetVersion {
		if err := uc.reportStatus(ctx, payload, "apply_ok", "", "", version.Version); err != nil {
			return err
		}
		return uc.clearPendingState(opts.dir)
	}

	if err := uc.reportStatus(ctx, payload, "apply_failed", "version_mismatch_after_restart", "", version.Version); err != nil {
		return err
	}
	return uc.clearPendingState(opts.dir)
}

func (uc *SelfUpdate) reportStatusBestEffort(ctx context.Context, payload *UpdatePayload, status, reasonCode, reasonMessage, currentVersion string) {
	if err := uc.reportStatus(ctx, payload, status, reasonCode, reasonMessage, currentVersion); err != nil {
		uc.log.Error(logger.KV("self update report failed",
			"status", status,
			"version", payloadVersion(payload),
			"err", err,
		))
	}
}

func (uc *SelfUpdate) reportStatus(ctx context.Context, payload *UpdatePayload, status, reasonCode, reasonMessage, currentVersion string) error {
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
	enc, err := json.Marshal(body)
	if err != nil {
		return err
	}

	urls := []string{}
	if strings.EqualFold(uc.cfg.AgentMode, "relay") && strings.TrimSpace(uc.cfg.HubURL) != "" {
		urls = append(urls, strings.TrimRight(strings.TrimSpace(uc.cfg.HubURL), "/")+"/v1/agent/update-report")
	}
	urls = append(urls, uc.cfg.APIEndpoint("/v1/agent/update-report"))

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

func (uc *SelfUpdate) savePendingState(rootDir, targetVersion, fromVersion string) error {
	path := uc.pendingStatePath(rootDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data := pendingUpdateState{
		TargetVersion: strings.TrimSpace(targetVersion),
		FromVersion:   strings.TrimSpace(fromVersion),
		RequestedAtMs: time.Now().UnixMilli(),
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

func payloadVersion(payload *UpdatePayload) string {
	if payload == nil {
		return ""
	}
	return strings.TrimSpace(payload.Version)
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
