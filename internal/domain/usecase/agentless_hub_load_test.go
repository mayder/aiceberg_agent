package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	local "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestAgentlessHubMixedLoadSurvivesDeliveryOutageAndRestart(t *testing.T) {
	device, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	probes := &localHubLoadProbes{firstWave: make(chan struct{}), activeByClient: map[int]int{}, calls: map[int]int{}}
	receipts := &localHubLoadReceipts{confirmed: map[localHubObservationKey]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/probe/", probes.serve)
	mux.HandleFunc("/v1/hub-agentless/observations", receipts.serve)
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.Config{APIBaseURL: server.URL, AgentlessFlushBatch: 8, AgentlessDeliveryPriority: true}
	path := filepath.Join(t.TempDir(), "hub-load.db")
	store := openLocalHubLoadQueue(t, path)
	hub := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(store), nil, nil)
	hub.setTargets(localHubLoadJobs(server.URL, device.LocalAddr().(*net.UDPAddr).Port))
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	deviceDone := make(chan error, 1)
	deadline, _ := ctx.Deadline()
	go func() { deviceDone <- serveTwoSNMPGets(device, deadline) }()

	started := time.Now()
	if err := hub.PollAndRun(ctx); err != nil {
		t.Fatal(err)
	}
	collectionTime := time.Since(started)
	if err := <-deviceDone; err != nil {
		t.Fatal(err)
	}
	if n, used := store.Len(); n != 64 || used <= 0 {
		t.Fatalf("unexpected durable queue after collection: records=%d bytes=%d", n, used)
	}
	probes.assertCollection(t)

	var delivery *remote.AgentlessDeliveryError
	if err := hub.Flush(ctx); !errors.As(err, &delivery) || delivery.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("expected temporary receiver outage: %v", err)
	}
	if n, _ := store.Len(); n != 56 {
		t.Fatalf("the outage did not preserve the 56 unconfirmed observations: %d", n)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	recovered := openLocalHubLoadQueue(t, path)
	restartedHub := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(recovered), nil, nil)
	started = time.Now()
	for range 2 {
		if n, _ := recovered.Len(); n == 0 {
			break
		}
		if err := restartedHub.Flush(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := recovered.Len(); n != 0 {
		t.Fatalf("the recovered receiver left %d observations pending", n)
	}
	receipts.assertDelivery(t)
	probes.assertCollection(t)
	drainTime := time.Since(started)
	if err := device.SetReadDeadline(time.Now().Add(30 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var timeout net.Error
	if _, _, err := device.ReadFrom(make([]byte, 2048)); !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("unexpected UDP traffic after delivery recovery: %v", err)
	}
	t.Logf("62 jobs, 64 receipts, peak HTTP concurrency 8, collect=%s, drain-after-reopen=%s", collectionTime, drainTime)
}

func openLocalHubLoadQueue(t *testing.T, path string) *local.BoltStore {
	t.Helper()
	store, err := local.NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func localHubLoadJobs(baseURL string, snmpPort int) []entities.AgentlessJob {
	jobs := make([]entities.AgentlessJob, 0, 62)
	for client := 1; client <= 9; client++ {
		count := 5
		if client == 1 {
			count = 21
		}
		for range count {
			id := len(jobs) + 1
			jobs = append(jobs, entities.AgentlessJob{ClienteID: client, AtivoID: id, CheckID: id, Tipo: "http",
				Endpoint: &entities.AgentlessEndpoint{ID: id, Tipo: "url", Endereco: fmt.Sprintf("%s/probe/%d/%d", baseURL, client, id)}})
		}
	}
	return append(jobs, entities.AgentlessJob{
		ClienteID: 1, AtivoID: 62, CheckID: 62, Tipo: "snmp", TimeoutMs: 600,
		Endpoint: &entities.AgentlessEndpoint{ID: 62, Tipo: "ip", Endereco: "127.0.0.1"},
		SNMP:     &entities.AgentlessSnmpProfile{ProfileID: 10, Port: snmpPort, Version: "v2c", Community: "fixture-read"},
		Config: entities.AgentlessConfig{"groups": []string{"system"}, "fetch_mode": "get_only", "flush_partial": true,
			"custom_get_oids": []string{"1.3.6.1.2.1.1.3.0"}},
	})
}

type localHubLoadProbes struct {
	mu             sync.Mutex
	firstWave      chan struct{}
	closed         bool
	active         int
	peak           int
	clientOverlap  bool
	activeByClient map[int]int
	calls          map[int]int
}

func (p *localHubLoadProbes) serve(w http.ResponseWriter, r *http.Request) {
	var client, check int
	if n, _ := fmt.Sscanf(r.URL.Path, "/probe/%d/%d", &client, &check); n != 2 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	p.mu.Lock()
	p.active++
	p.peak = max(p.peak, p.active)
	p.calls[check]++
	p.activeByClient[client]++
	p.clientOverlap = p.clientOverlap || p.activeByClient[client] > 1
	if p.active == agentlessBatchWorkers && !p.closed {
		close(p.firstWave)
		p.closed = true
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.active--
		p.activeByClient[client]--
		p.mu.Unlock()
	}()
	select {
	case <-p.firstWave:
		w.WriteHeader(http.StatusNoContent)
	case <-r.Context().Done():
	}
}

func (p *localHubLoadProbes) assertCollection(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.peak != agentlessBatchWorkers || p.clientOverlap || len(p.calls) != 61 {
		t.Fatalf("unexpected HTTP collection: peak=%d client-overlap=%t distinct=%d", p.peak, p.clientOverlap, len(p.calls))
	}
	for id, calls := range p.calls {
		if calls != 1 {
			t.Fatalf("HTTP check %d executed %d times", id, calls)
		}
	}
}

type localHubObservationKey struct {
	check   int
	segment int
}

type localHubLoadReceipts struct {
	mu        sync.Mutex
	requests  int
	confirmed map[localHubObservationKey]int
	invalid   bool
	snmpSeq   int
}

func (r *localHubLoadReceipts) serve(w http.ResponseWriter, req *http.Request) {
	var batch struct {
		Observations []struct {
			CheckID int    `json:"check_id"`
			Segment int    `json:"segment_seq"`
			Status  string `json:"status"`
		} `json:"observations"`
	}
	if err := json.NewDecoder(req.Body).Decode(&batch); err != nil || len(batch.Observations) == 0 || len(batch.Observations) > 8 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests++
	if r.requests == 2 {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	for _, item := range batch.Observations {
		r.invalid = r.invalid || item.Status != "ok"
		if item.CheckID == 62 {
			r.snmpSeq++
			r.invalid = r.invalid || item.Segment != r.snmpSeq
		}
		r.confirmed[localHubObservationKey{check: item.CheckID, segment: item.Segment}]++
	}
	_, _ = fmt.Fprintf(w, `{"status":"ok","saved":%d}`, len(batch.Observations))
}

func (r *localHubLoadReceipts) assertDelivery(t *testing.T) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.requests < 9 || r.requests > 11 || len(r.confirmed) != 64 || r.invalid {
		t.Fatalf("unexpected load delivery: attempts=%d distinct=%d invalid=%t", r.requests, len(r.confirmed), r.invalid)
	}
	for id := 1; id <= 61; id++ {
		if r.confirmed[localHubObservationKey{check: id}] != 1 {
			t.Fatalf("HTTP check %d did not deliver exactly once", id)
		}
	}
	for segment := 1; segment <= 3; segment++ {
		if r.confirmed[localHubObservationKey{check: 62, segment: segment}] != 1 {
			t.Fatalf("SNMP segment %d did not deliver exactly once", segment)
		}
	}
}
