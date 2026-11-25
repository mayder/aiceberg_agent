package usecase

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
	store "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

type fixtureAssignmentClient struct {
	offer     entities.RelaySNMPOffer
	job       entities.RelaySNMPAssignment
	result    entities.AgentlessObservation
	reportErr error
	onReport  func(entities.AgentlessObservation)
	claims    int
	reports   int
}

func (f *fixtureAssignmentClient) FetchAssignmentOffer(context.Context) (entities.RelaySNMPOffer, error) {
	return f.offer, nil
}

func (f *fixtureAssignmentClient) ClaimAssignment(context.Context, entities.RelaySNMPOffer) (entities.RelaySNMPAssignment, error) {
	f.claims++
	return f.job, nil
}

func (f *fixtureAssignmentClient) ReportAssignment(_ context.Context, observation entities.AgentlessObservation) error {
	f.result = observation
	f.reports++
	if f.onReport != nil {
		f.onReport(observation)
	}
	return f.reportErr
}

func TestRelayAssignmentVerifiesJobAndRetriesDeliveryWithoutRepeatingSNMP(t *testing.T) {
	current := time.Now()
	sensor, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sensor.Close()
	sensorPort := sensor.LocalAddr().(*net.UDPAddr).Port
	done := make(chan error, 1)
	go func() { done <- serveDelegatedSNMPFixture(sensor) }()
	raw := []byte(fmt.Sprintf(`{
		"check_id":72,"ativo_id":52,"cliente_id":3,"tipo":"snmp","command_id":"fixture-cycle-1",
		"credential_delivery":"reference-v1","interval_sec":60,"timeout_ms":1500,"time_budget_ms":1500,
		"probe_policy_v1":{"contract":"monitoring.probe-policy.v1","budget_ms":1500,"freshness_sec":180,"credential_ref":"snmp_profile:9"},
		"endpoint":{"id":14,"endereco":"127.0.0.1"},
		"config":{"groups":["system"],"exclude_groups":["system"],"fetch_mode":"get_only","custom_get_oids":[
			{"oid":"1.3.6.1.4.1.99999.1.0","canonical_key":"cpu_percent","unit":"percent"}]},
		"snmp":{"profile_id":9,"binding_id":11,"version":"v1","port":%d,"timeout_ms":1500}
	}`, sensorPort))
	client, public := makeAssignmentFixture(current, raw)
	dir := t.TempDir()
	path := filepath.Join(dir, "relay-assignment.db")
	runs, lookups := 0, 0
	newWorker := func() (*RelaySNMPAssignmentWorker, *store.BoltStore) {
		disk, err := store.NewBoltStore(path, 1)
		if err != nil {
			t.Fatal(err)
		}
		w := NewRelaySNMPAssignmentWorker(client, repositories.NewAgentlessOutboxRepository(disk), disk,
			monitoring.CollectorAssignmentScope{ClientID: 3, HubID: 84, OwnerID: 424}, public,
			func(ctx context.Context, job entities.AgentlessJob) entities.AgentlessObservation {
				runs++
				deadline, hasDeadline := ctx.Deadline()
				if !hasDeadline || deadline.UnixMilli() != client.job.Grant.ExpiresAtMS || job.CheckID != 72 {
					t.Error("leased execution context missing")
				}
				return agentless.RunJobWithCredentials(ctx, job, nil, func(_ context.Context, scoped entities.AgentlessJob) (*entities.AgentlessSnmpProfile, error) {
					lookups++
					if scoped.SNMP.Community != "" || scoped.ProbePolicy.CredentialRef != "snmp_profile:9" ||
						scoped.CommandID != "fixture-cycle-1" || scoped.SNMP.Port != sensorPort {
						t.Error("credential requested outside the signed job")
					}
					profile := *scoped.SNMP
					profile.Community = "fixture-delegated-community"
					return &profile, nil
				})
			})
		return w, disk
	}
	w, disk := newWorker()
	client.offer.Epoch++
	if err := w.Tick(context.Background()); !errors.Is(err, monitoring.ErrCollectorAssignmentInvalid) || runs != 0 {
		t.Fatalf("out-of-epoch execution: runs=%d err=%v", runs, err)
	}
	client.offer.Epoch--
	validOffer := client.offer
	client.onReport = func(result entities.AgentlessObservation) {
		t.Helper()
		persisted, err := disk.Peek(1)
		if err != nil || len(persisted) != 1 || persisted[0].DedupeKey != result.DedupeKey {
			t.Fatalf("result was reported before durable append: count=%d, %v", len(persisted), err)
		}
	}
	client.reportErr = errors.New("fixture delivery unavailable")
	if err := w.Tick(context.Background()); !errors.Is(err, client.reportErr) || runs != 1 || client.reports != 1 {
		t.Fatalf("valid job did not attempt delivery in the same tick: runs=%d reports=%d err=%v", runs, client.reports, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = disk.Close()
	w, disk = newWorker()
	defer disk.Close()
	client.reportErr = nil
	client.offer.Status = "idle"
	if err := w.Tick(context.Background()); err != nil || client.reports != 2 || client.result.DedupeKey == "" ||
		client.result.Assignment == nil || client.result.Assignment.Epoch != validOffer.Epoch {
		t.Fatal("persisted result lost assignment identity or was not retried")
	}
	client.offer = validOffer
	payload, err := json.Marshal(client.result)
	if err != nil || client.result.Status != "ok" || client.result.ProbeReport == nil || client.result.ProbeReport.Outcome != "ok" ||
		!strings.Contains(string(payload), `"cpu_percent":17`) || strings.Contains(string(payload), "fixture-delegated-community") {
		t.Fatalf("delegated SNMP result failed qualification or redaction: %s, %v", client.result.Code, err)
	}
	if err := w.Tick(context.Background()); err != nil || runs != 1 || lookups != 1 || client.claims != 3 || client.reports != 2 {
		t.Fatalf("replay executed a second SNMP job: runs=%d lookups=%d claims=%d err=%v", runs, lookups, client.claims, err)
	}
	if err := sensor.SetReadDeadline(time.Now().Add(25 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sensor.ReadFrom(make([]byte, 2048)); err == nil {
		t.Fatal("delegated replay sent another UDP request")
	}
}

func serveDelegatedSNMPFixture(conn net.PacketConn) error {
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	buf := make([]byte, 2048)
	n, peer, err := conn.ReadFrom(buf)
	if err != nil {
		return err
	}
	decoder := *gosnmp.Default
	packet, err := decoder.SnmpDecodePacket(buf[:n])
	if err != nil {
		return err
	}
	if packet.Version != gosnmp.Version1 || packet.PDUType != gosnmp.GetRequest || packet.Community != "fixture-delegated-community" ||
		len(packet.Variables) != 1 || packet.Variables[0].Name != ".1.3.6.1.4.1.99999.1.0" {
		return errors.New("unexpected delegated SNMPv1 request")
	}
	response := &gosnmp.SnmpPacket{Version: packet.Version, Community: packet.Community, PDUType: gosnmp.GetResponse, RequestID: packet.RequestID,
		Variables: []gosnmp.SnmpPDU{{Name: packet.Variables[0].Name, Type: gosnmp.Integer, Value: 17}}}
	raw, err := response.MarshalMsg()
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(raw, peer)
	return err
}

func makeAssignmentFixture(now time.Time, raw []byte) (*fixtureAssignmentClient, ed25519.PublicKey) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	grant := monitoring.CollectorAssignmentGrant{Version: 1, ClientID: 3, CheckID: 72, HubID: 84, OwnerID: 424, Epoch: 2,
		ExpiresAtMS: now.Add(30 * time.Second).UnixMilli(), JobSHA256: fmt.Sprintf("%x", sha256.Sum256(raw))}
	msg := fmt.Sprintf("aiceberg.monitoring.collector-assignment.v1\n3\n72\n84\n424\n2\n%d\n%s\n", grant.ExpiresAtMS, grant.JobSHA256)
	grant.Signature = hex.EncodeToString(ed25519.Sign(key, []byte(msg)))
	return &fixtureAssignmentClient{
		offer: entities.RelaySNMPOffer{Status: "offered", CheckID: 72, Epoch: 2},
		job:   entities.RelaySNMPAssignment{Status: "ready", Grant: grant, Job: raw},
	}, key.Public().(ed25519.PublicKey)
}
