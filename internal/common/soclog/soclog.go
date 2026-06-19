package soclog

import (
	"strconv"
	"strings"
)

type Hints struct {
	Transport      string
	SourceTool     string
	SourceCategory string
	Path           string
	Channel        string
	Provider       string
	EventID        uint64
	Level          string
	Message        string
	Attributes     map[string]any
}

type Contract struct {
	Transport        string
	ToolOrigin       string
	SourceCategory   string
	SOCSourceType    string
	SOCEligible      string
	OriginConfidence string
	RouteReason      string
	Promoted         map[string]string
}

func Build(h Hints) Contract {
	tool := firstNonEmpty(configuredToolOrigin(h.Attributes), h.SourceTool, inferToolOrigin(h))
	transport := firstNonEmpty(attrString(h.Attributes, "aiceberg_transport"), h.Transport)
	category := canonicalSourceCategory(firstNonEmpty(attrString(h.Attributes, "aiceberg_source_category"), h.SourceCategory))
	confidence := "inferred"
	if attrString(h.Attributes, "aiceberg_tool_origin") != "" || attrString(h.Attributes, "aiceberg_source_category") != "" {
		confidence = "configured"
	}
	if tool == "" || tool == "unknown" {
		tool = "unknown"
		confidence = "unknown"
	}
	contract := Contract{
		Transport:        transport,
		ToolOrigin:       tool,
		SourceCategory:   category,
		SOCSourceType:    "none",
		SOCEligible:      "no",
		OriginConfidence: confidence,
		RouteReason:      "default_non_security_source",
		Promoted:         promotedFields(h),
	}
	classify(&contract, h)
	if configured := attrString(h.Attributes, "aiceberg_soc_source_type"); configured != "" {
		contract.SOCSourceType = canonicalSOCSourceType(configured)
		contract.OriginConfidence = "configured"
	}
	if configured := attrString(h.Attributes, "aiceberg_soc_eligible"); configured != "" {
		contract.SOCEligible = canonicalEligibility(configured)
		contract.OriginConfidence = "configured"
	}
	if configured := attrString(h.Attributes, "aiceberg_route_reason"); configured != "" {
		contract.RouteReason = configured
		contract.OriginConfidence = "configured"
	}
	return contract
}

func EnrichMap(event map[string]any) {
	if event == nil {
		return
	}
	attrs, _ := event["attributes"].(map[string]any)
	attrs = mergeTopLevelContract(attrs, event)
	hints := Hints{
		Transport:      stringValue(event["transport"]),
		SourceTool:     stringValue(event["source_tool"]),
		SourceCategory: stringValue(event["source_category"]),
		Path:           firstNonEmpty(stringValue(event["path"]), stringValue(event["file"])),
		Channel:        stringValue(event["channel"]),
		Provider:       stringValue(event["provider"]),
		EventID:        uintValue(firstNonEmpty(stringValue(event["event_id"]), stringValue(event["event_code"]))),
		Level:          firstNonEmpty(stringValue(event["level"]), stringValue(event["severity"])),
		Message:        stringValue(event["message"]),
		Attributes:     attrs,
	}
	contract := Build(hints)
	apply(event, contract)
}

func mergeTopLevelContract(attrs map[string]any, event map[string]any) map[string]any {
	out := attrs
	for _, key := range []string{
		"aiceberg_transport",
		"aiceberg_tool_origin",
		"aiceberg_source_category",
		"aiceberg_soc_source_type",
		"aiceberg_soc_eligible",
		"aiceberg_origin_confidence",
		"aiceberg_route_reason",
	} {
		value := stringValue(event[key])
		if value == "" {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		out[key] = value
	}
	return out
}

func apply(event map[string]any, contract Contract) {
	event["aiceberg_transport"] = contract.Transport
	event["aiceberg_tool_origin"] = contract.ToolOrigin
	event["aiceberg_source_category"] = contract.SourceCategory
	event["aiceberg_soc_source_type"] = contract.SOCSourceType
	event["aiceberg_soc_eligible"] = contract.SOCEligible
	event["aiceberg_origin_confidence"] = contract.OriginConfidence
	event["aiceberg_route_reason"] = contract.RouteReason
	for key, value := range contract.Promoted {
		if _, exists := event[key]; !exists && value != "" {
			event[key] = value
		}
	}
}

func classify(c *Contract, h Hints) {
	tool := strings.ToLower(c.ToolOrigin)
	channel := strings.ToLower(strings.TrimSpace(h.Channel))
	path := strings.ToLower(strings.TrimSpace(h.Path))
	provider := strings.ToLower(strings.TrimSpace(h.Provider))
	message := strings.ToLower(strings.TrimSpace(h.Message))

	switch {
	case tool == "ad_security" || channel == "security" || strings.Contains(provider, "security-auditing"):
		c.ToolOrigin = "ad_security"
		c.SourceCategory = "soc"
		c.SOCSourceType = "windows_security"
		if isHighValueWindowsSecurityEvent(h.EventID) {
			c.SOCEligible = "yes"
			c.RouteReason = "windows_security_event"
		} else {
			c.SOCEligible = "conditional"
			c.RouteReason = "windows_security_context"
		}
	case strings.Contains(channel, "sysmon") || strings.Contains(provider, "sysmon"):
		c.ToolOrigin = "windows_eventlog"
		c.SourceCategory = "soc"
		c.SOCSourceType = "windows_security"
		c.SOCEligible = "yes"
		c.RouteReason = "sysmon_security_telemetry"
	case tool == "windows_eventlog":
		c.SourceCategory = "observability"
		c.SOCSourceType = "none"
		c.SOCEligible = "no"
		c.RouteReason = "windows_operational_eventlog"
		if h.EventID == 10028 || strings.Contains(message, "distributedcom") {
			c.RouteReason = "windows_distributedcom_operational"
		}
	case tool == "linux_auth" || strings.Contains(path, "auth.log") || strings.HasSuffix(path, "/secure"):
		c.ToolOrigin = "linux_auth"
		c.SourceCategory = "soc"
		c.SOCSourceType = "linux_security"
		if securityMessage(message) || severityAtLeast(h.Level, "error") {
			c.SOCEligible = "yes"
			c.RouteReason = "linux_auth_security_event"
		} else {
			c.SOCEligible = "conditional"
			c.RouteReason = "linux_auth_context"
		}
	case tool == "graylog_gelf":
		c.Transport = firstNonEmpty(c.Transport, "graylog")
		classifyGraylog(c, h)
	case isSecurityVendor(h.Attributes):
		c.SourceCategory = "soc"
		c.SOCSourceType = securityVendorType(h.Attributes)
		c.SOCEligible = "yes"
		c.RouteReason = "security_vendor_fields"
	case tool == "opentelemetry" || tool == "otlp_log":
		c.ToolOrigin = "otlp_log"
		if c.SourceCategory != "soc" {
			c.SourceCategory = "conditional"
		}
		c.SOCSourceType = "application"
		c.SOCEligible = "conditional"
		c.RouteReason = "otlp_application_log"
	case tool == "docker" || tool == "container_log":
		c.ToolOrigin = "container_log"
		if c.SourceCategory != "soc" {
			c.SourceCategory = "conditional"
		}
		c.SOCSourceType = "container"
		c.SOCEligible = "conditional"
		c.RouteReason = "container_application_log"
	case tool == "kubernetes" || tool == "kubernetes_log":
		c.ToolOrigin = "kubernetes_log"
		if c.SourceCategory != "soc" {
			c.SourceCategory = "conditional"
		}
		c.SOCSourceType = "kubernetes"
		c.SOCEligible = "conditional"
		c.RouteReason = "kubernetes_application_log"
	case tool == "journald":
		if strings.Contains(path, "ssh") || strings.Contains(path, "auth") || securityMessage(message) {
			c.SourceCategory = "soc"
			c.SOCSourceType = "linux_security"
			c.SOCEligible = "yes"
			c.RouteReason = "journald_security_event"
		} else {
			c.SourceCategory = "observability"
			c.SOCSourceType = "none"
			c.SOCEligible = "no"
			c.RouteReason = "journald_operational_event"
		}
	case tool == "linux_syslog":
		c.SourceCategory = "observability"
		c.SOCSourceType = "none"
		c.SOCEligible = "no"
		c.RouteReason = "linux_syslog_operational"
	case tool == "application":
		if c.SourceCategory != "soc" {
			c.SourceCategory = "conditional"
		}
		c.SOCSourceType = "application"
		c.SOCEligible = "conditional"
		c.RouteReason = "application_log"
	}
}

func classifyGraylog(c *Contract, h Hints) {
	if origin := configuredToolOrigin(h.Attributes); origin != "" && origin != "graylog_gelf" {
		c.ToolOrigin = origin
		c.OriginConfidence = "configured"
		classify(c, Hints{
			Transport:      c.Transport,
			SourceTool:     origin,
			SourceCategory: c.SourceCategory,
			Path:           h.Path,
			Channel:        h.Channel,
			Provider:       h.Provider,
			EventID:        h.EventID,
			Level:          h.Level,
			Message:        h.Message,
			Attributes:     h.Attributes,
		})
		return
	}
	if isSecurityVendor(h.Attributes) {
		c.SourceCategory = "soc"
		c.SOCSourceType = securityVendorType(h.Attributes)
		c.SOCEligible = "yes"
		c.RouteReason = "graylog_security_vendor_fields"
		return
	}
	c.SourceCategory = "conditional"
	c.SOCSourceType = "none"
	c.SOCEligible = "conditional"
	c.RouteReason = "graylog_unknown_origin"
}

func promotedFields(h Hints) map[string]string {
	attrs := h.Attributes
	out := map[string]string{}
	if h.EventID > 0 {
		out["event_code"] = strconv.FormatUint(h.EventID, 10)
	}
	for target, keys := range map[string][]string{
		"event_code":   {"event_code", "event_id", "event.id", "winlog.event_id"},
		"vendor":       {"vendor", "device_vendor", "observer.vendor", "event.vendor"},
		"product":      {"product", "device_product", "observer.product", "event.product"},
		"src_ip":       {"src_ip", "source_ip", "source.ip", "client.ip", "src"},
		"dst_ip":       {"dst_ip", "destination_ip", "destination.ip", "server.ip", "dst"},
		"src_host":     {"src_host", "source_host", "source.host.name", "client.hostname"},
		"dst_host":     {"dst_host", "destination_host", "destination.host.name", "server.hostname"},
		"username":     {"username", "user", "user.name", "TargetUserName", "SubjectUserName"},
		"process_name": {"process_name", "process.name", "Image", "NewProcessName"},
		"command_line": {"command_line", "process.command_line", "CommandLine"},
		"file_hash":    {"file_hash", "hash", "file.hash.sha256", "sha256"},
		"domain":       {"domain", "user.domain", "TargetDomainName"},
		"url":          {"url", "url.full", "http.url"},
		"action":       {"action", "event.action", "act"},
		"rule_name":    {"rule_name", "rule.name", "alert.rule"},
		"technique_id": {"technique_id", "threat.technique.id", "mitre.technique_id"},
		"alert_id":     {"alert_id", "event.id", "alert.id"},
	} {
		for _, key := range keys {
			value := attrString(attrs, key)
			if value == "" || isSensitiveKey(key) || isSensitiveValue(value) {
				continue
			}
			out[target] = value
			break
		}
	}
	return out
}

func inferToolOrigin(h Hints) string {
	path := strings.ToLower(strings.TrimSpace(h.Path))
	switch {
	case strings.EqualFold(h.Channel, "Security"):
		return "ad_security"
	case strings.Contains(strings.ToLower(h.Channel), "sysmon"):
		return "windows_eventlog"
	case strings.Contains(path, "auth.log") || strings.HasSuffix(path, "/secure"):
		return "linux_auth"
	case strings.Contains(path, "syslog") || strings.Contains(path, "messages"):
		return "linux_syslog"
	case strings.Contains(path, "journald"):
		return "journald"
	default:
		return "application"
	}
}

func configuredToolOrigin(attrs map[string]any) string {
	for _, key := range []string{"aiceberg_tool_origin", "tool_origin", "_aiceberg_tool_origin", "event.module", "observer.product"} {
		if value := attrString(attrs, key); value != "" {
			return canonicalToolOrigin(value)
		}
	}
	if vendor := strings.ToLower(attrString(attrs, "vendor")); strings.Contains(vendor, "crowdstrike") {
		return "crowdstrike"
	}
	return ""
}

func canonicalToolOrigin(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	v = strings.ReplaceAll(v, "-", "_")
	v = strings.ReplaceAll(v, " ", "_")
	switch {
	case strings.Contains(v, "crowdstrike"):
		return "crowdstrike"
	case strings.Contains(v, "darktrace"):
		return "darktrace"
	case strings.Contains(v, "fortinet") || strings.Contains(v, "fortigate"):
		return "fortinet"
	case strings.Contains(v, "palo_alto") || strings.Contains(v, "panw"):
		return "palo_alto"
	case strings.Contains(v, "windows_security") || strings.Contains(v, "ad_security"):
		return "ad_security"
	case strings.Contains(v, "windows_event"):
		return "windows_eventlog"
	case strings.Contains(v, "linux_auth"):
		return "linux_auth"
	case strings.Contains(v, "linux_syslog"):
		return "linux_syslog"
	case strings.Contains(v, "graylog"):
		return "graylog_gelf"
	case strings.Contains(v, "kubernetes"):
		return "kubernetes_log"
	case strings.Contains(v, "container") || strings.Contains(v, "docker"):
		return "container_log"
	case strings.Contains(v, "otlp") || strings.Contains(v, "opentelemetry"):
		return "otlp_log"
	default:
		return v
	}
}

func canonicalSourceCategory(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "soc", "security":
		return "soc"
	case "noc":
		return "noc"
	case "observability", "ops", "operational", "pod_log", "container_log":
		return "observability"
	case "conditional":
		return "conditional"
	default:
		return "log"
	}
}

func canonicalSOCSourceType(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "edr", "xdr", "ndr", "firewall", "ad_dc", "iam", "dns", "vpn", "windows_security", "linux_security", "cloud", "waf", "application", "container", "kubernetes", "none":
		return v
	default:
		return "none"
	}
}

func canonicalEligibility(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "1", "sim":
		return "yes"
	case "conditional", "condicional":
		return "conditional"
	default:
		return "no"
	}
}

func isHighValueWindowsSecurityEvent(eventID uint64) bool {
	switch eventID {
	case 4625, 4672, 4688, 4697, 4720, 4726, 4728, 4732, 4735, 4740, 7045:
		return true
	default:
		return false
	}
}

func securityMessage(message string) bool {
	return strings.Contains(message, "failed password") ||
		strings.Contains(message, "authentication failure") ||
		strings.Contains(message, "invalid user") ||
		strings.Contains(message, "failed logon") ||
		strings.Contains(message, "sudo:") ||
		strings.Contains(message, "privilege") ||
		strings.Contains(message, "attack") ||
		strings.Contains(message, "malware") ||
		strings.Contains(message, "blocked") ||
		strings.Contains(message, "deny")
}

func isSecurityVendor(attrs map[string]any) bool {
	origin := configuredToolOrigin(attrs)
	switch origin {
	case "crowdstrike", "darktrace", "fortinet", "palo_alto":
		return true
	default:
		return false
	}
}

func securityVendorType(attrs map[string]any) string {
	switch configuredToolOrigin(attrs) {
	case "crowdstrike":
		return "edr"
	case "darktrace":
		return "ndr"
	case "fortinet", "palo_alto":
		return "firewall"
	default:
		return "none"
	}
}

func severityAtLeast(level, minimum string) bool {
	current, okCurrent := severityRank(level)
	min, okMin := severityRank(minimum)
	return okCurrent && okMin && current >= min
}

func severityRank(value string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug", "trace", "verbose":
		return 1, true
	case "info", "information", "notice":
		return 2, true
	case "warn", "warning":
		return 3, true
	case "err", "error":
		return 4, true
	case "crit", "critical", "fatal", "emerg", "emergency", "alert":
		return 5, true
	default:
		return 0, false
	}
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	default:
		return ""
	}
}

func uintValue(value string) uint64 {
	v, _ := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return v
}

func attrString(attrs map[string]any, key string) string {
	if attrs == nil {
		return ""
	}
	if value := stringValue(attrs[key]); value != "" {
		return value
	}
	for current, value := range attrs {
		if strings.EqualFold(current, key) {
			return stringValue(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "passwd") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "cookie") ||
		strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "apikey")
}

func isSensitiveValue(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "bearer ") ||
		strings.Contains(lower, "basic ") ||
		strings.Contains(lower, "password=") ||
		strings.Contains(lower, "token=") ||
		strings.Contains(lower, "secret=")
}
