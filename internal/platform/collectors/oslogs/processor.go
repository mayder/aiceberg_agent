package oslogs

import (
	"encoding/json"
	"hash/fnv"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/you/aiceberg_agent/internal/common/config"
)

const logSchemaVersion = 1

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*:\s*(?:bearer|basic)\s+)[^\s]+`),
	regexp.MustCompile(`(?i)("(?:password|passwd|pwd|token|secret|api[_-]?key|cookie)"\s*:\s*)"[^"]*"`),
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|cookie)\s*=\s*("[^"]+"|'[^']+'|[^\s&;]+)`),
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|cookie)\s*:\s*("[^"]+"|'[^']+'|[^\s,}]+)`),
}

func redactMessage(message string) (string, string) {
	redacted := message
	for _, pattern := range sensitivePatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, func(match string) string {
			if strings.HasPrefix(strings.TrimSpace(match), `"`) {
				if idx := strings.Index(match, ":"); idx >= 0 {
					return match[:idx+1] + `"[redacted]"`
				}
			}
			if idx := strings.Index(match, "="); idx >= 0 {
				return match[:idx+1] + "[redacted]"
			}
			if idx := strings.Index(match, ":"); idx >= 0 {
				prefix := match[:idx+1]
				if strings.Contains(strings.ToLower(prefix), "authorization") {
					parts := strings.Fields(match)
					if len(parts) >= 2 {
						return parts[0] + " " + parts[1] + " [redacted]"
					}
				}
				return prefix + "[redacted]"
			}
			return "[redacted]"
		})
	}
	if redacted != message {
		return redacted, "redacted"
	}
	return message, "none"
}

func jsonAttributes(message string) map[string]any {
	msg := strings.TrimSpace(message)
	if msg == "" || (!strings.HasPrefix(msg, "{") && !strings.HasPrefix(msg, "[")) {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(msg), &data); err != nil {
		return nil
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		if isSensitiveKey(key) {
			out[k] = "[redacted]"
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shouldDropLogEvent(ev logEvent, includeRegex, excludeRegex, minSeverity string) bool {
	target := logEventTarget(ev)
	if minSeverity != "" && !severityAllowed(ev.Level, minSeverity) {
		return true
	}
	if includeRegex != "" {
		re, err := regexp.Compile(includeRegex)
		if err == nil && !re.MatchString(target) {
			return true
		}
	}
	if excludeRegex != "" {
		re, err := regexp.Compile(excludeRegex)
		if err == nil && re.MatchString(target) {
			return true
		}
	}
	return false
}

func processLogEvent(ev logEvent, processors []config.LogProcessorConfig) (logEvent, bool) {
	for _, processor := range processors {
		var keep bool
		ev, keep = applyLogProcessor(ev, processor)
		if !keep {
			return ev, false
		}
	}
	return enrichSOCEvent(ev), true
}

func applyLogProcessor(ev logEvent, processor config.LogProcessorConfig) (logEvent, bool) {
	switch strings.ToLower(strings.TrimSpace(processor.Type)) {
	case "parse":
		return processParse(ev), true
	case "remap":
		return processRemap(ev, processor), true
	case "drop":
		return ev, !processorMatches(ev, processor.Pattern)
	case "mask":
		return processMask(ev, processor), true
	case "route":
		return processRoute(ev, processor), true
	case "sample":
		return ev, processorSampleKeeps(ev, processor)
	case "enrich":
		return processEnrich(ev, processor), true
	default:
		return ev, true
	}
}

func sanitizeLogProcessors(processors []config.LogProcessorConfig) []config.LogProcessorConfig {
	out := make([]config.LogProcessorConfig, 0, len(processors))
	for _, processor := range processors {
		kind := strings.ToLower(strings.TrimSpace(processor.Type))
		if !isSupportedLogProcessor(kind) {
			continue
		}
		processor.Type = kind
		out = append(out, processor)
	}
	return out
}

func isSupportedLogProcessor(kind string) bool {
	switch kind {
	case "parse", "remap", "drop", "mask", "route", "sample", "enrich":
		return true
	default:
		return false
	}
}

func processParse(ev logEvent) logEvent {
	if ev.Attributes == nil {
		ev.Attributes = jsonAttributes(ev.Message)
	}
	return ev
}

func processRemap(ev logEvent, processor config.LogProcessorConfig) logEvent {
	value := eventAttributeString(ev, processor.Key)
	if value == "" {
		value = processor.Value
	}
	return setEventField(ev, processor.Field, value)
}

func processMask(ev logEvent, processor config.LogProcessorConfig) logEvent {
	pattern, err := regexp.Compile(processor.Pattern)
	if err != nil || processor.Pattern == "" {
		return ev
	}
	replacement := processor.Replacement
	if replacement == "" {
		replacement = "[redacted]"
	}
	ev.Message = pattern.ReplaceAllString(ev.Message, replacement)
	ev.Attributes = maskStringAttributes(ev.Attributes, pattern, replacement)
	ev.RedactionStatus = "redacted"
	return ev
}

func processRoute(ev logEvent, processor config.LogProcessorConfig) logEvent {
	target := strings.TrimSpace(processor.Value)
	if target == "" {
		target = strings.TrimSpace(processor.Field)
	}
	if target != "" {
		ev.SourceCategory = target
	}
	return ev
}

func processEnrich(ev logEvent, processor config.LogProcessorConfig) logEvent {
	key := strings.TrimSpace(processor.Key)
	if key == "" {
		return ev
	}
	if ev.Attributes == nil {
		ev.Attributes = map[string]any{}
	}
	ev.Attributes[key] = processor.Value
	return ev
}

func processorSampleKeeps(ev logEvent, processor config.LogProcessorConfig) bool {
	if processor.Rate <= 0 {
		return false
	}
	if processor.Rate >= 1 {
		return true
	}
	key := eventAttributeString(ev, processor.Key)
	if key == "" {
		key = ev.Message
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return float64(hash.Sum32()%10000)/10000 < processor.Rate
}

func processorMatches(ev logEvent, pattern string) bool {
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(logEventTarget(ev))
}

func logEventTarget(ev logEvent) string {
	return strings.Join([]string{
		ev.Message,
		ev.Path,
		ev.Source,
		ev.Host,
		ev.Service,
		ev.Level,
		ev.Category,
		ev.SourceTool,
		ev.SourceCategory,
	}, " ")
}

func eventAttributeString(ev logEvent, key string) string {
	key = strings.TrimSpace(key)
	if key == "" || ev.Attributes == nil {
		return ""
	}
	value, ok := ev.Attributes[key]
	if !ok {
		return ""
	}
	if stringValue, ok := value.(string); ok {
		return stringValue
	}
	return ""
}

func setEventField(ev logEvent, field string, value string) logEvent {
	if value == "" {
		return ev
	}
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "service":
		ev.Service = value
	case "level":
		ev.Level = value
	case "severity":
		ev.Severity = value
	case "category":
		ev.Category = value
	case "source_category":
		ev.SourceCategory = value
	}
	return ev
}

func maskStringAttributes(attrs map[string]any, pattern *regexp.Regexp, replacement string) map[string]any {
	if attrs == nil {
		return nil
	}
	out := make(map[string]any, len(attrs))
	for key, value := range attrs {
		if stringValue, ok := value.(string); ok {
			out[key] = pattern.ReplaceAllString(stringValue, replacement)
			continue
		}
		out[key] = value
	}
	return out
}

func severityAllowed(level, minSeverity string) bool {
	minRank, ok := severityRank(minSeverity)
	if !ok {
		return true
	}
	currentRank, ok := severityRank(level)
	if !ok {
		return false
	}
	return currentRank >= minRank
}

func severityRank(value string) (int, bool) {
	switch normalizeSeverityString(value) {
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

func normalizeSeverityString(value string) string {
	v := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "\x00", "")))
	if v == "" {
		return ""
	}
	if n, err := strconv.Atoi(v); err == nil {
		return severityName(n)
	}
	switch v {
	case "err", "erro":
		return "error"
	case "warn", "aviso", "advertencia", "advertência":
		return "warning"
	case "crit", "critico", "crítico", "fatal", "panic":
		return "critical"
	case "emerg":
		return "emergency"
	case "information", "informational", "informacao", "informação", "informacoes", "informações":
		return "info"
	default:
		return v
	}
}

func severityName(code int) string {
	switch code {
	case 0:
		return "emergency"
	case 1:
		return "alert"
	case 2:
		return "critical"
	case 3:
		return "error"
	case 4:
		return "warning"
	case 5:
		return "notice"
	case 6:
		return "info"
	case 7:
		return "debug"
	default:
		return ""
	}
}

func isSensitiveKey(key string) bool {
	return strings.Contains(key, "password") ||
		strings.Contains(key, "passwd") ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "authorization") ||
		strings.Contains(key, "cookie") ||
		strings.Contains(key, "api_key") ||
		strings.Contains(key, "apikey")
}

func sourceCategoryForUnix(path, facility string) string {
	p := strings.ToLower(filepath.Base(path))
	f := strings.ToLower(facility)
	switch {
	case isLinuxAuthPath(p) || f == "auth" || f == "authpriv":
		return "security"
	case strings.Contains(p, "syslog") || strings.Contains(p, "messages"):
		return "observability"
	default:
		return "log"
	}
}

func sourceToolForUnix(path string, attrs map[string]any) string {
	if isGraylogGELF(attrs) {
		return "graylog_gelf"
	}
	p := strings.ToLower(filepath.Base(path))
	switch {
	case isLinuxAuthPath(p):
		return "linux_auth"
	case strings.Contains(p, "syslog") || strings.Contains(p, "messages"):
		return "linux_syslog"
	case len(attrs) > 0:
		return "application"
	default:
		return "file"
	}
}

func isLinuxAuthPath(base string) bool {
	return strings.Contains(base, "auth") || base == "secure"
}

func isGraylogGELF(attrs map[string]any) bool {
	if attrs == nil {
		return false
	}
	if _, ok := attrs["short_message"]; !ok {
		return false
	}
	if _, ok := attrs["host"]; !ok {
		return false
	}
	if _, ok := attrs["version"]; ok {
		return true
	}
	if _, ok := attrs["_gl2_source_input"]; ok {
		return true
	}
	return false
}

func graylogMessage(attrs map[string]any) string {
	if short := attrString(attrs, "short_message"); short != "" {
		return short
	}
	return attrString(attrs, "full_message")
}

func serviceFromAttributes(attrs map[string]any) string {
	for _, key := range []string{"service", "service.name", "app", "application", "application_name", "program", "_app", "_program"} {
		if value := attrString(attrs, key); value != "" {
			return value
		}
	}
	return ""
}

func attrString(attrs map[string]any, key string) string {
	if attrs == nil {
		return ""
	}
	value, ok := attrs[key]
	if !ok {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func looksLikeNewLogEntry(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "{") {
		return true
	}
	if len(trimmed) >= 10 && trimmed[4] == '-' && trimmed[7] == '-' {
		return true
	}
	if len(trimmed) >= 15 && trimmed[3] == ' ' {
		month := strings.ToLower(trimmed[:3])
		switch month {
		case "jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec":
			return true
		}
	}
	return false
}
