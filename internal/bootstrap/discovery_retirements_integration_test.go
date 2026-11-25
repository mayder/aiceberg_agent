package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/outbox"
	"github.com/you/aiceberg_agent/internal/data/remote/transport"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func TestDiscoveryRetirementsSurviveRestartAndLostAcknowledgment(t *testing.T) {
	const endpoint = "/v1/ingest/metrics"
	path := filepath.Join(t.TempDir(), "discovery-outbox.db")
	want := bufferDiscoveryFixture(t, path, endpoint)
	expected := make(map[string]entities.Envelope, len(want))
	for _, envelope := range want {
		expected[envelope.ID] = envelope
	}
	var lock sync.Mutex
	calls := 0
	received := make(map[string]bool, len(want))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lock.Lock()
		defer lock.Unlock()
		calls++
		var batch []entities.Envelope
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path != endpoint || r.Header.Get("Authorization") != "Token fixture-rotated" ||
			r.Header.Get("X-Agent-Identity") != "fixture-current-identity" {
			t.Error("replayed delivery lost the ingest route or current credentials")
		}
		assertDiscoveryDelivery(t, expected, batch)
		duplicates := 0
		for _, envelope := range batch {
			if received[envelope.ID] {
				duplicates++
			}
			received[envelope.ID] = true
		}
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, `{"received":%d,"skipped":%d,"ack_safe":true,"errors_by_reason":{"duplicate_envelope_id":%d}}`,
			len(batch)-duplicates, duplicates, duplicates)
	}))
	defer server.Close()

	flushDiscoveryAfterRestart(t, path, server.URL, len(want), false)
	flushDiscoveryAfterRestart(t, path, server.URL, len(want), true)
	lock.Lock()
	defer lock.Unlock()
	if calls != 2 || len(received) != len(want) {
		t.Fatalf("HTTP attempts=%d unique envelopes=%d, want 2 and %d", calls, len(received), len(want))
	}
}

func bufferDiscoveryFixture(t *testing.T, path, endpoint string) []entities.Envelope {
	t.Helper()
	store := openDiscoveryFixtureOutbox(t, path)
	defer store.Close()
	repo := repositories.NewOutboxRepository(store)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sources := []struct {
		source   string
		name     string
		env      string
		identity []string
	}{
		{"interfaces", "interface_discovery", "DISCOVERY_INTERFACE", []string{"02:00:00:00:00:03"}},
		{"disks", "disk_discovery", "DISCOVERY_DISK", []string{"loop240"}},
		{"sensors", "sensor_discovery", "DISCOVERY_SENSOR", []string{"coretemp_core_0"}},
		{"services", "service_discovery", "DISCOVERY_SERVICE", []string{"FixtureAgent"}},
		{"applications", "application_discovery", "DISCOVERY_APPLICATION", []string{"fixture-package", "", "dpkg"}},
		{"applications", "application_discovery", "DISCOVERY_APPLICATION", []string{"fixture-rpm", "aarch64", "rpm"}},
		{"applications", "application_discovery", "DISCOVERY_APPLICATION", []string{"Fixture App", "", "registry"}},
		{"log_sources", "log_source_retirement", "DISCOVERY_LOG_SOURCE", []string{strings.Repeat("d", 64)}},
	}
	completed := make(chan error, len(sources))
	want := make(map[monitoring.DiscoveryEvent]bool, len(sources))
	for _, source := range sources {
		t.Setenv(source.env+"_CLIENT_ID", "7")
		t.Setenv(source.env+"_ASSET_ID", "17")
		event, err := monitoring.NewDiscoveryEvent(7, 17, source.source, source.identity, "tombstone", 100, 300, time.Now().Add(-time.Second))
		if err != nil {
			t.Fatal(err)
		}
		want[event] = true
		startDiscoveryRetirement(ctx, endpoint, "Token fixture-old", func() string { return "fixture-old-identity" },
			repo, testBootstrapLogger{}, source.name, source.env,
			func(ctx context.Context, client, asset int64, publish func(context.Context, []monitoring.DiscoveryEvent) error) error {
				if client != event.ClientID || asset != event.AssetID {
					err := fmt.Errorf("wrong retirement scope: client=%d asset=%d", client, asset)
					completed <- err
					return err
				}
				err := publish(ctx, []monitoring.DiscoveryEvent{event})
				completed <- err
				return err
			})
	}
	for range sources {
		select {
		case err := <-completed:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("retirement was not persisted before restart")
		}
	}
	batch, err := repo.ReadBatch(len(sources) + 1)
	if err != nil || len(batch) != len(sources) {
		t.Fatalf("outbox contains %d events, want %d: %v", len(batch), len(sources), err)
	}
	for _, envelope := range batch {
		event := bufferedDiscoveryEvent(t, envelope)
		body := canonicalDiscoveryBody(envelope.Body)
		if !want[event] || envelope.TSUnixMs < event.ObservedMS ||
			body["collector_name"] != envelope.Sub || body["ingest_endpoint"] != endpoint {
			t.Fatalf("incorrect retirement wire event: %+v", event)
		}
		delete(want, event)
	}
	return batch
}

func openDiscoveryFixtureOutbox(t *testing.T, path string) *outbox.BoltStore {
	t.Helper()
	store, err := outbox.NewBoltStoreWithOptions(path, 4, outbox.BoltOptions{RawBodies: true, BatchWrites: true, RebuildFreelist: true})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func bufferedDiscoveryEvent(t *testing.T, envelope entities.Envelope) monitoring.DiscoveryEvent {
	t.Helper()
	raw, err := json.Marshal(envelope.Body)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Events []monitoring.DiscoveryEvent `json:"discovery_events"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || len(payload.Events) != 1 {
		t.Fatalf("invalid persisted retirement batch: events=%d error=%v", len(payload.Events), err)
	}
	return payload.Events[0]
}

func assertDiscoveryDelivery(t *testing.T, expected map[string]entities.Envelope, batch []entities.Envelope) {
	t.Helper()
	if len(batch) != len(expected) {
		t.Errorf("request contained %d envelopes, want %d", len(batch), len(expected))
	}
	seen := make(map[string]bool, len(batch))
	for _, sent := range batch {
		saved, found := expected[sent.ID]
		if !found || seen[sent.ID] {
			t.Errorf("unexpected or repeated envelope %q", sent.ID)
			continue
		}
		seen[sent.ID] = true
		body, savedBody := canonicalDiscoveryBody(sent.Body), canonicalDiscoveryBody(saved.Body)
		if sent.AgentID != saved.AgentID || sent.Endpoint != saved.Endpoint || sent.Sub != saved.Sub ||
			sent.TSUnixMs != saved.TSUnixMs || !reflect.DeepEqual(body, savedBody) {
			t.Errorf("retirement envelope %q changed after reopening Bolt", sent.ID)
		}
	}
}

func canonicalDiscoveryBody(body any) map[string]any {
	raw, _ := json.Marshal(body)
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)
	return decoded
}

func flushDiscoveryAfterRestart(t *testing.T, path, apiURL string, size int, acknowledged bool) {
	t.Helper()
	store := openDiscoveryFixtureOutbox(t, path)
	defer store.Close()
	repo := repositories.NewOutboxRepository(store)
	flush := usecase.NewFlushOutboxWithOptions(repo, transport.NewHTTPJSONClient(config.Config{APIBaseURL: apiURL}),
		testBootstrapLogger{}, "Token fixture-rotated", nil, usecase.FlushOutboxOptions{
			BatchSize: size, IdentityHeaderProvider: func() string { return "fixture-current-identity" },
			RefreshCredentialsForEnvelope: func(env entities.Envelope) bool { return env.Meta["via"] != "hub" },
		})
	acked, err := flush.Execute(context.Background())
	pending, _ := repo.Len()
	if acknowledged {
		if err != nil || acked != size || pending != 0 {
			t.Fatalf("duplicate acknowledgment did not clear the retained batch: %d acked, %d pending, %v", acked, pending, err)
		}
	} else if err == nil || acked != 0 || pending != size {
		t.Fatalf("failed request did not preserve the batch: %d acked, %d pending, %v", acked, pending, err)
	}
}
