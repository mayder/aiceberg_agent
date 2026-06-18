package otlp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const schemaVersion = 1

type Receiver struct {
	prefs       func() config.CollectPrefs
	baseEnabled bool
	addr        string
	interval    time.Duration
	maxItems    int
	maxBytes    int
	startOnce   sync.Once
	startErr    error
	server      *http.Server
	store       *store
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
	return &Receiver{
		prefs:       prefsProvider,
		baseEnabled: cfg.OTLPEnabled,
		addr:        cfg.OTLPHTTPAddr,
		interval:    interval,
		maxItems:    maxItems,
		maxBytes:    maxBytes,
		store:       &store{maxItems: maxItems},
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
		for _, item := range snap.Items {
			events = append(events, map[string]any{
				"schema_version":   schemaVersion,
				"timestamp":        stringValue(item["timestamp_utc"]),
				"timestamp_utc":    stringValue(item["timestamp_utc"]),
				"source":           firstString(item, "host", "service", "source"),
				"message":          stringValue(item["message"]),
				"severity":         stringValue(item["severity"]),
				"service":          stringValue(item["service"]),
				"attributes":       item["attributes"],
				"redaction_status": "pending",
				"transport":        "otlp_http_json",
				"source_tool":      "opentelemetry",
				"trace_id":         stringValue(item["trace_id"]),
				"span_id":          stringValue(item["span_id"]),
			})
		}
		return json.Marshal(map[string]any{"events": events, "dropped_count": snap.DroppedCount})
	}
	return json.Marshal(map[string]any{"otlp": snap})
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
				item["attributes"] = attributes(s["attributes"])
				out = append(out, item)
			}
		}
	}
	return out
}

func resourceAttributes(v any) map[string]any {
	m := mapValue(v)
	if resource := mapValue(m["resource"]); resource != nil {
		return attributes(resource["attributes"])
	}
	return nil
}

func attributes(v any) map[string]any {
	out := map[string]any{}
	for _, attr := range array(v) {
		m := mapValue(attr)
		key := stringValue(m["key"])
		if key == "" {
			continue
		}
		out[key] = valueToString(m["value"])
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
