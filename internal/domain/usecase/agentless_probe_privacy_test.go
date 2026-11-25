package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	local "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestAgentlessHTTPKeepsRawEndpointAndStatusOutOfDebugAndOutbox(t *testing.T) {
	const private = "fixture-private-marker"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, buf, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		if _, err := fmt.Fprintf(buf, "HTTP/1.1 503 %s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", private); err != nil {
			t.Error(err)
		}
		if err := buf.Flush(); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	job := batchJob(42, strings.Replace(server.URL, "://", "://fixture:"+private+"@", 1)+"/"+private+"?token="+private)
	job.ClienteID = 7
	job.Tipo = "http"
	job.Endpoint.Tipo = "url"
	job.Endpoint.ID = 51
	job.Endpoint.TLSSNI = private

	path := filepath.Join(t.TempDir(), "outbox.db")
	store, err := local.NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	log := &fakeLogger{}
	hub := NewAgentlessHub(config.Config{AgentlessDebug: true}, log, nil, repositories.NewAgentlessOutboxRepository(store), nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var appendMu sync.Mutex
	if _, err := hub.runJob(ctx, job, &appendMu); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if _, err := hub.runJob(ctx, job, &appendMu); err != nil {
		t.Fatal(err)
	}
	assertPrivateHTTPObservations(t, store, private)
	logs := strings.Join(log.info, "\n")
	if strings.Contains(logs, private) || !strings.Contains(logs, "endpoint_id=51") || !strings.Contains(logs, "code=http_fail") {
		t.Fatal("debug leaked raw input or lost diagnostic fields")
	}
}

func assertPrivateHTTPObservations(t *testing.T, store *local.BoltStore, private string) {
	t.Helper()
	batch, err := store.Peek(2)
	if err != nil || len(batch) != 2 {
		t.Fatalf("expected two durable observations: count=%d err=%v", len(batch), err)
	}
	found := map[string]entities.AgentlessObservation{}
	for _, observation := range batch {
		wire, err := json.Marshal(observation)
		if err != nil || strings.Contains(string(wire), private) {
			t.Fatal("outbox persisted credentials or a remote status line")
		}
		found[observation.Code] = observation
	}
	status := found["http_status"]
	if status.Message != "503 Service Unavailable" || status.Payload["status_code"] != float64(503) {
		t.Fatal("HTTP status lost its safe structured representation")
	}
	connection := found["http_fail"].ProbeReport
	if connection == nil || connection.Reason != "connection" {
		t.Fatal("connection error lost its canonical classification")
	}
}
