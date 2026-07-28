//go:build !windows
// +build !windows

package oslogs

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type wordpressAccessSignal struct {
	timestamp time.Time
	method    string
	path      string
	status    int
	bodyBytes int64
	sourceIP  string
	username  string
	userAgent string
	action    string
	level     string
	queryKeys []string
}

func parseWordPressAccessSignal(line string) (wordpressAccessSignal, bool) {
	match := nginxCombinedAccessPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 10 {
		return wordpressAccessSignal{}, false
	}

	requestURL, err := url.Parse(match[5])
	if err != nil {
		return wordpressAccessSignal{}, false
	}
	path := requestURL.EscapedPath()
	if path == "" {
		path = "/"
	}
	if !isWordPressHTTPPath(path, requestURL.Query()) {
		return wordpressAccessSignal{}, false
	}

	status, err := strconv.Atoi(match[6])
	if err != nil {
		return wordpressAccessSignal{}, false
	}
	bodyBytes, _ := strconv.ParseInt(match[7], 10, 64)
	occurredAt, err := time.Parse("02/Jan/2006:15:04:05 -0700", match[3])
	if err != nil {
		return wordpressAccessSignal{}, false
	}

	signal := wordpressAccessSignal{
		timestamp: occurredAt.UTC(),
		method:    strings.ToUpper(match[4]),
		path:      path,
		status:    status,
		bodyBytes: bodyBytes,
		sourceIP:  match[1],
		username:  emptyDash(match[2]),
		userAgent: safeAccessField(match[9], 512),
		queryKeys: safeQueryKeys(requestURL.Query()),
	}
	signal.action, signal.level = classifyWordPressAccess(signal, requestURL.Query())
	return signal, signal.action != ""
}

func applyWordPressAccessSignal(ev logEvent, signal wordpressAccessSignal) logEvent {
	ev.Timestamp = signal.timestamp.Format(time.RFC3339)
	ev.TimestampUTC = ev.Timestamp
	ev.Message = strings.Join([]string{
		"wordpress_http_access",
		"action=" + signal.action,
		"method=" + signal.method,
		"path=" + signal.path,
		"status=" + strconv.Itoa(signal.status),
	}, " ")
	ev.Level = signal.level
	ev.Severity = signal.level
	ev.App = "wordpress"
	ev.Service = "wordpress"
	ev.SourceTool = "nginx_access"
	ev.SourceCategory = "soc"
	ev.Category = "web_security"
	ev.SrcIP = signal.sourceIP
	ev.Username = signal.username
	ev.URL = signal.path
	ev.Action = signal.action
	ev.Product = "wordpress"
	ev.RedactionStatus = "query_values_removed"
	ev.Attributes = map[string]any{
		"aiceberg_tool_origin":     "nginx_access",
		"aiceberg_source_category": "soc",
		"aiceberg_soc_source_type": "application",
		"aiceberg_soc_eligible":    "yes",
		"aiceberg_route_reason":    "wordpress_security_http_signal",
		"product":                  "wordpress",
		"source_ip":                signal.sourceIP,
		"http_method":              signal.method,
		"url_path":                 signal.path,
		"http_status":              strconv.Itoa(signal.status),
		"http_response_bytes":      strconv.FormatInt(signal.bodyBytes, 10),
		"user_agent":               signal.userAgent,
		"action":                   signal.action,
		"query_keys":               strings.Join(signal.queryKeys, ","),
	}
	if signal.username != "" {
		ev.Attributes["username"] = signal.username
	}
	return enrichSOCEvent(ev)
}

func classifyWordPressAccess(signal wordpressAccessSignal, query url.Values) (string, string) {
	path := strings.ToLower(signal.path)
	restRoute := strings.ToLower(query.Get("rest_route"))
	switch {
	case signal.method == "POST" && path == "/wp-json/batch/v1":
		return "wordpress_rest_batch", "error"
	case signal.method == "POST" && restRoute == "/batch/v1":
		return "wordpress_rest_route_batch", "error"
	case signal.method == "POST" && strings.HasPrefix(path, "/wp-json/wp/v2/users"):
		return "wordpress_user_create_rest", "critical"
	case signal.method == "POST" && path == "/wp-login.php" && signal.status >= 300 && signal.status < 400:
		return "wordpress_login_success_inferred", "error"
	case signal.method == "POST" && path == "/wp-admin/update.php" && strings.EqualFold(query.Get("action"), "upload-plugin"):
		return "wordpress_plugin_upload", "error"
	case strings.HasPrefix(path, "/wp-admin/plugins.php") && strings.EqualFold(query.Get("action"), "activate"):
		return "wordpress_plugin_activation", "error"
	case isExecutableWordPressRequest(path) && hasCommandLikeQuery(query):
		return "wordpress_webshell_request", "critical"
	default:
		return "", ""
	}
}

func isWordPressHTTPPath(path string, query url.Values) bool {
	path = strings.ToLower(path)
	return strings.Contains(path, "/wp-json/") ||
		strings.Contains(path, "/wp-admin/") ||
		strings.Contains(path, "/wp-content/") ||
		strings.HasSuffix(path, "/wp-login.php") ||
		strings.Contains(strings.ToLower(query.Get("rest_route")), "/batch/v1")
}

func isExecutableWordPressRequest(path string) bool {
	lower := strings.ToLower(path)
	return (strings.Contains(lower, "/wp-content/uploads/") || strings.Contains(lower, "/wp-content/plugins/")) &&
		(strings.HasSuffix(lower, ".php") || strings.HasSuffix(lower, ".phtml") || strings.HasSuffix(lower, ".phar"))
}

func hasCommandLikeQuery(query url.Values) bool {
	for _, key := range []string{"c", "cmd", "command", "exec", "shell", "system"} {
		if _, exists := query[key]; exists {
			return true
		}
	}
	return false
}

func safeQueryKeys(query url.Values) []string {
	keys := make([]string, 0, len(query))
	for key := range query {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" || len(key) > 64 {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 20 {
		keys = keys[:20]
	}
	return keys
}

func safeAccessField(value string, max int) string {
	value = strings.TrimSpace(value)
	if value == "-" {
		return ""
	}
	if len(value) > max {
		return value[:max]
	}
	return value
}

func emptyDash(value string) string {
	if value == "-" {
		return ""
	}
	return safeAccessField(value, 191)
}
