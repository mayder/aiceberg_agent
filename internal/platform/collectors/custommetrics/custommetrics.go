package custommetrics

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const schemaVersion = 1

type collector struct {
	prefs       func() config.CollectPrefs
	baseEnabled bool
	interval    time.Duration
	udpAddr     string
	httpAddr    string
	maxSeries   int
	maxBytes    int
	startOnce   sync.Once
	startErr    error
	aggregator  *aggregator
	httpServer  *http.Server
	udpConn     *net.UDPConn
}

func New(cfg config.Config, prefsProvider func() config.CollectPrefs) ports.Collector {
	interval := cfg.CustomMetricsInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	maxSeries := cfg.CustomMetricsMaxSeries
	if maxSeries <= 0 {
		maxSeries = 1000
	}
	maxBytes := cfg.CustomMetricsMaxBytes
	if maxBytes <= 0 {
		maxBytes = 65536
	}
	return &collector{
		prefs:       prefsProvider,
		baseEnabled: cfg.CustomMetricsEnabled,
		interval:    interval,
		udpAddr:     cfg.CustomMetricsUDPAddr,
		httpAddr:    cfg.CustomMetricsHTTPAddr,
		maxSeries:   maxSeries,
		maxBytes:    maxBytes,
		aggregator:  newAggregator(maxSeries),
	}
}

func (c *collector) Name() string { return "custommetrics" }

func (c *collector) Interval() time.Duration { return c.interval }

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	if !c.enabled() {
		return nil, nil
	}
	c.startOnce.Do(func() {
		c.startErr = c.start(ctx)
	})
	if c.startErr != nil {
		return nil, c.startErr
	}
	snapshot := c.aggregator.flush()
	if len(snapshot.Series) == 0 && snapshot.DroppedCount == 0 {
		return nil, nil
	}
	snapshot.SchemaVersion = schemaVersion
	snapshot.Source = "local_custom_metrics"
	snapshot.FlushWindowSec = int(c.interval.Seconds())
	return json.Marshal(map[string]any{"custom_metrics": snapshot})
}

func (c *collector) enabled() bool {
	p := config.CollectPrefs{}
	if c.prefs != nil {
		p = c.prefs()
	}
	if strings.TrimSpace(p.Version) == "" {
		return c.baseEnabled
	}
	return p.CustomMetricsEnabled
}

func (c *collector) start(ctx context.Context) error {
	var errs []string
	if strings.TrimSpace(c.udpAddr) != "" {
		if err := c.startUDP(ctx); err != nil {
			errs = append(errs, "udp: "+err.Error())
		}
	}
	if strings.TrimSpace(c.httpAddr) != "" {
		if err := c.startHTTP(ctx); err != nil {
			errs = append(errs, "http: "+err.Error())
		}
	}
	if len(errs) > 0 && c.udpConn == nil && c.httpServer == nil {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (c *collector) startUDP(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp", c.udpAddr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	c.udpConn = conn
	go func() {
		defer conn.Close()
		buf := make([]byte, c.maxBytes)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				return
			}
			if !isLoopback(remote.IP) {
				c.aggregator.drop(1)
				continue
			}
			c.ingestLines(string(buf[:n]))
		}
	}()
	return nil
}

func (c *collector) startHTTP(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/custom-metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host != "" && !isLoopback(net.ParseIP(host)) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, int64(c.maxBytes)+1))
		if err != nil || len(body) > c.maxBytes {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		accepted := c.ingestHTTP(body)
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted": accepted})
	})
	server := &http.Server{Addr: c.httpAddr, Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	ln, err := net.Listen("tcp", c.httpAddr)
	if err != nil {
		return err
	}
	c.httpServer = server
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			c.aggregator.drop(1)
		}
	}()
	return nil
}

func (c *collector) ingestLines(raw string) int {
	accepted := 0
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		if metric, ok := parseDogStatsD(scanner.Text(), time.Now().UTC()); ok {
			c.aggregator.add(metric)
			accepted++
		}
	}
	return accepted
}

func (c *collector) ingestHTTP(body []byte) int {
	var payload struct {
		Metrics []customMetric `json:"metrics"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return c.ingestLines(string(body))
	}
	accepted := 0
	for _, metric := range payload.Metrics {
		metric.normalize(time.Now().UTC())
		if metric.Valid() {
			c.aggregator.add(metric)
			accepted++
		}
	}
	return accepted
}

func isLoopback(ip net.IP) bool {
	return ip != nil && ip.IsLoopback()
}

type customMetric struct {
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Value     float64           `json:"value"`
	Tags      []string          `json:"tags,omitempty"`
	Host      string            `json:"host,omitempty"`
	Service   string            `json:"service,omitempty"`
	Env       string            `json:"env,omitempty"`
	Source    string            `json:"source,omitempty"`
	Timestamp string            `json:"timestamp_utc,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

func (m *customMetric) normalize(now time.Time) {
	m.Name = canonicalName(m.Name)
	m.Type = canonicalType(m.Type)
	m.Tags = canonicalTags(m.Tags)
	if m.Source == "" {
		m.Source = "local"
	}
	if m.Timestamp == "" {
		m.Timestamp = now.Format(time.RFC3339Nano)
	}
}

func (m customMetric) Valid() bool {
	return m.Name != "" && m.Type != "" && !math.IsNaN(m.Value) && !math.IsInf(m.Value, 0)
}

func parseDogStatsD(line string, now time.Time) (customMetric, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.Contains(line, "\x00") {
		return customMetric{}, false
	}
	parts := strings.Split(line, "|")
	if len(parts) < 2 {
		return customMetric{}, false
	}
	nameValue := strings.SplitN(parts[0], ":", 2)
	if len(nameValue) != 2 {
		return customMetric{}, false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(nameValue[1]), 64)
	if err != nil {
		return customMetric{}, false
	}
	metric := customMetric{
		Name:      nameValue[0],
		Type:      parts[1],
		Value:     value,
		Source:    "dogstatsd",
		Timestamp: now.Format(time.RFC3339Nano),
	}
	for _, part := range parts[2:] {
		if strings.HasPrefix(part, "#") {
			metric.Tags = strings.Split(strings.TrimPrefix(part, "#"), ",")
		}
	}
	metric.normalize(now)
	return metric, metric.Valid()
}

func canonicalType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "c", "count", "counter":
		return "count"
	case "g", "gauge":
		return "gauge"
	case "h", "histogram":
		return "histogram"
	case "d", "distribution":
		return "distribution"
	case "s", "set":
		return "set"
	case "sc", "service_check":
		return "service_check"
	default:
		return ""
	}
}

func canonicalName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, value)
	return strings.Trim(value, "._-")
}

func canonicalTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || strings.Contains(tag, "\x00") || len(tag) > 128 {
			continue
		}
		if !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

type aggregateSeries struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Value      float64  `json:"value,omitempty"`
	Count      int      `json:"count,omitempty"`
	Sum        float64  `json:"sum,omitempty"`
	Min        float64  `json:"min,omitempty"`
	Max        float64  `json:"max,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Host       string   `json:"host,omitempty"`
	Service    string   `json:"service,omitempty"`
	Env        string   `json:"env,omitempty"`
	Source     string   `json:"source,omitempty"`
	LastStatus int      `json:"last_status,omitempty"`
}

type snapshot struct {
	SchemaVersion  int               `json:"schema_version"`
	Source         string            `json:"source"`
	FlushWindowSec int               `json:"flush_window_sec"`
	Series         []aggregateSeries `json:"series"`
	AcceptedCount  int               `json:"accepted_count"`
	DroppedCount   int               `json:"dropped_count,omitempty"`
}

type aggregator struct {
	mu      sync.Mutex
	series  map[string]*aggregateSeries
	sets    map[string]map[string]struct{}
	max     int
	accept  int
	dropped int
}

func newAggregator(max int) *aggregator {
	return &aggregator{series: map[string]*aggregateSeries{}, sets: map[string]map[string]struct{}{}, max: max}
}

func (a *aggregator) add(metric customMetric) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := metricKey(metric)
	series, ok := a.series[key]
	if !ok {
		if len(a.series) >= a.max {
			a.dropped++
			return
		}
		series = &aggregateSeries{
			Name: metric.Name, Type: metric.Type, Tags: metric.Tags,
			Host: metric.Host, Service: metric.Service, Env: metric.Env, Source: metric.Source,
			Min: metric.Value, Max: metric.Value,
		}
		a.series[key] = series
	}
	switch metric.Type {
	case "count":
		series.Value += metric.Value
	case "gauge":
		series.Value = metric.Value
	case "histogram", "distribution":
		series.Count++
		series.Sum += metric.Value
		if metric.Value < series.Min {
			series.Min = metric.Value
		}
		if metric.Value > series.Max {
			series.Max = metric.Value
		}
	case "set":
		set := a.sets[key]
		if set == nil {
			set = map[string]struct{}{}
			a.sets[key] = set
		}
		set[strconv.FormatFloat(metric.Value, 'f', -1, 64)] = struct{}{}
		series.Value = float64(len(set))
	case "service_check":
		series.LastStatus = int(metric.Value)
	}
	a.accept++
}

func (a *aggregator) drop(n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dropped += n
}

func (a *aggregator) flush() snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]aggregateSeries, 0, len(a.series))
	for _, series := range a.series {
		cp := *series
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return strings.Join(out[i].Tags, ",") < strings.Join(out[j].Tags, ",")
		}
		return out[i].Name < out[j].Name
	})
	snap := snapshot{Series: out, AcceptedCount: a.accept, DroppedCount: a.dropped}
	a.series = map[string]*aggregateSeries{}
	a.sets = map[string]map[string]struct{}{}
	a.accept = 0
	a.dropped = 0
	return snap
}

func metricKey(metric customMetric) string {
	return strings.Join([]string{
		metric.Name,
		metric.Type,
		metric.Host,
		metric.Service,
		metric.Env,
		strings.Join(metric.Tags, ","),
	}, "|")
}
