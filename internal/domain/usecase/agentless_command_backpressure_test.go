package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/common/config"
	local "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestCancelledSNMPDiagnosticSurvivesOutboxReopen(t *testing.T) {
	device, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	port := device.LocalAddr().(*net.UDPAddr).Port
	var deliveries atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/hub-agentless/jobs":
			_, _ = fmt.Fprintf(w, `{"status":"ok","jobs":[{"check_id":72,"ativo_id":52,"cliente_id":3,
				"tipo":"snmp","command_id":"diagnostic-fixture","correlation_id":"trace-fixture",
				"endpoint":{"id":17,"tipo":"ip","endereco":"127.0.0.1"},
				"snmp":{"profile_id":10,"version":"v1","community":"fixture-read","port":%d},
				"config":{"snmp_basic_probe":true,"snmp_basic_oid":"1.3.6.1.2.1.1.3.0"}}]}`, port)
		case "/v1/hub-agentless/observations":
			deliveries.Add(1)
			_, _ = w.Write([]byte(`{"status":"ok","saved":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()
	cfg := config.Config{APIBaseURL: api.URL}
	path := filepath.Join(t.TempDir(), "diagnostic-outbox.db")
	store := openLocalHubLoadQueue(t, path)
	hub := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(store), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- hub.CollectCommand(ctx, AgentlessCommandRequest{CommandID: "diagnostic-fixture", CorrelationID: "trace-fixture", CheckIDs: []int{72}})
	}()
	if err := device.SetReadDeadline(time.Now().Add(4 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := device.ReadFrom(make([]byte, 2048)); err != nil {
		t.Fatalf("probe did not contact the UDP fixture: %v", err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation after the first datagram: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	recovered := openLocalHubLoadQueue(t, path)
	pending, err := recovered.Peek(2)
	if err != nil || len(pending) != 1 {
		t.Fatalf("expected one diagnostic receipt in the reopened outbox: count=%d error=%v", len(pending), err)
	}
	receipt := pending[0]
	if receipt.ProbeReport == nil || receipt.ProbeReport.Outcome != "cancelled" || receipt.Status != "fail" ||
		receipt.CommandID != "diagnostic-fixture" || receipt.CorrelationID != "trace-fixture" || !receipt.IsFinal ||
		receipt.IsPartial || receipt.Payload["diagnostic_probe"] != true || receipt.MonitoringSamples != nil {
		t.Fatal("diagnostic cancellation lost its scope or produced operational samples")
	}
	reopenedHub := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(recovered), nil, nil)
	if err := reopenedHub.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if size, _ := recovered.Len(); size != 0 || deliveries.Load() != 1 {
		t.Fatalf("cancellation receipt was not acknowledged exactly once: pending=%d deliveries=%d", size, deliveries.Load())
	}
}

func TestAgentlessByteBackpressureStopsLeasesAndRecoversAfterFlush(t *testing.T) {
	var pulls, probes, deliveries atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/hub-agentless/jobs":
			pulls.Add(1)
			_, _ = fmt.Fprintf(w, `{"status":"ok","jobs":[{"check_id":72,"ativo_id":52,"cliente_id":3,
				"tipo":"http","command_id":"byte-fixture","endpoint":{"tipo":"url","endereco":"http://%s/probe"}}]}`, r.Host)
		case "/v1/hub-agentless/observations":
			deliveries.Add(1)
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Error(err)
			}
			_, _ = w.Write([]byte(`{"status":"ok","saved":1}`))
		case "/probe":
			probes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()

	store, err := local.NewBoltStore(filepath.Join(t.TempDir(), "outbox.db"), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fillAgentlessByteLimit(t, store)
	cfg := config.Config{APIBaseURL: api.URL}
	hub := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(store), nil, nil)
	hub.setTargets([]entities.AgentlessJob{{CheckID: 72, Tipo: "http",
		Endpoint: &entities.AgentlessEndpoint{Tipo: "url", Endereco: api.URL + "/probe"}}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := AgentlessCommandRequest{CommandID: "byte-fixture", CheckIDs: []int{72}}
	if err := hub.PollAndRun(ctx); err != nil {
		t.Fatal(err)
	}
	if err := hub.SyncTargets(ctx); err != nil {
		t.Fatal(err)
	}
	if err := hub.CollectCommand(ctx, req); !errors.Is(err, ErrAgentlessOutboxBusy) {
		t.Fatalf("outbox with one byte free did not report busy: %v", err)
	}
	if pulls.Load() != 0 || probes.Load() != 0 {
		t.Fatal("a byte-saturated outbox allowed requests or probes")
	}
	if err := hub.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := hub.CollectCommand(ctx, req); err != nil {
		t.Fatal(err)
	}
	if pulls.Load() != 1 || probes.Load() != 1 || deliveries.Load() != 2 {
		t.Fatal("collection did not resume after acknowledging the pending observation")
	}
}

func fillAgentlessByteLimit(t *testing.T, store *local.BoltStore) {
	t.Helper()
	entry := entities.AgentlessObservation{ID: "byte-filler", CheckID: 11, Status: "ok", Payload: map[string]any{"fixture": ""}}
	if err := store.Push(entry); err != nil {
		t.Fatal(err)
	}
	_, overhead := store.Len()
	entry.Payload["fixture"] = strings.Repeat("a", (1<<20)-int(overhead)-1)
	if err := store.Push(entry); err != nil {
		t.Fatal(err)
	}
	count, used := store.Len()
	if count != 1 || used != (1<<20)-1 {
		t.Fatalf("the fixture did not fill the byte limit: count=%d bytes=%d", count, used)
	}
	if err := store.Push(entities.AgentlessObservation{ID: "extra"}); !errors.Is(err, local.ErrOutboxFull) {
		t.Fatalf("Bolt accepted an observation beyond its byte limit: %v", err)
	}
}

func TestManualSNMPBackpressureReservesEveryObservation(t *testing.T) {
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.LocalAddr().(*net.UDPAddr).Port
	var pulls, parts, finals atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/hub-agentless/jobs":
			pulls.Add(1)
			_, _ = fmt.Fprintf(w, `{"status":"ok","jobs":[{"check_id":72,"ativo_id":52,"cliente_id":3,
				"tipo":"snmp","command_id":"manual-fixture","endpoint":{"tipo":"ip","endereco":"127.0.0.1"},
				"snmp":{"profile_id":10,"version":"v2c","community":"fixture-read","port":%d},
				"config":{"groups":["system"],"fetch_mode":"get_only","flush_partial":true,
				"custom_get_oids":["1.3.6.1.2.1.1.3.0"]}}]}`, port)
		case "/v1/hub-agentless/observations":
			var batch struct {
				Observations []struct {
					CommandID string `json:"command_id"`
					Status    string `json:"status"`
					Code      string `json:"code"`
					IsFinal   bool   `json:"is_final"`
					IsPartial bool   `json:"is_partial"`
				} `json:"observations"`
			}
			if err := json.NewDecoder(r.Body).Decode(&batch); err != nil || len(batch.Observations) != 1 {
				t.Error("unexpected delivery batch")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			obs := batch.Observations[0]
			if obs.CommandID == "manual-fixture" {
				if obs.Status != "ok" {
					t.Errorf("unexpected SNMP result: %s", obs.Code)
				}
				if obs.IsFinal {
					finals.Add(1)
				} else if obs.IsPartial {
					parts.Add(1)
				}
			}
			_, _ = w.Write([]byte(`{"status":"ok","saved":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()

	store := filledAgentlessCommandStore(t)
	cfg := config.Config{APIBaseURL: api.URL, AgentlessFlushBatch: 1}
	hub := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(store), nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	request := AgentlessCommandRequest{CommandID: "manual-fixture", CheckIDs: []int{72}}

	if err := hub.CollectCommand(ctx, request); !errors.Is(err, ErrAgentlessOutboxBusy) || pulls.Load() != 0 {
		t.Fatalf("a full outbox allowed a manual lease: %v", err)
	}
	if err := store.Delete([]string{"pending-0"}); err != nil {
		t.Fatal(err)
	}
	if err := hub.CollectCommand(ctx, request); !errors.Is(err, ErrAgentlessOutboxBusy) || pulls.Load() != 1 {
		t.Fatalf("one slot admitted a three-observation check: %v", err)
	}
	if n, _ := store.Len(); n != agentlessFlushBatches-1 {
		t.Fatalf("busy command changed the outbox: count=%d", n)
	}
	if err := store.Delete([]string{"pending-1", "pending-2"}); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- serveTwoSNMPGets(listener, time.Now().Add(4*time.Second)) }()
	if err := hub.CollectCommand(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n, _ := store.Len(); n != 0 || pulls.Load() != 2 || parts.Load() != 2 || finals.Load() != 1 {
		t.Fatalf("recovery did not deliver every segment: pending=%d leases=%d parts=%d finals=%d",
			n, pulls.Load(), parts.Load(), finals.Load())
	}
}

func filledAgentlessCommandStore(t *testing.T) *local.BoltStore {
	t.Helper()
	store, err := local.NewBoltStore(filepath.Join(t.TempDir(), "outbox.db"), 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for i := range agentlessFlushBatches {
		obs := entities.AgentlessObservation{ID: fmt.Sprintf("pending-%d", i), CheckID: 11, Status: "ok"}
		if err := store.Push(obs); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func serveTwoSNMPGets(listener net.PacketConn, deadline time.Time) error {
	if err := listener.SetReadDeadline(deadline); err != nil {
		return err
	}
	decoder := &gosnmp.GoSNMP{}
	buf := make([]byte, 2048)
	for range 2 {
		n, peer, err := listener.ReadFrom(buf)
		if err != nil {
			return err
		}
		packet, err := decoder.SnmpDecodePacket(buf[:n])
		if err != nil {
			return err
		}
		packet.PDUType = gosnmp.GetResponse
		for i := range packet.Variables {
			packet.Variables[i].Type = gosnmp.TimeTicks
			packet.Variables[i].Value = uint32(11)
		}
		raw, err := packet.MarshalMsg()
		if err != nil {
			return err
		}
		if _, err := listener.WriteTo(raw, peer); err != nil {
			return err
		}
	}
	return nil
}
