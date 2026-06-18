package localchecks

import (
	"bufio"
	"io"
	"strconv"
	"strings"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func parseOpenMetricsForCheck(r io.Reader, check config.LocalCheckConfig) []map[string]any {
	maxMetrics := configInt(check.Config, "max_metrics", 200)
	if maxMetrics <= 0 || maxMetrics > 1000 {
		maxMetrics = 200
	}
	maxLabelValues := configInt(check.Config, "max_label_values", 50)
	if maxLabelValues <= 0 || maxLabelValues > 500 {
		maxLabelValues = 50
	}
	metricAllowlist := splitCSVSet(check.Config["metric_allowlist"])
	labelAllowlist := splitCSVSet(check.Config["label_allowlist"])
	labelValues := make(map[string]map[string]struct{})

	out := []map[string]any{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, labelsRaw, value, ok := parseOpenMetricsLine(line)
		if !ok || !metricAllowed(name, metricAllowlist) {
			continue
		}
		labels := filterLabels(labelsRaw, labelAllowlist, labelValues, maxLabelValues)
		row := map[string]any{"name": name, "type": "gauge", "value": value}
		if len(labels) > 0 {
			row["labels"] = labels
		}
		out = append(out, row)
		if len(out) >= maxMetrics {
			break
		}
	}
	return out
}

func parseOpenMetricsLine(line string) (string, map[string]string, float64, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", nil, 0, false
	}
	value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
	if err != nil {
		return "", nil, 0, false
	}
	namePart := fields[0]
	labels := map[string]string{}
	if idx := strings.Index(namePart, "{"); idx >= 0 {
		end := strings.LastIndex(namePart, "}")
		if end > idx {
			labels = parseOpenMetricsLabels(namePart[idx+1 : end])
		}
		namePart = namePart[:idx]
	}
	name := sanitizeMetricName(namePart)
	if name == "" {
		return "", nil, 0, false
	}
	return name, labels, value, true
}

func parseOpenMetricsLabels(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = sanitizeLabelName(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if key == "" || value == "" {
			continue
		}
		if len(value) > 120 {
			value = value[:120]
		}
		out[key] = value
	}
	return out
}

func filterLabels(labels map[string]string, allowlist map[string]struct{}, seen map[string]map[string]struct{}, maxValues int) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range labels {
		if len(allowlist) > 0 {
			if _, ok := allowlist[key]; !ok {
				continue
			}
		}
		if seen[key] == nil {
			seen[key] = make(map[string]struct{})
		}
		if _, exists := seen[key][value]; !exists && len(seen[key]) >= maxValues {
			continue
		}
		seen[key][value] = struct{}{}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func metricAllowed(name string, allowlist map[string]struct{}) bool {
	if len(allowlist) == 0 {
		return true
	}
	for item := range allowlist {
		if strings.HasSuffix(item, "*") && strings.HasPrefix(name, strings.TrimSuffix(item, "*")) {
			return true
		}
		if name == item {
			return true
		}
	}
	return false
}

func splitCSVSet(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}

func configInt(values map[string]string, key string, fallback int) int {
	if values == nil {
		return fallback
	}
	raw := strings.TrimSpace(values[key])
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func sanitizeMetricName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == ':' {
			b.WriteRune(r)
		}
		if b.Len() >= 200 {
			break
		}
	}
	return b.String()
}

func sanitizeLabelName(value string) string {
	return sanitizeMetricName(strings.TrimSpace(value))
}
