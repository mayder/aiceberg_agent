package otlp

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
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

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}
