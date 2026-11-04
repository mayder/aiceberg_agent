package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type countedMetricBody struct {
	calls *int
	fail  bool
}

func (b countedMetricBody) MarshalJSON() ([]byte, error) {
	*b.calls++
	if b.fail {
		return nil, errors.New("synthetic invalid body")
	}
	return []byte(`{"value":42}`), nil
}

func TestMetricsBypassLogNormalizationButKeepRequestValidation(t *testing.T) {
	calls := 0
	env := entities.Envelope{ID: "metric", Endpoint: "/v1/ingest/metrics", Body: countedMetricBody{&calls, true}}
	repo := &fakeOutbox{batch: []entities.Envelope{env}}
	tx := &fakeTransport{body: []byte(`{"received":1,"status":"ok"}`)}
	uc := NewFlushOutbox(repo, tx, &fakeLogger{}, "Token synthetic", nil)
	if n, err := uc.normalizeOversizedLogEnvelopes(repo.batch); err != nil || n != 0 || calls != 0 {
		t.Fatal("metrics serialized in log-only pass")
	}
	if n, err := uc.Execute(context.Background()); err == nil || n != 0 || calls == 0 || len(tx.calls) != 0 {
		t.Fatal("invalid request bypassed send validation")
	}
}

func TestRawLogsStillValidateDuringNormalization(t *testing.T) {
	calls := 0
	env := entities.Envelope{ID: "log", Endpoint: rawLogsEndpoint, Body: countedMetricBody{&calls, true}}
	uc := NewFlushOutbox(&fakeOutbox{}, &fakeTransport{}, &fakeLogger{}, "Token synthetic", nil)
	if _, err := uc.normalizeOversizedLogEnvelopes([]entities.Envelope{env}); err == nil || calls != 1 {
		t.Fatal("raw-log validation bypassed")
	}
}

func BenchmarkMetricLogNormalization(b *testing.B) {
	batch := make([]entities.Envelope, 64)
	for i := range batch {
		batch[i] = entities.Envelope{ID: fmt.Sprint(i), Endpoint: "/v1/ingest/metrics", Body: map[string]any{"value": strings.Repeat("x", 16384)}}
	}
	uc := NewFlushOutbox(&fakeOutbox{}, &fakeTransport{}, &fakeLogger{}, "Token synthetic", nil)
	for _, legacy := range []bool{true, false} {
		b.Run(fmt.Sprintf("legacy=%v", legacy), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if legacy {
					for _, env := range batch {
						if _, err := json.Marshal(env); err != nil {
							b.Fatal(err)
						}
					}
				} else if _, err := uc.normalizeOversizedLogEnvelopes(batch); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
