package localchecks

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/you/aiceberg_agent/internal/common/config"
)

type integrationInfo struct {
	Kind        string   `json:"kind"`
	Version     string   `json:"version"`
	Status      string   `json:"status"`
	Permissions []string `json:"permissions,omitempty"`
	Rollback    string   `json:"rollback,omitempty"`
}

var integrationCatalog = map[string]integrationInfo{
	"openmetrics":     {Kind: "openmetrics", Version: "2", Status: "official", Permissions: []string{"http_get"}, Rollback: "desativar check openmetrics"},
	"redis":           {Kind: "redis", Version: "1", Status: "official", Permissions: []string{"tcp_connect"}, Rollback: "desativar check redis"},
	"postgresql":      {Kind: "postgresql", Version: "1", Status: "beta", Permissions: []string{"tcp_connect", "credentials_ref"}, Rollback: "desativar check postgresql"},
	"mysql":           {Kind: "mysql", Version: "1", Status: "beta", Permissions: []string{"tcp_connect", "credentials_ref"}, Rollback: "desativar check mysql"},
	"jmx":             {Kind: "jmx", Version: "1", Status: "beta", Permissions: []string{"jolokia_http_get", "credentials_ref"}, Rollback: "desativar check jmx"},
	"nginx":           {Kind: "nginx", Version: "1", Status: "beta", Permissions: []string{"http_get"}, Rollback: "desativar check nginx"},
	"apache":          {Kind: "apache", Version: "1", Status: "beta", Permissions: []string{"http_get"}, Rollback: "desativar check apache"},
	"iis_wmi":         {Kind: "iis_wmi", Version: "1", Status: "experimental", Permissions: []string{"windows_perf_counter"}, Rollback: "desativar check iis_wmi"},
	"windows_service": {Kind: "windows_service", Version: "1", Status: "experimental", Permissions: []string{"windows_service_query"}, Rollback: "desativar check windows_service"},
}

func integrationManifest(kind string) map[string]any {
	info, ok := integrationCatalog[normalizeKind(kind)]
	if !ok {
		info = integrationInfo{Kind: normalizeKind(kind), Version: "1", Status: "experimental", Rollback: "desativar check"}
	}
	return map[string]any{
		"kind":        info.Kind,
		"version":     info.Version,
		"status":      info.Status,
		"permissions": info.Permissions,
		"rollback":    info.Rollback,
	}
}

func activationGate(kind string, check config.LocalCheckConfig) (map[string]any, error) {
	info, ok := integrationCatalog[normalizeKind(kind)]
	if !ok || info.Status == "official" {
		return nil, nil
	}
	if integrationHomologated(check.Config) {
		return nil, nil
	}
	serviceCheck := map[string]any{
		"status":             "blocked",
		"integration":        integrationManifest(kind),
		"activation_blocked": true,
		"reason":             "integration_not_homologated",
	}
	return serviceCheck, errors.New("integracao local nao homologada para ativacao produtiva")
}

func integrationHomologated(values map[string]string) bool {
	if values == nil {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(values["homologation_status"]))
	ref := strings.TrimSpace(values["homologation_ref"])
	return (status == "approved" || status == "homologated" || status == "homologado") && ref != ""
}

func runJMX(ctx context.Context, check config.LocalCheckConfig, maxBytes int64) ([]map[string]any, []string, map[string]any, error) {
	mode := strings.ToLower(strings.TrimSpace(check.Config["mode"]))
	if mode == "" {
		mode = "jolokia"
	}
	serviceCheck := map[string]any{"status": "critical", "integration": integrationManifest("jmx"), "mode": mode}
	if mode != "jolokia" {
		return nil, nil, serviceCheck, errors.New("jmx suporta apenas Jolokia HTTP neste pacote")
	}
	target, err := validateHTTPURL(check.Target)
	if err != nil {
		return nil, nil, serviceCheck, err
	}
	client := &http.Client{
		Timeout:   timeoutFromContext(ctx),
		Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, serviceCheck, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, serviceCheck, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		serviceCheck["http_status"] = resp.StatusCode
		return nil, nil, serviceCheck, nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, nil, serviceCheck, err
	}
	metrics := parseJolokiaMetrics(raw)
	serviceCheck["status"] = "ok"
	serviceCheck["http_status"] = resp.StatusCode
	return metrics, nil, serviceCheck, nil
}

func parseJolokiaMetrics(raw []byte) []map[string]any {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	value, _ := doc["value"].(map[string]any)
	out := make([]map[string]any, 0, 8)
	add := func(name string, v any) {
		switch n := v.(type) {
		case float64:
			out = append(out, map[string]any{"name": name, "type": "gauge", "value": n, "integration": "jmx"})
		case int:
			out = append(out, map[string]any{"name": name, "type": "gauge", "value": n, "integration": "jmx"})
		}
	}
	if heap, ok := value["HeapMemoryUsage"].(map[string]any); ok {
		add("jvm.heap.used", heap["used"])
		add("jvm.heap.max", heap["max"])
	}
	add("jvm.threads.count", value["ThreadCount"])
	add("jvm.gc.collection_count", value["CollectionCount"])
	add("jvm.gc.collection_time_ms", value["CollectionTime"])
	return out
}

func runWindowsIntegration(ctx context.Context, check config.LocalCheckConfig) ([]map[string]any, []string, map[string]any, error) {
	kind := normalizeKind(check.Kind)
	serviceCheck := map[string]any{"status": "critical", "integration": integrationManifest(kind)}
	switch kind {
	case "iis_wmi":
		return runIISWindowsCounters(ctx, serviceCheck)
	case "windows_service":
		return runWindowsServiceCheck(ctx, check, serviceCheck)
	default:
		return nil, nil, serviceCheck, errors.New("integracao windows nao suportada")
	}
}

func runIISWindowsCounters(ctx context.Context, serviceCheck map[string]any) ([]map[string]any, []string, map[string]any, error) {
	if runtime.GOOS != "windows" {
		serviceCheck["status"] = "skipped"
		serviceCheck["reason"] = "windows_only"
		return nil, nil, serviceCheck, errors.New("integracao WMI/IIS exige Windows")
	}
	script := `$os=Get-CimInstance Win32_OperatingSystem; $iis=(Get-Counter '\Web Service(_Total)\Current Connections' -ErrorAction SilentlyContinue).CounterSamples[0].CookedValue; [pscustomobject]@{FreePhysicalMemoryKB=$os.FreePhysicalMemory;TotalVisibleMemoryKB=$os.TotalVisibleMemorySize;IisCurrentConnections=$iis} | ConvertTo-Json -Compress`
	raw, err := runPowerShellJSON(ctx, script)
	if err != nil {
		return nil, nil, serviceCheck, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, serviceCheck, err
	}
	metrics := make([]map[string]any, 0, 3)
	addMetric := func(name string, value any) {
		if n, ok := numericValue(value); ok {
			metrics = append(metrics, map[string]any{"name": name, "type": "gauge", "value": n, "integration": "iis_wmi"})
		}
	}
	addMetric("windows.memory.free_kb", doc["FreePhysicalMemoryKB"])
	addMetric("windows.memory.total_kb", doc["TotalVisibleMemorySize"])
	addMetric("iis.current_connections", doc["IisCurrentConnections"])
	serviceCheck["status"] = "ok"
	return metrics, nil, serviceCheck, nil
}

func runWindowsServiceCheck(ctx context.Context, check config.LocalCheckConfig, serviceCheck map[string]any) ([]map[string]any, []string, map[string]any, error) {
	serviceName := strings.TrimSpace(check.Config["service_name"])
	if serviceName == "" {
		serviceName = strings.TrimSpace(check.Target)
	}
	if !safeWindowsIdentifier(serviceName) {
		return nil, nil, serviceCheck, errors.New("windows service_name invalido")
	}
	if runtime.GOOS != "windows" {
		serviceCheck["status"] = "skipped"
		serviceCheck["reason"] = "windows_only"
		return nil, nil, serviceCheck, errors.New("integracao Windows Service exige Windows")
	}
	script := "$svc=Get-Service -Name '" + strings.ReplaceAll(serviceName, "'", "''") + "' -ErrorAction Stop; [pscustomobject]@{Name=$svc.Name;Status=$svc.Status.ToString()} | ConvertTo-Json -Compress"
	raw, err := runPowerShellJSON(ctx, script)
	if err != nil {
		return nil, nil, serviceCheck, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, serviceCheck, err
	}
	status := strings.ToLower(strings.TrimSpace(fmtAny(doc["Status"])))
	running := status == "running"
	serviceCheck["status"] = "critical"
	if running {
		serviceCheck["status"] = "ok"
	}
	serviceCheck["windows_status"] = status
	metrics := []map[string]any{{
		"name":        "windows.service.running",
		"type":        "gauge",
		"value":       boolNumber(running),
		"integration": "windows_service",
		"labels":      map[string]string{"service": serviceName},
	}}
	return metrics, nil, serviceCheck, nil
}

func runPowerShellJSON(ctx context.Context, script string) ([]byte, error) {
	return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
}

func safeWindowsIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func numericValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func boolNumber(value bool) int {
	if value {
		return 1
	}
	return 0
}

func fmtAny(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}
