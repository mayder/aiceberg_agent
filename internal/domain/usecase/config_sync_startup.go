package usecase

import (
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
)

// NewStartupConfigSync restores remote policies that exist only in memory even
// when the persisted collection preferences already have the server version.
// Authentication, signature checks and subsequent conditional polls are unchanged.
func NewStartupConfigSync(cfg config.Config, log logger.Logger, store *prefs.Store, commands chan<- ControlCommand) *ConfigSync {
	uc := NewConfigSync(cfg, log, store, commands)
	uc.RequestFullSync()
	return uc
}

// RequestFullSync preserves the normal polling cadence while requiring a full
// authenticated response, so a version applied through ingest can be reported.
func (uc *ConfigSync) RequestFullSync() {
	uc.forceFullSync.Store(true)
}
