package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type fakeCollector struct {
	name     string
	interval time.Duration
	data     []byte
	err      error
}

func (f *fakeCollector) Name() string            { return f.name }
func (f *fakeCollector) Interval() time.Duration { return f.interval }
func (f *fakeCollector) Collect(_ context.Context) ([]byte, error) {
	return f.data, f.err
}

type fakeOutbox struct {
	appendErr     error
	batch         []entities.Envelope
	acked         [][]string
	readBatchSize int
	replaced      map[string][]entities.Envelope
}

func (f *fakeOutbox) Append(env entities.Envelope) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	f.batch = append(f.batch, env)
	return nil
}

func (f *fakeOutbox) ReadBatch(n int) ([]entities.Envelope, error) {
	f.readBatchSize = n
	if n > len(f.batch) {
		n = len(f.batch)
	}
	cp := make([]entities.Envelope, n)
	copy(cp, f.batch[:n])
	return cp, nil
}

func (f *fakeOutbox) Ack(ids []string) error {
	cp := make([]string, len(ids))
	copy(cp, ids)
	f.acked = append(f.acked, cp)
	return nil
}

func (f *fakeOutbox) Len() (int, int64) { return len(f.batch), 0 }

func (f *fakeOutbox) ReplaceEnvelope(originalID string, replacements []entities.Envelope) error {
	if f.replaced == nil {
		f.replaced = make(map[string][]entities.Envelope)
	}
	f.replaced[originalID] = append([]entities.Envelope(nil), replacements...)
	for i, envelope := range f.batch {
		if envelope.ID != originalID {
			continue
		}
		next := make([]entities.Envelope, 0, len(f.batch)-1+len(replacements))
		next = append(next, f.batch[:i]...)
		next = append(next, replacements...)
		next = append(next, f.batch[i+1:]...)
		f.batch = next
		break
	}
	return nil
}

type fakeLogger struct {
	info  []string
	err   []string
	fatal []string
}

func (l *fakeLogger) Info(msg string)  { l.info = append(l.info, msg) }
func (l *fakeLogger) Error(msg string) { l.err = append(l.err, msg) }
func (l *fakeLogger) Fatal(msg string, _ ...any) {
	l.fatal = append(l.fatal, msg)
}
func (l *fakeLogger) Sync() {}

func TestCollectAndBuffer_AppendsEnvelope(t *testing.T) {
	outbox := &fakeOutbox{}
	log := &fakeLogger{}
	collector := &fakeCollector{
		name: "sysmetrics",
		data: []byte(`{"ok":true}`),
	}
	uc := NewCollectAndBufferWithIdentity(collector, outbox, log, "Token test", "identity-header", "/v1/ingest/metrics")
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(outbox.batch) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(outbox.batch))
	}
	env := outbox.batch[0]
	if env.AuthHeader != "Token test" {
		t.Fatalf("expected auth header, got %q", env.AuthHeader)
	}
	if env.IdentityHeader != "identity-header" {
		t.Fatalf("expected identity header, got %q", env.IdentityHeader)
	}
	if env.Endpoint != "/v1/ingest/metrics" {
		t.Fatalf("expected endpoint, got %q", env.Endpoint)
	}
	if env.Kind != "metric" {
		t.Fatalf("expected kind metric, got %q", env.Kind)
	}
	if env.Sub != "sysmetrics" {
		t.Fatalf("expected sub sysmetrics, got %q", env.Sub)
	}
	if env.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", env.SchemaVersion)
	}
	if env.ID == "" {
		t.Fatalf("expected non-empty id")
	}
	hostname, _ := os.Hostname()
	if hostname != "" && env.AgentID != hostname {
		t.Fatalf("expected agent_id %q, got %q", hostname, env.AgentID)
	}
	raw, ok := env.Body.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage body, got %T", env.Body)
	}
	if bytes.Equal(raw, collector.data) {
		t.Fatalf("expected body to include runtime metadata")
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("expected original body field, got %#v", body)
	}
	if body["agent_pipeline_version"] != "2-compatible" {
		t.Fatalf("expected pipeline metadata, got %#v", body)
	}
	if body["collector_name"] != "sysmetrics" || body["ingest_endpoint"] != "/v1/ingest/metrics" {
		t.Fatalf("unexpected metadata %#v", body)
	}
}

func TestCollectAndBuffer_UsesIdentityProviderPerCollect(t *testing.T) {
	outbox := &fakeOutbox{}
	log := &fakeLogger{}
	collector := &fakeCollector{
		name: "sysmetrics",
		data: []byte(`{"ok":true}`),
	}
	calls := 0
	uc := NewCollectAndBufferWithIdentityProvider(collector, outbox, log, "Token test", func() string {
		calls++
		return "identity-" + strconv.Itoa(calls)
	}, "/v1/ingest/metrics")

	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("first execute: %v", err)
	}
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if len(outbox.batch) != 2 {
		t.Fatalf("expected 2 envelopes, got %d", len(outbox.batch))
	}
	if outbox.batch[0].IdentityHeader != "identity-1" || outbox.batch[1].IdentityHeader != "identity-2" {
		t.Fatalf("expected refreshed identity headers, got %q and %q", outbox.batch[0].IdentityHeader, outbox.batch[1].IdentityHeader)
	}
}

func TestCollectAndBuffer_AppendsControlledExtraLogEndpoints(t *testing.T) {
	outbox := &fakeOutbox{}
	log := &fakeLogger{}
	collector := &fakeCollector{
		name: "oslogs",
		data: []byte(`{"events":[{"message":"ok"}]}`),
	}
	uc := NewCollectAndBufferWithIdentityAndExtraEndpoints(
		collector,
		outbox,
		log,
		"Token test",
		"identity-header",
		"/v1/logs/raw",
		[]string{"/v1/logs/archive", "https://unsafe.example/logs", "/v1/logs/raw", "/v1/ingest/metrics"},
	)

	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(outbox.batch) != 2 {
		t.Fatalf("expected primary plus one safe extra envelope, got %d", len(outbox.batch))
	}
	if outbox.batch[0].Endpoint != "/v1/logs/raw" {
		t.Fatalf("expected primary logs endpoint, got %q", outbox.batch[0].Endpoint)
	}
	if outbox.batch[1].Endpoint != "/v1/logs/archive" {
		t.Fatalf("expected safe extra logs endpoint, got %q", outbox.batch[1].Endpoint)
	}
	if !bytes.Equal(outbox.batch[0].Body.(json.RawMessage), outbox.batch[1].Body.(json.RawMessage)) {
		t.Fatalf("expected dual-shipped payload body to match primary body")
	}
}

func TestCollectAndBuffer_ExtraEndpointsProviderIsDynamic(t *testing.T) {
	outbox := &fakeOutbox{}
	log := &fakeLogger{}
	extras := []string{"/v1/logs/archive"}
	collector := &fakeCollector{
		name: "oslogs",
		data: []byte(`{"events":[{"message":"ok"}]}`),
	}
	uc := NewCollectAndBufferWithIdentityAndExtraEndpointsProvider(
		collector,
		outbox,
		log,
		"Token test",
		"identity-header",
		"/v1/logs/raw",
		func() []string { return extras },
	)

	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("first execute: %v", err)
	}
	extras = []string{"/v1/logs/secondary"}
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if got := outbox.batch[len(outbox.batch)-1].Endpoint; got != "/v1/logs/secondary" {
		t.Fatalf("expected dynamic extra endpoint, got %q", got)
	}
}

func TestCollectAndBuffer_InvalidCollectorPayloadIsNotBuffered(t *testing.T) {
	outbox := &fakeOutbox{}
	log := &fakeLogger{}
	collector := &fakeCollector{
		name: "sysmetrics",
		data: []byte(`not-json`),
	}
	uc := NewCollectAndBufferWithIdentity(collector, outbox, log, "Token test", "identity-header", "/v1/ingest/metrics")
	if err := uc.Execute(context.Background()); err == nil {
		t.Fatalf("expected invalid payload error")
	}
	if len(outbox.batch) != 0 {
		t.Fatalf("expected no envelope appended")
	}
}

func TestCollectAndBuffer_CollectError(t *testing.T) {
	outbox := &fakeOutbox{}
	log := &fakeLogger{}
	collector := &fakeCollector{
		name: "sysmetrics",
		err:  errors.New("collect failed"),
	}
	uc := NewCollectAndBuffer(collector, outbox, log, "Token test", "/v1/ingest/metrics")
	if err := uc.Execute(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
	if len(outbox.batch) != 0 {
		t.Fatalf("expected no envelope appended")
	}
}

func TestCachedIdentityHeaderProviderReusesHeaderWithinTTL(t *testing.T) {
	calls := 0
	provider := NewCachedIdentityHeaderProvider(time.Hour, func() string {
		calls++
		return "identity-" + strconv.Itoa(calls)
	})

	first := provider()
	second := provider()
	if first != "identity-1" || second != "identity-1" {
		t.Fatalf("expected cached identity, got %q and %q", first, second)
	}
	if calls != 1 {
		t.Fatalf("expected provider called once, got %d", calls)
	}
}

var _ ports.Collector = (*fakeCollector)(nil)
