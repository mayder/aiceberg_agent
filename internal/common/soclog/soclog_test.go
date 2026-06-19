package soclog

import "testing"

func TestBuildClassifiesWindowsSecurityAndOperationalDCOM(t *testing.T) {
	security := Build(Hints{
		Transport:      "agent_windows_eventlog",
		SourceTool:     "windows_eventlog",
		SourceCategory: "security",
		Channel:        "Security",
		Provider:       "Microsoft-Windows-Security-Auditing",
		EventID:        4728,
		Level:          "info",
		Message:        "A member was added to a security-enabled global group",
	})
	if security.ToolOrigin != "ad_security" || security.SourceCategory != "soc" || security.SOCSourceType != "windows_security" || security.SOCEligible != "yes" {
		t.Fatalf("unexpected windows security contract: %#v", security)
	}

	dcom := Build(Hints{
		Transport:      "agent_windows_eventlog",
		SourceTool:     "windows_eventlog",
		SourceCategory: "observability",
		Channel:        "System",
		Provider:       "Microsoft-Windows-DistributedCOM",
		EventID:        10028,
		Level:          "error",
		Message:        "DistributedCOM remote activation failed",
	})
	if dcom.SourceCategory != "observability" || dcom.SOCEligible != "no" || dcom.RouteReason != "windows_distributedcom_operational" {
		t.Fatalf("unexpected DCOM contract: %#v", dcom)
	}
}

func TestBuildClassifiesGraylogSecurityVendorAndLinuxAuth(t *testing.T) {
	graylog := Build(Hints{
		Transport:  "agent_file",
		SourceTool: "graylog_gelf",
		Level:      "error",
		Message:    "blocked exploit",
		Attributes: map[string]any{
			"short_message":          "blocked exploit",
			"host":                   "fw01",
			"version":                "1.1",
			"aiceberg_tool_origin":   "fortinet",
			"aiceberg_transport":     "graylog",
			"src_ip":                 "10.0.0.10",
			"destination.ip":         "192.0.2.10",
			"authorization":          "Bearer must-not-promote",
			"process.command_line":   "curl https://example.invalid",
			"threat.technique.id":    "T1190",
			"password":               "must-not-promote",
			"event.action":           "deny",
			"rule.name":              "IPS critical",
			"observer.product":       "FortiGate",
			"device_vendor":          "Fortinet",
			"device_product":         "FortiGate",
			"event.id":               "alert-1",
			"file.hash.sha256":       "abc",
			"source.host.name":       "src-host",
			"destination.host.name":  "dst-host",
			"user.name":              "alice",
			"destination_ip_ignored": "not-used",
		},
	})
	if graylog.Transport != "graylog" || graylog.ToolOrigin != "fortinet" || graylog.SOCSourceType != "firewall" || graylog.SOCEligible != "yes" {
		t.Fatalf("unexpected graylog contract: %#v", graylog)
	}
	if graylog.Promoted["src_ip"] != "10.0.0.10" || graylog.Promoted["dst_ip"] != "192.0.2.10" || graylog.Promoted["command_line"] == "" {
		t.Fatalf("expected promoted SOC fields, got %#v", graylog.Promoted)
	}
	if _, ok := graylog.Promoted["password"]; ok {
		t.Fatalf("sensitive field was promoted: %#v", graylog.Promoted)
	}

	auth := Build(Hints{
		Transport:  "agent_file",
		SourceTool: "linux_auth",
		Path:       "/var/log/auth.log",
		Level:      "error",
		Message:    "Failed password for invalid user root from 10.0.0.5",
	})
	if auth.SourceCategory != "soc" || auth.SOCSourceType != "linux_security" || auth.SOCEligible != "yes" {
		t.Fatalf("unexpected linux auth contract: %#v", auth)
	}
}

func TestEnrichMapAddsCanonicalFields(t *testing.T) {
	event := map[string]any{
		"transport":       "kubernetes_pod_log",
		"source_tool":     "kubernetes",
		"source_category": "pod_log",
		"message":         "application error",
		"severity":        "error",
		"attributes": map[string]any{
			"service.name": "api",
		},
	}
	EnrichMap(event)
	if event["aiceberg_tool_origin"] != "kubernetes_log" || event["aiceberg_source_category"] != "conditional" || event["aiceberg_soc_source_type"] != "kubernetes" {
		t.Fatalf("unexpected enriched event: %#v", event)
	}
}
