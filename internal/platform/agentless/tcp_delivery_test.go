package agentless

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	local "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
)

func TestCanonicalTCPOutboxHTTPToPHP(t *testing.T) {
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		t.Skip("explicit Web checkout required")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	done := make(chan struct{})
	acceptedConnection := make(chan struct{}, 1)
	go func() {
		defer close(done)
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			calls.Add(1)
			select {
			case acceptedConnection <- struct{}{}:
			default:
			}
			c.Close()
		}
	}()
	observation := RunJob(context.Background(), canonicalTCPJob(listener.Addr().(*net.TCPAddr).Port))
	select {
	case <-acceptedConnection:
	case <-time.After(2 * time.Second):
		listener.Close()
		<-done
		t.Fatal("TCP fixture did not accept connection")
	}
	listener.Close()
	<-done
	if observation.Status != "ok" || observation.MonitoringSamples == nil || observation.ProbeReport == nil || calls.Load() != 1 {
		t.Fatal("real TCP collection failed")
	}
	// Proposed payload metadata and latency never supply the persisted sample.
	observation.Payload = map[string]any{"latency_ms": 999, "monitoring_samples_v1": map[string]any{"scope": map[string]any{"client_id": 999}, "samples": []any{map[string]any{"value": 999}}}}
	originalPayload, _ := json.Marshal(observation.Payload)
	path := filepath.Join(t.TempDir(), "tcp-outbox.db")
	store, err := local.NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Push(observation); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = local.NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	batch, err := store.Peek(1)
	if err != nil || len(batch) != 1 || !reflect.DeepEqual(batch[0].MonitoringSamples, observation.MonitoringSamples) ||
		!reflect.DeepEqual(batch[0].ProbeReport, observation.ProbeReport) {
		t.Fatal("durable metadata changed", err)
	}
	persisted, _ := json.Marshal(batch[0].Payload)
	if string(originalPayload) != string(persisted) || batch[0].ID != observation.ID || batch[0].DedupeKey != observation.DedupeKey {
		t.Fatal("legacy payload or ID changed")
	}
	var accepted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token fixture-canonical-auth" || r.URL.Path != "/v1/hub-agentless/observations" {
			w.WriteHeader(403)
			return
		}
		if !accepted.Load() {
			w.WriteHeader(503)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "php", filepath.Join(root, "common/tests/_support/canonical-tcp-delivery.php"))
		cmd.Env = os.Environ()
		cmd.Stdin = strings.NewReader(string(raw))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("PHP TCP persistence: %v %s", err, output)
			w.WriteHeader(500)
			return
		}
		var receipt map[string]any
		if json.Unmarshal(output, &receipt) != nil || receipt["samples_equal"] != true || receipt["canonical_scope_verified"] != true || receipt["probe_report_verified"] != true {
			t.Error("invalid TCP receipt")
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(output)
	}))
	defer server.Close()
	client := remote.NewAgentlessHubClient(config.Config{APIBaseURL: server.URL, Agent: config.AgentCfg{Token: "fixture-canonical-auth"}, AgentClientID: 3, AgentID: 42})
	if err = client.SendObservations(context.Background(), batch); err == nil {
		t.Fatal("503 acknowledged")
	}
	pending, _ := store.Peek(1)
	if len(pending) != 1 {
		t.Fatal("failed delivery lost observation")
	}
	accepted.Store(true)
	if err = client.SendObservations(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	if err = store.Delete([]string{batch[0].ID}); err != nil {
		t.Fatal(err)
	} // This owned synthetic outbox only.
	pending, _ = store.Peek(1)
	if len(pending) != 0 || calls.Load() != 1 {
		t.Fatal("transport recollected or failed to acknowledge")
	}
}
