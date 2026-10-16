package app

import (
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func hubOutboxFlushOptions(mode string, cfg config.Config, opts usecase.FlushOutboxOptions) usecase.FlushOutboxOptions {
	if mode != "hub" {
		return opts
	}
	batch := cfg.HubOutboxFlushBatch
	if batch <= 0 {
		batch = 400
	}
	if batch > 400 {
		batch = 400
	}
	concurrency := cfg.HubOutboxFlushConcurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	if concurrency > 8 {
		concurrency = 8
	}
	opts.BatchSize = batch
	opts.Concurrency = concurrency
	return opts
}
