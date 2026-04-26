package modechange

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type Applier struct {
	cfg config.Config
	log logger.Logger
}

func NewApplier(cfg config.Config, log logger.Logger) *Applier {
	return &Applier{cfg: cfg, log: log}
}

func (a *Applier) Apply(ctx context.Context, cmd entities.SelfHealCommand) (string, map[string]any, error) {
	targetMode, err := targetModeFromPayload(cmd.Payload)
	if err != nil {
		return "invalid agent mode payload", nil, err
	}
	overridePath := strings.TrimSpace(a.cfg.AgentModeOverridePath)
	evidence := map[string]any{
		"target_mode":        targetMode,
		"previous_mode":      a.cfg.Mode(),
		"mode_override_path": overridePath,
		"restart_strategy":   "self_exit",
		"restart_scheduled":  false,
	}
	if err := a.validatePrerequisites(targetMode, overridePath); err != nil {
		return "agent mode prerequisites failed", evidence, err
	}
	if err := writeOverrideValue(overridePath, targetMode); err != nil {
		return "failed to persist agent mode", evidence, err
	}
	evidence["persisted"] = true

	if err := scheduleRestart(ctx); err != nil {
		return "failed to schedule agent service restart", evidence, err
	}
	evidence["restart_scheduled"] = true
	evidence["restart_delay_sec"] = 2
	a.log.Info(logger.KV("agent mode change applied",
		"target_mode", targetMode,
		"mode_override_path", overridePath,
		"restart_strategy", "self_exit",
	))
	return "agent mode persisted and service restart scheduled", evidence, nil
}

func targetModeFromPayload(payload map[string]any) (string, error) {
	raw := ""
	for _, key := range []string{"target_mode", "mode", "mode_alias"} {
		if v, ok := payload[key]; ok {
			raw = strings.TrimSpace(fmt.Sprint(v))
			if raw != "" {
				break
			}
		}
	}
	switch strings.ToLower(raw) {
	case "direct", "direto":
		return "direct", nil
	case "hub":
		return "hub", nil
	case "relay":
		return "relay", nil
	default:
		return "", fmt.Errorf("unsupported target mode: %s", raw)
	}
}

func (a *Applier) validatePrerequisites(targetMode, overridePath string) error {
	if strings.TrimSpace(overridePath) == "" {
		return fmt.Errorf("agent mode override path unavailable")
	}
	dir := filepath.Dir(overridePath)
	if info, err := os.Stat(dir); err != nil {
		return fmt.Errorf("agent mode override directory unavailable: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("agent mode override parent is not a directory")
	}
	if targetMode == "relay" && strings.TrimSpace(a.cfg.HubURL) == "" {
		return fmt.Errorf("relay mode requires HUB_URL")
	}
	if targetMode == "hub" && strings.TrimSpace(a.cfg.APIBaseURL) == "" {
		return fmt.Errorf("hub mode requires API_BASE_URL")
	}
	return nil
}

func writeOverrideValue(path, value string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("empty mode override path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value+"\n"), 0o600)
}

func scheduleRestart(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	go func() {
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			os.Exit(0)
		}
	}()
	return nil
}
