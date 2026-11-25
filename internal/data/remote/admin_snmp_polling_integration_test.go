package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	local "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

func TestAdminSNMPPollingFromWebToUDPAndBack(t *testing.T) {
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
	store, err := local.NewBoltStore(filepath.Join(t.TempDir(), "snmp-outbox.db"), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var lastID int64
	for cycle := range 3 {
		lastID = verifyAdminSNMPCycle(t, ctx, bridge, client, store, lastID, "ok", "finished", 12345)
		if requests.Load() != int64(7*(cycle+1)) {
			t.Fatalf("unexpected count of real system GET requests: %d", requests.Load())
		}
	}
	silent.Store(true)
	lastID = verifyAdminSNMPCycle(t, ctx, bridge, client, store, lastID, "timeout", "failed", 0)
	afterTimeout := requests.Load()
	if afterTimeout <= 21 {
		t.Fatal("the timeout test did not send a UDP request")
	}
	var beforeDenial, afterDenial map[string]any
	bridge.fixture(t, map[string]any{"path": "/fixture/check-state"}, &beforeDenial)
	verifyAdminSNMPCycle(t, ctx, bridge, client, store, lastID, "failed", "failed", 0)
	bridge.fixture(t, map[string]any{"path": "/fixture/check-state"}, &afterDenial)
	if requests.Load() != afterTimeout {
		t.Fatal("the HUB reached the device after credential resolution was denied")
	}
	if len(beforeDenial) == 0 || !reflect.DeepEqual(beforeDenial, afterDenial) {
		t.Fatal("credential denial changed the check availability state")
	}
}

type adminSNMPCommand struct {
	CheckID       int    `json:"check_id"`
	CommandID     string `json:"command_id"`
	CorrelationID string `json:"correlation_id"`
}

func verifyAdminSNMPCycle(t *testing.T, ctx context.Context, bridge *adminSNMPBridge, client *AgentlessHubClient,
	store *local.BoltStore, lastID int64, outcome, executionStatus string, sample int,
) int64 {
	t.Helper()
	var command adminSNMPCommand
	bridge.fixture(t, map[string]any{"path": "/fixture/queue", "method": "POST"}, &command)
	jobs, err := client.FetchJobsWithOptions(ctx, 1, true, 60, AgentlessFetchOptions{
		CommandID: command.CommandID, CorrelationID: command.CorrelationID, CheckIDs: []int{command.CheckID},
	})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("could not obtain the Admin polling job: jobs=%d err=%v", len(jobs), err)
	}
	job := jobs[0]
	if job.CommandID != command.CommandID || job.CredentialDelivery != "reference-v1" || job.SNMP == nil || job.SNMP.Community != "" {
		t.Fatal("incorrect polling scope or inline credential")
	}
	if outcome == "failed" {
		var control []any
		bridge.fixture(t, map[string]any{"path": "/fixture/revoke-snmpv1"}, &control)
	}
	observation := agentless.RunJobWithCredentials(ctx, job, nil, client.ResolveSNMPCredentials)
	if observation.ProbeReport == nil || observation.ProbeReport.Outcome != outcome {
		t.Fatalf("polling probe failed: %s", observation.Code)
	}
	if err := store.Push(observation); err != nil {
		t.Fatal(err)
	}
	batch, err := store.Peek(1)
	if err != nil || len(batch) != 1 {
		t.Fatal("outbox did not keep the observation", err)
	}
	wire, err := json.Marshal(batch)
	if err != nil || bytes.Contains(wire, []byte("synthetic-snmp-secret")) {
		t.Fatal("SNMP credentials escaped into the outbox")
	}
	if lastID == 0 {
		verifyAdminSNMPReceiptRollback(t, ctx, bridge, client, store, batch, command.CommandID)
	}
	if err := client.SendObservations(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete([]string{batch[0].ID}); err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Status           string `json:"status"`
		ObservationID    int64  `json:"observation_id"`
		EndpointID       int    `json:"endpoint_id"`
		Source           string `json:"source"`
		Code             string `json:"code"`
		SNMPVersion      string `json:"snmp_version"`
		Sample           int    `json:"sample"`
		ProbeOutcome     string `json:"probe_outcome"`
		CredentialLeaked bool   `json:"credential_leaked"`
		ObservedAt       string `json:"observed_at"`
	}
	query := map[string]any{"path": "/fixture/receipt", "command_id": command.CommandID}
	bridge.fixture(t, query, &receipt)
	if receipt.Status != executionStatus || receipt.ObservationID <= lastID || receipt.Source != "hub" ||
		receipt.EndpointID != job.Endpoint.ID || receipt.Sample != sample || receipt.ProbeOutcome != outcome ||
		receipt.CredentialLeaked || receipt.ObservedAt == "" {
		t.Fatalf("Admin receipt does not match the UDP result: %+v", receipt)
	}
	if outcome == "failed" {
		if receipt.Code != "snmp_credential_unavailable" || observation.ProbeReport.Reason != "configuration" ||
			observation.Payload["diagnostic_probe"] == true {
			t.Fatalf("credential denial must complete the Admin polling command: %+v", receipt)
		}
	} else if receipt.SNMPVersion != "v1" {
		t.Fatalf("incorrect SNMP version in receipt: %+v", receipt)
	}
	receiptID := receipt.ObservationID
	if err := client.SendObservations(ctx, batch); err != nil {
		t.Fatal("the same observation was not safe to redeliver", err)
	}
	bridge.fixture(t, query, &receipt)
	if receipt.ObservationID != receiptID || receipt.Status != executionStatus {
		t.Fatal("redelivery duplicated or changed the Admin receipt")
	}
	return receiptID
}

func verifyAdminSNMPReceiptRollback(t *testing.T, ctx context.Context, bridge *adminSNMPBridge, client *AgentlessHubClient,
	store *local.BoltStore, batch []entities.AgentlessObservation, commandID string,
) {
	t.Helper()
	var control []any
	bridge.fixture(t, map[string]any{"path": "/fixture/block-receipt-write"}, &control)
	var failure *AgentlessDeliveryError
	err := client.SendObservations(ctx, batch)
	if !errors.As(err, &failure) || failure.PayloadRejected {
		t.Fatalf("receipt failure should return a retryable error: %v", err)
	}
	bridge.fixture(t, map[string]any{"path": "/fixture/delivery-pending", "command_id": commandID}, &control)
	var receipt struct {
		Status             string `json:"status"`
		StoredObservations int    `json:"stored_observations"`
	}
	bridge.fixture(t, map[string]any{"path": "/fixture/receipt", "command_id": commandID}, &receipt)
	if receipt.Status != "received" || receipt.StoredObservations != 0 {
		t.Fatalf("failed receipt left committed state behind: %+v", receipt)
	}
	pending, err := store.Peek(1)
	if err != nil || len(pending) != 1 || pending[0].ID != batch[0].ID {
		t.Fatal("the observation was lost from the outbox after a failed receipt", err)
	}
	bridge.fixture(t, map[string]any{"path": "/fixture/restore-receipt-write"}, &control)
}

type adminSNMPBridge struct {
	mu     sync.Mutex
	stdin  *json.Encoder
	stdout *json.Decoder
}

func startAdminSNMPBridge(t *testing.T, ctx context.Context, root string, port int) *adminSNMPBridge {
	t.Helper()
	command := exec.CommandContext(ctx, "php", filepath.Join(root, "common", "tests", "_support", "admin-snmp-polling-bridge.php"), strconv.Itoa(port))
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = command.Wait()
	})
	return &adminSNMPBridge{stdin: json.NewEncoder(stdin), stdout: json.NewDecoder(stdout)}
}

func (b *adminSNMPBridge) roundTrip(input map[string]any) (int, json.RawMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var reply struct {
		Status int
		Body   json.RawMessage
	}
	if err := b.stdin.Encode(input); err != nil {
		return 0, nil, err
	}
	err := b.stdout.Decode(&reply)
	return reply.Status, reply.Body, err
}

func (b *adminSNMPBridge) fixture(t *testing.T, input map[string]any, output any) {
	t.Helper()
	status, body, err := b.roundTrip(input)
	if err != nil || status != http.StatusOK {
		t.Fatalf("fixture %v unavailable: status=%d err=%v", input["path"], status, err)
	}
	if err := json.Unmarshal(body, output); err != nil {
		t.Fatal(err)
	}
}

func (b *adminSNMPBridge) serveHTTP(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var body map[string]any
	if len(raw) != 0 && json.Unmarshal(raw, &body) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	query := make(map[string]string)
	for key := range r.URL.Query() {
		query[key] = r.URL.Query().Get(key)
	}
	status, output, err := b.roundTrip(map[string]any{
		"path": r.URL.Path, "method": r.Method, "query": query, "body": body, "authorization": r.Header.Get("Authorization"),
	})
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(output)
}

func startAdminSNMPDevice(t *testing.T) (int, *atomic.Int64, *atomic.Bool) {
	t.Helper()
	socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int64
	var silent atomic.Bool
	done := make(chan error, 1)
	go func() { done <- serveAdminSNMPDevice(socket, &requests, &silent) }()
	t.Cleanup(func() {
		_ = socket.Close()
		if err := <-done; err != nil && !errors.Is(err, net.ErrClosed) {
			t.Error(err)
		}
	})
	return socket.LocalAddr().(*net.UDPAddr).Port, &requests, &silent
}

func serveAdminSNMPDevice(socket net.PacketConn, requests *atomic.Int64, silent *atomic.Bool) error {
	buffer := make([]byte, 2048)
	decoder := *gosnmp.Default
	for {
		n, peer, err := socket.ReadFrom(buffer)
		if err != nil {
			return err
		}
		req, err := decoder.SnmpDecodePacket(buffer[:n])
		if err != nil || req.Version != gosnmp.Version1 || req.Community != "synthetic-snmp-secret" ||
			req.PDUType != gosnmp.GetRequest || len(req.Variables) != 1 {
			return errors.New("unexpected UDP poll in the SNMPv1 loopback fixture")
		}
		binding, err := adminSNMPSystemValue(req.Variables[0].Name)
		if err != nil {
			return err
		}
		requests.Add(1)
		if silent.Load() {
			continue
		}
		reply, err := (&gosnmp.SnmpPacket{Version: req.Version, Community: req.Community, RequestID: req.RequestID,
			PDUType: gosnmp.GetResponse, Variables: []gosnmp.SnmpPDU{binding}}).MarshalMsg()
		if err != nil {
			return err
		}
		if _, err := socket.WriteTo(reply, peer); err != nil {
			return err
		}
	}
}

func adminSNMPSystemValue(oid string) (gosnmp.SnmpPDU, error) {
	value := gosnmp.SnmpPDU{Name: oid, Type: gosnmp.OctetString, Value: "fixture"}
	switch oid {
	case ".1.3.6.1.2.1.1.3.0":
		value.Type, value.Value = gosnmp.TimeTicks, uint32(12345)
	case ".1.3.6.1.2.1.1.2.0":
		value.Type, value.Value = gosnmp.ObjectIdentifier, ".1.3.6.1.4.1.8072"
	case ".1.3.6.1.2.1.1.7.0":
		value.Type, value.Value = gosnmp.Integer, 2
	case ".1.3.6.1.2.1.1.1.0", ".1.3.6.1.2.1.1.4.0", ".1.3.6.1.2.1.1.5.0", ".1.3.6.1.2.1.1.6.0":
	default:
		return gosnmp.SnmpPDU{}, fmt.Errorf("unexpected OID in system polling fixture: %q", oid)
	}
	return value, nil
}
