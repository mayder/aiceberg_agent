package app

import (
	"context"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"strings"
	"time"
)

// Legacy installations may have no numeric IDs in env. Reuse measured bootstrap
// identity, never a fabricated installation/host or weaker server authentication.
func relayProbeIdentity(ctx context.Context, cfg config.Config) func() string {
	guid := ""
	if cfg.AgentID <= 0 && cfg.AgentClientID <= 0 && cfg.AgentInstallationID == "" {
		if state, err := loadBootstrapState(); err == nil && state.Token == cfg.Agent.Token {
			guid = strings.TrimSpace(state.HostGUID)
		}
		if guid == "" {
			bounded, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			if info, err := host.InfoWithContext(bounded); err == nil && info != nil {
				guid = strings.TrimSpace(info.HostID)
			}
		}
	}
	return usecase.NewCachedIdentityHeaderProvider(time.Hour, func() string { return cfg.AgentIdentityHeader(guid) })
}
