package localchecks

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
