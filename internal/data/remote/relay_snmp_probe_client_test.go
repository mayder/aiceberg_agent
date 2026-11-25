package remote

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

func TestRelaySNMPClientUsesOnlyHubAndRequiresExplicitAcceptance(t *testing.T) {
	cloudCalls, hubCalls := 0, 0
	cloud := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { cloudCalls++ }))
	defer cloud.Close()
	accepted := false
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hubCalls++
		if r.Method != "POST" || r.Header.Get("Authorization") != "Token fixture-relay-secret" || r.Header.Get("X-Agent-Identity") == "" {
			t.Error("missing authenticated relay request")
		}
		if r.URL.Path != "/v1/relay-snmp-probe/result" {
			t.Error("wrong fixed route")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["command_id"] != "fixture-command-0001" {
			t.Error("command reference lost")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "accepted": accepted})
	}))
	defer hub.Close()
	cfg := config.Config{APIBaseURL: cloud.URL, HubURL: hub.URL, AgentMode: "relay", AgentID: 424, AgentClientID: 3, Agent: config.AgentCfg{Token: "fixture-relay-secret"}}
	client := NewRelaySNMPProbeClient(cfg)
	observation := entities.AgentlessObservation{CommandID: "fixture-command-0001"}
	if err := client.ReportProbe(context.Background(), observation); err == nil {
		t.Fatal("unconfirmed result acknowledged")
	}
	accepted = true
	if err := client.ReportProbe(context.Background(), observation); err != nil {
		t.Fatal(err)
	}
	hub.Close()
	if err := client.ReportProbe(context.Background(), observation); err == nil {
		t.Fatal("offline HUB acknowledged")
	}
	if cloudCalls != 0 || hubCalls != 2 {
		t.Fatal("relay bypassed HUB or replayed report")
	}
	cfg.HubURL = ""
	client = NewRelaySNMPProbeClient(cfg)
	if err := client.ReportProbe(context.Background(), observation); err == nil {
		t.Fatal("missing HUB accepted")
	}
}
func TestRelaySNMPClientNeverReturnsRawHTTPBodyOrWrongExecutor(t *testing.T) {
	valid := false
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !valid {
			w.WriteHeader(403)
			_, _ = w.Write([]byte("fixture-snmp-secret"))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ready","executor_id":999,"job":{"cliente_id":3}}`))
	}))
	defer hub.Close()
	client := NewRelaySNMPProbeClient(config.Config{HubURL: hub.URL, AgentMode: "relay", AgentID: 424, AgentClientID: 3, Agent: config.AgentCfg{Token: "fixture-relay-key"}})
	if _, err := client.FetchProbe(context.Background(), "fixture-command-0001"); err == nil || strings.Contains(err.Error(), "fixture-snmp-secret") {
		t.Fatal("raw upstream error returned")
	}
	valid = true
	if _, err := client.FetchProbe(context.Background(), "fixture-command-0001"); err == nil {
		t.Fatal("wrong executor accepted")
	}
}

func TestRelaySNMPClientResolvesCredentialWithinCommandAwayFromClaim(t *testing.T) {
	sensor, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sensor.Close()
	port := sensor.LocalAddr().(*net.UDPAddr).Port
	received := make(chan error, 1)
	go func() { received <- answerRelayCredentialFixture(sensor) }()
	var requests atomic.Int32
	relay := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if req.Header.Get("Authorization") != "Token fixture-relay-key" || req.Header.Get("X-Agent-Identity") == "" ||
			body["command_id"] != "fixture-command-0001" {
			t.Error("command or authentication lost")
		}
		if requests.Add(1) == 1 {
			if body["credential_mode"] != "reference-v1" || len(body) != 2 {
				t.Error("unexpected claim parameters")
			}
			job := entities.AgentlessJob{Tipo: "snmp", ClienteID: 3, AtivoID: 52, CheckID: 72, CommandID: "fixture-command-0001",
				CredentialDelivery: entities.AgentlessCredentialReferenceV1, BaseIntervalSec: 60, TimeoutMs: 1500,
				Endpoint: &entities.AgentlessEndpoint{ID: 6, Endereco: "127.0.0.1"},
				SNMP:     &entities.AgentlessSnmpProfile{ProfileID: 4, BindingID: 5, Version: "v1", Port: port, Community: "untrusted-inline"},
				ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, CredentialRef: "snmp_profile:4",
					BudgetMS: 5000, FreshnessSec: 180},
				Config: entities.AgentlessConfig{"snmp_basic_probe": true, "snmp_basic_oid": "1.3.6.1.2.1.1.3.0"}}
			_ = json.NewEncoder(w).Encode(entities.RelaySNMPJob{Status: "ready", ExecutorID: 424, Job: &job})
			return
		}
		if body["operation"] != "resolve_snmp_profile" || body["credential_ref"] != "snmp_profile:4" ||
			body["check_id"] != float64(72) || body["binding_id"] != float64(5) || body["endpoint_id"] != float64(6) ||
			body["port"] != float64(port) {
			t.Error("incorrect lease lookup")
		}
		_, _ = w.Write([]byte(`{"status":"ok","command_id":"fixture-command-0001","credential_ref":"snmp_profile:4","credential":{"community":"resolved-fixture"}}`))
	}))
	defer relay.Close()
	client := NewRelaySNMPProbeClient(config.Config{HubURL: relay.URL, AgentMode: "relay", AgentID: 424,
		AgentClientID: 3, Agent: config.AgentCfg{Token: "fixture-relay-key"}})
	client.cl = relay.Client()
	claimed, err := client.FetchProbe(context.Background(), "fixture-command-0001")
	if err != nil || claimed.Job == nil || claimed.Job.SNMP.Community != "" || requests.Load() != 1 {
		t.Fatal("claim accepted inline credentials or pre-resolved the profile")
	}
	observation := agentless.RunJobWithCredentials(context.Background(), *claimed.Job, nil, client.ResolveSNMPCredentials)
	if err := <-received; err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(observation)
	if err != nil || observation.Status != "ok" || observation.ProbeReport == nil ||
		claimed.Job.SNMP.Community != "" || requests.Load() != 2 || strings.Contains(string(wire), "resolved-fixture") {
		t.Fatal("resolved credential failed or escaped to the claim or observation")
	}
}

func answerRelayCredentialFixture(sensor net.PacketConn) error {
	_ = sensor.SetReadDeadline(time.Now().Add(5 * time.Second))
	buffer := make([]byte, 2048)
	n, peer, err := sensor.ReadFrom(buffer)
	if err != nil {
		return err
	}
	decoder := *gosnmp.Default
	packet, err := decoder.SnmpDecodePacket(buffer[:n])
	if err != nil || packet.Version != gosnmp.Version1 || packet.PDUType != gosnmp.GetRequest ||
		packet.Community != "resolved-fixture" || len(packet.Variables) != 1 {
		return errors.New("unexpected diagnostic packet")
	}
	reply := gosnmp.SnmpPacket{Version: packet.Version, Community: packet.Community, PDUType: gosnmp.GetResponse,
		RequestID: packet.RequestID, Variables: []gosnmp.SnmpPDU{{Name: packet.Variables[0].Name, Type: gosnmp.TimeTicks, Value: uint32(100)}}}
	raw, err := reply.MarshalMsg()
	if err != nil {
		return err
	}
	_, err = sensor.WriteTo(raw, peer)
	return err
}

func TestRelaySNMPClientRetriesOnlyLegacyClaimRejection(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusServiceUnavailable} {
		var calls atomic.Int32
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(status)
				return
			}
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil || len(body) != 1 ||
				body["command_id"] != "fixture-command-0001" {
				t.Error("legacy retry changed its command")
			}
			_, _ = w.Write([]byte(`{"status":"already_claimed"}`))
		}))
		client := NewRelaySNMPProbeClient(config.Config{HubURL: server.URL, AgentMode: "relay", AgentID: 424,
			Agent: config.AgentCfg{Token: "fixture-relay-key"}})
		client.cl = server.Client()
		_, err := client.FetchProbe(context.Background(), "fixture-command-0001")
		server.Close()
		if status == http.StatusBadRequest && (err != nil || calls.Load() != 2) {
			t.Fatal("legacy pre-claim rejection was not handled")
		}
		if status != http.StatusBadRequest && (err == nil || calls.Load() != 1) {
			t.Fatal("transport or authorization error caused a downgrade")
		}
	}
}
