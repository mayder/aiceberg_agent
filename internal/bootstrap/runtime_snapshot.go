package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	bolt "go.etcd.io/bbolt"
)

func buildSelfHealRuntimeSnapshot(
	cfg config.Config,
	mode string,
	prefs config.CollectPrefs,
	settings usecase.AgentlessSettings,
	workerAvailable bool,
	selfUpdateUC *usecase.SelfUpdate,
) map[string]any {
	out := map[string]any{
		"agent_mode_runtime":          mode,
		"agentless_enabled_env":       cfg.AgentlessEnabled,
		"agentless_enabled_prefs":     prefs.AgentlessEnabled,
		"agentless_effective_enabled": settings.Enabled,
		"prefs_version":               strings.TrimSpace(prefs.Version),
		"worker_available":            workerAvailable,
		"agentless_runtime": map[string]any{
			"poll_sec":    settings.PollSec,
			"flush_sec":   settings.FlushSec,
			"jobs_limit":  settings.JobsLimit,
			"lock_sec":    settings.LockSec,
			"flush_batch": settings.FlushBatch,
		},
		"prefs_snapshot": sanitizePrefsSnapshot(prefs),
		"local_state": map[string]any{
			"prefs_store":         inspectLocalStateEntry(cfg.PrefsPath, true),
			"outbox_store":        inspectLocalStateEntry(cfg.OutboxPath, false),
			"agentless_outbox":    inspectLocalStateEntry(cfg.AgentlessOutboxPath, false),
			"agentless_targets":   inspectLocalStateEntry(os.Getenv("AGENTLESS_TARGETS_PATH"), true),
			"agent_token_storage": inspectLocalStateEntry(os.Getenv("AGENT_TOKEN_PATH"), false),
		},
	}
	out["agent_env"] = buildAgentEnvSnapshot(cfg)
	if selfUpdateUC != nil {
		out["auto_update_runtime"] = selfUpdateUC.Snapshot()
	}
	return out
}

func sanitizePrefsSnapshot(p config.CollectPrefs) map[string]any {
	return map[string]any{
		"version":                  strings.TrimSpace(p.Version),
		"paused":                   p.Paused,
		"agentless_enabled":        p.AgentlessEnabled,
		"agentless_poll_interval":  p.AgentlessPollSec,
		"agentless_flush_interval": p.AgentlessFlushSec,
		"agentless_jobs_limit":     p.AgentlessJobsLimit,
		"agentless_lock_sec":       p.AgentlessLockSec,
		"agentless_flush_batch":    p.AgentlessFlushBatch,
		"network_passive_mode":     strings.TrimSpace(p.NetworkPassiveMode),
		"collect_flags": map[string]bool{
			"cpu":        p.CPU,
			"memory":     p.Memory,
			"disk":       p.Disk,
			"network":    p.Network,
			"net_active": p.NetActive,
			"host":       p.Host,
			"sensors":    p.Sensors,
			"power":      p.Power,
			"sanity":     p.Sanity,
			"gpu":        p.GPU,
			"services":   p.Services,
			"time_sync":  p.TimeSync,
			"logs":       p.Logs,
			"updates":    p.Updates,
			"agent":      p.Agent,
			"processes":  p.Processes,
			"vulns":      p.Vulns,
			"inventory":  p.Inventory,
		},
	}
}

func buildAgentEnvSnapshot(cfg config.Config) map[string]any {
	path := resolveAgentEnvPath()
	fileMeta := inspectLocalFile(path, true)
	return map[string]any{
		"path":              path,
		"file":              fileMeta,
		"runtime_values":    readRuntimeEnvAllowlist(),
		"file_values":       readEnvFileAllowlist(path),
		"goos":              runtime.GOOS,
		"goarch":            runtime.GOARCH,
		"agent_mode":        strings.TrimSpace(cfg.AgentMode),
		"api_base_url":      strings.TrimSpace(cfg.APIBaseURL),
		"hub_url":           strings.TrimSpace(cfg.HubURL),
		"hub_listen_addr":   strings.TrimSpace(cfg.HubListenAddr),
		"skip_bootstrap":    cfg.SkipBootstrap,
		"selfheal_poll_sec": int(cfg.SelfHealPollInterval.Seconds()),
	}
}

func resolveAgentEnvPath() string {
	if v := strings.TrimSpace(os.Getenv("AGENT_ENV_FILE")); v != "" {
		return v
	}
	candidates := []string{
		"/etc/aiceberg/agent.env",
		"./configs/agent.env",
	}
	if runtime.GOOS == "windows" {
		if pd := strings.TrimSpace(os.Getenv("ProgramData")); pd != "" {
			candidates = append([]string{filepath.Join(pd, "AIceberg", "agent.env")}, candidates...)
		}
	}
	for _, c := range candidates {
		if strings.TrimSpace(c) == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}

func inspectLocalFile(path string, withSHA bool) map[string]any {
	path = strings.TrimSpace(path)
	out := map[string]any{
		"path": path,
	}
	if path == "" {
		out["exists"] = false
		return out
	}
	info, err := os.Stat(path)
	if err != nil {
		out["exists"] = false
		out["error"] = err.Error()
		return out
	}
	out["exists"] = true
	out["size_bytes"] = info.Size()
	out["is_dir"] = info.IsDir()
	out["modified_at_utc"] = info.ModTime().UTC().Format(time.RFC3339)
	if withSHA && !info.IsDir() {
		if sha, err := fileSHA256(path); err == nil {
			out["sha256"] = sha
		} else {
			out["sha256_error"] = err.Error()
		}
	}
	return out
}

func inspectLocalStateEntry(path string, withSHA bool) map[string]any {
	out := inspectLocalFile(path, withSHA)
	exists, _ := out["exists"].(bool)
	isDir, _ := out["is_dir"].(bool)
	if !exists || isDir {
		return out
	}
	if !looksLikeBoltDBPath(path) {
		return out
	}
	dbStats, err := inspectBoltDB(path)
	if err != nil {
		out["bolt_error"] = err.Error()
		return out
	}
	out["bolt"] = dbStats
	return out
}

func looksLikeBoltDBPath(path string) bool {
	p := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(p, ".db") || strings.HasSuffix(p, ".bolt") || strings.HasSuffix(p, ".bbolt")
}

func inspectBoltDB(path string) (map[string]any, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("empty bolt path")
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{
		ReadOnly: true,
		Timeout:  200 * time.Millisecond,
	})
	if err != nil {
		return nil, err
	}
	defer db.Close()

	dbStats := db.Stats()
	out := map[string]any{
		"free_page_n":          dbStats.FreePageN,
		"pending_page_n":       dbStats.PendingPageN,
		"free_alloc_bytes":     dbStats.FreeAlloc,
		"freelist_inuse_bytes": dbStats.FreelistInuse,
		"tx_n":                 dbStats.TxN,
		"open_tx_n":            dbStats.OpenTxN,
	}
	buckets := map[string]any{}
	if err := db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			if b == nil {
				return nil
			}
			st := b.Stats()
			buckets[string(name)] = map[string]any{
				"key_n":              st.KeyN,
				"depth":              st.Depth,
				"branch_page_n":      st.BranchPageN,
				"leaf_page_n":        st.LeafPageN,
				"branch_inuse_bytes": st.BranchInuse,
				"leaf_inuse_bytes":   st.LeafInuse,
				"bucket_n":           st.BucketN,
				"inline_bucket_n":    st.InlineBucketN,
			}
			return nil
		})
	}); err != nil {
		return nil, err
	}
	out["bucket_count"] = len(buckets)
	out["buckets"] = buckets
	return out, nil
}

func readRuntimeEnvAllowlist() map[string]any {
	keys := []string{
		"AGENT_ENV_FILE",
		"AGENT_MODE",
		"API_BASE_URL",
		"HUB_URL",
		"HUB_LISTEN_ADDR",
		"SKIP_BOOTSTRAP",
		"PREFS_PATH",
		"OUTBOX_PATH",
		"AGENTLESS_OUTBOX_PATH",
		"AGENTLESS_TARGETS_PATH",
		"AGENTLESS_ENABLED",
		"AGENTLESS_POLL_INTERVAL",
		"AGENTLESS_FLUSH_INTERVAL",
		"AGENTLESS_JOBS_LIMIT",
		"AGENTLESS_LOCK_SEC",
		"AGENTLESS_FLUSH_BATCH",
		"AUTO_UPDATE_ENABLED",
		"AUTO_UPDATE_DIR",
		"AUTO_UPDATE_MAX_MB",
		"AUTO_UPDATE_TIMEOUT",
		"AUTO_UPDATE_RETRY_INTERVAL",
		"AUTO_UPDATE_USE_AGENT_AUTH",
		"AUTO_UPDATE_COMMAND",
		"AUTO_UPDATE_WORKDIR",
		"PING_INTERVAL",
		"CONFIG_SYNC_INTERVAL",
		"SELFHEAL_POLL_INTERVAL",
		"LOG_LEVEL",
		"HEALTH_PORT",
	}
	out := map[string]any{}
	for _, key := range keys {
		val := os.Getenv(key)
		if strings.TrimSpace(val) == "" {
			continue
		}
		out[key] = sanitizeEnvValue(key, val)
	}
	for _, key := range []string{"AGENT_TOKEN", "API_KEY", "HUB_TOKEN"} {
		if v := os.Getenv(key); strings.TrimSpace(v) != "" {
			out[key] = sanitizeEnvValue(key, v)
		}
	}
	return out
}

func readEnvFileAllowlist(path string) map[string]any {
	path = strings.TrimSpace(path)
	if path == "" {
		return map[string]any{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{
			"read_error": err.Error(),
		}
	}
	lines := strings.Split(string(raw), "\n")
	out := map[string]any{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
		}
		idx := strings.Index(trimmed, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		val := strings.TrimSpace(trimmed[idx+1:])
		if strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"") && len(val) >= 2 {
			val = strings.Trim(val, "\"")
		}
		if strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'") && len(val) >= 2 {
			val = strings.Trim(val, "'")
		}
		if !isEnvAllowlisted(key) {
			continue
		}
		out[key] = sanitizeEnvValue(key, val)
	}
	return out
}

func isEnvAllowlisted(key string) bool {
	k := strings.ToUpper(strings.TrimSpace(key))
	switch k {
	case "AGENT_TOKEN", "API_KEY", "HUB_TOKEN":
		return true
	}
	prefixes := []string{
		"AGENT_",
		"API_",
		"HUB_",
		"AGENTLESS_",
		"AUTO_UPDATE_",
		"SELFHEAL_",
		"PING_",
		"CONFIG_SYNC_",
		"OUTBOX_",
		"PREFS_",
		"OSLOG_",
		"LOG_",
		"HEALTH_",
		"TLS_",
		"HTTP_",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

func sanitizeEnvValue(key, value string) any {
	k := strings.ToUpper(strings.TrimSpace(key))
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if k == "AGENT_TOKEN" || k == "API_KEY" || k == "HUB_TOKEN" {
		return maskSecret(v)
	}
	if strings.HasSuffix(k, "_ENABLED") || strings.HasSuffix(k, "_AUTH") || strings.HasSuffix(k, "_DEBUG") {
		if b, err := strconv.ParseBool(strings.ToLower(v)); err == nil {
			return b
		}
	}
	return v
}

func maskSecret(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(raw)-4) + raw[len(raw)-4:]
}

func fileSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
