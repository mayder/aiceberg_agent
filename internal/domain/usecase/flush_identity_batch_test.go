package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func signedRelayEnvelope(id string, agent int, host, endpoint string) entities.Envelope {
	cfg := config.Config{Agent: config.AgentCfg{Token: "synthetic-secret"}, AgentClientID: 2, AgentID: agent, AgentInstallationID: "install"}
	return entities.Envelope{ID: id, AgentID: host, Endpoint: endpoint, AuthHeader: "Token synthetic-secret",
		IdentityHeader: cfg.AgentIdentityHeader(host), Body: json.RawMessage(`{"retained":true}`)}
}

func TestVerifiedRelayBatchPreservesOrderDurabilityAndExactAck(t *testing.T) {
	repo := &deferredOutbox{acked: make(map[string]int)}
	originals := make(map[string]string)
	for i := 0; i < 100; i++ {
		env := signedRelayEnvelope(fmt.Sprintf("%03d", i), 84, "host", "/v1/ingest/metrics")
		originals[env.ID] = env.IdentityHeader
		_ = repo.Append(env)
	}
	calls := 0
	transport := deferredTransportFunc(func(batch []entities.Envelope, auth, endpoint string) ([]byte, error) {
		calls++
		if len(batch) != 100 || endpoint != "/v1/ingest/metrics" {
			t.Fatal("same authenticated subject fragmented")
		}
		for i, env := range batch {
			if env.ID != fmt.Sprintf("%03d", i) {
				t.Fatal("input order changed")
			}
			if _, _, ok := config.VerifiedIdentityBatchScope(env.IdentityHeader, auth, time.Now().Unix()); !ok {
				t.Fatal("request uses an invented or invalid identity")
			}
		}
		if calls == 1 {
			return nil, fakeHTTPStatusErr{code: 503}
		}
		return []byte(`{"received":100,"skipped":0}`), nil
	})
	uc := NewFlushOutboxWithOptions(repo, transport, &fakeLogger{}, "", nil, FlushOutboxOptions{BatchSize: 100, BatchVerifiedIdentities: true})
	now := time.Now()
	uc.now = func() time.Time { return now }
	if n, err := uc.Execute(context.Background()); n != 0 || err == nil {
		t.Fatal("failed request acknowledged")
	}
	for _, env := range repo.pending {
		if originals[env.ID] != env.IdentityHeader {
			t.Fatal("durable identity was rewritten")
		}
	}
	now = now.Add(61 * time.Second)
	if n, err := uc.Execute(context.Background()); n != 100 || err != nil {
		t.Fatal("confirmed delivery failed", n, err)
	}
	if n, _ := repo.Len(); n != 0 || calls != 2 {
		t.Fatal("wrong acknowledgement or duplicate request")
	}
	s := uc.Snapshot()
	if s.LastInputGroups != 100 || s.LastDispatchGroups != 1 {
		t.Fatal("fragmentation evidence missing", s.LastInputGroups, s.LastDispatchGroups)
	}
}

func TestVerifiedRelayBatchDoesNotCrossSubjectsRoutesOrLegacyHeaders(t *testing.T) {
	batch := []entities.Envelope{signedRelayEnvelope("a", 1, "host", "/v1/ingest/metrics"),
		signedRelayEnvelope("b", 2, "host", "/v1/ingest/metrics"),
		signedRelayEnvelope("c", 1, "other-host", "/v1/ingest/metrics"),
		signedRelayEnvelope("d", 1, "host", "/v1/logs/raw"),
		{ID: "legacy", AgentID: "host", AuthHeader: "Token synthetic-secret", IdentityHeader: "opaque", Endpoint: "/v1/ingest/metrics"}}
	uc := NewFlushOutboxWithOptions(nil, nil, &fakeLogger{}, "", nil, FlushOutboxOptions{BatchVerifiedIdentities: true})
	groups, invalid := uc.groupFlushBatch(batch)
	if len(groups) != 5 || len(invalid) != 0 {
		t.Fatal("forwarding boundaries changed", len(groups))
	}
}

func TestVerifiedRelayBatchFreshNonceCannotBypassAuthenticationCooldown(t *testing.T) {
	repo := &deferredOutbox{acked: make(map[string]int)}
	_ = repo.Append(signedRelayEnvelope("a", 1, "host", "/v1/logs/raw"))
	calls := 0
	transport := deferredTransportFunc(func([]entities.Envelope, string, string) ([]byte, error) {
		calls++
		return nil, fakeHTTPStatusErr{code: 401}
	})
	uc := NewFlushOutboxWithOptions(repo, transport, &fakeLogger{}, "", nil, FlushOutboxOptions{BatchSize: 10, BatchVerifiedIdentities: true})
	now := time.Now()
	uc.now = func() time.Time { return now }
	_, _ = uc.Execute(context.Background())
	_ = repo.Append(signedRelayEnvelope("b", 1, "host", "/v1/logs/raw"))
	_, _ = uc.Execute(context.Background())
	if calls != 1 {
		t.Fatal("new nonce bypassed the existing cooldown")
	}
	if n, _ := repo.Len(); n != 2 {
		t.Fatal("failed data removed")
	}
	now = now.Add(61 * time.Second)
	_, _ = uc.Execute(context.Background())
	if calls != 2 {
		t.Fatal("retry did not resume after cooldown")
	}
}

func BenchmarkVerifiedRelayBatch(b *testing.B) {
	batch := make([]entities.Envelope, 800)
	for i := range batch {
		batch[i] = signedRelayEnvelope(fmt.Sprint(i), i%200+1, fmt.Sprintf("relay-%d", i%200), "/v1/ingest/metrics")
	}
	for _, verified := range []bool{false, true} {
		b.Run(fmt.Sprintf("verified_%t", verified), func(b *testing.B) {
			uc := NewFlushOutboxWithOptions(nil, nil, &fakeLogger{}, "", nil, FlushOutboxOptions{BatchVerifiedIdentities: verified})
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				groups, _ := uc.groupFlushBatch(batch)
				if len(groups) != map[bool]int{false: 800, true: 200}[verified] {
					b.Fatal("unexpected group count")
				}
			}
		})
	}
}
