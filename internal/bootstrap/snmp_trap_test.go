package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/common/config"
	agentlessstore "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func TestSNMPTrapHubDeliversThroughIsolatedQueueAndKeepsIdentityOnRetry(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("private receiver profiles are supported on Linux and macOS")
	}
	dir := t.TempDir()
	profiles := filepath.Join(dir, "profiles.json")
	const community = "fixture-private-trap-community"
	raw := []byte(`[{"source_ip":"127.0.0.1","version":"v2c","community":"` + community +
		`","client_id":3,"asset_id":31,"check_id":32,"endpoint_id":33,"trap_oids":["1.3.6.1.6.3.1.1.5.3"]}]`)
	if err := os.WriteFile(profiles, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	tmp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	bind := tmp.LocalAddr().(*net.UDPAddr)
	_ = tmp.Close()
	t.Setenv("AICEBERG_SNMP_TRAP_LISTEN", bind.String())
	t.Setenv("AICEBERG_SNMP_TRAP_PROFILES_FILE", profiles)
	attempts := make(chan string, 2)
	var attemptCount atomic.Int32
	acknowledged := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wire, err := io.ReadAll(r.Body)
		if err != nil || bytes.Contains(wire, []byte(community)) {
			t.Error("invalid body or credential leak")
		}
		var body struct {
			Observations []struct {
				CheckID   int    `json:"check_id"`
				DedupeKey string `json:"dedupe_key"`
				IsFinal   bool   `json:"is_final"`
			} `json:"observations"`
		}
		if json.Unmarshal(wire, &body) != nil || len(body.Observations) != 1 ||
			body.Observations[0].CheckID != 32 || !body.Observations[0].IsFinal ||
			r.URL.Path != "/v1/hub-agentless/observations" || r.Header.Get("Authorization") != "Token fixture-hub-token" {
			t.Error("event failed the HUB wire contract")
			w.WriteHeader(400)
			return
		}
		key := body.Observations[0].DedupeKey
		if r.Header.Get("Content-Type") != "application/json" || key == "" {
			t.Error("event missing durable identity")
		}
		if attemptCount.Add(1) == 1 {
			w.WriteHeader(503)
			attempts <- key
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","saved":1,"failed":0}`))
		attempts <- key
	}))
	defer server.Close()
	cfg := config.Config{AgentMode: "hub", APIBaseURL: server.URL, Agent: config.AgentCfg{Token: "fixture-hub-token"},
		AgentlessOutboxPath: filepath.Join(dir, "routine.db"), AgentlessOutboxMaxMB: 4}
	stop := startSNMPTrapReceiver(context.Background(), cfg, trapACKLogger{ack: acknowledged}, remote.NewAgentlessHubClient(cfg),
		func() usecase.AgentlessSettings { return usecase.AgentlessSettings{Enabled: true} })
	defer stop()
	sendFixtureTrap(t, bind, community)

	select {
	case <-acknowledged:
	case <-time.After(14 * time.Second):
		t.Fatal("did not receive a durable acknowledgement after retry")
	}
	if len(attempts) != 2 {
		t.Fatal("expected exactly two deliveries")
	}
	if first, second := <-attempts, <-attempts; first == "" || first != second {
		t.Fatal("HTTP retry changed the event identity")
	}
	trapPath := cfg.AgentlessOutboxPath + ".snmp-traps"
	if _, err := os.Stat(cfg.AgentlessOutboxPath); !os.IsNotExist(err) {
		t.Fatal("passive receiver wrote to the routine store")
	}
	stop()
	queue, err := agentlessstore.NewBoltStore(trapPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	if pending, _ := queue.Len(); pending != 0 {
		t.Fatal("acknowledged event is still queued")
	}
}

type trapACKLogger struct {
	testBootstrapLogger
	ack chan<- struct{}
}

func (l trapACKLogger) Info(message string) {
	if strings.HasPrefix(message, "agentless delivery acknowledged") {
		l.ack <- struct{}{}
	}
}

func sendFixtureTrap(t *testing.T, addr *net.UDPAddr, community string) {
	t.Helper()
	packet := &gosnmp.SnmpPacket{Version: gosnmp.Version2c, PDUType: gosnmp.SNMPv2Trap, Community: community,
		Variables: []gosnmp.SnmpPDU{
			{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(42)},
			{Name: ".1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.6.3.1.1.5.3"},
		}}
	data, err := packet.MarshalMsg()
	if err != nil {
		t.Fatal(err)
	}
	sender, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	if _, err := sender.Write(data); err != nil {
		t.Fatal(err)
	}
}
