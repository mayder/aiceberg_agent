package usecase

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/channel"
)

type ChannelDoctorCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message,omitempty"`
}

type ChannelDoctorReport struct {
	Status string               `json:"status"`
	Mode   string               `json:"mode"`
	Checks []ChannelDoctorCheck `json:"checks"`
}

func RunChannelDoctor(ctx context.Context, cfg config.Config, configErr error) ChannelDoctorReport {
	rawMode := strings.TrimSpace(cfg.AgentMode)
	if rawMode == "" {
		rawMode = channel.ModeDirect
	}
	mode, modeOK := channel.NormalizeMode(rawMode)
	report := ChannelDoctorReport{
		Status: "ok",
		Mode:   mode,
	}
	add := func(name, status, severity, message string) {
		report.Checks = append(report.Checks, ChannelDoctorCheck{
			Name:     name,
			Status:   status,
			Severity: severity,
			Message:  message,
		})
		if status == "fail" {
			report.Status = "fail"
			return
		}
		if status == "warn" && report.Status == "ok" {
			report.Status = "warn"
		}
	}

	if configErr != nil {
		add("config_load", "warn", "warning", configErr.Error())
	}
	if !modeOK {
		add("mode", "fail", "error", fmt.Sprintf("invalid AGENT_MODE %q", rawMode))
	} else {
		add("mode", "ok", "", "mode "+mode)
	}

	token := strings.TrimSpace(cfg.Agent.Token)
	if token == "" {
		add("agent_token", "fail", "error", "AGENT_TOKEN ausente")
	} else {
		add("agent_token", "ok", "", "token configurado")
	}

	apiBase := strings.TrimSpace(cfg.APIBaseURL)
	if apiBase == "" {
		add("api_base_url", "fail", "error", "API_BASE_URL ausente")
	} else {
		add("api_base_url", "ok", "", apiBase)
	}

	switch mode {
	case channel.ModeDirect:
		add("direct_topology", "ok", "", "direct conecta outbound no AIceberg")
	case channel.ModeHub:
		add("hub_topology", "ok", "", "hub conecta outbound no AIceberg e aceita relays")
		if strings.TrimSpace(cfg.HubListenAddr) == "" {
			add("hub_listen_addr", "warn", "warning", "HUB_LISTEN_ADDR vazio; sera usado padrao interno")
		} else {
			add("hub_listen_addr", "ok", "", cfg.HubListenAddr)
		}
	case channel.ModeRelay:
		add("relay_topology", "ok", "", "relay conecta somente no hub")
		if strings.TrimSpace(cfg.HubURL) == "" {
			add("hub_url", "fail", "error", "HUB_URL obrigatorio em modo relay")
		} else {
			add("hub_url", "ok", "", cfg.HubURL)
		}
	}

	if apiBase != "" {
		status, message := probeHTTP(ctx, strings.TrimRight(apiBase, "/"))
		add("api_reachable", status, severityForStatus(status), message)
	}
	if mode == channel.ModeRelay && strings.TrimSpace(cfg.HubURL) != "" {
		status, message := probeHTTP(ctx, strings.TrimRight(cfg.HubURL, "/"))
		add("hub_reachable", status, severityForStatus(status), message)
	}

	return report
}

func ChannelDoctorExitCode(report ChannelDoctorReport) int {
	if report.Status == "fail" {
		return 1
	}
	return 0
}

func probeHTTP(ctx context.Context, url string) (string, string) {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if err != nil {
		return "fail", err.Error()
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "fail", err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return "fail", resp.Status
	}
	if resp.StatusCode >= 400 {
		return "warn", resp.Status
	}
	return "ok", resp.Status
}

func severityForStatus(status string) string {
	if status == "fail" {
		return "error"
	}
	if status == "warn" {
		return "warning"
	}
	return ""
}
