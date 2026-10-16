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
		if o.BatchSize != 400 || o.Concurrency < 4 || o.Concurrency > 8 {
			t.Fatalf("unsafe hub capacity: %#v", o)
		}
	}
	o := hubOutboxFlushOptions("hub", config.Config{HubOutboxFlushBatch: 50, HubOutboxFlushConcurrency: 1}, base)
	if o.BatchSize != 50 || o.Concurrency != 1 {
		t.Fatal("rollback settings ignored")
	}
}
