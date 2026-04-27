package usecase

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/channel"
)

func TestRunChannelDoctorDirectDetectsTokenAndAPI(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	report := RunChannelDoctor(context.Background(), config.Config{
		Agent:      config.AgentCfg{Token: "token"},
		APIBaseURL: api.URL,
		AgentMode:  channel.ModeDirect,
	}, nil)

	if report.Status != "ok" {
		t.Fatalf("expected ok report, got %#v", report)
	}
	if checkStatus(report, "direct_topology") != "ok" || checkStatus(report, "api_reachable") != "ok" {
		t.Fatalf("expected direct/api ok checks, got %#v", report.Checks)
	}
}

func TestRunChannelDoctorRelayRequiresHubURLAndToken(t *testing.T) {
	report := RunChannelDoctor(context.Background(), config.Config{
		APIBaseURL: "http://127.0.0.1:1",
		AgentMode:  channel.ModeRelay,
	}, errors.New("AGENT_TOKEN obrigatorio"))

	if report.Status != "fail" {
		t.Fatalf("expected fail report, got %#v", report)
	}
	if checkStatus(report, "agent_token") != "fail" || checkStatus(report, "hub_url") != "fail" {
		t.Fatalf("expected token and hub_url failures, got %#v", report.Checks)
	}
}

func TestRunChannelDoctorHubWarnsDefaultListenAddr(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer api.Close()

	report := RunChannelDoctor(context.Background(), config.Config{
		Agent:      config.AgentCfg{Token: "token"},
		APIBaseURL: api.URL,
		AgentMode:  channel.ModeHub,
	}, nil)

	if report.Status != "warn" {
		t.Fatalf("expected warn report, got %#v", report)
	}
	if checkStatus(report, "hub_topology") != "ok" || checkStatus(report, "hub_listen_addr") != "warn" {
		t.Fatalf("expected hub checks, got %#v", report.Checks)
	}
}

func checkStatus(report ChannelDoctorReport, name string) string {
	for _, check := range report.Checks {
		if check.Name == name {
			return check.Status
		}
	}
	return ""
}
