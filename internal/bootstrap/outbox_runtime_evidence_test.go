package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

type evidenceOutbox struct{}

func (evidenceOutbox) Append(entities.Envelope) error             { return nil }
func (evidenceOutbox) ReadBatch(int) ([]entities.Envelope, error) { return nil, nil }
func (evidenceOutbox) Ack([]string) error                         { return nil }
func (evidenceOutbox) Len() (int, int64)                          { return 93, 208000000 }

func TestOutboxRuntimeEvidenceIsReadOnlyAndDoesNotExposeTransportPayload(t *testing.T) {
	repo := evidenceOutbox{}
	flush := usecase.NewFlushOutbox(repo, nil, nil, "Token private", nil)
	evidence := buildOutboxRuntimeEvidence(repo, flush)
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if evidence["queue_items"] != 93 || evidence["queue_bytes"] != int64(208000000) {
		t.Fatalf("missing queue counters: %s", raw)
	}
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "last_error\"") {
		t.Fatalf("raw error or credential exposed: %s", raw)
	}
	for message, want := range map[string]string{"": "none", "ingest response did not acknowledge batch": "ingest_ack_rejected", "HTTP status 401 with password=secret": "http_401", "request timeout with token=secret": "timeout", "private arbitrary body": "other"} {
		if got := outboxErrorKind(message); got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	}
}
