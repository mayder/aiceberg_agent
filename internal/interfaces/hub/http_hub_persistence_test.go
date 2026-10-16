package hub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/remote/transport"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

type failingHubOutbox struct {
	testHubOutbox
	failAt int
	calls  int
}

func (o *failingHubOutbox) Append(e entities.Envelope) error {
	o.calls++
	if o.calls == o.failAt {
		return errors.New("outbox full (max MB reached)")
	}
	return o.testHubOutbox.Append(e)
}

func TestHubDoesNotAcknowledgeFailedOrPartialPersistence(t *testing.T) {
	for _, route := range []string{"/v1/ingest/metrics", "/v1/logs/raw"} {
		for _, failAt := range []int{1, 2} {
			o := &failingHubOutbox{failAt: failAt}
			h := NewHandler(config.Config{}, o, testHubLogger{}, nil)
			r := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`[{"envelope_id":"one","body":{}},{"envelope_id":"two","body":{}}]`))
			r.Header.Set("Authorization", "Token test")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "15" {
				t.Fatalf("route=%s failure=%d: falsely acknowledged persistence: %d", route, failAt, w.Code)
			}
			if len(o.batch) != 1 {
				t.Fatalf("partial persisted evidence must remain: %d", len(o.batch))
			}
		}
	}
}

type relayEvidenceOutbox struct {
	testHubOutbox
	acked int
}

func (o *relayEvidenceOutbox) Ack(ids []string) error { o.acked += len(ids); return nil }

func TestRealRelayTransportRetainsEnvelopeWhenHubCannotPersist(t *testing.T) {
	destination := &failingHubOutbox{failAt: 1}
	server := httptest.NewServer(NewHandler(config.Config{}, destination, testHubLogger{}, nil))
	defer server.Close()
	source := &relayEvidenceOutbox{testHubOutbox: testHubOutbox{batch: []entities.Envelope{{ID: "relay-envelope", AgentID: "relay", SchemaVersion: 1, Kind: "metric", TSUnixMs: 1, Endpoint: "/v1/ingest/metrics", Body: json.RawMessage(`{"cpu":42}`)}}}}
	client := transport.NewHubClient(config.Config{HubURL: server.URL})
	flush := usecase.NewFlushOutbox(source, client, testHubLogger{}, "Token fixture", nil)
	count, err := flush.Execute(context.Background())
	if err == nil || count != 0 || source.acked != 0 {
		t.Fatalf("relay discarded unpersisted data: count=%d ack=%d err=%v", count, source.acked, err)
	}
	if len(source.batch) != 1 || len(destination.batch) != 0 {
		t.Fatal("failed delivery must remain at relay")
	}
}
