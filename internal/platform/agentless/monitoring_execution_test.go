package agentless

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/common/config"
	local "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestCanonicalSNMPUDPOutboxHTTPToPHP(t *testing.T) {
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		t.Skip("explicit integrated Web checkout required")
	}
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	done := make(chan error, 1)
	go func() { done <- serveCanonicalSNMP(conn, &calls) }()
	defer func() {
		_ = conn.Close()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	job := newSNMPJob()
	job.ClienteID, job.AtivoID, job.CheckID, job.IntervalSec = 3, 52, 72, 60
	job.Endpoint.Endereco = "127.0.0.1"
	job.SNMP.Version, job.SNMP.Community, job.SNMP.Port = "v1", "fixture-canonical-private", conn.LocalAddr().(*net.UDPAddr).Port
	job.Config["groups"] = []string{"system"}
	job.Config["exclude_groups"] = []string{"system"}
	job.Config["fetch_mode"] = "get_only"
	job.Config["custom_get_oids"] = []any{
		map[string]any{"oid": "1.3.6.1.4.1.99999.1.0", "canonical_key": "cpu_percent", "unit": "percent"},
		map[string]any{"oid": "1.3.6.1.4.1.99999.2.0", "canonical_key": "memory_percent", "unit": "percent"},
	}
	observation := RunJob(context.Background(), job)
	if observation.Status != "ok" || observation.MonitoringSamples == nil {
		t.Fatalf("collection/canonical metadata missing: %s", observation.Status)
	}
	beforeCalls := calls.Load()
	if beforeCalls != 2 {
		t.Fatalf("expected two OIDs in one collection, got %d", beforeCalls)
	}
	originalPayload, _ := json.Marshal(observation.Payload)
	storePath := filepath.Join(t.TempDir(), "outbox.db")
	store, err := local.NewBoltStore(storePath, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Push(observation); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = local.NewBoltStore(storePath, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	batch, err := store.Peek(1)
	if err != nil || len(batch) != 1 || !reflect.DeepEqual(batch[0].MonitoringSamples, observation.MonitoringSamples) {
		t.Fatal("durable metadata changed", err)
	}
	persistedPayload, _ := json.Marshal(batch[0].Payload)
	if string(originalPayload) != string(persistedPayload) || batch[0].ID != observation.ID || batch[0].DedupeKey != observation.DedupeKey {
		t.Fatal("legacy payload/identity changed")
	}
	var accepted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token fixture-canonical-auth" || r.URL.Path != "/v1/hub-agentless/observations" {
			w.WriteHeader(403)
			return
		}
		if !accepted.Load() {
			w.WriteHeader(503)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "php", filepath.Join(root, "common/tests/_support/canonical-snmp-delivery.php"))
		cmd.Env = append(os.Environ(), "CANONICAL_SNMP_MYSQL_DSN="+os.Getenv("CANONICAL_SNMP_MYSQL_DSN"))
		cmd.Stdin = strings.NewReader(string(raw))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("PHP persistence/consumer: %v %s", err, output)
			w.WriteHeader(500)
			return
		}
		var receipt map[string]any
		if json.Unmarshal(output, &receipt) != nil || receipt["samples_equal"] != true || receipt["canonical_scope_verified"] != true {
			t.Error("invalid consumer receipt")
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(output)
	}))
	defer server.Close()
	client := remote.NewAgentlessHubClient(config.Config{APIBaseURL: server.URL, Agent: config.AgentCfg{Token: "fixture-canonical-auth"}, AgentClientID: 3, AgentID: 42})
	if err = client.SendObservations(context.Background(), batch); err == nil {
		t.Fatal("HTTP failure acknowledged")
	}
	pending, _ := store.Peek(1)
	if len(pending) != 1 {
		t.Fatal("unacknowledged collection lost")
	}
	accepted.Store(true)
	if err = client.SendObservations(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	if err = store.Delete([]string{batch[0].ID}); err != nil {
		t.Fatal(err)
	} // Only this owned synthetic outbox.
	pending, _ = store.Peek(1)
	if len(pending) != 0 || calls.Load() != beforeCalls {
		t.Fatal("transport recollected or failed to clear acknowledged fixture")
	}
}

func serveCanonicalSNMP(conn net.PacketConn, calls *atomic.Int64) error {
	decoder := *gosnmp.Default
	decoder.Version = gosnmp.Version1
	decoder.Community = "fixture-canonical-private"
	buffer := make([]byte, 4096)
	for {
		n, peer, err := conn.ReadFrom(buffer)
		if err != nil {
			if strings.Contains(err.Error(), "closed") {
				return nil
			}
			return err
		}
		packet, err := decoder.SnmpDecodePacket(buffer[:n])
		if err != nil {
			return err
		}
		calls.Add(int64(len(packet.Variables)))
		vars := make([]gosnmp.SnmpPDU, 0, len(packet.Variables))
		for _, v := range packet.Variables {
			value := 17
			if strings.HasSuffix(v.Name, ".2.0") {
				value = 62
			}
			vars = append(vars, gosnmp.SnmpPDU{Name: v.Name, Type: gosnmp.Integer, Value: value})
		}
		response := gosnmp.SnmpPacket{Version: packet.Version, Community: packet.Community, PDUType: gosnmp.GetResponse, RequestID: packet.RequestID, Variables: vars}
		raw, err := response.MarshalMsg()
		if err != nil {
			return err
		}
		if _, err = conn.WriteTo(raw, peer); err != nil {
			return err
		}
	}
}

func TestMonitoringMetadataTracksPartialsAndNeverAdoptsDiagnostics(t *testing.T) {
	job := newSNMPJob()
	job.ClienteID, job.AtivoID, job.CheckID = 3, 52, 72
	at := time.Unix(1800000000, 0)
	o := entities.AgentlessObservation{CheckID: 72, Status: "ok", IsFinal: true, ObservedAt: at, Payload: map[string]any{"cpu_percent": 17, "memory_percent": 62}}
	first := withMonitoringSamples(job, o)
	if first.MonitoringSamples == nil || first.MonitoringSamples.Samples[0].State != "ok" {
		t.Fatal("routine final samples absent")
	}
	o.IsPartial, o.IsFinal = true, false
	partial := withMonitoringSamples(job, o)
	if partial.MonitoringSamples.Samples[0].Gap != "partial" || partial.MonitoringSamples.Samples[0].Value != nil {
		t.Fatal("partial promoted")
	}
	job.Config["diagnostic_probe"] = true
	o.MonitoringSamples = first.MonitoringSamples
	if withMonitoringSamples(job, o).MonitoringSamples != nil {
		t.Fatal("diagnostic became routine metric")
	}
	delete(job.Config, "diagnostic_probe")
	o.ObservedAt = time.Time{}
	if withMonitoringSamples(job, o).MonitoringSamples != nil {
		t.Fatal("invalid clock emitted")
	}
	o.ObservedAt = at
	o.CheckID = 73
	if withMonitoringSamples(job, o).MonitoringSamples != nil {
		t.Fatal("foreign observation projected")
	}
	o.CheckID = 72
	job.Tipo = "discovery"
	if withMonitoringSamples(job, o).MonitoringSamples != nil {
		t.Fatal("unsupported collector projected")
	}
	job.Tipo = "snmp"
	job.ClienteID = 0
	if withMonitoringSamples(job, o).MonitoringSamples != nil {
		t.Fatal("invalid scope guessed")
	}
}
