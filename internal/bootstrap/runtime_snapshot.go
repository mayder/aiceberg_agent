package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/version"
	agentruntime "github.com/you/aiceberg_agent/internal/domain/runtime"
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
		"agent_pipeline_version":      agentruntime.PipelineVersion,
		"agentless_enabled_env":       cfg.AgentlessEnabled,
		"agentless_enabled_prefs":     prefs.AgentlessEnabled,
		"agentless_effective_enabled": settings.Enabled,
		"prefs_version":               strings.TrimSpace(prefs.Version),
		"worker_available":            workerAvailable,
		"ingest_runtime": map[string]any{
			"timeout_sec":        int64(cfg.IngestTimeout.Seconds()),
			"flush_interval_sec": int64(cfg.OutboxFlushInterval.Seconds()),
			"flush_batch":        cfg.OutboxFlushBatch,
		},
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
	out["fleet_runtime"] = buildFleetRuntimeSnapshot(cfg, mode, prefs)
	out["security_runtime"] = buildSecurityRuntimeSnapshot(cfg)
	out["edr_ndr_readiness"] = buildEDRNDRReadinessSnapshot(cfg, mode, prefs)
	out["contextual_evidence"] = buildContextualEvidenceSnapshot(cfg, mode, prefs, settings, workerAvailable)
	out["scheduler_snapshot"] = agentruntime.SchedulerSnapshotForCollectors(runtimeCollectorSpecs(cfg))
	if selfUpdateUC != nil {
		out["auto_update_runtime"] = selfUpdateUC.Snapshot()
	}
	return out
}

func buildContextualEvidenceSnapshot(
	cfg config.Config,
	mode string,
	prefs config.CollectPrefs,
	settings usecase.AgentlessSettings,
	workerAvailable bool,
) map[string]any {
	return map[string]any{
		"schema_version": 1,
		"host_evidence": map[string]any{
			"includes": []string{"health", "processes", "services", "ports", "logs", "network", "runtime"},
			"gaps":     contextualEvidenceGaps(prefs, settings, workerAvailable),
			"noc_soc_attachment_policy": map[string]any{
				"send_raw_sensitive_payload": false,
				"attach_summary":             true,
				"attach_gap_list":            true,
			},
		},
		"local_ai": map[string]any{
			"mode":               "deterministic_rules",
			"llm_required":       false,
			"destructive_action": false,
			"rules": []string{
				"redaction",
				"dedupe",
				"noise_scoring",
				"gap_detection",
				"agent_agentless_divergence",
			},
			"noise_reduction": buildLocalAINoiseReduction(prefs),
			"verdict_policy": map[string]any{
				"automatic_verdict":     false,
				"human_review_required": true,
				"decision_scope":        "triage_only",
				"allowed_actions": []string{
					"redact",
					"dedupe",
					"score_noise",
					"flag_gaps",
					"attach_evidence",
				},
				"blocked_actions": []string{
					"close_incident",
					"suppress_alert",
					"change_threshold",
					"execute_command",
					"declare_superiority",
				},
			},
		},
		"offline_first": buildOfflineFirstEvidence(cfg, mode, settings),
		"privacy": map[string]any{
			"profile":              privacyProfile(),
			"sensitive_mode":       sensitiveModeEnabled(),
			"minimized_collectors": minimizedCollectors(prefs),
			"raw_secret_logging":   false,
		},
		"agent_agentless": map[string]any{
			"agentless_effective_enabled": settings.Enabled,
			"agentless_worker_available":  workerAvailable,
			"correlation_strategy": []string{
				"host_ok_network_failing",
				"network_ok_local_service_failing",
				"agent_recent_snmp_stale",
			},
		},
		"superiority_benchmark": buildSuperiorityBenchmarkEvidence(),
	}
}

func buildLocalAINoiseReduction(prefs config.CollectPrefs) map[string]any {
	return map[string]any{
		"enabled":                  true,
		"strategy":                 "deterministic_preclassification",
		"inputs":                   []string{"redacted_logs", "collector_gaps", "agent_agentless_divergence", "duplicate_fingerprints"},
		"signals":                  []string{"duplicate_candidate", "low_context", "missing_agentless", "missing_logs", "network_local_divergence"},
		"keeps_original_evidence":  true,
		"drops_raw_events":         false,
		"requires_benchmark":       true,
		"disabled_collectors":      minimizedCollectors(prefs),
		"automatic_suppression":    false,
		"human_review_for_closure": true,
	}
}

func buildOfflineFirstEvidence(cfg config.Config, mode string, settings usecase.AgentlessSettings) map[string]any {
	return map[string]any{
		"outbox_path_configured":  strings.TrimSpace(cfg.OutboxPath) != "",
		"outbox_max_mb":           cfg.OutboxMaxMB,
		"agentless_outbox_max_mb": cfg.AgentlessOutboxMaxMB,
		"http_idempotency":        cfg.HTTPIdempotency,
		"compression_supported":   cfg.HTTPGzip,
		"hub_or_relay_mode":       mode == "hub" || mode == "relay",
		"proxy_configured":        proxyConfigured(),
		"retention_policy": map[string]any{
			"local_outbox_max_mb":     cfg.OutboxMaxMB,
			"agentless_outbox_max_mb": cfg.AgentlessOutboxMaxMB,
			"flush_batch":             cfg.OutboxFlushBatch,
			"flush_interval_sec":      int64(cfg.OutboxFlushInterval.Seconds()),
			"agentless_flush_batch":   settings.FlushBatch,
			"idempotent_replay":       cfg.HTTPIdempotency,
		},
		"replay_safety": map[string]any{
			"durable_until_ack":        true,
			"ack_idempotent":           true,
			"retry_scope":              "route_identity",
			"requires_24h_validation":  true,
			"relay_to_hub_only":        mode == "relay",
			"direct_api_from_relay":    false,
			"preserve_legacy_contract": true,
		},
		"local_export": map[string]any{
			"format":              "support_flare_redacted_json",
			"signed":              true,
			"signature_algorithm": "sha256",
			"signature_scope":     "sanitized_runtime_offline_evidence",
			"signature":           offlineFirstSignature(cfg, mode, settings),
		},
		"local_export_support": "support_flare_redacted",
	}
}

func buildSuperiorityBenchmarkEvidence() map[string]any {
	return map[string]any{
		"claim_allowed": false,
		"status":        "pending_evidence",
		"comparison_policy": map[string]any{
			"declare_superiority_without_benchmark": false,
			"requires_same_scenario":                true,
			"requires_raw_evidence_reference":       true,
			"requires_operator_review":              true,
		},
		"scenarios": []map[string]any{
			{
				"code":   "noc_soc_context",
				"target": "NOC/SOC terceirizado com evidência executiva",
				"metrics": []string{
					"time_to_diagnosis",
					"evidence_completeness",
					"operator_steps",
				},
			},
			{
				"code":   "sovereign_offline",
				"target": "ambiente restrito sem internet direta",
				"metrics": []string{
					"offline_replay_success",
					"duplicate_rate",
					"support_export_integrity",
				},
			},
			{
				"code":   "agent_plus_agentless",
				"target": "falha cruzada entre host local e rede Agentless",
				"metrics": []string{
					"correlation_detected",
					"false_positive_rate",
					"agentless_observation_link",
				},
			},
			{
				"code":   "noise_reduction",
				"target": "redução de ruído sem veredito automático",
				"metrics": []string{
					"noise_before",
					"noise_after",
					"manual_review_required",
				},
			},
		},
		"required_evidence": []string{
			"time_to_diagnosis",
			"noise_reduction",
			"executive_evidence",
			"deployment_effort",
			"agent_plus_agentless",
			"datadog_reference",
		},
	}
}

func offlineFirstSignature(cfg config.Config, mode string, settings usecase.AgentlessSettings) string {
	payload := map[string]any{
		"agent_pipeline_version":  agentruntime.PipelineVersion,
		"agentless_flush_batch":   settings.FlushBatch,
		"agentless_outbox_max_mb": cfg.AgentlessOutboxMaxMB,
		"compression_supported":   cfg.HTTPGzip,
		"flush_batch":             cfg.OutboxFlushBatch,
		"flush_interval_sec":      int64(cfg.OutboxFlushInterval.Seconds()),
		"http_idempotency":        cfg.HTTPIdempotency,
		"mode":                    mode,
		"outbox_max_mb":           cfg.OutboxMaxMB,
	}
	return hashMap(payload)
}

func contextualEvidenceGaps(prefs config.CollectPrefs, settings usecase.AgentlessSettings, workerAvailable bool) []string {
	gaps := make([]string, 0, 8)
	if !prefs.Logs {
		gaps = append(gaps, "logs_disabled")
	}
	if !prefs.Processes {
		gaps = append(gaps, "processes_disabled")
	}
	if !prefs.Services {
		gaps = append(gaps, "services_disabled")
	}
	if !prefs.Network {
		gaps = append(gaps, "network_metrics_disabled")
	}
	if strings.TrimSpace(prefs.NetworkPassiveMode) == "" || strings.EqualFold(strings.TrimSpace(prefs.NetworkPassiveMode), "socket") {
		gaps = append(gaps, "advanced_network_limited")
	}
	if settings.Enabled && !workerAvailable {
		gaps = append(gaps, "agentless_worker_unavailable")
	}
	if !settings.Enabled {
		gaps = append(gaps, "agentless_disabled")
	}
	return uniqueStrings(gaps, 16)
}

func minimizedCollectors(prefs config.CollectPrefs) []string {
	disabled := make([]string, 0, 12)
	flags := map[string]bool{
		"logs":       prefs.Logs,
		"processes":  prefs.Processes,
		"services":   prefs.Services,
		"network":    prefs.Network,
		"inventory":  prefs.Inventory,
		"oslog_diag": prefs.OSLogDiag,
	}
	for name, enabled := range flags {
		if !enabled {
			disabled = append(disabled, name)
		}
	}
	return uniqueStrings(disabled, 16)
}

func privacyProfile() string {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("PRIVACY_PROFILE")))
	switch value {
	case "standard", "sensitive", "minimal":
		return value
	default:
		return "standard"
	}
}

func sensitiveModeEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("SENSITIVE_MODE")))
	return value == "true" || value == "1" || value == "yes"
}

func uniqueStrings(items []string, limit int) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func buildSecurityRuntimeSnapshot(cfg config.Config) map[string]any {
	return map[string]any{
		"remote_config_signature_required":          cfg.RemoteConfigSignatureRequired,
		"remote_config_signature_secret_configured": strings.TrimSpace(cfg.RemoteConfigSignatureSecret) != "",
		"remote_config_unsigned_sensitive_allowed":  cfg.RemoteConfigAllowUnsignedSensitive,
		"auto_update_trust_required":                cfg.AutoUpdateTrustRequired,
		"auto_update_trust_public_key_configured":   strings.TrimSpace(cfg.AutoUpdateTrustPublicKey) != "",
		"tls_insecure_skip_verify":                  cfg.TLSInsecureSkip,
		"tls_insecure_allow_prod":                   cfg.TLSInsecureAllowProd,
		"proxy_configured":                          proxyConfigured(),
		"fips_mode":                                 "not_claimed",
		"secrets_sources": map[string]any{
			"agent_token_path":             strings.TrimSpace(os.Getenv("AGENT_TOKEN_PATH")) != "",
			"kubernetes_token_path":        strings.TrimSpace(cfg.KubernetesTokenPath) != "",
			"local_checks_credentials_ref": localChecksHaveCredentials(cfg.LocalChecks),
		},
		"remote_command_policy": map[string]any{
			"allowlist_enabled": true,
			"shell_blocked":     true,
		},
	}
}

func buildEDRNDRReadinessSnapshot(cfg config.Config, mode string, prefs config.CollectPrefs) map[string]any {
	enabled := cfg.EDRSafe || prefs.EDRSafe
	profile := strings.TrimSpace(cfg.EDRSafeProfile)
	if prefs.EDRSafeProfile != "" {
		profile = strings.TrimSpace(prefs.EDRSafeProfile)
	}
	if profile == "" {
		profile = "standard"
	}
	gaps := edrNDRGaps(cfg, enabled)
	status := "ready"
	if !enabled {
		status = "limited"
	} else if len(gaps) > 0 {
		status = "attention"
	}
	return map[string]any{
		"schema_version": 1,
		"status":         status,
		"mode":           map[string]any{"edr_safe": enabled, "profile": profile, "agent_mode": strings.TrimSpace(mode)},
		"policy": map[string]any{
			"remote_shell_blocked":             true,
			"arbitrary_execution_blocked":      true,
			"destructive_actions_blocked":      true,
			"sensitive_config_signed_required": cfg.RemoteConfigSignatureRequired,
			"auto_update_hash_required":        true,
			"auto_update_signature_required":   cfg.AutoUpdateTrustRequired,
			"log_min_severity":                 effectiveOSLogSeverity(cfg, prefs),
			"log_source_discovery_bounded":     true,
			"credential_payload_blocked":       true,
			"supplier_claim_without_evidence":  false,
		},
		"manifest": edrNDRManifest(cfg),
		"modules": map[string]any{
			"core_preserved": []string{"cpu", "memory", "disk", "network", "host", "agent_health", "secure_update"},
			"logs": map[string]any{
				"enabled":          cfg.OSLogEnabled || prefs.Logs,
				"min_severity":     effectiveOSLogSeverity(cfg, prefs),
				"eventlog_enabled": runtime.GOOS == "windows",
				"journald_enabled": cfg.OSLogJournaldEnabled || len(cfg.OSLogJournaldUnits) > 0,
				"raw_file_allowed": len(cfg.OSLogFiles) > 0 || len(prefs.OSLogFilesList) > 0,
			},
			"bounded_collectors": []string{"log_source_discovery", "local_checks", "containers", "kubernetes", "otlp"},
			"blocked_actions":    []string{"shell", "powershell", "cmd", "bash", "script", "delete", "isolate", "disable_security_tool"},
		},
		"allowlist": edrNDRAllowlist(cfg),
		"gaps":      gaps,
		"validation": map[string]any{
			"windows_defender_eventlog": "fixture_required_or_real_when_available",
			"linux_audit_journald":      "fixture_required_or_real_when_available",
			"edr_ndr_vendor_fixture":    "required",
			"crowdstrike_darktrace":     "no_claim_without_customer_environment",
		},
	}
}

func edrNDRGaps(cfg config.Config, enabled bool) []string {
	gaps := make([]string, 0, 8)
	if !enabled {
		gaps = append(gaps, "edr_safe_disabled")
	}
	if !cfg.RemoteConfigSignatureRequired {
		gaps = append(gaps, "remote_config_signature_not_required")
	}
	if !cfg.AutoUpdateTrustRequired {
		gaps = append(gaps, "auto_update_signature_not_required")
	}
	if cfg.TLSInsecureSkip {
		gaps = append(gaps, "tls_insecure_skip_verify")
	}
	if strings.TrimSpace(cfg.AgentIdentitySecret) == "" {
		gaps = append(gaps, "agent_identity_secret_not_reported")
	}
	return uniqueStrings(gaps, 16)
}

func edrNDRManifest(cfg config.Config) map[string]any {
	exePath, _ := os.Executable()
	apiBase := strings.TrimRight(strings.TrimSpace(cfg.APIBaseURL), "/")
	return map[string]any{
		"binary": inspectLocalFile(exePath, true),
		"process": map[string]any{
			"pid":     os.Getpid(),
			"goos":    runtime.GOOS,
			"goarch":  runtime.GOARCH,
			"version": strings.TrimSpace(version.Version),
		},
		"service": map[string]any{
			"name": serviceNameForOS(),
			"user": runtimeUserName(),
		},
		"directories": map[string]any{
			"prefs":             inspectLocalFile(cfg.PrefsPath, true),
			"outbox":            inspectLocalFile(cfg.OutboxPath, false),
			"agentless_outbox":  inspectLocalFile(cfg.AgentlessOutboxPath, false),
			"update_directory":  inspectLocalFile(cfg.AutoUpdateDir, false),
			"log_cursor":        inspectLocalFile(cfg.OSLogCursorPath, true),
			"log_discovery_dir": inspectLocalFile(filepath.Dir(cfg.PrefsPath), false),
		},
		"network": map[string]any{
			"api_base_url": apiBase,
			"endpoints": []string{
				apiBase + "/v1/ingest/metrics",
				apiBase + "/v1/ingest/health",
				apiBase + "/v1/logs/raw",
				apiBase + "/v1/agent/config",
				apiBase + "/v1/agent/channel",
				apiBase + "/v1/agent/update-report",
			},
			"ports":            edrNDRPorts(cfg),
			"proxy_configured": proxyConfigured(),
		},
		"publisher": map[string]any{
			"name":       "AIceberg",
			"signer":     "not_reported_by_runtime",
			"claim_safe": false,
		},
		"update_policy": map[string]any{
			"enabled":                     cfg.AutoUpdateEnabled,
			"hash_validation_required":    true,
			"signature_validation":        cfg.AutoUpdateTrustRequired,
			"trust_public_key_configured": strings.TrimSpace(cfg.AutoUpdateTrustPublicKey) != "",
			"max_mb":                      cfg.AutoUpdateMaxMB,
			"rollback_directory":          cfg.AutoUpdateDir,
		},
	}
}

func edrNDRAllowlist(cfg config.Config) map[string]any {
	return map[string]any{
		"paths": uniqueStrings([]string{
			os.Getenv("AGENT_ENV_FILE"),
			cfg.PrefsPath,
			cfg.OutboxPath,
			cfg.AgentlessOutboxPath,
			cfg.AutoUpdateDir,
			cfg.OSLogCursorPath,
		}, 16),
		"processes": []string{"aiceberg_agent", "aiceberg-agent"},
		"services":  []string{serviceNameForOS()},
		"domains":   uniqueStrings([]string{hostFromURL(cfg.APIBaseURL), hostFromURL(cfg.HubURL)}, 8),
		"ports":     edrNDRPorts(cfg),
		"scope":     "least_privilege_readonly_observability",
		"notes": []string{
			"no_security_tool_bypass",
			"no_global_exclusion_required",
			"validate_hash_before_allowlisting_binary",
		},
	}
}

func effectiveOSLogSeverity(cfg config.Config, prefs config.CollectPrefs) string {
	if strings.TrimSpace(prefs.OSLogMinSeverity) != "" {
		return strings.ToLower(strings.TrimSpace(prefs.OSLogMinSeverity))
	}
	if strings.TrimSpace(cfg.OSLogMinSeverity) != "" {
		return strings.ToLower(strings.TrimSpace(cfg.OSLogMinSeverity))
	}
	return "not_configured"
}

func edrNDRPorts(cfg config.Config) []string {
	ports := make([]string, 0, 4)
	if cfg.HealthPort > 0 {
		ports = append(ports, strconv.Itoa(cfg.HealthPort)+"/tcp")
	}
	if strings.TrimSpace(cfg.CustomMetricsUDPAddr) != "" {
		ports = append(ports, cfg.CustomMetricsUDPAddr+"/udp")
	}
	if strings.TrimSpace(cfg.CustomMetricsHTTPAddr) != "" {
		ports = append(ports, cfg.CustomMetricsHTTPAddr+"/tcp")
	}
	if strings.TrimSpace(cfg.OTLPHTTPAddr) != "" {
		ports = append(ports, cfg.OTLPHTTPAddr+"/tcp")
	}
	return uniqueStrings(ports, 8)
}

func hostFromURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	if idx := strings.Index(value, "/"); idx >= 0 {
		value = value[:idx]
	}
	if idx := strings.Index(value, ":"); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func serviceNameForOS() string {
	if runtime.GOOS == "windows" {
		return "AIcebergAgent"
	}
	return "aiceberg-agent"
}

func runtimeUserName() string {
	for _, key := range []string{"USER", "USERNAME", "LOGNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "not_reported"
}

func proxyConfigured() bool {
	for _, key := range []string{"HTTPS_PROXY", "HTTP_PROXY", "https_proxy", "http_proxy"} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func localChecksHaveCredentials(checks []config.LocalCheckConfig) bool {
	for _, check := range checks {
		if strings.TrimSpace(check.CredentialsRef) != "" {
			return true
		}
	}
	return false
}

func buildFleetRuntimeSnapshot(cfg config.Config, mode string, prefs config.CollectPrefs) map[string]any {
	prefsSnapshot := sanitizePrefsSnapshot(prefs)
	configHash := hashMap(prefsSnapshot)
	driftStatus := "unknown"
	if strings.TrimSpace(prefs.Version) != "" {
		driftStatus = "applied"
	}
	return map[string]any{
		"agent_version":        strings.TrimSpace(version.Version),
		"goos":                 runtime.GOOS,
		"goarch":               runtime.GOARCH,
		"mode":                 strings.TrimSpace(mode),
		"config_version":       strings.TrimSpace(prefs.Version),
		"config_hash":          configHash,
		"config_drift_status":  driftStatus,
		"auto_update_enabled":  cfg.AutoUpdateEnabled,
		"rollback_state":       inspectLocalStateEntry(filepath.Join(cfg.AutoUpdateDir, ".pending_update.json"), true),
		"last_snapshot_source": "runtime",
	}
}

func hashMap(value map[string]any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func runtimeCollectorSpecs(cfg config.Config) []agentruntime.CollectorSpec {
	return []agentruntime.CollectorSpec{
		{Name: "sysmetrics", Version: "legacy-compatible", Endpoint: "/v1/ingest/metrics", Interval: 10 * time.Second, Priority: 10},
		{Name: "sysmetrics_health", Version: "legacy-compatible", Endpoint: "/v1/ingest/health", Interval: 10 * time.Minute, Priority: 30},
		{Name: "sysmetrics_inventory", Version: "legacy-compatible", Endpoint: "/v1/ingest/inventory", Interval: 8 * time.Hour, Priority: 40},
		{Name: "sysmetrics_bootstrap", Version: "legacy-compatible", Endpoint: "/v1/ingest/bootstrap", Interval: 24 * time.Hour, Priority: 50},
		{Name: "custommetrics", Version: "1-compatible", Endpoint: "/v1/ingest/metrics", Interval: cfg.CustomMetricsInterval, Priority: 15},
		{Name: "otlp_metrics", Version: "1-http-json", Endpoint: "/v1/ingest/metrics", Interval: cfg.OTLPInterval, Priority: 16},
		{Name: "otlp_logs", Version: "1-http-json", Endpoint: "/v1/logs/raw", Interval: cfg.OTLPInterval, Priority: 16},
		{Name: "otlp_traces", Version: "1-http-json", Endpoint: "/v1/ingest/metrics", Interval: cfg.OTLPInterval, Priority: 16},
		{Name: "containers", Version: "1-docker-socket", Endpoint: "/v1/ingest/metrics", Interval: cfg.ContainerInterval, Priority: 18},
		{Name: "kubernetes", Version: "1-api", Endpoint: "/v1/ingest/metrics", Interval: cfg.KubernetesInterval, Priority: 19},
		{Name: "localchecks", Version: "1-safe-allowlist", Endpoint: "/v1/ingest/metrics", Interval: cfg.LocalChecksInterval, Priority: 19},
		{Name: "log_source_discovery", Version: "1-readonly", Endpoint: "/v1/ingest/metrics", Interval: cfg.LogDiscoveryInterval, Priority: 19},
		{Name: "networkcapture", Version: "legacy-compatible", Endpoint: "/v1/ingest/network_capture", Interval: 10 * time.Second, Priority: 20},
		{Name: "oslogs", Version: "legacy-compatible", Endpoint: "/v1/logs/raw", Interval: cfg.OSLogInterval, Priority: 20},
	}
}

func sanitizePrefsSnapshot(p config.CollectPrefs) map[string]any {
	return map[string]any{
		"version":                      strings.TrimSpace(p.Version),
		"paused":                       p.Paused,
		"agentless_enabled":            p.AgentlessEnabled,
		"agentless_poll_interval":      p.AgentlessPollSec,
		"agentless_flush_interval":     p.AgentlessFlushSec,
		"agentless_jobs_limit":         p.AgentlessJobsLimit,
		"agentless_lock_sec":           p.AgentlessLockSec,
		"agentless_flush_batch":        p.AgentlessFlushBatch,
		"custom_metrics_enabled":       p.CustomMetricsEnabled,
		"custom_metrics_interval":      p.CustomMetricsIntervalSec,
		"custom_metrics_max_series":    p.CustomMetricsMaxSeries,
		"otlp_enabled":                 p.OTLPEnabled,
		"otlp_interval":                p.OTLPIntervalSec,
		"otlp_max_items":               p.OTLPMaxItems,
		"container_enabled":            p.ContainerEnabled,
		"container_runtime":            strings.TrimSpace(p.ContainerRuntime),
		"container_interval":           p.ContainerIntervalSec,
		"container_max_items":          p.ContainerMaxItems,
		"kubernetes_enabled":           p.KubernetesEnabled,
		"kubernetes_interval":          p.KubernetesIntervalSec,
		"kubernetes_max_items":         p.KubernetesMaxItems,
		"kubernetes_max_events":        p.KubernetesMaxEvents,
		"kubernetes_logs_enabled":      p.KubernetesLogsEnabled,
		"kubernetes_logs_max_lines":    p.KubernetesLogsMaxLines,
		"kubernetes_logs_max_bytes":    p.KubernetesLogsMaxBytes,
		"local_checks_enabled":         p.LocalChecksEnabled,
		"local_checks_interval":        p.LocalChecksIntervalSec,
		"local_checks_max_checks":      p.LocalChecksMaxChecks,
		"local_check_manifest_dirs":    len(p.LocalCheckManifestDirs),
		"log_discovery_enabled":        p.LogDiscoveryEnabled,
		"log_discovery_interval":       p.LogDiscoveryIntervalSec,
		"log_discovery_max_candidates": p.LogDiscoveryMaxCandidates,
		"network_passive_mode":         strings.TrimSpace(p.NetworkPassiveMode),
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
		"mode_override":     inspectLocalFile(cfg.AgentModeOverridePath, true),
		"api_base_url":      strings.TrimSpace(cfg.APIBaseURL),
		"hub_url":           strings.TrimSpace(cfg.HubURL),
		"hub_listen_addr":   strings.TrimSpace(cfg.HubListenAddr),
		"skip_bootstrap":    cfg.SkipBootstrap,
		"selfheal_poll_sec": int(cfg.SelfHealPollInterval.Seconds()),
		"custom_metrics": map[string]any{
			"enabled":      cfg.CustomMetricsEnabled,
			"udp_addr":     strings.TrimSpace(cfg.CustomMetricsUDPAddr),
			"http_addr":    strings.TrimSpace(cfg.CustomMetricsHTTPAddr),
			"interval_sec": int(cfg.CustomMetricsInterval.Seconds()),
			"max_series":   cfg.CustomMetricsMaxSeries,
			"max_bytes":    cfg.CustomMetricsMaxBytes,
		},
		"otlp": map[string]any{
			"enabled":      cfg.OTLPEnabled,
			"http_addr":    strings.TrimSpace(cfg.OTLPHTTPAddr),
			"interval_sec": int(cfg.OTLPInterval.Seconds()),
			"max_items":    cfg.OTLPMaxItems,
			"max_bytes":    cfg.OTLPMaxBytes,
		},
		"containers": map[string]any{
			"enabled":              cfg.ContainerEnabled,
			"runtime":              strings.TrimSpace(cfg.ContainerRuntime),
			"docker_socket":        strings.TrimSpace(cfg.ContainerDockerSocket),
			"containerd_socket":    strings.TrimSpace(cfg.ContainerContainerdSocket),
			"containerd_namespace": strings.TrimSpace(cfg.ContainerContainerdNamespace),
			"ctr_path":             strings.TrimSpace(cfg.ContainerCtrPath),
			"interval_sec":         int(cfg.ContainerInterval.Seconds()),
			"max_items":            cfg.ContainerMaxItems,
			"logs_enabled":         cfg.ContainerLogsEnabled,
			"logs_max_lines":       cfg.ContainerLogsMaxLines,
			"logs_max_bytes":       cfg.ContainerLogsMaxBytes,
		},
		"kubernetes": map[string]any{
			"enabled":        cfg.KubernetesEnabled,
			"api_url":        strings.TrimSpace(cfg.KubernetesAPIURL),
			"token_path":     strings.TrimSpace(cfg.KubernetesTokenPath),
			"ca_path":        strings.TrimSpace(cfg.KubernetesCAPath),
			"node_name":      strings.TrimSpace(cfg.KubernetesNodeName),
			"namespace":      strings.TrimSpace(cfg.KubernetesNamespace),
			"interval_sec":   int(cfg.KubernetesInterval.Seconds()),
			"max_items":      cfg.KubernetesMaxItems,
			"max_events":     cfg.KubernetesMaxEvents,
			"logs_enabled":   cfg.KubernetesLogsEnabled,
			"logs_max_lines": cfg.KubernetesLogsMaxLines,
			"logs_max_bytes": cfg.KubernetesLogsMaxBytes,
		},
		"local_checks": map[string]any{
			"enabled":       cfg.LocalChecksEnabled,
			"interval":      int(cfg.LocalChecksInterval.Seconds()),
			"max_checks":    cfg.LocalChecksMaxChecks,
			"max_bytes":     cfg.LocalChecksMaxBytes,
			"manifest_dirs": len(cfg.LocalCheckManifestDirs),
			"checks":        sanitizeLocalCheckConfigs(cfg.LocalChecks),
		},
		"log_source_discovery": map[string]any{
			"enabled":            cfg.LogDiscoveryEnabled,
			"interval_sec":       int(cfg.LogDiscoveryInterval.Seconds()),
			"max_candidates":     cfg.LogDiscoveryMaxCandidates,
			"max_evidence_bytes": cfg.LogDiscoveryMaxEvidenceBytes,
		},
	}
}

func sanitizeLocalCheckConfigs(checks []config.LocalCheckConfig) []map[string]any {
	out := make([]map[string]any, 0, len(checks))
	for _, check := range checks {
		out = append(out, map[string]any{
			"id":          strings.TrimSpace(check.ID),
			"kind":        strings.TrimSpace(check.Kind),
			"version":     strings.TrimSpace(check.Version),
			"interval":    check.IntervalSec,
			"timeout_ms":  check.TimeoutMs,
			"tags":        check.Tags,
			"target":      sanitizeLocalCheckTarget(check.Target),
			"credentials": strings.TrimSpace(check.CredentialsRef) != "",
			"enabled":     check.Enabled,
		})
	}
	return out
}

func sanitizeLocalCheckTarget(target string) string {
	target = strings.TrimSpace(target)
	if idx := strings.Index(target, "?"); idx >= 0 {
		return target[:idx] + "?[redacted]"
	}
	return target
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
		"AGENT_MODE_OVERRIDE_PATH",
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
	for _, key := range []string{"AGENT_TOKEN", "API_KEY", "HUB_TOKEN", "AGENT_IDENTITY_SECRET"} {
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
		"CUSTOM_METRICS_",
		"OTLP_",
		"CONTAINER_",
		"KUBERNETES_",
		"LOCAL_CHECKS_",
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
	if isSensitiveEnvKey(k) {
		return maskSecret(v)
	}
	if strings.HasSuffix(k, "_ENABLED") || strings.HasSuffix(k, "_AUTH") || strings.HasSuffix(k, "_DEBUG") {
		if b, err := strconv.ParseBool(strings.ToLower(v)); err == nil {
			return b
		}
	}
	return v
}

func isSensitiveEnvKey(key string) bool {
	k := strings.ToUpper(strings.TrimSpace(key))
	return k == "AGENT_TOKEN" ||
		k == "API_KEY" ||
		k == "HUB_TOKEN" ||
		strings.Contains(k, "TOKEN") ||
		strings.Contains(k, "SECRET")
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
