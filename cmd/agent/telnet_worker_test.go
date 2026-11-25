//go:build linux || darwin

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
	"github.com/you/aiceberg_agent/internal/platform/telnetprobe"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == telnetprobe.WorkerArgument {
		main()
	}
	os.Exit(m.Run())
}

func TestAgentlessTelnetRunsWorkerWithTheAuthorizedSocket(t *testing.T) {
	listener, job := telnetFixture(t)
	const privateBanner = "local-fixture-login"
	wire := make(chan []byte, 1)
	go func() {
		socket, err := listener.AcceptTCP()
		if err != nil {
			return
		}
		defer socket.Close()
		_ = socket.SetDeadline(time.Now().Add(time.Second))
		_, _ = socket.Write([]byte{255, 251, 1, 255, 253, 3})
		var refusal [6]byte
		if _, err := io.ReadFull(socket, refusal[:]); err != nil {
			return
		}
		_, _ = socket.Write([]byte(privateBanner + "\r\n"))
		followup, _ := io.ReadAll(socket)
		wire <- append(refusal[:], followup...)
	}()

	observation := agentless.RunJob(context.Background(), job)
	if observation.Status != "ok" || observation.ProbeReport == nil || observation.ProbeReport.Protocol != "telnet" ||
		observation.MonitoringSamples == nil || len(observation.MonitoringSamples.Samples) != 1 {
		t.Fatalf("no successful Telnet observation from worker: %s", observation.Code)
	}
	sample := observation.MonitoringSamples.Samples[0]
	if sample.Value == nil || *sample.Value != 1 || sample.Unit != "none" {
		t.Fatalf("missing banner presence: %+v", sample)
	}
	select {
	case got := <-wire:
		if !bytes.Equal(got, []byte{255, 254, 1, 255, 252, 3}) {
			t.Fatalf("unexpected bytes on Telnet socket: %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not close the socket")
	}
	if payload, err := json.Marshal(observation); err != nil || bytes.Contains(payload, []byte(privateBanner)) {
		t.Fatal("raw Telnet banner leaked to the observation")
	}
}

func TestAgentlessTelnetRejectsMissingConsentAndForeignScopeBeforeNetwork(t *testing.T) {
	listener, job := telnetFixture(t)
	foreign := job
	foreign.ClienteID++
	if result := agentless.RunJob(context.Background(), foreign); result.Code != "telnet_target_not_allowed" {
		t.Fatalf("foreign target accepted: %s", result.Code)
	}
	job.Config["legacy_opt_in"] = false
	if result := agentless.RunJob(context.Background(), job); result.Code != "invalid_telnet_config" {
		t.Fatalf("disabled check accepted: %s", result.Code)
	}
	job.Config["legacy_opt_in"] = true
	job.Config["command"] = "fixture command"
	if result := agentless.RunJob(context.Background(), job); result.Code != "invalid_telnet_config" {
		t.Fatalf("arbitrary command accepted: %s", result.Code)
	}
	_ = listener.SetDeadline(time.Now().Add(50 * time.Millisecond))
	if socket, err := listener.AcceptTCP(); err == nil {
		_ = socket.Close()
		t.Fatal("scope failure opened a network connection")
	}
}

func TestAgentlessTelnetCancelsTheWorkerAndSocket(t *testing.T) {
	listener, job := telnetFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readClosed := make(chan error, 1)
	go func() {
		socket, err := listener.AcceptTCP()
		if err != nil {
			readClosed <- err
			return
		}
		defer socket.Close()
		_ = socket.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = socket.Write([]byte{255, 251, 1})
		var rejection [3]byte
		_, err = io.ReadFull(socket, rejection[:])
		if err != nil || rejection != [3]byte{255, 254, 1} {
			readClosed <- err
			return
		}
		cancel()
		_, err = socket.Read(make([]byte, 1))
		readClosed <- err
	}()
	observation := agentless.RunJob(ctx, job)
	if observation.ProbeReport == nil || observation.ProbeReport.Outcome != "cancelled" {
		t.Fatalf("worker did not propagate cancellation: %+v", observation.ProbeReport)
	}
	if err := <-readClosed; err != io.EOF {
		t.Fatalf("worker retained the connected socket: %v", err)
	}
}

func TestAgentlessTelnetReportsAnExhaustedBudget(t *testing.T) {
	listener, job := telnetFixture(t)
	job.ProbePolicy.BudgetMS = 500
	readClosed := make(chan error, 1)
	go func() {
		socket, err := listener.AcceptTCP()
		if err != nil {
			readClosed <- err
			return
		}
		defer socket.Close()
		_ = socket.SetDeadline(time.Now().Add(2 * time.Second))
		_, err = socket.Read(make([]byte, 1))
		readClosed <- err
	}()
	observation := agentless.RunJob(context.Background(), job)
	if observation.Code != "telnet_read_failed" || observation.ProbeReport == nil ||
		observation.ProbeReport.Outcome != "timeout" || observation.ProbeReport.Reason != "budget" {
		t.Fatalf("unexpected timeout observation: %s %+v", observation.Code, observation.ProbeReport)
	}
	if err := <-readClosed; err != io.EOF {
		t.Fatalf("worker retained the socket after the deadline: %v", err)
	}
}

func TestAgentlessTelnetAgainstIndependentServer(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		value float64
	}{
		{"banner", 1},
		{"quiet", 0},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			raw := os.Getenv("AICEBERG_TELNET_TEST_" + strings.ToUpper(fixture.name) + "_PORT")
			if raw == "" {
				t.Skip("requires an independent Telnet server bound to loopback")
			}
			port, err := strconv.Atoi(raw)
			if err != nil || port < 1 || port > 65535 {
				t.Fatal("invalid Telnet fixture port")
			}
			observation := agentless.RunJob(context.Background(), telnetJobForLocalPort(t, port))
			if observation.Status != "ok" || observation.ProbeReport == nil || observation.ProbeReport.Protocol != "telnet" ||
				observation.MonitoringSamples == nil || len(observation.MonitoringSamples.Samples) != 1 {
				t.Fatalf("could not identify the Telnet service: %s", observation.Code)
			}
			sample := observation.MonitoringSamples.Samples[0]
			if sample.Value == nil || *sample.Value != fixture.value || sample.Unit != "none" {
				t.Fatalf("unexpected banner sample: %+v", sample)
			}
			payload, err := json.Marshal(observation)
			if err != nil || bytes.Contains(payload, []byte("independent-fixture-banner")) {
				t.Fatal("the observation exposed the Telnet banner")
			}
		})
	}
}

func telnetFixture(t *testing.T) (*net.TCPListener, entities.AgentlessJob) {
	t.Helper()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	port := listener.Addr().(*net.TCPAddr).Port
	return listener, telnetJobForLocalPort(t, port)
}

func telnetJobForLocalPort(t *testing.T, port int) entities.AgentlessJob {
	t.Helper()
	targets := map[string]any{"targets": []any{map[string]any{
		"client_id": 3, "asset_id": 52, "check_id": 72, "endpoint_id": 17, "ip": "127.0.0.1", "port": port,
	}}}
	data, err := json.Marshal(targets)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "targets.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AICEBERG_TELNET_TARGETS_FILE", path)
	return entities.AgentlessJob{
		ClienteID: 3, AtivoID: 52, CheckID: 72, Tipo: "telnet", BaseIntervalSec: 60,
		Endpoint: &entities.AgentlessEndpoint{ID: 17, Tipo: "ip", Endereco: "127.0.0.1", Porta: &port},
		Config: entities.AgentlessConfig{"legacy_opt_in": true,
			"metrics": []any{map[string]any{"id": "banner", "preset": "banner_present"}}},
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, BudgetMS: 4000, FreshnessSec: 180},
	}
}
