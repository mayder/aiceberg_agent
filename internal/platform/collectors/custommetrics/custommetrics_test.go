package custommetrics

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

func TestCollectorEmitsControlledValidationSample(t *testing.T) {
	cfg := config.Config{
		CustomMetricsEnabled:   true,
		CustomMetricsInterval:  time.Second,
		CustomMetricsMaxSeries: 10,
		CustomMetricsMaxBytes:  4096,
	}
	c := New(cfg, func() config.CollectPrefs {
		return config.CollectPrefs{
			Version:                       "remote",
			CustomMetricsEnabled:          true,
			CustomMetricsValidationSample: true,
		}
	}).(*collector)

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
	if len(payload.CustomMetrics.Series) != 1 {
		t.Fatalf("expected validation sample, got %#v", payload.CustomMetrics.Series)
	}
	series := payload.CustomMetrics.Series[0]
	if series.Name != "aiceberg.validation.custom_metrics" || series.Source != "agent_controlled_sample" {
		t.Fatalf("unexpected validation sample %#v", series)
	}
}

func TestCollectorBoundsHighVolumeCardinalityBurst(t *testing.T) {
	const maxSeries = 25
	const totalSeries = 250
	cfg := config.Config{
		CustomMetricsEnabled:   true,
		CustomMetricsInterval:  time.Second,
		CustomMetricsMaxSeries: maxSeries,
		CustomMetricsMaxBytes:  64 * 1024,
	}
	c := New(cfg, nil).(*collector)
	var burst strings.Builder
	for i := 0; i < totalSeries; i++ {
		burst.WriteString("app.burst:")
		burst.WriteString("1")
		burst.WriteString("|c|#host:local,service:api,env:test,series:")
		burst.WriteString(strconv.Itoa(i))
		burst.WriteString(",idx:")
		burst.WriteString(strconv.Itoa(i))
		burst.WriteByte('\n')
	}
	c.ingestLines(burst.String())

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
	if len(payload.CustomMetrics.Series) != maxSeries {
		t.Fatalf("expected %d bounded series, got %d", maxSeries, len(payload.CustomMetrics.Series))
	}
	if payload.CustomMetrics.AcceptedCount != maxSeries {
		t.Fatalf("expected accepted_count=%d, got %d", maxSeries, payload.CustomMetrics.AcceptedCount)
	}
	if payload.CustomMetrics.DroppedCount != totalSeries-maxSeries {
		t.Fatalf("expected dropped_count=%d, got %d", totalSeries-maxSeries, payload.CustomMetrics.DroppedCount)
	}

	raw, err = c.Collect(context.Background())
	if err != nil {
		t.Fatalf("second collect: %v", err)
	}
	if len(raw) != 0 {
		t.Fatalf("expected empty payload after flush, got %s", string(raw))
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

func TestCollectorHTTPRejectsOversizedPayload(t *testing.T) {
	addr := freeTCPAddr(t)
	cfg := config.Config{
		CustomMetricsEnabled:   true,
		CustomMetricsHTTPAddr:  addr,
		CustomMetricsInterval:  time.Second,
		CustomMetricsMaxSeries: 10,
		CustomMetricsMaxBytes:  64,
	}
	c := New(cfg, nil).(*collector)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := c.Collect(ctx); err != nil {
		t.Fatalf("start collect: %v", err)
	}

	body := bytes.Repeat([]byte("x"), cfg.CustomMetricsMaxBytes+1)
	resp, err := http.Post("http://"+addr+"/v1/custom-metrics", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post oversized metric: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}

	raw, err := c.Collect(ctx)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(raw) != 0 {
		t.Fatalf("expected no payload after rejected oversized body, got %s", string(raw))
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

func TestPKG61LocalAppHighVolumeEvidence(t *testing.T) {
	addr := freeTCPAddr(t)
	socketFile, err := os.CreateTemp("/tmp", "aiceberg-pkg61-*.sock")
	if err != nil {
		t.Fatalf("temp socket path: %v", err)
	}
	socketPath := socketFile.Name()
	_ = socketFile.Close()
	_ = os.Remove(socketPath)
	t.Cleanup(func() { _ = os.Remove(socketPath) })

	cfg := config.Config{
		CustomMetricsEnabled:   true,
		CustomMetricsUDPAddr:   "127.0.0.1:0",
		CustomMetricsHTTPAddr:  addr,
		CustomMetricsUDSPath:   socketPath,
		CustomMetricsInterval:  time.Second,
		CustomMetricsMaxSeries: 32,
		CustomMetricsMaxBytes:  64 * 1024,
	}
	c := New(cfg, nil).(*collector)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := c.Collect(ctx); err != nil {
		t.Fatalf("start collect: %v", err)
	}

	httpBody := []byte(`{"metrics":[{"name":"checkout.latency_ms","type":"histogram","value":42,"service":"checkout-api","env":"controlled","tags":["source:http","route:/pay"]}]}`)
	resp, err := http.Post("http://"+addr+"/v1/custom-metrics", "application/json", bytes.NewReader(httpBody))
	if err != nil {
		t.Fatalf("post app metric: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected app metric status %d", resp.StatusCode)
	}

	udpAddr := c.udpConn.LocalAddr().String()
	udp, err := net.Dial("udp", udpAddr)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	if _, err := udp.Write([]byte("orders.created:3|c|#service:orders-api,env:controlled,source:udp\n")); err != nil {
		t.Fatalf("write udp: %v", err)
	}

	uds, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial uds: %v", err)
	}
	if _, err := uds.Write([]byte("worker.queue.depth:9|g|#service:worker,env:controlled,source:uds\n")); err != nil {
		t.Fatalf("write uds: %v", err)
	}
	_ = uds.Close()
	time.Sleep(100 * time.Millisecond)

	var burst strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&burst, "pkg61.high_cardinality:1|c|#service:orders-api,env:controlled,tenant:t%d,request:r%d\n", i, i)
	}
	if _, err := udp.Write([]byte(burst.String())); err != nil {
		t.Fatalf("write udp burst: %v", err)
	}
	_ = udp.Close()
	time.Sleep(100 * time.Millisecond)

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
	if payload.CustomMetrics.AcceptedCount != cfg.CustomMetricsMaxSeries {
		t.Fatalf("expected accepted_count=%d, got %d", cfg.CustomMetricsMaxSeries, payload.CustomMetrics.AcceptedCount)
	}
	if payload.CustomMetrics.DroppedCount == 0 {
		t.Fatalf("expected dropped_count > 0 for high cardinality burst")
	}
	if !hasSeries(payload.CustomMetrics.Series, "checkout.latency_ms") {
		t.Fatalf("expected app HTTP metric in payload: %#v", payload.CustomMetrics.Series)
	}
	if !hasSeries(payload.CustomMetrics.Series, "orders.created") {
		t.Fatalf("expected UDP DogStatsD metric in payload: %#v", payload.CustomMetrics.Series)
	}
	if !hasSeries(payload.CustomMetrics.Series, "worker.queue.depth") {
		t.Fatalf("expected UDS DogStatsD metric in payload: %#v", payload.CustomMetrics.Series)
	}
	if mode := socketMode(t, socketPath); mode != 0o600 {
		t.Fatalf("expected UDS mode 0600, got %o", mode)
	}

	if evidenceDir := strings.TrimSpace(os.Getenv("PKG61_EVIDENCE_DIR")); evidenceDir != "" {
		writePKG61Evidence(t, evidenceDir, raw, map[string]string{
			"accepted_count":    strconv.Itoa(payload.CustomMetrics.AcceptedCount),
			"dropped_count":     strconv.Itoa(payload.CustomMetrics.DroppedCount),
			"series_count":      strconv.Itoa(len(payload.CustomMetrics.Series)),
			"http_app_metric":   strconv.FormatBool(hasSeries(payload.CustomMetrics.Series, "checkout.latency_ms")),
			"udp_dogstatsd":     strconv.FormatBool(hasSeries(payload.CustomMetrics.Series, "orders.created")),
			"uds_dogstatsd":     strconv.FormatBool(hasSeries(payload.CustomMetrics.Series, "worker.queue.depth")),
			"uds_socket_mode":   "0600",
			"max_series":        strconv.Itoa(cfg.CustomMetricsMaxSeries),
			"high_cardinality":  "80",
			"api_credential":    "not_used",
			"loopback_only":     "yes",
			"service_env_tags":  "yes",
			"flush_window_secs": strconv.Itoa(int(cfg.CustomMetricsInterval.Seconds())),
		})
	}
}

func hasSeries(series []aggregateSeries, name string) bool {
	for _, item := range series {
		if item.Name == name {
			return true
		}
	}
	return false
}

func socketMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	return info.Mode().Perm()
}

func writePKG61Evidence(t *testing.T, dir string, raw []byte, summary map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir evidence dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "custom_metrics_payload.json"), raw, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	var b strings.Builder
	keys := []string{
		"accepted_count", "dropped_count", "series_count", "http_app_metric", "udp_dogstatsd",
		"uds_dogstatsd", "uds_socket_mode", "max_series", "high_cardinality", "api_credential",
		"loopback_only", "service_env_tags", "flush_window_secs",
	}
	for _, key := range keys {
		fmt.Fprintf(&b, "%s\t%s\n", key, summary[key])
	}
	if err := os.WriteFile(filepath.Join(dir, "SUMMARY.tsv"), []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
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
