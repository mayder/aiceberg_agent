package app

import (
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"testing"
)

func TestHubOutboxCapacityIsBoundedAndDoesNotChangeRelayOrDirect(t *testing.T) {
	base := usecase.FlushOutboxOptions{BatchSize: 50}
	for _, mode := range []string{"relay", "direct"} {
		o := hubOutboxFlushOptions(mode, config.Config{}, base)
		if o.BatchSize != 50 || o.Concurrency != 0 {
			t.Fatal("non-hub changed")
		}
	}
	for _, cfg := range []config.Config{{}, {HubOutboxFlushBatch: 10000, HubOutboxFlushConcurrency: 10000}} {
		o := hubOutboxFlushOptions("hub", cfg, base)
		if o.BatchSize != 800 || o.Concurrency < 8 || o.Concurrency > 8 {
			t.Fatalf("unsafe hub capacity: %#v", o)
		}
	}
	o := hubOutboxFlushOptions("hub", config.Config{HubOutboxFlushBatch: 50, HubOutboxFlushConcurrency: 1}, base)
	if o.BatchSize != 50 || o.Concurrency != 1 {
		t.Fatal("rollback settings ignored")
	}
}

func TestHubOutboxBatchWritesStayWithinHubScopeAndAllowRollback(t *testing.T) {
	for _, mode := range []string{"direct", "relay", "hub"} {
		cfg := config.Config{AgentMode: mode, HubOutboxWriteBatchEnabled: true}
		if got := hubOutboxStorageOptions(cfg).BatchWrites; got != (mode == "hub") {
			t.Fatalf("mode=%s batch=%v", mode, got)
		}
	}
	if hubOutboxStorageOptions(config.Config{AgentMode: "hub"}).BatchWrites {
		t.Fatal("opt-out ignored")
	}
}

func TestHubFreelistRebuildIsScopedAndAllowsOptOut(t *testing.T) {
	for _, mode := range []string{"hub", "direct", "relay"} {
		cfg := config.Config{AgentMode: mode, HubOutboxFreelistRebuildEnabled: true}
		if hubOutboxStorageOptions(cfg).RebuildFreelist != (mode == "hub") {
			t.Fatal("scope changed")
		}
	}
	if hubOutboxStorageOptions(config.Config{AgentMode: "hub"}).RebuildFreelist {
		t.Fatal("opt-out ignored")
	}
}

func TestHubFreelistRebuildDoesNotDependOnWriteBatching(t *testing.T) {
	cfg := config.Config{AgentMode: "hub", HubOutboxFreelistRebuildEnabled: true}
	opts := hubOutboxStorageOptions(cfg)
	if !opts.RebuildFreelist || opts.BatchWrites {
		t.Fatal("independent opt-outs changed")
	}
}
