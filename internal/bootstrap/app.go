package app

import (
	"context"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/data/local/outbox"
	"github.com/you/aiceberg_agent/internal/data/remote/transport"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/interfaces/health"
	"github.com/you/aiceberg_agent/internal/platform/collectors/sysmetrics"
)

func Run(cfg config.Config, log logger.Logger) error {
	ctx := context.Background()

	// Adapters mínimos
	store := outbox.NewMemStore()
	outboxRepo := repositories.NewOutboxRepository(store)
	httpTx := transport.NewHTTPJSONClient(cfg)

	// Use cases
	collector := sysmetrics.New()
	collectUC := usecase.NewCollectAndBuffer(collector, outboxRepo, log)
	flushUC := usecase.NewFlushOutbox(outboxRepo, httpTx, log)

	if cfg.HealthPort > 0 {
		go health.Serve(cfg.HealthPort, log)
	}

	tCollect := time.NewTicker(10 * time.Second)
	tFlush := time.NewTicker(15 * time.Second)
	defer tCollect.Stop()
	defer tFlush.Stop()

	log.Info("agent started")

	for {
		select {
		case <-ctx.Done():
			log.Info("shutdown")
			return nil
		case <-tCollect.C:
			_ = collectUC.Execute(ctx)
		case <-tFlush.C:
			_ = flushUC.Execute(ctx)
		}
	}
}
