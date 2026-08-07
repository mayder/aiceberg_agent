package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type transportCall struct {
	auth     string
	identity string
	endpoint string
	size     int
	bytes    int
}

type fakeTransport struct {
	calls         []transportCall
	err           error
	body          []byte
	errByEndpoint map[string]error
}

type fakeHTTPStatusErr struct{ code int }

func (e fakeHTTPStatusErr) Error() string   { return http.StatusText(e.code) }
func (e fakeHTTPStatusErr) StatusCode() int { return e.code }

func (f *fakeTransport) SendWithAuth(batch []entities.Envelope, authHeader string, endpoint string) ([]byte, error) {
	identity := ""
	if len(batch) > 0 {
		identity = batch[0].IdentityHeader
	}
	raw, _ := json.Marshal(batch)
	f.calls = append(f.calls, transportCall{auth: authHeader, identity: identity, endpoint: endpoint, size: len(batch), bytes: len(raw)})
	if f.errByEndpoint != nil && f.errByEndpoint[endpoint] != nil {
		return nil, f.errByEndpoint[endpoint]
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.body, nil
}

func TestFlushOutbox_SplitsRequestBySerializedSize(t *testing.T) {
	body := json.RawMessage(`{"payload":"` + strings.Repeat("x", 80) + `"}`)
	outbox := &fakeOutbox{batch: []entities.Envelope{
		{ID: "1", AuthHeader: "Token a", Endpoint: "/v1/ingest/metrics", Body: body},
		{ID: "2", AuthHeader: "Token a", Endpoint: "/v1/ingest/metrics", Body: body},
		{ID: "3", AuthHeader: "Token a", Endpoint: "/v1/ingest/metrics", Body: body},
	}}
	tx := &fakeTransport{body: []byte(`{"received":1,"skipped":0,"status":"ok"}`)}
	uc := NewFlushOutboxWithOptions(outbox, tx, &fakeLogger{}, "Token default", nil, FlushOutboxOptions{
		BatchSize: 3, MaxBatchBytes: 260,
	})

	n, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if n != 3 || len(tx.calls) != 3 {
		t.Fatalf("expected three safe requests and acks, got acked=%d calls=%#v", n, tx.calls)
	}
	for _, call := range tx.calls {
		if call.bytes > 260 {
			t.Fatalf("request exceeded byte limit: %#v", call)
		}
	}
}

func TestFlushOutbox_RetainsSingleEnvelopeAboveRequestLimit(t *testing.T) {
	outbox := &fakeOutbox{batch: []entities.Envelope{{
		ID: "oversized", Endpoint: "/v1/logs/raw",
		Body: json.RawMessage(`{"payload":"` + strings.Repeat("x", 300) + `"}`),
	}}}
	tx := &fakeTransport{}
	uc := NewFlushOutboxWithOptions(outbox, tx, &fakeLogger{}, "Token default", nil, FlushOutboxOptions{
		BatchSize: 1, MaxBatchBytes: 200,
	})

	n, err := uc.Execute(context.Background())
	if err == nil || n != 0 {
		t.Fatalf("expected retained oversized envelope, got acked=%d err=%v", n, err)
	}
	if len(tx.calls) != 0 || len(outbox.acked) != 0 {
		t.Fatalf("oversized envelope must not reach transport or be acked")
	}
}

func TestFlushOutbox_GroupsByIdentityHeader(t *testing.T) {
	outbox := &fakeOutbox{batch: []entities.Envelope{
		{ID: "1", AuthHeader: "Token a", IdentityHeader: "identity-a", Endpoint: "/v1/ingest/metrics", AgentID: "a"},
		{ID: "2", AuthHeader: "Token a", IdentityHeader: "identity-b", Endpoint: "/v1/ingest/metrics", AgentID: "b"},
	}}
	tx := &fakeTransport{}
	uc := NewFlushOutbox(outbox, tx, &fakeLogger{}, "Token default", nil)

	n, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 acked, got %d", n)
	}
	if len(tx.calls) != 2 {
		t.Fatalf("expected 2 transport calls, got %d", len(tx.calls))
	}
	seen := map[string]bool{}
	for _, call := range tx.calls {
		seen[call.identity] = true
	}
	if !seen["identity-a"] || !seen["identity-b"] {
		t.Fatalf("expected both identity groups, got %#v", seen)
	}
}

func TestFlushOutbox_GroupAndAck(t *testing.T) {
	outbox := &fakeOutbox{}
	log := &fakeLogger{}
	batch := []entities.Envelope{
		{ID: "1", AuthHeader: "", Endpoint: "", AgentID: "a"},
		{ID: "2", AuthHeader: "Token a", Endpoint: "/v1/ingest/health", AgentID: "a"},
		{ID: "3", AuthHeader: "Token a", Endpoint: "/v1/ingest/metrics", AgentID: "a"},
		{ID: "", AuthHeader: "Token b", Endpoint: "/v1/ingest", AgentID: "a"},
	}
	outbox.batch = batch
	tx := &fakeTransport{}
	uc := NewFlushOutbox(outbox, tx, log, "Token default", nil)

	n, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 acked, got %d", n)
	}
	if len(tx.calls) != 3 {
		t.Fatalf("expected 3 transport calls, got %d", len(tx.calls))
	}
	callMap := map[string]int{}
	for _, c := range tx.calls {
		key := c.auth + "|" + c.endpoint
		callMap[key] = c.size
	}
	if callMap["Token default|/v1/ingest"] != 1 {
		t.Fatalf("expected default auth ingest call")
	}
	if callMap["Token a|/v1/ingest/health"] != 1 {
		t.Fatalf("expected health call")
	}
	if callMap["Token a|/v1/ingest/metrics"] != 1 {
		t.Fatalf("expected metrics call")
	}
	if len(outbox.acked) != 4 {
		t.Fatalf("expected 4 ack calls, got %d", len(outbox.acked))
	}
	valid := make([]string, 0, 3)
	for _, ids := range outbox.acked {
		for _, id := range ids {
			if id != "" {
				valid = append(valid, id)
			}
		}
	}
	sort.Strings(valid)
	if want := []string{"1", "2", "3"}; !equalStrings(valid, want) {
		t.Fatalf("expected acked %v, got %v", want, valid)
	}
}

func TestFlushOutbox_TransportError(t *testing.T) {
	outbox := &fakeOutbox{}
	log := &fakeLogger{}
	outbox.batch = []entities.Envelope{
		{ID: "1", AuthHeader: "Token a", Endpoint: "/v1/ingest", AgentID: "a"},
	}
	tx := &fakeTransport{err: errors.New("transport down")}
	uc := NewFlushOutbox(outbox, tx, log, "Token default", nil)

	if _, err := uc.Execute(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
	if len(outbox.acked) != 0 {
		t.Fatalf("expected no ack on transport error")
	}
}

func TestFlushOutbox_RetainsBatchWhenIngestDidNotPersist(t *testing.T) {
	outbox := &fakeOutbox{batch: []entities.Envelope{
		{ID: "network-1", AuthHeader: "Token a", Endpoint: "/v1/ingest/network_capture", AgentID: "a"},
	}}
	log := &fakeLogger{}
	tx := &fakeTransport{body: []byte(`{"received":0,"skipped":1,"status":"ok","errors_by_reason":{"persist_failed":1}}`)}
	uc := NewFlushOutbox(outbox, tx, log, "Token default", nil)

	n, err := uc.Execute(context.Background())
	if err == nil {
		t.Fatalf("expected ingest ack error")
	}
	if n != 0 {
		t.Fatalf("expected no acked envelopes, got %d", n)
	}
	if len(outbox.acked) != 0 {
		t.Fatalf("expected no outbox ack, got %#v", outbox.acked)
	}
	snap := uc.Snapshot()
	if snap.LastErrorRoute != "/v1/ingest/network_capture" || snap.LastRetained != 1 {
		t.Fatalf("expected retained network capture snapshot, got %#v", snap)
	}
	if len(log.err) == 0 {
		t.Fatalf("expected error log for retained ingest batch")
	}
}

func TestFlushOutbox_AcksDuplicateEnvelopeSkip(t *testing.T) {
	outbox := &fakeOutbox{batch: []entities.Envelope{
		{ID: "network-dup", AuthHeader: "Token a", Endpoint: "/v1/ingest/network_capture", AgentID: "a"},
	}}
	tx := &fakeTransport{body: []byte(`{"received":0,"skipped":1,"status":"ok","errors_by_reason":{"duplicate_envelope_id":1}}`)}
	uc := NewFlushOutbox(outbox, tx, &fakeLogger{}, "Token default", nil)

	n, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if n != 1 {
		t.Fatalf("expected duplicate envelope to be acked, got %d", n)
	}
	if len(outbox.acked) != 1 || !equalStrings(outbox.acked[0], []string{"network-dup"}) {
		t.Fatalf("expected duplicate ack, got %#v", outbox.acked)
	}
}

func TestFlushOutbox_PartialAckWhenMetricsTimeout(t *testing.T) {
	outbox := &fakeOutbox{}
	log := &fakeLogger{}
	outbox.batch = []entities.Envelope{
		{ID: "health-1", AuthHeader: "Token a", Endpoint: "/v1/ingest/health", AgentID: "a", TSUnixMs: 1000},
		{ID: "metrics-1", AuthHeader: "Token a", Endpoint: "/v1/ingest/metrics", AgentID: "a", TSUnixMs: 1000},
	}
	tx := &fakeTransport{errByEndpoint: map[string]error{
		"/v1/ingest/metrics": errors.New("context deadline exceeded"),
	}}
	uc := NewFlushOutbox(outbox, tx, log, "Token default", nil)

	n, err := uc.Execute(context.Background())
	if err == nil {
		t.Fatalf("expected metrics error")
	}
	if n != 1 {
		t.Fatalf("expected 1 acked, got %d", n)
	}
	if len(outbox.acked) != 1 || !equalStrings(outbox.acked[0], []string{"health-1"}) {
		t.Fatalf("expected only health acked, got %#v", outbox.acked)
	}
	snap := uc.Snapshot()
	if snap.LastErrorRoute != "/v1/ingest/metrics" {
		t.Fatalf("expected metrics last error, got %#v", snap)
	}
	if snap.LastAckRoute != "/v1/ingest/health" {
		t.Fatalf("expected health last ack, got %#v", snap)
	}
}

func TestFlushOutbox_ConfigurableBatchSize(t *testing.T) {
	outbox := &fakeOutbox{}
	for i := 0; i < 4; i++ {
		outbox.batch = append(outbox.batch, entities.Envelope{
			ID:       string(rune('a' + i)),
			Endpoint: "/v1/ingest/health",
			AgentID:  "a",
		})
	}
	tx := &fakeTransport{}
	uc := NewFlushOutboxWithOptions(outbox, tx, &fakeLogger{}, "Token default", nil, FlushOutboxOptions{BatchSize: 2})

	n, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 acked, got %d", n)
	}
	if outbox.readBatchSize != 2 {
		t.Fatalf("expected read batch 2, got %d", outbox.readBatchSize)
	}
}

func TestFlushOutbox_BackoffSkipsFailedRouteTemporarily(t *testing.T) {
	outbox := &fakeOutbox{batch: []entities.Envelope{
		{ID: "metrics-1", AuthHeader: "Token a", Endpoint: "/v1/ingest/metrics", AgentID: "a"},
	}}
	tx := &fakeTransport{errByEndpoint: map[string]error{
		"/v1/ingest/metrics": errors.New("context deadline exceeded"),
	}}
	uc := NewFlushOutbox(outbox, tx, &fakeLogger{}, "Token default", nil)
	now := time.Unix(100, 0)
	uc.now = func() time.Time { return now }

	if _, err := uc.Execute(context.Background()); err == nil {
		t.Fatalf("expected first error")
	}
	tx.errByEndpoint = nil
	if _, err := uc.Execute(context.Background()); err == nil {
		t.Fatalf("expected backoff error")
	}
	if len(tx.calls) != 1 {
		t.Fatalf("expected second run to skip transport, got %d calls", len(tx.calls))
	}
}

func TestFlushOutbox_TransientTransportFailureDoesNotLogError(t *testing.T) {
	outbox := &fakeOutbox{batch: []entities.Envelope{
		{ID: "metrics-1", AuthHeader: "Token a", Endpoint: "/v1/ingest/metrics", AgentID: "a"},
	}}
	tx := &fakeTransport{errByEndpoint: map[string]error{
		"/v1/ingest/metrics": context.DeadlineExceeded,
	}}
	log := &fakeLogger{}
	uc := NewFlushOutbox(outbox, tx, log, "Token default", nil)

	if _, err := uc.Execute(context.Background()); err == nil {
		t.Fatalf("expected transient transport error")
	}
	if len(log.err) != 0 {
		t.Fatalf("transient transport failure must not be logged as ERROR, got %#v", log.err)
	}
}

func TestFlushOutbox_PermanentHTTPErrorUsesCooldownWithoutExponentialRetry(t *testing.T) {
	outbox := &fakeOutbox{batch: []entities.Envelope{
		{ID: "metrics-1", AuthHeader: "Token a", Endpoint: "/v1/ingest/metrics", AgentID: "a"},
	}}
	tx := &fakeTransport{errByEndpoint: map[string]error{
		"/v1/ingest/metrics": fakeHTTPStatusErr{code: http.StatusBadRequest},
	}}
	uc := NewFlushOutbox(outbox, tx, &fakeLogger{}, "Token default", nil)

	if _, err := uc.Execute(context.Background()); err == nil {
		t.Fatalf("expected first permanent error")
	}
	if _, err := uc.Execute(context.Background()); err == nil {
		t.Fatalf("expected cooldown error")
	}
	if len(tx.calls) != 1 {
		t.Fatalf("expected permanent cooldown to skip second API call, got %d calls", len(tx.calls))
	}
	if snap := uc.Snapshot(); snap.LastBackoffRoute != "/v1/ingest/metrics" || snap.LastBackoffUntilUnix == 0 {
		t.Fatalf("permanent cooldown must be visible in snapshot, got %#v", snap)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

var _ ports.Transport = (*fakeTransport)(nil)
