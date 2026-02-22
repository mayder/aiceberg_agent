package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
}

type updateDownloadSource struct {
	URL     string
	Name    string
	UseAuth bool
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
	if !uc.cfg.AutoUpdateEnabled {
		uc.log.Info(logger.KV("self update ignored",
			"version", payload.Version,
			"reason", "feature_disabled",
		))
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
		return nil
	}
	if uc.shouldSkip(payload.Version) {
		uc.log.Info(logger.KV("self update skipped",
			"version", payload.Version,
			"reason", "cooldown",
		))
		return nil
	}

	localFile, err := uc.download(ctx, payload)
	if err != nil {
		return err
	}

	if strings.TrimSpace(uc.cfg.AutoUpdateCommand) == "" {
		uc.log.Info(logger.KV("self update downloaded",
			"version", payload.Version,
			"file", localFile,
			"reason", "command_not_configured",
		))
		return nil
	}

	if err := uc.runCommand(ctx, payload, localFile); err != nil {
		return err
	}

	uc.log.Info(logger.KV("self update command executed",
		"version", payload.Version,
		"file", localFile,
	))
	return nil
}

func (uc *SelfUpdate) shouldSkip(targetVersion string) bool {
	retry := uc.cfg.AutoUpdateRetryInterval
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

func (uc *SelfUpdate) download(ctx context.Context, payload *UpdatePayload) (string, error) {
	timeout := uc.cfg.AutoUpdateTimeout
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

	rootDir := uc.cfg.AutoUpdateDir
	if strings.TrimSpace(rootDir) == "" {
		rootDir = "./data/updates"
	}
	targetDir := filepath.Join(rootDir, sanitizePathSegment(payload.Version))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create update dir: %w", err)
	}
	finalPath := filepath.Join(targetDir, name)
	tmpPath := finalPath + ".part"

	maxMB := uc.cfg.AutoUpdateMaxMB
	if maxMB <= 0 {
		maxMB = 300
	}
	maxBytes := int64(maxMB) * 1024 * 1024
	expectedSHA := normalizeSHA256(payload.SHA256)
	sources := uc.downloadSources(payload.URL)
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

func (uc *SelfUpdate) downloadSources(rawURL string) []updateDownloadSource {
	sources := []updateDownloadSource{{
		URL:     rawURL,
		Name:    "direct",
		UseAuth: uc.cfg.AutoUpdateUseAgentAuth,
	}}
	if strings.EqualFold(uc.cfg.AgentMode, "relay") && strings.TrimSpace(uc.cfg.HubURL) != "" {
		proxyURL := buildHubUpdateProxyURL(uc.cfg.HubURL, rawURL, uc.cfg.AutoUpdateUseAgentAuth)
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

func (uc *SelfUpdate) runCommand(ctx context.Context, payload *UpdatePayload, filePath string) error {
	timeout := uc.cfg.AutoUpdateTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmdLine := strings.TrimSpace(uc.cfg.AutoUpdateCommand)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", cmdLine)
	} else {
		cmd = exec.CommandContext(cmdCtx, "/bin/sh", "-c", cmdLine)
	}

	workDir := strings.TrimSpace(uc.cfg.AutoUpdateWorkDir)
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
