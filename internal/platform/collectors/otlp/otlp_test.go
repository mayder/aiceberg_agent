package otlp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestReceiverAcceptsOTLPHTTPJSONSignals(t *testing.T) {
	addr := freeTCPAddr(t)
	receiver := NewReceiver(config.Config{
		OTLPEnabled:  true,
		OTLPHTTPAddr: addr,
		OTLPInterval: time.Second,
		OTLPMaxItems: 10,
		OTLPMaxBytes: 4096,
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := receiver.MetricsCollector().Collect(ctx); err != nil {
		t.Fatalf("start receiver: %v", err)
	}

	postOTLP(t, addr, "/v1/metrics", `{"resourceMetrics":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}},{"key":"deployment.environment","value":{"stringValue":"test"}}]},"scopeMetrics":[{"metrics":[{"name":"http.server.duration","unit":"ms","gauge":{"dataPoints":[{"asDouble":12}]}}]}]}]}`)
	postOTLP(t, addr, "/v1/logs", `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},"scopeLogs":[{"logRecords":[{"timeUnixNano":"1","severityText":"INFO","body":{"stringValue":"hello"},"traceId":"abc","spanId":"def"}]}]}]}`)
	postOTLP(t, addr, "/v1/traces", `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},"scopeSpans":[{"spans":[{"traceId":"abc","spanId":"def","name":"GET /health","startTimeUnixNano":"1","endTimeUnixNano":"2"}]}]}]}`)

	metricsRaw, err := receiver.MetricsCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	var metricsPayload struct {
		OTLP snapshot `json:"otlp"`
	}
	if err := json.Unmarshal(metricsRaw, &metricsPayload); err != nil {
		t.Fatalf("invalid metrics payload: %v", err)
	}
	if metricsPayload.OTLP.Kind != "metrics" || len(metricsPayload.OTLP.Items) != 1 {
		t.Fatalf("unexpected metrics payload %#v", metricsPayload.OTLP)
	}

	logsRaw, err := receiver.LogsCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect logs: %v", err)
	}
	var logsPayload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(logsRaw, &logsPayload); err != nil {
		t.Fatalf("invalid logs payload: %v", err)
	}
	if len(logsPayload.Events) != 1 || logsPayload.Events[0]["message"] != "hello" {
		t.Fatalf("unexpected logs payload %#v", logsPayload.Events)
	}

	tracesRaw, err := receiver.TracesCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect traces: %v", err)
	}
	var tracesPayload struct {
		OTLP snapshot `json:"otlp"`
	}
	if err := json.Unmarshal(tracesRaw, &tracesPayload); err != nil {
		t.Fatalf("invalid traces payload: %v", err)
	}
	if tracesPayload.OTLP.Kind != "traces" || len(tracesPayload.OTLP.Items) != 1 {
		t.Fatalf("unexpected traces payload %#v", tracesPayload.OTLP)
	}
}

func TestReceiverEmitsControlledValidationTrace(t *testing.T) {
	addr := freeTCPAddr(t)
	receiver := NewReceiver(config.Config{
		OTLPEnabled:        true,
		OTLPHTTPAddr:       addr,
		OTLPInterval:       time.Second,
		OTLPMaxItems:       10,
		OTLPMaxBytes:       4096,
		APMTraceSampleRate: 0,
	}, func() config.CollectPrefs {
		return config.CollectPrefs{
			Version:              "remote",
			OTLPEnabled:          true,
			OTLPValidationSample: true,
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracesRaw, err := receiver.TracesCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect traces: %v", err)
	}
	var payload struct {
		OTLP snapshot `json:"otlp"`
	}
	if err := json.Unmarshal(tracesRaw, &payload); err != nil {
		t.Fatalf("invalid traces payload: %v", err)
	}
	if payload.OTLP.Kind != "traces" || len(payload.OTLP.Items) != 1 {
		t.Fatalf("unexpected traces payload %#v", payload.OTLP)
	}
	trace := payload.OTLP.Items[0]
	if trace["service"] != "aiceberg-agent-validation" || trace["sampling_reason"] != "validation_sample" {
		t.Fatalf("unexpected validation trace %#v", trace)
	}
	attrs, ok := trace["attributes"].(map[string]any)
	if !ok || attrs["aiceberg.validation_sample"] != true || attrs["source"] != "agent_controlled_sample" {
		t.Fatalf("unexpected validation trace attributes %#v", trace["attributes"])
	}
}

func TestReceiverRedactsAndFiltersOTLPLogs(t *testing.T) {
	addr := freeTCPAddr(t)
	receiver := NewReceiver(config.Config{
		OTLPEnabled:       true,
		OTLPHTTPAddr:      addr,
		OTLPInterval:      time.Second,
		OTLPMaxItems:      10,
		OTLPMaxBytes:      4096,
		OSLogExcludeRegex: "health check",
		OSLogMinSeverity:  "info",
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := receiver.LogsCollector().Collect(ctx); err != nil {
		t.Fatalf("start receiver: %v", err)
	}

	postOTLP(t, addr, "/v1/logs", `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},"scopeLogs":[{"logRecords":[{"timeUnixNano":"1","severityText":"INFO","body":{"stringValue":"Authorization: Bearer secret-token password=hunter2"},"attributes":[{"key":"token","value":{"stringValue":"abc123"}},{"key":"route","value":{"stringValue":"/login"}}]},{"timeUnixNano":"2","severityText":"INFO","body":{"stringValue":"health check password=secret"}}]}]}]}`)

	logsRaw, err := receiver.LogsCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect logs: %v", err)
	}
	var payload struct {
		Events       []map[string]any `json:"events"`
		DroppedCount int              `json:"dropped_count"`
	}
	if err := json.Unmarshal(logsRaw, &payload); err != nil {
		t.Fatalf("invalid logs payload: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected 1 event after filter, got %#v", payload.Events)
	}
	if payload.DroppedCount != 1 {
		t.Fatalf("expected dropped_count=1, got %d", payload.DroppedCount)
	}
	msg, _ := payload.Events[0]["message"].(string)
	if msg == "" || strings.Contains(msg, "secret-token") || strings.Contains(msg, "hunter2") {
		t.Fatalf("message leaked secret: %q", msg)
	}
	if payload.Events[0]["redaction_status"] != "redacted" {
		t.Fatalf("expected redacted status, got %#v", payload.Events[0]["redaction_status"])
	}
	attrs, ok := payload.Events[0]["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("expected attributes, got %#v", payload.Events[0])
	}
	if attrs["token"] != "[redacted]" || attrs["route"] != "/login" {
		t.Fatalf("unexpected attributes: %#v", attrs)
	}
	if payload.Events[0]["aiceberg_tool_origin"] != "otlp_log" || payload.Events[0]["aiceberg_source_category"] != "conditional" || payload.Events[0]["aiceberg_soc_source_type"] != "application" {
		t.Fatalf("expected OTLP log SOC contract, got %#v", payload.Events[0])
	}
}

func TestReceiverDropsOTLPLogWithoutSeverityWhenMinimumConfigured(t *testing.T) {
	addr := freeTCPAddr(t)
	receiver := NewReceiver(config.Config{
		OTLPEnabled:      true,
		OTLPHTTPAddr:     addr,
		OTLPInterval:     time.Second,
		OTLPMaxItems:     10,
		OTLPMaxBytes:     4096,
		OSLogMinSeverity: "error",
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := receiver.LogsCollector().Collect(ctx); err != nil {
		t.Fatalf("start receiver: %v", err)
	}

	postOTLP(t, addr, "/v1/logs", `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},"scopeLogs":[{"logRecords":[{"timeUnixNano":"1","body":{"stringValue":"no level"}},{"timeUnixNano":"2","severityText":"ERROR","body":{"stringValue":"real error"}}]}]}]}`)

	logsRaw, err := receiver.LogsCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect logs: %v", err)
	}
	var payload struct {
		Events       []map[string]any `json:"events"`
		DroppedCount int              `json:"dropped_count"`
	}
	if err := json.Unmarshal(logsRaw, &payload); err != nil {
		t.Fatalf("invalid logs payload: %v", err)
	}
	if len(payload.Events) != 1 || payload.Events[0]["message"] != "real error" || payload.DroppedCount != 1 {
		t.Fatalf("expected only severity error event, got %#v dropped=%d", payload.Events, payload.DroppedCount)
	}
}

func TestReceiverLimitsAttributesAndPreservesEssentials(t *testing.T) {
	addr := freeTCPAddr(t)
	receiver := NewReceiver(config.Config{
		OTLPEnabled:  true,
		OTLPHTTPAddr: addr,
		OTLPInterval: time.Second,
		OTLPMaxItems: 10,
		OTLPMaxBytes: 8192,
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := receiver.MetricsCollector().Collect(ctx); err != nil {
		t.Fatalf("start receiver: %v", err)
	}

	postOTLP(t, addr, "/v1/metrics", otlpMetricWithManyResourceAttributes())

	metricsRaw, err := receiver.MetricsCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	var payload struct {
		OTLP snapshot `json:"otlp"`
	}
	if err := json.Unmarshal(metricsRaw, &payload); err != nil {
		t.Fatalf("invalid metrics payload: %v", err)
	}
	if len(payload.OTLP.Items) != 1 {
		t.Fatalf("expected one metric item, got %#v", payload.OTLP.Items)
	}
	resource, ok := payload.OTLP.Items[0]["resource"].(map[string]any)
	if !ok {
		t.Fatalf("expected resource attributes, got %#v", payload.OTLP.Items[0]["resource"])
	}
	if len(resource) > maxAttributeCount {
		t.Fatalf("expected resource capped at %d attrs, got %d", maxAttributeCount, len(resource))
	}
	for _, key := range []string{"service.name", "deployment.environment", "host.name"} {
		if resource[key] == "" {
			t.Fatalf("expected essential attribute %q preserved in %#v", key, resource)
		}
	}
	if resource["token"] != "[redacted]" {
		t.Fatalf("expected sensitive attribute redacted, got %#v", resource["token"])
	}
	if payload.OTLP.Items[0]["service"] != "api" || payload.OTLP.Items[0]["env"] != "prod" {
		t.Fatalf("expected mapped service/env, got %#v", payload.OTLP.Items[0])
	}
}

func TestReceiverSamplesTracesButKeepsErrorsAndSlowSpans(t *testing.T) {
	addr := freeTCPAddr(t)
	receiver := NewReceiver(config.Config{
		OTLPEnabled:             true,
		OTLPHTTPAddr:            addr,
		OTLPInterval:            time.Second,
		OTLPMaxItems:            10,
		OTLPMaxBytes:            4096,
		APMTraceSampleRate:      0,
		APMTraceSlowThresholdMs: 100,
		APMTracePreserveErrors:  true,
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := receiver.TracesCollector().Collect(ctx); err != nil {
		t.Fatalf("start receiver: %v", err)
	}

	postOTLP(t, addr, "/v1/traces", `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},"scopeSpans":[{"spans":[{"traceId":"trace-fast","spanId":"span-fast","name":"fast","startTimeUnixNano":"0","endTimeUnixNano":"1000000"},{"traceId":"trace-slow","spanId":"span-slow","name":"slow","startTimeUnixNano":"0","endTimeUnixNano":"250000000"},{"traceId":"trace-error","spanId":"span-error","name":"error","startTimeUnixNano":"0","endTimeUnixNano":"1000000","status":{"code":2,"message":"boom"}}]}]}]}`)

	tracesRaw, err := receiver.TracesCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect traces: %v", err)
	}
	var payload struct {
		OTLP snapshot `json:"otlp"`
	}
	if err := json.Unmarshal(tracesRaw, &payload); err != nil {
		t.Fatalf("invalid traces payload: %v", err)
	}
	if len(payload.OTLP.Items) != 2 {
		t.Fatalf("expected slow and error spans only, got %#v", payload.OTLP.Items)
	}
	if payload.OTLP.DroppedCount != 1 {
		t.Fatalf("expected dropped_count=1, got %d", payload.OTLP.DroppedCount)
	}
	reasons := map[string]bool{}
	for _, item := range payload.OTLP.Items {
		reasons[stringValue(item["sampling_reason"])] = true
		if item["name"] == "slow" && intValue(item["duration_ms"]) != 250 {
			t.Fatalf("expected slow duration 250ms, got %#v", item)
		}
	}
	if !reasons["slow"] || !reasons["error"] {
		t.Fatalf("expected slow and error sampling reasons, got %#v", reasons)
	}
}

func TestPKG63APMHighVolumeErrorJourneyEvidence(t *testing.T) {
	addr := freeTCPAddr(t)
	receiver := NewReceiver(config.Config{
		OTLPEnabled:             true,
		OTLPHTTPAddr:            addr,
		OTLPInterval:            time.Second,
		OTLPMaxItems:            120,
		OTLPMaxBytes:            256 * 1024,
		APMTraceSampleRate:      0,
		APMTraceSlowThresholdMs: 100,
		APMTracePreserveErrors:  true,
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := receiver.TracesCollector().Collect(ctx); err != nil {
		t.Fatalf("start receiver: %v", err)
	}

	const traceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const spanID = "bbbbbbbbbbbbbbbb"
	const hostName = "pkg63-host"
	const serviceName = "checkout-api"
	resourceAttrs := `{"key":"service.name","value":{"stringValue":"` + serviceName + `"}},{"key":"deployment.environment","value":{"stringValue":"controlled"}},{"key":"service.version","value":{"stringValue":"1.2.3"}},{"key":"host.name","value":{"stringValue":"` + hostName + `"}}`
	postOTLP(t, addr, "/v1/logs", `{"resourceLogs":[{"resource":{"attributes":[`+resourceAttrs+`]},"scopeLogs":[{"logRecords":[{"timeUnixNano":"1000000000","severityText":"ERROR","body":{"stringValue":"checkout failed trace_id=`+traceID+` span_id=`+spanID+`"},"traceId":"`+traceID+`","spanId":"`+spanID+`","attributes":[{"key":"http.route","value":{"stringValue":"/checkout"}}]}]}]}]}`)
	postOTLP(t, addr, "/v1/traces", pkg63TraceBurstPayload(resourceAttrs, traceID, spanID))

	logsRaw, err := receiver.LogsCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect logs: %v", err)
	}
	tracesRaw, err := receiver.TracesCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect traces: %v", err)
	}

	var logsPayload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(logsRaw, &logsPayload); err != nil {
		t.Fatalf("invalid logs payload: %v", err)
	}
	var tracesPayload struct {
		OTLP snapshot `json:"otlp"`
	}
	if err := json.Unmarshal(tracesRaw, &tracesPayload); err != nil {
		t.Fatalf("invalid traces payload: %v", err)
	}
	if len(logsPayload.Events) != 1 {
		t.Fatalf("expected one correlated error log, got %#v", logsPayload.Events)
	}
	if logsPayload.Events[0]["trace_id"] != traceID || logsPayload.Events[0]["span_id"] != spanID || logsPayload.Events[0]["service"] != serviceName {
		t.Fatalf("expected correlated log with service, got %#v", logsPayload.Events[0])
	}
	if tracesPayload.OTLP.AcceptedCount != 2 || tracesPayload.OTLP.DroppedCount != 78 {
		t.Fatalf("expected high volume sampling 2 accepted and 78 dropped, got accepted=%d dropped=%d", tracesPayload.OTLP.AcceptedCount, tracesPayload.OTLP.DroppedCount)
	}
	reasons := map[string]bool{}
	for _, trace := range tracesPayload.OTLP.Items {
		reasons[stringValue(trace["sampling_reason"])] = true
		if trace["trace_id"] == traceID && trace["span_id"] == spanID {
			if trace["span_id"] != spanID || trace["service"] != serviceName || trace["host"] != hostName {
				t.Fatalf("expected log -> trace -> service -> host journey, got %#v", trace)
			}
			if trace["error"] != true || !strings.EqualFold(stringValue(trace["status"]), "error") {
				t.Fatalf("expected application error span, got %#v", trace)
			}
		}
	}
	if !reasons["error"] || !reasons["slow"] {
		t.Fatalf("expected error and slow spans preserved, got reasons %#v", reasons)
	}

	if evidenceDir := strings.TrimSpace(os.Getenv("PKG63_EVIDENCE_DIR")); evidenceDir != "" {
		writePKG63Evidence(t, evidenceDir, logsRaw, tracesRaw, map[string]string{
			"input_spans":        "80",
			"accepted_count":     strconv.Itoa(tracesPayload.OTLP.AcceptedCount),
			"dropped_count":      strconv.Itoa(tracesPayload.OTLP.DroppedCount),
			"trace_items":        strconv.Itoa(len(tracesPayload.OTLP.Items)),
			"logs_events":        strconv.Itoa(len(logsPayload.Events)),
			"application_error":  "yes",
			"sampling_error":     boolString(reasons["error"]),
			"sampling_slow":      boolString(reasons["slow"]),
			"journey_log_trace":  "yes",
			"service":            serviceName,
			"host":               hostName,
			"api_credential":     "not_used",
			"transport":          "otlp_http_json",
			"profiler_scope":     "out_of_scope_by_decision",
			"overhead_reference": "docs/evidence/pkg69/high-volume-overhead-20260619T033436Z/evidence.md",
		})
	}
}

func TestReceiverRejectsOversizedPayload(t *testing.T) {
	addr := freeTCPAddr(t)
	receiver := NewReceiver(config.Config{
		OTLPEnabled:  true,
		OTLPHTTPAddr: addr,
		OTLPInterval: time.Second,
		OTLPMaxItems: 10,
		OTLPMaxBytes: 64,
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := receiver.MetricsCollector().Collect(ctx); err != nil {
		t.Fatalf("start receiver: %v", err)
	}

	resp, err := http.Post("http://"+addr+"/v1/metrics", "application/json", bytes.NewReader(bytes.Repeat([]byte("x"), 65)))
	if err != nil {
		t.Fatalf("post oversized otlp: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}

	raw, err := receiver.MetricsCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	if len(raw) != 0 {
		t.Fatalf("expected no payload after rejected oversized body, got %s", string(raw))
	}
}

func TestPKG62ExampleServiceOTLPEvidence(t *testing.T) {
	addr := freeTCPAddr(t)
	receiver := NewReceiver(config.Config{
		OTLPEnabled:  true,
		OTLPHTTPAddr: addr,
		OTLPInterval: time.Second,
		OTLPMaxItems: 20,
		OTLPMaxBytes: 32 * 1024,
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := receiver.MetricsCollector().Collect(ctx); err != nil {
		t.Fatalf("start receiver: %v", err)
	}

	const traceID = "0102030405060708090a0b0c0d0e0f10"
	const spanID = "1112131415161718"
	resourceAttrs := `{"key":"service.name","value":{"stringValue":"checkout-service"}},{"key":"deployment.environment","value":{"stringValue":"controlled"}},{"key":"service.version","value":{"stringValue":"1.0.0"}},{"key":"host.name","value":{"stringValue":"pkg62-local"}}`
	postOTLP(t, addr, "/v1/metrics", `{"resourceMetrics":[{"resource":{"attributes":[`+resourceAttrs+`]},"scopeMetrics":[{"metrics":[{"name":"checkout.requests","unit":"1","sum":{"dataPoints":[{"asDouble":3}]}},{"name":"checkout.duration","unit":"ms","histogram":{"dataPoints":[{"count":1,"sum":42}]}}]}]}]}`)
	postOTLP(t, addr, "/v1/logs", `{"resourceLogs":[{"resource":{"attributes":[`+resourceAttrs+`]},"scopeLogs":[{"logRecords":[{"timeUnixNano":"1000000000","severityText":"ERROR","body":{"stringValue":"checkout payment failed"},"traceId":"`+traceID+`","spanId":"`+spanID+`","attributes":[{"key":"http.route","value":{"stringValue":"/checkout"}},{"key":"payment.token","value":{"stringValue":"must-redact"}}]}]}]}]}`)
	postOTLP(t, addr, "/v1/traces", `{"resourceSpans":[{"resource":{"attributes":[`+resourceAttrs+`]},"scopeSpans":[{"spans":[{"traceId":"`+traceID+`","spanId":"`+spanID+`","name":"POST /checkout","startTimeUnixNano":"1000000000","endTimeUnixNano":"1250000000","attributes":[{"key":"http.method","value":{"stringValue":"POST"}},{"key":"http.route","value":{"stringValue":"/checkout"}},{"key":"http.status_code","value":{"intValue":"500"}}],"status":{"code":2,"message":"payment failed"}}]}]}]}`)

	metricsRaw, err := receiver.MetricsCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	logsRaw, err := receiver.LogsCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect logs: %v", err)
	}
	tracesRaw, err := receiver.TracesCollector().Collect(ctx)
	if err != nil {
		t.Fatalf("collect traces: %v", err)
	}

	var metricsPayload struct {
		OTLP snapshot `json:"otlp"`
	}
	if err := json.Unmarshal(metricsRaw, &metricsPayload); err != nil {
		t.Fatalf("invalid metrics payload: %v", err)
	}
	var logsPayload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(logsRaw, &logsPayload); err != nil {
		t.Fatalf("invalid logs payload: %v", err)
	}
	var tracesPayload struct {
		OTLP snapshot `json:"otlp"`
	}
	if err := json.Unmarshal(tracesRaw, &tracesPayload); err != nil {
		t.Fatalf("invalid traces payload: %v", err)
	}
	if metricsPayload.OTLP.Kind != "metrics" || len(metricsPayload.OTLP.Items) != 2 {
		t.Fatalf("unexpected metrics payload %#v", metricsPayload.OTLP)
	}
	if len(logsPayload.Events) != 1 || logsPayload.Events[0]["trace_id"] != traceID || logsPayload.Events[0]["span_id"] != spanID {
		t.Fatalf("unexpected logs payload %#v", logsPayload.Events)
	}
	if logsPayload.Events[0]["service"] != "checkout-service" || logsPayload.Events[0]["severity"] != "ERROR" {
		t.Fatalf("expected service/severity in log event, got %#v", logsPayload.Events[0])
	}
	if attrs, ok := logsPayload.Events[0]["attributes"].(map[string]any); !ok || attrs["payment.token"] != "[redacted]" {
		t.Fatalf("expected sensitive OTLP log attribute redacted, got %#v", logsPayload.Events[0]["attributes"])
	}
	if tracesPayload.OTLP.Kind != "traces" || len(tracesPayload.OTLP.Items) != 1 {
		t.Fatalf("unexpected traces payload %#v", tracesPayload.OTLP)
	}
	trace := tracesPayload.OTLP.Items[0]
	if trace["trace_id"] != traceID || trace["span_id"] != spanID || trace["service"] != "checkout-service" {
		t.Fatalf("expected correlated trace, got %#v", trace)
	}
	if intValue(trace["duration_ms"]) != 250 || trace["error"] != true {
		t.Fatalf("expected 250ms error span, got %#v", trace)
	}

	if evidenceDir := strings.TrimSpace(os.Getenv("PKG62_EVIDENCE_DIR")); evidenceDir != "" {
		writePKG62Evidence(t, evidenceDir, metricsRaw, logsRaw, tracesRaw, map[string]string{
			"metrics_items":       strconv.Itoa(len(metricsPayload.OTLP.Items)),
			"logs_events":         strconv.Itoa(len(logsPayload.Events)),
			"traces_items":        strconv.Itoa(len(tracesPayload.OTLP.Items)),
			"service":             "checkout-service",
			"env":                 "controlled",
			"trace_correlation":   "yes",
			"redaction":           "yes",
			"api_credential":      "not_used",
			"transport":           "otlp_http_json",
			"simple_service_span": "yes",
		})
	}
}

func postOTLP(t *testing.T, addr, path, body string) {
	t.Helper()
	resp, err := http.Post("http://"+addr+path, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("post %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post %s status %d", path, resp.StatusCode)
	}
}

func writePKG62Evidence(t *testing.T, dir string, metricsRaw, logsRaw, tracesRaw []byte, summary map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir evidence dir: %v", err)
	}
	files := map[string][]byte{
		"metrics_payload.json": metricsRaw,
		"logs_payload.json":    logsRaw,
		"traces_payload.json":  tracesRaw,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	keys := []string{"metrics_items", "logs_events", "traces_items", "service", "env", "trace_correlation", "redaction", "api_credential", "transport", "simple_service_span"}
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s\t%s\n", key, summary[key])
	}
	if err := os.WriteFile(filepath.Join(dir, "SUMMARY.tsv"), []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}
}

func writePKG63Evidence(t *testing.T, dir string, logsRaw, tracesRaw []byte, summary map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir evidence dir: %v", err)
	}
	files := map[string][]byte{
		"logs_payload.json":   logsRaw,
		"traces_payload.json": tracesRaw,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	keys := []string{"input_spans", "accepted_count", "dropped_count", "trace_items", "logs_events", "application_error", "sampling_error", "sampling_slow", "journey_log_trace", "service", "host", "api_credential", "transport", "profiler_scope", "overhead_reference"}
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s\t%s\n", key, summary[key])
	}
	if err := os.WriteFile(filepath.Join(dir, "SUMMARY.tsv"), []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}
}

func pkg63TraceBurstPayload(resourceAttrs, traceID, spanID string) string {
	spans := make([]string, 0, 80)
	for i := 0; i < 80; i++ {
		id := fmt.Sprintf("%016x", i+1)
		duration := int64(1_000_000)
		status := ""
		if i == 7 {
			id = spanID
			duration = 150_000_000
			status = `,"status":{"code":2,"message":"checkout failed"}`
		}
		if i == 42 {
			duration = 250_000_000
		}
		spans = append(spans, `{"traceId":"`+traceID+`","spanId":"`+id+`","name":"POST /checkout","startTimeUnixNano":"1000000000","endTimeUnixNano":"`+strconv.FormatInt(1000000000+duration, 10)+`","attributes":[{"key":"http.route","value":{"stringValue":"/checkout"}}]`+status+`}`)
	}
	return `{"resourceSpans":[{"resource":{"attributes":[` + resourceAttrs + `]},"scopeSpans":[{"spans":[` + strings.Join(spans, ",") + `]}]}]}`
}

func boolString(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func otlpMetricWithManyResourceAttributes() string {
	attrs := []string{
		`{"key":"token","value":{"stringValue":"abc123"}}`,
	}
	for i := 0; i < 40; i++ {
		attrs = append(attrs, `{"key":"custom.attr.`+strconv.Itoa(i)+`","value":{"stringValue":"value`+strconv.Itoa(i)+`"}}`)
	}
	attrs = append(attrs,
		`{"key":"service.name","value":{"stringValue":"api"}}`,
		`{"key":"deployment.environment","value":{"stringValue":"prod"}}`,
		`{"key":"host.name","value":{"stringValue":"host-a"}}`,
	)
	return `{"resourceMetrics":[{"resource":{"attributes":[` + strings.Join(attrs, ",") + `]},"scopeMetrics":[{"metrics":[{"name":"jobs.processed","gauge":{"dataPoints":[{"asDouble":1}]}}]}]}]}`
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}
