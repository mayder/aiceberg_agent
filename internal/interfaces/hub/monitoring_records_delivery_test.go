package hub

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/envelopejson"
	"github.com/you/aiceberg_agent/internal/data/local/outbox"
	"github.com/you/aiceberg_agent/internal/data/remote/transport"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/domain/ports"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

type monitoringDeliveryCase struct {
	Name       string                         `json:"name"`
	Definition monitoring.Definition          `json:"definition"`
	Authority  monitoring.DefinitionAuthority `json:"authority"`
	Now        int64                          `json:"now_ms"`
	Batch      monitoring.RecordsBatch        `json:"batch"`
	Hops       []monitoring.TransportProfile  `json:"hops"`
}

func monitoringDeliveryCases(t *testing.T) []monitoringDeliveryCase {
	t.Helper()
	raw, err := os.ReadFile("../../domain/monitoring/testdata/transport-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []monitoringDeliveryCase
	if json.Unmarshal(raw, &cases) != nil || len(cases) < 45 {
		t.Fatal("missing source/type matrix")
	}
	return cases[:45]
}

func openMonitoringStore(t *testing.T, path string, raw bool) (*outbox.BoltStore, ports.OutboxRepo) {
	t.Helper()
	store, err := outbox.NewBoltStoreWithOptions(path, 2, outbox.BoltOptions{RawBodies: raw})
	if err != nil {
		t.Fatal(err)
	}
	return store, repositories.NewOutboxRepository(store)
}

func seedMonitoringDelivery(t *testing.T, repo ports.OutboxRepo, cases []monitoringDeliveryCase) {
	t.Helper()
	for _, c := range cases {
		raw, err := monitoring.EncodeTransportRecords(c.Batch, c.Definition, c.Authority, c.Now, c.Hops)
		if err != nil {
			t.Fatal(err)
		}
		e := entities.Envelope{ID: c.Name, AgentID: "fixture-relay", TenantID: "3", SchemaVersion: 1, Kind: "metric", Sub: "monitoring.records.v1", TSUnixMs: c.Batch.EvaluatedAtMS, Endpoint: "/v1/ingest/metrics", AuthHeader: "Token fixture-monitoring", IdentityHeader: "fixture-identity", Body: json.RawMessage(raw)}
		if err = repo.Append(e); err != nil {
			t.Fatal(err)
		}
	}
}

func executeMonitoringFlush(t *testing.T, repo ports.OutboxRepo, tx ports.Transport, want int, fail bool) {
	t.Helper()
	// A new process after reopen has no volatile backoff, but the same stable IDs.
	flush := usecase.NewFlushOutboxWithOptions(repo, tx, testHubLogger{}, "", nil, usecase.FlushOutboxOptions{BatchSize: 50, MaxBatchBytes: 4096})
	got, err := flush.Execute(context.Background())
	if got != want || (err != nil) != fail {
		t.Fatalf("ack=%d want=%d err=%v", got, want, err)
	}
}

func TestMonitoringRecordsRelayBoltHTTPPHPAndRetry(t *testing.T) {
	if os.Getenv("AICEBERG_MONITORING_WEB_ROOT") == "" {
		t.Skip("explicit Web checkout required")
	}
	for _, raw := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "opaque"}[raw], func(t *testing.T) {
			cases := monitoringDeliveryCases(t)
			path := filepath.Join(t.TempDir(), "relay.db")
			source, repo := openMonitoringStore(t, path, raw)
			seedMonitoringDelivery(t, repo, cases)
			hubPath := filepath.Join(t.TempDir(), "hub.db")
			destination, hubRepo := openMonitoringStore(t, hubPath, raw)
			server := httptest.NewServer(NewHandler(config.Config{HubOutboxRawBodyEnabled: raw}, hubRepo, testHubLogger{}, nil))
			defer server.Close()
			relayTx := transport.NewHubClient(config.Config{HubURL: server.URL})
			executeMonitoringFlush(t, repo, relayTx, len(cases), false)
			source.Close()
			source, repo = openMonitoringStore(t, path, raw)
			defer source.Close()
			if n, _ := repo.Len(); n != 0 {
				t.Fatal("durable hub ack did not release relay")
			}
			if n, _ := hubRepo.Len(); n != len(cases) {
				t.Fatal("hub lost records")
			}
			server.Close()
			if err := destination.Close(); err != nil {
				t.Fatal(err)
			}
			qualifyMonitoringDownstream(t, hubPath, raw, cases)
		})
	}
}

func qualifyMonitoringDownstream(t *testing.T, path string, raw bool, cases []monitoringDeliveryCase) {
	t.Helper()
	var mode atomic.Int32
	var deliveries atomic.Int32
	consumer := monitoringConsumer(t, cases, &mode, &deliveries)
	defer consumer.Close()
	tx := transport.NewHTTPJSONClient(config.Config{APIBaseURL: consumer.URL, HTTPGzip: true, HTTPIdempotency: true})
	for stage := int32(0); stage <= 3; stage++ {
		mode.Store(stage)
		store, repo := openMonitoringStore(t, path, raw)
		want := 0
		if stage == 3 {
			want = len(cases)
		}
		executeMonitoringFlush(t, repo, tx, want, stage != 3)
		remaining, _ := repo.Len()
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		if remaining != len(cases)-want {
			t.Fatal("failure discarded pending batches")
		}
	}
	if deliveries.Load() != int32(len(cases)) {
		t.Fatal("missing delivery or ACK")
	}

}

type monitoringReceiptConsumer struct {
	t               *testing.T
	lookup          map[string]monitoringDeliveryCase
	mode, delivered *atomic.Int32
	mu              sync.Mutex
	seen            map[string]string
}

func monitoringConsumer(t *testing.T, cases []monitoringDeliveryCase, mode, delivered *atomic.Int32) *httptest.Server {
	t.Helper()
	c := &monitoringReceiptConsumer{t: t, lookup: map[string]monitoringDeliveryCase{}, seen: map[string]string{}, mode: mode, delivered: delivered}
	for _, item := range cases {
		c.lookup[item.Name] = item
	}
	return httptest.NewServer(http.HandlerFunc(c.serveHTTP))
}

func (c *monitoringReceiptConsumer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Token fixture-monitoring" || r.Header.Get("X-Agent-Identity") != "fixture-identity" || r.URL.Path != "/v1/ingest/metrics" {
		c.t.Error("lost auth/identity/route")
		w.WriteHeader(403)
		return
	}
	if c.mode.Load() == 0 {
		w.WriteHeader(503)
		return
	}
	batch := readMonitoringHTTPRequest(c.t, r)
	if c.mode.Load() == 1 {
		if err := json.NewEncoder(w).Encode(map[string]any{"received": 0, "errors_by_reason": map[string]int{"unsupported_schema": len(batch)}}); err != nil {
			c.t.Error(err)
		}
		return
	}
	requests, valid := c.validate(batch)
	if !valid || !monitoringPHPReceipt(c.t, requests) {
		w.WriteHeader(400)
		return
	}
	if !c.remember(requests) {
		w.WriteHeader(409)
		return
	}
	if c.mode.Load() == 2 {
		w.WriteHeader(503)
		return
	} // Durable accept, lost response.
	if err := json.NewEncoder(w).Encode(map[string]int{"received": len(batch)}); err != nil {
		c.t.Error(err)
	}
}

func (c *monitoringReceiptConsumer) validate(batch []entities.Envelope) ([]map[string]any, bool) {
	requests := []map[string]any{}
	for _, e := range batch {
		item, ok := c.lookup[e.ID]
		if !ok {
			c.t.Error("unknown envelope")
			return nil, false
		}
		raw, err := json.Marshal(e.Body)
		if err != nil {
			c.t.Error(err)
			return nil, false
		}
		b, err := monitoring.DecodeTransportRecords(raw, item.Definition, item.Authority, item.Now+86400000, item.Hops)
		if err != nil || !monitoringSemanticEqual(b, item.Batch) {
			c.t.Errorf("record changed %s: %v", e.ID, err)
			return nil, false
		}
		requests = append(requests, map[string]any{"envelope_id": e.ID, "raw": string(raw), "definition": item.Definition, "authority": item.Authority, "now_ms": item.Now + 86400000, "hops": item.Hops})
	}
	return requests, true
}

func (c *monitoringReceiptConsumer) remember(requests []map[string]any) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, request := range requests {
		raw := request["raw"].(string)
		id := request["envelope_id"].(string)
		if previous, exists := c.seen[id]; exists {
			if previous != raw {
				c.t.Error("replay mutated payload")
				return false
			}
		} else {
			c.seen[id] = raw
			c.delivered.Add(1)
		}
	}
	return true
}

func readMonitoringHTTPRequest(t *testing.T, r *http.Request) []entities.Envelope {
	t.Helper()
	if r.Header.Get("Content-Encoding") != "gzip" {
		t.Error("missing direct gzip")
	}
	gz, err := gzip.NewReader(r.Body)
	if err != nil {
		t.Error(err)
		return nil
	}
	defer gz.Close()
	raw, err := io.ReadAll(io.LimitReader(gz, 4097))
	if err != nil || len(raw) > 4096 {
		t.Error("outer request cap exceeded", err)
		return nil
	}
	batch, err := envelopejson.DecodeBatch(raw)
	if err != nil {
		t.Error(err)
	}
	return batch
}

func monitoringPHPReceipt(t *testing.T, requests []map[string]any) bool {
	t.Helper()
	raw, _ := json.Marshal(requests)
	cmd := exec.Command("php", filepath.Join(os.Getenv("AICEBERG_MONITORING_WEB_ROOT"), "common/tests/_support/monitoring-transport.php"))
	cmd.Stdin = bytes.NewReader(raw)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Error("PHP receipt", err)
		return false
	}
	var receipts []struct {
		Valid bool `json:"valid"`
	}
	if json.Unmarshal(output, &receipts) != nil || len(receipts) != len(requests) {
		t.Error("invalid receipt")
		return false
	}
	for _, r := range receipts {
		if !r.Valid {
			t.Error("PHP consumer rejected typed body")
			return false
		}
	}
	return true
}

func TestMonitoringRecordsHubGzipRemainsUnacknowledged(t *testing.T) {
	source, repo := openMonitoringStore(t, filepath.Join(t.TempDir(), "relay.db"), true)
	defer source.Close()
	seedMonitoringDelivery(t, repo, monitoringDeliveryCases(t)[:1])
	destination := &testHubOutbox{}
	server := httptest.NewServer(NewHandler(config.Config{}, destination, testHubLogger{}, nil))
	defer server.Close()
	tx := transport.NewHubClient(config.Config{HubURL: server.URL, HTTPGzip: true})
	executeMonitoringFlush(t, repo, tx, 0, true)
	if n, _ := repo.Len(); n != 1 || len(destination.batch) != 0 {
		t.Fatal("unsupported compression acknowledged")
	}
}

func monitoringSemanticEqual(left, right monitoring.RecordsBatch) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func TestMonitoringRecordsOpaqueHubPreservesRejectedJSON(t *testing.T) {
	c := monitoringDeliveryCases(t)[0]
	raw, err := monitoring.EncodeTransportRecords(c.Batch, c.Definition, c.Authority, c.Now, c.Hops)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte(`"schema_version":1`), []byte(`"schema_version":2,"schema_version":1`), 1)
	env := entities.Envelope{ID: "untrusted-duplicate", AgentID: "relay", Body: json.RawMessage(raw)}
	body, err := json.Marshal([]entities.Envelope{env})
	if err != nil {
		t.Fatal(err)
	}
	store := &testHubOutbox{}
	handler := NewHandler(config.Config{HubOutboxRawBodyEnabled: true}, store, testHubLogger{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/ingest/metrics", bytes.NewReader(body))
	req.Header.Set("Authorization", "Token fixture")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != 202 || len(store.batch) != 1 {
		t.Fatal("opaque transit failed")
	}
	preserved, err := json.Marshal(store.batch[0].Body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = monitoring.DecodeTransportRecords(preserved, c.Definition, c.Authority, c.Now, c.Hops); err == nil {
		t.Fatal("transit erased duplicate-key evidence")
	}
}
