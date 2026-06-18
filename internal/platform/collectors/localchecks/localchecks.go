package localchecks

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const schemaVersion = 1

type collector struct {
	prefs        func() config.CollectPrefs
	baseEnabled  bool
	baseChecks   []config.LocalCheckConfig
	interval     time.Duration
	maxChecks    int
	maxBytes     int64
	manifestDirs []string
}

func New(cfg config.Config, prefsProvider func() config.CollectPrefs) ports.Collector {
	interval := cfg.LocalChecksInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	maxChecks := cfg.LocalChecksMaxChecks
	if maxChecks <= 0 {
		maxChecks = 100
	}
	maxBytes := cfg.LocalChecksMaxBytes
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	return &collector{
		prefs:        prefsProvider,
		baseEnabled:  cfg.LocalChecksEnabled,
		baseChecks:   cfg.LocalChecks,
		interval:     interval,
		maxChecks:    maxChecks,
		maxBytes:     int64(maxBytes),
		manifestDirs: cfg.LocalCheckManifestDirs,
	}
}

func (c *collector) Name() string { return "localchecks" }

func (c *collector) Interval() time.Duration { return c.interval }

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	enabled, checks := c.settings()
	if !enabled {
		return nil, nil
	}
	if len(checks) > c.maxChecks {
		checks = checks[:c.maxChecks]
	}
	results := make([]map[string]any, 0, len(checks))
	for _, check := range checks {
		if !check.Enabled {
			continue
		}
		results = append(results, c.runCheck(ctx, check))
	}
	payload := map[string]any{
		"local_checks": map[string]any{
			"schema_version": schemaVersion,
			"results":        results,
			"integrations":   installedIntegrations(c.effectiveManifestDirs()),
			"dropped_count":  maxInt(0, len(checks)-c.maxChecks),
		},
	}
	return json.Marshal(payload)
}

func (c *collector) effectiveManifestDirs() []string {
	if c.prefs != nil {
		p := c.prefs()
		if len(p.LocalCheckManifestDirs) > 0 {
			return p.LocalCheckManifestDirs
		}
	}
	return c.manifestDirs
}

func (c *collector) settings() (bool, []config.LocalCheckConfig) {
	p := config.CollectPrefs{}
	if c.prefs != nil {
		p = c.prefs()
	}
	if strings.TrimSpace(p.Version) == "" {
		return c.baseEnabled, c.baseChecks
	}
	return p.LocalChecksEnabled, p.LocalChecks
}

func (c *collector) runCheck(ctx context.Context, check config.LocalCheckConfig) map[string]any {
	start := time.Now()
	timeout := time.Duration(check.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := map[string]any{
		"check_id":        strings.TrimSpace(check.ID),
		"kind":            normalizeKind(check.Kind),
		"version":         strings.TrimSpace(check.Version),
		"interval":        check.IntervalSec,
		"timeout_ms":      int(timeout / time.Millisecond),
		"tags":            check.Tags,
		"target":          safeTarget(check.Target),
		"credentials_ref": safeCredentialsRef(check.CredentialsRef),
	}
	metrics, logs, serviceCheck, err := execute(runCtx, check, c.maxBytes)
	result["duration_ms"] = time.Since(start).Milliseconds()
	result["result"] = statusFromError(err)
	result["metrics"] = metrics
	result["logs"] = logs
	result["service_check"] = serviceCheck
	if err != nil {
		result["error"] = map[string]any{
			"message": redact(err.Error()),
		}
	}
	return result
}

func execute(ctx context.Context, check config.LocalCheckConfig, maxBytes int64) ([]map[string]any, []string, map[string]any, error) {
	switch normalizeKind(check.Kind) {
	case "http", "nginx", "apache":
		return runHTTP(ctx, check, maxBytes)
	case "openmetrics":
		return runOpenMetrics(ctx, check, maxBytes)
	case "jmx":
		return runJMX(ctx, check, maxBytes)
	case "tcp", "postgresql", "mysql", "redis", "iis_wmi", "windows_service":
		return runIntegrationTCP(ctx, check)
	default:
		return nil, nil, map[string]any{"status": "critical"}, errors.New("tipo de check local nao permitido")
	}
}

func runHTTP(ctx context.Context, check config.LocalCheckConfig, maxBytes int64) ([]map[string]any, []string, map[string]any, error) {
	target, err := validateHTTPURL(check.Target)
	if err != nil {
		return nil, nil, map[string]any{"status": "critical"}, err
	}
	client := &http.Client{
		Timeout:   timeoutFromContext(ctx),
		Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, map[string]any{"status": "critical"}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, map[string]any{"status": "critical"}, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBytes))
	status := "ok"
	if resp.StatusCode >= 400 {
		status = "critical"
	}
	metrics := []map[string]any{{"name": "http.status_code", "type": "gauge", "value": resp.StatusCode}}
	return metrics, nil, map[string]any{"status": status, "http_status": resp.StatusCode}, nil
}

func runOpenMetrics(ctx context.Context, check config.LocalCheckConfig, maxBytes int64) ([]map[string]any, []string, map[string]any, error) {
	target, err := validateHTTPURL(check.Target)
	if err != nil {
		return nil, nil, map[string]any{"status": "critical"}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, map[string]any{"status": "critical"}, err
	}
	client := &http.Client{Timeout: timeoutFromContext(ctx)}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, map[string]any{"status": "critical"}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, nil, map[string]any{"status": "critical", "http_status": resp.StatusCode}, nil
	}
	metrics := parseOpenMetricsForCheck(io.LimitReader(resp.Body, maxBytes), check)
	return metrics, nil, map[string]any{"status": "ok", "http_status": resp.StatusCode}, nil
}

func runTCP(ctx context.Context, check config.LocalCheckConfig) ([]map[string]any, []string, map[string]any, error) {
	target := strings.TrimSpace(check.Target)
	if target == "" {
		return nil, nil, map[string]any{"status": "critical"}, errors.New("target vazio")
	}
	d := net.Dialer{Timeout: timeoutFromContext(ctx)}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, nil, map[string]any{"status": "critical"}, err
	}
	_ = conn.Close()
	return nil, nil, map[string]any{"status": "ok"}, nil
}

func runIntegrationTCP(ctx context.Context, check config.LocalCheckConfig) ([]map[string]any, []string, map[string]any, error) {
	kind := normalizeKind(check.Kind)
	metrics, logs, serviceCheck, err := runTCP(ctx, check)
	serviceCheck["integration"] = integrationManifest(kind)
	if err != nil {
		return metrics, logs, serviceCheck, err
	}
	metrics = append(metrics, map[string]any{
		"name":        "integration.reachable",
		"type":        "gauge",
		"value":       1,
		"integration": kind,
	})
	return metrics, logs, serviceCheck, nil
}

func validateHTTPURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || parsed.Host == "" {
		return "", errors.New("url invalida")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("scheme nao permitido")
	}
	return parsed.String(), nil
}

func timeoutFromContext(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 5 * time.Second
	}
	d := time.Until(deadline)
	if d <= 0 {
		return time.Millisecond
	}
	return d
}

func normalizeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func statusFromError(err error) string {
	if err != nil {
		return "failed"
	}
	return "ok"
}

func safeTarget(target string) string {
	target = redact(strings.TrimSpace(target))
	if len(target) > 500 {
		return target[:500]
	}
	return target
}

func safeCredentialsRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	return "[redacted-ref]"
}

func redact(value string) string {
	replacements := []string{"password=", "passwd=", "token=", "secret=", "apikey=", "api_key="}
	out := value
	lower := strings.ToLower(out)
	for _, key := range replacements {
		if idx := strings.Index(lower, key); idx >= 0 {
			end := idx + len(key)
			for end < len(out) && out[end] != '&' && out[end] != ' ' {
				end++
			}
			out = out[:idx+len(key)] + "[redacted]" + out[end:]
			lower = strings.ToLower(out)
		}
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
