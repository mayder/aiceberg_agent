package custommetrics

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestParseDogStatsDMetric(t *testing.T) {
	metric, ok := parseDogStatsD("app.requests:3|c|#service:web,env:test", time.Unix(0, 0).UTC())
	if !ok {
		t.Fatalf("expected metric")
	}
	if metric.Name != "app.requests" || metric.Type != "count" || metric.Value != 3 {
		t.Fatalf("unexpected metric %#v", metric)
	}
	if len(metric.Tags) != 2 || metric.Tags[0] != "env:test" || metric.Tags[1] != "service:web" {
		t.Fatalf("unexpected tags %#v", metric.Tags)
	}
}

func TestCollectorAggregatesAndDropsExcessCardinality(t *testing.T) {
	cfg := config.Config{
		CustomMetricsEnabled:   true,
		CustomMetricsInterval:  time.Second,
		CustomMetricsMaxSeries: 1,
		CustomMetricsMaxBytes:  4096,
	}
	c := New(cfg, nil).(*collector)
	c.ingestLines("app.requests:2|c|#service:web\napp.requests:3|c|#service:web\napp.other:1|g")

	raw, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	var payload struct {
		CustomMetrics snapshot `json:"custom_metrics"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if payload.CustomMetrics.SchemaVersion != schemaVersion {
		t.Fatalf("expected schema version, got %#v", payload.CustomMetrics)
	}
	if len(payload.CustomMetrics.Series) != 1 {
		t.Fatalf("expected one series, got %#v", payload.CustomMetrics.Series)
	}
	if payload.CustomMetrics.Series[0].Value != 5 {
		t.Fatalf("expected count sum=5, got %#v", payload.CustomMetrics.Series[0])
	}
	if payload.CustomMetrics.DroppedCount != 1 {
		t.Fatalf("expected dropped_count=1, got %d", payload.CustomMetrics.DroppedCount)
	}
}

func TestCollectorHTTPIngest(t *testing.T) {
	addr := freeTCPAddr(t)
	cfg := config.Config{
		CustomMetricsEnabled:   true,
		CustomMetricsHTTPAddr:  addr,
		CustomMetricsInterval:  time.Second,
		CustomMetricsMaxSeries: 10,
		CustomMetricsMaxBytes:  4096,
	}
	c := New(cfg, nil).(*collector)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := c.Collect(ctx); err != nil {
		t.Fatalf("start collect: %v", err)
	}

	body := []byte(`{"metrics":[{"name":"app.latency","type":"histogram","value":12,"tags":["env:test"]}]}`)
	resp, err := http.Post("http://"+addr+"/v1/custom-metrics", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post metric: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}

	raw, err := c.Collect(ctx)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	var payload struct {
		CustomMetrics snapshot `json:"custom_metrics"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.CustomMetrics.Series) != 1 || payload.CustomMetrics.Series[0].Name != "app.latency" {
		t.Fatalf("unexpected series %#v", payload.CustomMetrics.Series)
	}
}

func TestCollectorUDSIngest(t *testing.T) {
	socketFile, err := os.CreateTemp("/tmp", "aiceberg-cm-*.sock")
	if err != nil {
		t.Fatalf("temp socket path: %v", err)
	}
	socketPath := socketFile.Name()
	_ = socketFile.Close()
	_ = os.Remove(socketPath)
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	cfg := config.Config{
		CustomMetricsEnabled:   true,
		CustomMetricsUDSPath:   socketPath,
		CustomMetricsInterval:  time.Second,
		CustomMetricsMaxSeries: 10,
		CustomMetricsMaxBytes:  4096,
	}
	c := New(cfg, nil).(*collector)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := c.Collect(ctx); err != nil {
		t.Fatalf("start collect: %v", err)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial uds: %v", err)
	}
	if _, err := conn.Write([]byte("app.uds.requests:7|c|#service:worker\n")); err != nil {
		t.Fatalf("write uds: %v", err)
	}
	_ = conn.Close()

	var raw []byte
	for i := 0; i < 20; i++ {
		raw, err = c.Collect(ctx)
		if err != nil {
			t.Fatalf("collect: %v", err)
		}
		if len(raw) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(raw) == 0 {
		t.Fatalf("expected custom metrics payload")
	}
	var payload struct {
		CustomMetrics snapshot `json:"custom_metrics"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.CustomMetrics.Series) != 1 || payload.CustomMetrics.Series[0].Name != "app.uds.requests" {
		t.Fatalf("unexpected series %#v", payload.CustomMetrics.Series)
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
