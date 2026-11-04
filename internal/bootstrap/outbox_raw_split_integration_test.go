package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/outbox"
	"github.com/you/aiceberg_agent/internal/data/remote/transport"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func TestRawBoltBodiesKeepAtomicLogSplitAndAuthenticatedHTTP(t *testing.T) {
	var sequences []int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if len(data) > 400 || r.Header.Get("Authorization") != "Token synthetic" || r.Header.Get("X-Agent-Identity") != "synthetic-identity" || r.URL.Path != "/v1/logs/raw" {
			t.Error("request contract changed")
		}
		var frames []entities.Envelope
		if err := json.Unmarshal(data, &frames); err != nil {
			t.Error(err)
		}
		for _, e := range frames {
			raw, _ := json.Marshal(e.Body)
			var body struct {
				Events []struct {
					Sequence int `json:"sequence"`
				} `json:"events"`
				Dropped int `json:"dropped_count"`
			}
			if err := json.Unmarshal(raw, &body); err != nil || body.Dropped != 7 {
				t.Error("log metadata changed")
			}
			for _, event := range body.Events {
				sequences = append(sequences, event.Sequence)
			}
		}
		_, _ = w.Write([]byte(`{"received":1,"skipped":0}`))
	}))
	defer api.Close()
	s, err := outbox.NewBoltStoreWithOptions(filepath.Join(t.TempDir(), "synthetic.db"), 2, outbox.BoltOptions{RawBodies: true, BatchWrites: true, RebuildFreelist: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	repo := repositories.NewOutboxRepository(s)
	events := []map[string]any{{"message": strings.Repeat("a", 140), "sequence": 1}, {"message": strings.Repeat("b", 140), "sequence": 2}, {"message": strings.Repeat("c", 140), "sequence": 3}}
	if err := repo.Append(entities.Envelope{ID: "original", AgentID: "synthetic", Endpoint: "/v1/logs/raw", AuthHeader: "Token synthetic", IdentityHeader: "synthetic-identity", Body: map[string]any{"events": events, "dropped_count": 7}}); err != nil {
		t.Fatal(err)
	}
	flush := usecase.NewFlushOutboxWithOptions(repo, transport.NewHTTPJSONClient(config.Config{APIBaseURL: api.URL}), testBootstrapLogger{}, "", nil, usecase.FlushOutboxOptions{BatchSize: 10, MaxBatchBytes: 400})
	acked, err := flush.Execute(context.Background())
	if err != nil || acked != 3 {
		t.Fatalf("raw split failed: ack=%d err=%v", acked, err)
	}
	// Bolt retains child envelopes by deterministic ID, not logical event order.
	sort.Ints(sequences)
	if len(sequences) != 3 || sequences[0] != 1 || sequences[1] != 2 || sequences[2] != 3 {
		t.Fatal("log order changed")
	}
	if n, _ := repo.Len(); n != 0 {
		t.Fatal("acknowledged parts remained pending")
	}
}
