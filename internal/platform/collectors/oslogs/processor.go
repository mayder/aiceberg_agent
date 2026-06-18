package oslogs

import (
	"encoding/json"
	"regexp"
	"strings"
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
	target := strings.Join([]string{
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

func severityAllowed(level, minSeverity string) bool {
	minRank, ok := severityRank(minSeverity)
	if !ok {
		return true
	}
	currentRank, ok := severityRank(level)
	if !ok {
		return true
	}
	return currentRank >= minRank
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
	p := strings.ToLower(path)
	f := strings.ToLower(facility)
	switch {
	case strings.Contains(p, "auth") || f == "auth" || f == "authpriv":
		return "security"
	case strings.Contains(p, "syslog") || strings.Contains(p, "messages"):
		return "observability"
	default:
		return "log"
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
