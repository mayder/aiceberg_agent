package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestHubRawBodyPreservesPayloadAndOverwritesArrivalTime(t *testing.T) {
	store := &testHubOutbox{}
	h := NewHandler(config.Config{HubOutboxRawBodyEnabled: true}, store, testHubLogger{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/ingest/metrics", strings.NewReader(`[{"envelope_id":"one","agent_id":"relay","ts_unix_ms":42,"meta":{"hub_received_at_unix_ms":"forged","source":"fixture"},"body":{"value":9007199254740993}}]`))
	req.Header.Set("Authorization", "Token synthetic")
	req.Header.Set("X-Agent-Identity", "synthetic-identity")
	start := time.Now().UnixMilli()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	end := time.Now().UnixMilli()
	if w.Code != http.StatusAccepted || len(store.batch) != 1 {
		t.Fatal("HUB did not buffer", w.Code)
	}
	e := store.batch[0]
	body, ok := e.Body.(json.RawMessage)
	if !ok || string(body) != `{"value":9007199254740993}` {
		t.Fatal("raw body changed")
	}
	received, err := strconv.ParseInt(e.Meta["hub_received_at_unix_ms"], 10, 64)
	if err != nil || received < start || received > end || e.TSUnixMs != 42 || e.Meta["source"] != "fixture" {
		t.Fatal("invalid hop timestamp")
	}
	if e.AuthHeader != "Token synthetic" || e.IdentityHeader != "synthetic-identity" || e.Endpoint != "/v1/ingest/metrics" {
		t.Fatal("routing or identity changed")
	}
}

func TestHubRawBodiesDoNotAcknowledgePartialPersistence(t *testing.T) {
	store := &failingHubOutbox{failAt: 2}
	h := NewHandler(config.Config{HubOutboxRawBodyEnabled: true}, store, testHubLogger{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/ingest/metrics", strings.NewReader(`[{"envelope_id":"one","body":{}},{"envelope_id":"two","body":{}}]`))
	req.Header.Set("Authorization", "Token synthetic")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable || len(store.batch) != 1 {
		t.Fatal("partial persistence falsely acknowledged")
	}
}
