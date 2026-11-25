package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

// The Web fixture binds the SNMP profile to the seeded endpoint with ID 3.
const pilotFixtureEndpointID = 3

func TestSNMPPilotReceiptsFromUDPQualifyThroughTheWebExport(t *testing.T) {
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		t.Skip("explicit Web checkout required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	port, requests, silent := startAdminSNMPDevice(t)
	bridge := startAdminSNMPBridge(t, ctx, root, port)
	api := httptest.NewTLSServer(http.HandlerFunc(bridge.serveHTTP))
	defer api.Close()
	cfg := agentlessIdentityTestConfig(api.URL)
	cfg.Agent.Token = "controlled-1"
	cfg.AgentID, cfg.AgentClientID = 1, 1
	client := NewAgentlessHubClient(cfg)
	client.cl = api.Client()
	var before, after map[string]any
	bridge.fixture(t, map[string]any{"path": "/fixture/check-state"}, &before)

	start := time.Now().UTC().Truncate(time.Second)
	end := start.Add(15 * time.Second)
	var checkID int
	for cycle := range 3 {
		waitForSNMPPilotFixtureCycle(t, ctx, start.Add(time.Second+200*time.Millisecond+time.Duration(cycle)*2*time.Second))
		checkID = runSNMPPilotFixtureCycle(t, ctx, bridge, client, "ok")
		if requests.Load() != int64(cycle+1) {
			t.Fatal("a pilot cycle used more than one UDP request")
		}
	}
	report := validateSNMPPilotFixtureExport(t, ctx, root, bridge, checkID, start, end, 0)
	if !report.Qualified || len(report.ExecutionIDs) != 3 ||
		report.EndpointID != pilotFixtureEndpointID || report.ExecutorID != 1 || report.Version != "v1" {
		t.Fatalf("the Web export did not qualify the three local receipts: %+v", report)
	}

	silent.Store(true)
	runSNMPPilotFixtureCycle(t, ctx, bridge, client, "timeout")
	if requests.Load() != 4 {
		t.Fatal("the timed-out pilot cycle retried or did not send its GET")
	}
	if report := validateSNMPPilotFixtureExport(t, ctx, root, bridge, checkID, start, end, 1); report.Qualified || report.Consecutive != 0 {
		t.Fatalf("the latest timeout did not reset the exported sequence: %+v", report)
	}
	verifyCancelledSNMPPilotCycle(t, ctx, bridge, client, requests)
	bridge.fixture(t, map[string]any{"path": "/fixture/check-state"}, &after)
	if len(before) == 0 || !reflect.DeepEqual(before, after) {
		t.Fatal("diagnostic cycles changed the operational check state")
	}
}

func waitForSNMPPilotFixtureCycle(t *testing.T, ctx context.Context, due time.Time) {
	t.Helper()
	timer := time.NewTimer(time.Until(due))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func runSNMPPilotFixtureCycle(t *testing.T, ctx context.Context, bridge *adminSNMPBridge, client *AgentlessHubClient, expected string) int {
	t.Helper()
	command, job := fetchSNMPPilotFixtureCommand(t, ctx, bridge, client)
	obs := agentless.RunJobWithCredentials(ctx, job, nil, client.ResolveSNMPCredentials)
	if obs.ProbeReport == nil || obs.ProbeReport.Outcome != expected {
		t.Fatalf("the diagnostic outcome did not match the device: %s", obs.Code)
	}
	if err := client.SendObservations(ctx, []entities.AgentlessObservation{obs}); err != nil {
		t.Fatal(err)
	}
	return command.CheckID
}

func fetchSNMPPilotFixtureCommand(t *testing.T, ctx context.Context, bridge *adminSNMPBridge, client *AgentlessHubClient) (adminSNMPCommand, entities.AgentlessJob) {
	t.Helper()
	var command adminSNMPCommand
	bridge.fixture(t, map[string]any{"path": "/fixture/queue-pilot", "method": "POST"}, &command)
	jobs, err := client.FetchJobsWithOptions(ctx, 1, true, 60, AgentlessFetchOptions{
		CommandID: command.CommandID, CorrelationID: command.CorrelationID, CheckIDs: []int{command.CheckID},
	})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("pilot job dispatch failed: count=%d err=%v", len(jobs), err)
	}
	job := jobs[0]
	if job.Config["snmp_basic_probe"] != true || job.CredentialDelivery != "reference-v1" || job.SNMP == nil || job.SNMP.Community != "" {
		t.Fatal("the HUB did not receive a scoped single-GET diagnostic by reference")
	}
	return command, job
}

func verifyCancelledSNMPPilotCycle(t *testing.T, ctx context.Context, bridge *adminSNMPBridge, client *AgentlessHubClient, requests *atomic.Int64) {
	t.Helper()
	command, job := fetchSNMPPilotFixtureCommand(t, ctx, bridge, client)
	initialRequests := requests.Load()
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-probeCtx.Done():
				return
			case <-ticker.C:
				if requests.Load() > initialRequests {
					cancel()
					return
				}
			}
		}
	}()
	observation := agentless.RunJobWithCredentials(probeCtx, job, nil, client.ResolveSNMPCredentials)
	if observation.ProbeReport == nil || observation.ProbeReport.Outcome != "cancelled" || requests.Load() != initialRequests+1 {
		t.Fatalf("expected cancellation after a single GET: %s", observation.Code)
	}
	if err := client.SendObservations(ctx, []entities.AgentlessObservation{observation}); err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Status        string `json:"status"`
		Reason        string `json:"probe_reason"`
		ObservationID int    `json:"observation_id"`
	}
	bridge.fixture(t, map[string]any{"path": "/fixture/receipt", "command_id": command.CommandID}, &receipt)
	if receipt.Status != "failed" || receipt.Reason != "cancelled" || receipt.ObservationID != 0 {
		t.Fatalf("unexpected execution state after cancellation: %+v", receipt)
	}
	var html string
	bridge.fixture(t, map[string]any{"path": "/fixture/admin-view", "command_id": command.CommandID}, &html)
	if !strings.Contains(html, "A coleta foi cancelada.") || strings.Contains(html, "synthetic-snmp-secret") {
		t.Fatal("Admin detail did not present the safe cancellation message")
	}
}

type snmpPilotFixtureReport struct {
	Qualified    bool   `json:"qualified"`
	Consecutive  int    `json:"consecutive"`
	ExecutionIDs []int  `json:"execution_ids"`
	EndpointID   int    `json:"endpoint_id"`
	ExecutorID   int    `json:"executor_id"`
	Version      string `json:"version"`
}

func validateSNMPPilotFixtureExport(t *testing.T, ctx context.Context, root string, bridge *adminSNMPBridge,
	checkID int, start, end time.Time, expectedExit int,
) snmpPilotFixtureReport {
	t.Helper()
	from, until := start.Format(time.RFC3339), end.Format(time.RFC3339)
	var receipts json.RawMessage
	bridge.fixture(t, map[string]any{"path": "/fixture/pilot-export", "start": from, "end": until}, &receipts)
	for _, marker := range []string{"synthetic-snmp-secret", "uptime_seconds", "temperature_sensors", "result_payload_json"} {
		if bytes.Contains(receipts, []byte(marker)) {
			t.Fatal("pilot export leaked raw data")
		}
	}
	cmd := exec.CommandContext(ctx, "python3", filepath.Join(root, "scripts", "validate-snmp-availability-pilot.py"),
		"--cliente-id", "1", "--ativo-id", "1", "--asset-check-id", strconv.Itoa(checkID), "--hub-agente-id", "1",
		"--endpoint-id", strconv.Itoa(pilotFixtureEndpointID),
		"--executor-id", "1", "--version", "v1", "--source-timezone", "America/Sao_Paulo",
		"--window-start", from, "--window-end", until, "--interval-sec", "2", "--tolerance-sec", "1")
	cmd.Stdin = bytes.NewReader(receipts)
	output, err := cmd.Output()
	var exit *exec.ExitError
	if (expectedExit == 0 && err != nil) || (expectedExit != 0 && (!errors.As(err, &exit) || exit.ExitCode() != expectedExit)) {
		t.Fatalf("unexpected validator result: %v", err)
	}
	var report snmpPilotFixtureReport
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatal(err)
	}
	return report
}
