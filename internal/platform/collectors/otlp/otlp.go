package otlp

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/soclog"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const (
	schemaVersion      = 1
	maxAttributeCount  = 32
	maxAttributeLength = 256
)

type Receiver struct {
	prefs                func() config.CollectPrefs
	baseEnabled          bool
	addr                 string
	interval             time.Duration
	maxItems             int
	maxBytes             int
	traceSampleRate      float64
	traceSlowThresholdMs int
	tracePreserveErrors  bool
	include              string
	exclude              string
	minSeverity          string
	startOnce            sync.Once
	startErr             error
	server               *http.Server
	store                *store
}

func NewReceiver(cfg config.Config, prefsProvider func() config.CollectPrefs) *Receiver {
	interval := cfg.OTLPInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	maxItems := cfg.OTLPMaxItems
	if maxItems <= 0 {
		maxItems = 1000
	}
	maxBytes := cfg.OTLPMaxBytes
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	traceSampleRate := cfg.APMTraceSampleRate
	if traceSampleRate == 0 && cfg.APMTraceSlowThresholdMs == 0 && !cfg.APMTracePreserveErrors {
		traceSampleRate = 1
	}
	return &Receiver{
		prefs:                prefsProvider,
		baseEnabled:          cfg.OTLPEnabled,
		addr:                 cfg.OTLPHTTPAddr,
		interval:             interval,
		maxItems:             maxItems,
		maxBytes:             maxBytes,
		traceSampleRate:      normalizeSampleRate(traceSampleRate),
		traceSlowThresholdMs: cfg.APMTraceSlowThresholdMs,
		tracePreserveErrors:  cfg.APMTracePreserveErrors,
		include:              cfg.OSLogIncludeRegex,
		exclude:              cfg.OSLogExcludeRegex,
		minSeverity:          cfg.OSLogMinSeverity,
		store:                &store{maxItems: maxItems},
	}
}

func (r *Receiver) MetricsCollector() ports.Collector {
	return &collector{receiver: r, kind: "metrics"}
}
func (r *Receiver) LogsCollector() ports.Collector   { return &collector{receiver: r, kind: "logs"} }
func (r *Receiver) TracesCollector() ports.Collector { return &collector{receiver: r, kind: "traces"} }

func (r *Receiver) enabled() bool {
	p := config.CollectPrefs{}
	if r.prefs != nil {
		p = r.prefs()
	}
	if strings.TrimSpace(p.Version) == "" {
		return r.baseEnabled
	}
	return p.OTLPEnabled
}

func (r *Receiver) ensureStarted(ctx context.Context) error {
	if !r.enabled() {
		return nil
	}
	r.startOnce.Do(func() {
		r.startErr = r.start(ctx)
	})
	return r.startErr
}

func (r *Receiver) start(ctx context.Context) error {
	if strings.TrimSpace(r.addr) == "" {
		return errors.New("otlp http addr vazio")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/metrics", r.handleMetrics)
	mux.HandleFunc("/v1/logs", r.handleLogs)
	mux.HandleFunc("/v1/traces", r.handleTraces)
	server := &http.Server{Addr: r.addr, Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	ln, err := net.Listen("tcp", r.addr)
	if err != nil {
		return err
	}
	r.server = server
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			r.store.drop(1)
		}
	}()
	return nil
}

func (r *Receiver) handleMetrics(w http.ResponseWriter, req *http.Request) {
	r.handle(w, req, "metrics")
}
func (r *Receiver) handleLogs(w http.ResponseWriter, req *http.Request)   { r.handle(w, req, "logs") }
func (r *Receiver) handleTraces(w http.ResponseWriter, req *http.Request) { r.handle(w, req, "traces") }

func (r *Receiver) handle(w http.ResponseWriter, req *http.Request, kind string) {
	if req.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	host, _, _ := net.SplitHostPort(req.RemoteAddr)
	if host != "" && !isLoopback(net.ParseIP(host)) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, int64(r.maxBytes)+1))
	if err != nil || len(body) > r.maxBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	accepted := r.ingest(kind, body)
	_ = json.NewEncoder(w).Encode(map[string]any{"accepted": accepted})
}

func (r *Receiver) ingest(kind string, body []byte) int {
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		r.store.drop(1)
		return 0
	}
	resource := resourceAttributes(decoded)
	items := flattenOTLP(kind, decoded, resource)
	r.store.add(kind, items)
	return len(items)
}

type collector struct {
	receiver *Receiver
	kind     string
}

func (c *collector) Name() string            { return "otlp_" + c.kind }
func (c *collector) Interval() time.Duration { return c.receiver.interval }

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	if err := c.receiver.ensureStarted(ctx); err != nil {
		return nil, err
	}
	if !c.receiver.enabled() {
		return nil, nil
	}
	c.receiver.addValidationSample(c.kind, time.Now().UTC())
	snap := c.receiver.store.flush(c.kind)
	if len(snap.Items) == 0 && snap.DroppedCount == 0 {
		return nil, nil
	}
	snap.SchemaVersion = schemaVersion
	snap.Kind = c.kind
	snap.Source = "otlp_http_json"
	snap.FlushWindowSec = int(c.receiver.interval.Seconds())
	if c.kind == "logs" {
		events := make([]map[string]any, 0, len(snap.Items))
		droppedCount := snap.DroppedCount
		include, exclude, minSeverity := c.receiver.logFilters()
		for _, item := range snap.Items {
			message, redactionStatus := redactLogMessage(stringValue(item["message"]))
			event := map[string]any{
				"schema_version":   schemaVersion,
				"timestamp":        stringValue(item["timestamp_utc"]),
				"timestamp_utc":    stringValue(item["timestamp_utc"]),
				"source":           firstString(item, "host", "service", "source"),
				"message":          message,
				"severity":         stringValue(item["severity"]),
				"service":          stringValue(item["service"]),
				"attributes":       redactAttributes(item["attributes"]),
				"redaction_status": redactionStatus,
				"transport":        "otlp_http_json",
				"source_tool":      "opentelemetry",
				"source_category":  "conditional",
				"trace_id":         stringValue(item["trace_id"]),
				"span_id":          stringValue(item["span_id"]),
			}
			soclog.EnrichMap(event)
			if shouldDropLog(event, include, exclude, minSeverity) {
				droppedCount++
				continue
			}
			events = append(events, event)
		}
		return json.Marshal(map[string]any{"events": events, "dropped_count": droppedCount})
	}
	if c.kind == "traces" {
		items, dropped := c.receiver.sampleTraceItems(snap.Items)
		snap.Items = items
		snap.AcceptedCount = len(items)
		snap.DroppedCount += dropped
	}
	return json.Marshal(map[string]any{"otlp": snap})
}

func (r *Receiver) sampleTraceItems(items []map[string]any) ([]map[string]any, int) {
	rate, slowMs, preserveErrors := r.traceSamplingSettings()
	if rate >= 1 {
		return items, 0
	}
	out := make([]map[string]any, 0, len(items))
	dropped := 0
	for _, item := range items {
		if stringValue(item["sampling_reason"]) == "validation_sample" {
			out = append(out, item)
			continue
		}
		if preserveErrors && isTraceError(item) {
			item["sampling_reason"] = "error"
			out = append(out, item)
			continue
		}
		if slowMs > 0 && intValue(item["duration_ms"]) >= slowMs {
			item["sampling_reason"] = "slow"
			out = append(out, item)
			continue
		}
		if deterministicSample(firstString(item, "trace_id", "span_id", "name"), rate) {
			item["sampling_reason"] = "sampled"
			out = append(out, item)
			continue
		}
		dropped++
	}
	return out, dropped
}

func (r *Receiver) validationSampleEnabled() bool {
	if r.prefs == nil {
		return false
	}
	p := r.prefs()
	return strings.TrimSpace(p.Version) != "" && p.OTLPValidationSample
}

func (r *Receiver) addValidationSample(kind string, now time.Time) {
	if kind != "traces" || !r.validationSampleEnabled() {
		return
	}
	start := now.Add(-75 * time.Millisecond)
	traceSeed := strconv.FormatInt(now.UnixNano(), 16)
	r.store.add("traces", []map[string]any{{
		"trace_id":             leftPadHex(traceSeed, 32),
		"span_id":              leftPadHex(traceSeed, 16),
		"name":                 "Aiceberg controlled observability validation",
		"service":              "aiceberg-agent-validation",
		"env":                  "controlled-validation",
		"start_time_unix_nano": strconv.FormatInt(start.UnixNano(), 10),
		"end_time_unix_nano":   strconv.FormatInt(now.UnixNano(), 10),
		"duration_ms":          75,
		"status":               "ok",
		"attributes": map[string]any{
			"aiceberg.validation_sample": true,
			"source":                     "agent_controlled_sample",
			"purpose":                    "pipeline_validation",
		},
		"sampling_reason": "validation_sample",
	}})
}

func leftPadHex(value string, size int) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if len(value) >= size {
		return value[len(value)-size:]
	}
	return strings.Repeat("0", size-len(value)) + value
}

func (r *Receiver) traceSamplingSettings() (float64, int, bool) {
	rate := r.traceSampleRate
	slowMs := r.traceSlowThresholdMs
	preserveErrors := r.tracePreserveErrors
	if r.prefs != nil {
		p := r.prefs()
		if p.APMTraceSampleRate > 0 {
			rate = p.APMTraceSampleRate
		}
		if p.APMTraceSlowThresholdMs > 0 {
			slowMs = p.APMTraceSlowThresholdMs
		}
		if p.APMTracePreserveErrors {
			preserveErrors = true
		}
	}
	return normalizeSampleRate(rate), slowMs, preserveErrors
}

func normalizeSampleRate(rate float64) float64 {
	if rate <= 0 {
		return 0
	}
	if rate > 1 {
		return 1
	}
	return rate
}

func deterministicSample(key string, rate float64) bool {
	rate = normalizeSampleRate(rate)
	if rate >= 1 {
		return true
	}
	if rate <= 0 || strings.TrimSpace(key) == "" {
		return false
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	bucket := float64(h.Sum32()%10000) / 10000.0
	return bucket < rate
}

func isTraceError(item map[string]any) bool {
	if value, ok := item["error"].(bool); ok {
		return value
	}
	return strings.EqualFold(stringValue(item["status"]), "error")
}

func (r *Receiver) logFilters() (string, string, string) {
	include, exclude, minSeverity := r.include, r.exclude, r.minSeverity
	if r.prefs != nil {
		p := r.prefs()
		if p.OSLogIncludeRegex != "" {
			include = p.OSLogIncludeRegex
		}
		if p.OSLogExcludeRegex != "" {
			exclude = p.OSLogExcludeRegex
		}
		if p.OSLogMinSeverity != "" {
			minSeverity = p.OSLogMinSeverity
		}
	}
	return include, exclude, minSeverity
}

var otlpSensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*:\s*(?:bearer|basic)\s+)[^\s]+`),
	regexp.MustCompile(`(?i)("(?:password|passwd|pwd|token|secret|api[_-]?key|cookie)"\s*:\s*)"[^"]*"`),
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|cookie)\s*=\s*("[^"]+"|'[^']+'|[^\s&;]+)`),
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|cookie)\s*:\s*("[^"]+"|'[^']+'|[^\s,}]+)`),
}

func redactLogMessage(message string) (string, string) {
	redacted := message
	for _, pattern := range otlpSensitivePatterns {
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

func redactAttributes(raw any) map[string]any {
	attrs, ok := raw.(map[string]any)
	if !ok || len(attrs) == 0 {
		return nil
	}
	out := make(map[string]any, len(attrs))
	for key, value := range attrs {
		if isSensitiveLogKey(key) {
			out[key] = "[redacted]"
			continue
		}
		out[key] = value
	}
	return out
}

func shouldDropLog(event map[string]any, includeRegex, excludeRegex, minSeverity string) bool {
	target := strings.Join([]string{
		stringValue(event["message"]),
		stringValue(event["source"]),
		stringValue(event["service"]),
		stringValue(event["severity"]),
		stringValue(event["source_tool"]),
	}, " ")
	if minSeverity != "" && !severityAllowed(stringValue(event["severity"]), minSeverity) {
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
		return false
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

func isSensitiveLogKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "password") ||
		strings.Contains(key, "passwd") ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "authorization") ||
		strings.Contains(key, "cookie") ||
		strings.Contains(key, "api_key") ||
		strings.Contains(key, "apikey")
}

type snapshot struct {
	SchemaVersion  int              `json:"schema_version"`
	Kind           string           `json:"kind"`
	Source         string           `json:"source"`
	FlushWindowSec int              `json:"flush_window_sec"`
	Items          []map[string]any `json:"items"`
	AcceptedCount  int              `json:"accepted_count"`
	DroppedCount   int              `json:"dropped_count,omitempty"`
}

type store struct {
	mu       sync.Mutex
	maxItems int
	items    []storedItem
	dropped  int
}

type storedItem struct {
	kind string
	item map[string]any
}

func (s *store) add(kind string, items []map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		if len(s.items) >= s.maxItems {
			s.dropped++
			continue
		}
		s.items = append(s.items, storedItem{kind: kind, item: item})
	}
}

func (s *store) drop(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropped += n
}

func (s *store) flush(kind string) snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	remaining := s.items[:0]
	out := make([]map[string]any, 0)
	for _, item := range s.items {
		if item.kind == kind {
			out = append(out, item.item)
			continue
		}
		remaining = append(remaining, item)
	}
	s.items = remaining
	dropped := s.dropped
	if len(s.items) == 0 {
		s.dropped = 0
	}
	sort.Slice(out, func(i, j int) bool {
		return stringValue(out[i]["name"]) < stringValue(out[j]["name"])
	})
	return snapshot{Items: out, AcceptedCount: len(out), DroppedCount: dropped}
}

func flattenOTLP(kind string, decoded any, resource map[string]any) []map[string]any {
	root, _ := decoded.(map[string]any)
	switch kind {
	case "metrics":
		return flattenMetrics(root, resource)
	case "logs":
		return flattenLogs(root, resource)
	case "traces":
		return flattenTraces(root, resource)
	default:
		return nil
	}
}

func flattenMetrics(root map[string]any, resource map[string]any) []map[string]any {
	var out []map[string]any
	for _, rm := range array(root["resourceMetrics"]) {
		res := merge(resource, resourceAttributes(rm))
		for _, sm := range array(mapValue(rm)["scopeMetrics"]) {
			for _, metric := range array(mapValue(sm)["metrics"]) {
				m := mapValue(metric)
				item := baseItem(res)
				item["name"] = stringValue(m["name"])
				item["description"] = stringValue(m["description"])
				item["unit"] = stringValue(m["unit"])
				item["type"] = firstMetricType(m)
				out = append(out, item)
			}
		}
	}
	return out
}

func flattenLogs(root map[string]any, resource map[string]any) []map[string]any {
	var out []map[string]any
	for _, rl := range array(root["resourceLogs"]) {
		res := merge(resource, resourceAttributes(rl))
		for _, sl := range array(mapValue(rl)["scopeLogs"]) {
			for _, record := range array(mapValue(sl)["logRecords"]) {
				r := mapValue(record)
				item := baseItem(res)
				item["timestamp_utc"] = unixNanoString(r["timeUnixNano"])
				item["severity"] = firstString(r, "severityText", "severityNumber")
				item["message"] = valueToString(r["body"])
				item["trace_id"] = stringValue(r["traceId"])
				item["span_id"] = stringValue(r["spanId"])
				item["attributes"] = attributes(r["attributes"])
				out = append(out, item)
			}
		}
	}
	return out
}

func flattenTraces(root map[string]any, resource map[string]any) []map[string]any {
	var out []map[string]any
	for _, rs := range array(root["resourceSpans"]) {
		res := merge(resource, resourceAttributes(rs))
		for _, ss := range array(mapValue(rs)["scopeSpans"]) {
			for _, span := range array(mapValue(ss)["spans"]) {
				s := mapValue(span)
				item := baseItem(res)
				item["name"] = stringValue(s["name"])
				item["trace_id"] = stringValue(s["traceId"])
				item["span_id"] = stringValue(s["spanId"])
				item["parent_span_id"] = stringValue(s["parentSpanId"])
				item["start_time_unix_nano"] = stringValue(s["startTimeUnixNano"])
				item["end_time_unix_nano"] = stringValue(s["endTimeUnixNano"])
				item["duration_ms"] = durationMs(item["start_time_unix_nano"], item["end_time_unix_nano"])
				attrs := attributes(s["attributes"])
				item["attributes"] = attrs
				status := mapValue(s["status"])
				item["status"] = statusText(status)
				item["status_message"] = stringValue(status["message"])
				item["error"] = traceError(status, attrs)
				out = append(out, item)
			}
		}
	}
	return out
}

func durationMs(start, end any) int64 {
	startNano, errStart := strconv.ParseInt(stringValue(start), 10, 64)
	endNano, errEnd := strconv.ParseInt(stringValue(end), 10, 64)
	if errStart != nil || errEnd != nil || endNano <= startNano {
		return 0
	}
	return (endNano - startNano) / int64(time.Millisecond)
}

func statusText(status map[string]any) string {
	code := strings.ToLower(stringValue(status["code"]))
	switch code {
	case "2", "error":
		return "error"
	case "1", "ok":
		return "ok"
	default:
		return firstString(status, "code", "message")
	}
}

func traceError(status map[string]any, attrs map[string]any) bool {
	if statusText(status) == "error" {
		return true
	}
	for key, value := range attrs {
		lower := strings.ToLower(strings.TrimSpace(key))
		if strings.HasPrefix(lower, "exception.") {
			return true
		}
		if lower == "error" && strings.EqualFold(stringValue(value), "true") {
			return true
		}
	}
	return false
}

func resourceAttributes(v any) map[string]any {
	m := mapValue(v)
	if resource := mapValue(m["resource"]); resource != nil {
		return attributes(resource["attributes"])
	}
	return nil
}

func attributes(v any) map[string]any {
	raw := make([][2]string, 0)
	for _, attr := range array(v) {
		m := mapValue(attr)
		key := stringValue(m["key"])
		if key == "" {
			continue
		}
		raw = append(raw, [2]string{key, sanitizeAttributeValue(key, valueToString(m["value"]))})
	}
	out := limitedAttributes(raw)
	if len(out) == 0 {
		return nil
	}
	return out
}

func limitedAttributes(raw [][2]string) map[string]any {
	out := map[string]any{}
	for _, item := range raw {
		if isEssentialOTLPAttribute(item[0]) {
			out[item[0]] = item[1]
		}
	}
	for _, item := range raw {
		if len(out) >= maxAttributeCount {
			break
		}
		if _, exists := out[item[0]]; exists {
			continue
		}
		out[item[0]] = item[1]
	}
	return out
}

func sanitizeAttributeValue(key string, value string) string {
	if isSensitiveLogKey(key) {
		return "[redacted]"
	}
	if len(value) > maxAttributeLength {
		return value[:maxAttributeLength]
	}
	return value
}

func isEssentialOTLPAttribute(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "host.name", "service.name", "deployment.environment", "service.version",
		"telemetry.sdk.name", "telemetry.sdk.language", "telemetry.sdk.version",
		"http.method", "http.route", "http.status_code",
		"db.system", "db.name", "net.peer.name", "url.path",
		"k8s.namespace.name", "k8s.pod.name", "container.name",
		"exception.type", "exception.message":
		return true
	default:
		return false
	}
}

func baseItem(resource map[string]any) map[string]any {
	item := map[string]any{"resource": resource}
	for _, key := range []string{"host.name", "service.name", "deployment.environment", "service.version"} {
		if value := stringValue(resource[key]); value != "" {
			switch key {
			case "host.name":
				item["host"] = value
			case "service.name":
				item["service"] = value
			case "deployment.environment":
				item["env"] = value
			case "service.version":
				item["version"] = value
			}
		}
	}
	return item
}

func firstMetricType(metric map[string]any) string {
	for _, key := range []string{"gauge", "sum", "histogram", "exponentialHistogram", "summary"} {
		if _, ok := metric[key]; ok {
			return key
		}
	}
	return "unknown"
}

func mapValue(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func array(v any) []any {
	if a, ok := v.([]any); ok {
		return a
	}
	return nil
}

func merge(base, extra map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func valueToString(v any) string {
	m := mapValue(v)
	for _, key := range []string{"stringValue", "intValue", "doubleValue", "boolValue", "bytesValue"} {
		if value := stringValue(m[key]); value != "" {
			return value
		}
	}
	return stringValue(v)
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(m[key]); value != "" {
			return value
		}
	}
	return ""
}

func stringValue(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strings.TrimRight(strings.TrimRight(jsonNumber(t), "0"), ".")
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func intValue(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func jsonNumber(v float64) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func unixNanoString(v any) string {
	raw := stringValue(v)
	if raw == "" {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return raw
}

func isLoopback(ip net.IP) bool {
	return ip != nil && ip.IsLoopback()
}
