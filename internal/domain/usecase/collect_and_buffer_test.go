package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
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
	appendErr error
	batch     []entities.Envelope
	acked     [][]string
}

func (f *fakeOutbox) Append(env entities.Envelope) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	f.batch = append(f.batch, env)
	return nil
}

func (f *fakeOutbox) ReadBatch(n int) ([]entities.Envelope, error) {
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
	uc := NewCollectAndBuffer(collector, outbox, log, "Token test", "/v1/ingest/metrics")
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
	if !bytes.Equal(raw, collector.data) {
		t.Fatalf("unexpected body %s", string(raw))
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

var _ ports.Collector = (*fakeCollector)(nil)
