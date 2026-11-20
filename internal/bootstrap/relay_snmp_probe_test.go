package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	gosnmp "github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/relayprobe"
	store "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/interfaces/hub"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRelayProbeEndToEndUDPThroughHubAndDurableACK(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	done := make(chan error, 1)
	go func() { done <- serveRelayUDPFixture(conn) }()
	command := entities.SelfHealCommand{CommandID: "fixture-command-0001", Code: "diagnose_snmp_get", Mode: "manual", Payload: map[string]any{"execution_id": 12}}
	job := entities.AgentlessJob{CheckID: 72, AtivoID: 52, ClienteID: 3, Tipo: "snmp", TimeoutMs: 1500, CommandID: command.CommandID,
		Endpoint: &entities.AgentlessEndpoint{Endereco: "127.0.0.1"}, SNMP: &entities.AgentlessSnmpProfile{Version: "v1", Community: "fixture-snmp-private", Port: conn.LocalAddr().(*net.UDPAddr).Port, TimeoutMs: 1500},
		Config: entities.AgentlessConfig{"snmp_basic_probe": true, "snmp_basic_oid": "1.3.6.1.2.1.1.3.0", "snmp_collection_kind": "legacy"}}
	fetches, reports := 0, 0
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if r.Header.Get("X-Aiceberg-Hub-Proof") != relayprobe.Proof("fixture-hub-key", r.Header.Get("Authorization"), r.URL.Path, raw, r.Header.Get("X-Aiceberg-Hub-Time")) {
			t.Error("unproven HUB route")
		}
		if r.Header.Get("Authorization") != "Token fixture-relay-key" || r.Header.Get("X-Agent-Identity") == "" {
			t.Error("missing executor identity")
		}
		if r.URL.Path == relayprobe.JobPath {
			fetches++
			_ = json.NewEncoder(w).Encode(entities.RelaySNMPJob{Status: "ready", ExecutionID: 12, ExecutorID: 424, OwnerHubID: 84, ExpiresAt: time.Now().Add(15 * time.Second).Unix(), Job: &job})
			return
		}
		reports++
		var payload struct {
			Observation entities.AgentlessObservation `json:"observation"`
		}
		if err = json.Unmarshal(raw, &payload); err != nil {
			t.Error(err)
		}
		obs := payload.Observation
		basic, _ := obs.Payload["basic_probe"].(map[string]any)
		if obs.CommandID != command.CommandID || obs.CheckID != 72 || !obs.IsFinal || obs.Status != "ok" || basic["response_received"] != true || basic["sent_datagrams"] != float64(1) || basic["received_datagrams"] != float64(1) {
			t.Error("result identity or SNMP evidence lost")
		}
		if strings.Contains(string(raw), "fixture-snmp-private") {
			t.Error("credential leaked to result")
		}
		if reports == 1 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","accepted":true}`))
	}))
	defer cloud.Close()
	gateway := httptest.NewServer(hub.NewHandler(config.Config{AgentMode: "hub", APIBaseURL: cloud.URL, Agent: config.AgentCfg{Token: "fixture-hub-key"}}, nil, testBootstrapLogger{}, nil))
	defer gateway.Close()
	client := remote.NewRelaySNMPProbeClient(config.Config{AgentMode: "relay", HubURL: gateway.URL, APIBaseURL: "http://127.0.0.1:1", AgentClientID: 3, AgentID: 424, Agent: config.AgentCfg{Token: "fixture-relay-key"}})
	disk, err := store.NewBoltStore(filepath.Join(t.TempDir(), "relay.db"), 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disk.Close() })
	repo := repositories.NewAgentlessOutboxRepository(disk)
	worker := usecase.NewRelaySNMPProbe("relay", client, repo, agentless.RunJob)
	evidence, err := worker.Execute(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if evidence["delivery_confirmed"] != false {
		t.Fatal("delivery claimed before API ACK")
	}
	if err = worker.Flush(context.Background()); err == nil {
		t.Fatal("503 acknowledged")
	}
	if pending, _ := repo.Len(); pending != 1 {
		t.Fatal("result lost on upstream failure")
	}
	restarted := usecase.NewRelaySNMPProbe("relay", client, repo, agentless.RunJob)
	if err = restarted.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if pending, _ := repo.Len(); pending != 0 || fetches != 1 || reports != 2 {
		t.Fatal("ACK lost or GET repeated")
	}
}

func serveRelayUDPFixture(conn net.PacketConn) error {
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	raw := make([]byte, 2048)
	n, peer, err := conn.ReadFrom(raw)
	if err != nil {
		return err
	}
	decoder := *gosnmp.Default
	packet, err := decoder.SnmpDecodePacket(raw[:n])
	if err != nil {
		return err
	}
	if packet.Version != gosnmp.Version1 || packet.PDUType != gosnmp.GetRequest || len(packet.Variables) != 1 || strings.TrimPrefix(packet.Variables[0].Name, ".") != "1.3.6.1.2.1.1.3.0" {
		return errors.New("unexpected relay SNMP request")
	}
	reply := &gosnmp.SnmpPacket{Version: packet.Version, Community: packet.Community, PDUType: gosnmp.GetResponse, RequestID: packet.RequestID,
		Variables: []gosnmp.SnmpPDU{{Name: packet.Variables[0].Name, Type: gosnmp.TimeTicks, Value: uint32(12345)}}}
	encoded, err := reply.MarshalMsg()
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(encoded, peer)
	return err
}

func TestRelayProbeBootstrapRequiresDurabilityAndNeverEnablesHubRoutineWorker(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "blocker")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	run, stop := startRelaySNMPProbe(context.Background(), config.Config{AgentMode: "relay", AgentlessOutboxPath: filepath.Join(path, "not-a-directory")}, testBootstrapLogger{})
	stop()
	if run != nil {
		t.Fatal("memory fallback allowed")
	}
	run, stop = startRelaySNMPProbe(context.Background(), config.Config{AgentMode: "hub", AgentlessOutboxPath: filepath.Join(directory, "unused")}, testBootstrapLogger{})
	stop()
	if run != nil {
		t.Fatal("relay probe initialized in HUB role")
	}
	if _, err := os.Stat(filepath.Join(directory, "unused.relay-probe")); !os.IsNotExist(err) {
		t.Fatal("unneeded store created")
	}
}

func TestRelayProbeReusesMeasuredBootstrapIdentityForLegacyInstallation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap.ok")
	t.Setenv("AGENT_STATE_PATH", path)
	raw := []byte(`{"token":"fixture-relay-token","host_guid":"fixture-measured-host-guid"}`)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	provider := relayProbeIdentity(context.Background(), config.Config{Agent: config.AgentCfg{Token: "fixture-relay-token"}})
	data, err := base64.RawURLEncoding.DecodeString(provider())
	if err != nil {
		t.Fatal(err)
	}
	var claim map[string]any
	if err = json.Unmarshal(data, &claim); err != nil {
		t.Fatal(err)
	}
	if claim["host_guid"] != "fixture-measured-host-guid" || claim["signature"] == "" {
		t.Fatal("measured identity not preserved")
	}
	if _, exists := claim["installation_id"]; exists {
		t.Fatal("installation identity fabricated")
	}
}
