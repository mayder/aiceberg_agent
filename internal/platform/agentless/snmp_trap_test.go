package agentless

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	agentlessstore "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

const fixtureTrapOID = "1.3.6.1.6.3.1.1.5.3"

func fixtureTrapProfile(version string) SNMPTrapProfile {
	return SNMPTrapProfile{SourceIP: "127.0.0.1", Version: version, Community: "fixture-community",
		ClientID: 3, AssetID: 31, CheckID: 32, EndpointID: 33, OIDs: []string{fixtureTrapOID}}
}

func fixtureTrap(t *testing.T, profile SNMPTrapProfile) *gosnmp.SnmpPacket {
	t.Helper()
	packet := &gosnmp.SnmpPacket{Version: gosnmp.Version2c, PDUType: gosnmp.SNMPv2Trap, Community: profile.Community}
	if profile.Version == "v1" {
		packet.Version = gosnmp.Version1
		packet.PDUType = gosnmp.Trap
		packet.SnmpTrap = gosnmp.SnmpTrap{Enterprise: ".1.3.6.1.4.1.123", AgentAddress: "192.0.2.99", GenericTrap: 2, Timestamp: 42}
	} else {
		packet.Variables = []gosnmp.SnmpPDU{
			{Name: "." + uptimeName, Type: gosnmp.TimeTicks, Value: uint32(42)},
			{Name: "." + trapOIDName, Type: gosnmp.ObjectIdentifier, Value: "." + fixtureTrapOID},
			{Name: ".1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: "private fixture description"},
		}
	}
	return packet
}

func trapWire(t *testing.T, packet *gosnmp.SnmpPacket) []byte {
	t.Helper()
	raw, err := packet.MarshalMsg()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSNMPTrapUDPReceivesBothVersionsAndCancels(t *testing.T) {
	for _, version := range []string{"v1", "v2c"} {
		t.Run(version, func(t *testing.T) {
			profile := fixtureTrapProfile(version)
			receiver, err := ListenSNMPTraps("127.0.0.1:0", []SNMPTrapProfile{profile})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "queue.db")
			store, err := agentlessstore.NewBoltStore(path, 1)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			accepted := make(chan entities.AgentlessObservation, 1)
			done := make(chan error, 1)
			go func() {
				done <- receiver.Serve(ctx, func() bool { return true }, func(event entities.AgentlessObservation, proof *entities.SNMPTrapReplayProof) error {
					if err := store.PushSNMPTrap(event, proof); err != nil {
						return err
					}
					accepted <- event
					return nil
				}, func() {})
			}()
			sender, err := net.DialUDP("udp", nil, receiver.conn.LocalAddr().(*net.UDPAddr))
			if err != nil {
				t.Fatal(err)
			}
			defer sender.Close()
			if _, err := sender.Write(trapWire(t, fixtureTrap(t, profile))); err != nil {
				t.Fatal(err)
			}
			select {
			case observation := <-accepted:
				checkTrapObservation(t, profile, observation)
			case <-time.After(2 * time.Second):
				t.Fatal("trap not delivered")
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("receiver did not cancel")
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := agentlessstore.NewBoltStore(path, 1)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			pending, err := reopened.Peek(2)
			if err != nil || len(pending) != 1 || pending[0].DedupeKey == "" || pending[0].SegmentStartedAt == nil {
				t.Fatal("the original event identity did not survive restarting the durable queue")
			}

		})
	}
}

func checkTrapObservation(t *testing.T, profile SNMPTrapProfile, observation entities.AgentlessObservation) {
	t.Helper()
	if observation.CheckID != profile.CheckID || observation.EndpointID == nil || *observation.EndpointID != profile.EndpointID ||
		observation.Code != trapCode || !observation.IsFinal || observation.IsPartial {
		t.Fatal("wrong observation scope or kind")
	}
	event := observation.Payload["snmp_trap_v1"].(map[string]any)
	if event["source_ip"] != profile.SourceIP || event["version"] != profile.Version || event["oid"] != fixtureTrapOID ||
		event["uptime_ticks"] != uint32(42) || event["client_id"] != profile.ClientID || event["asset_id"] != profile.AssetID {
		t.Fatal("wrong normalized trap data")
	}
	data, err := json.Marshal(observation)
	if err != nil || (profile.Community != "" && bytes.Contains(data, []byte(profile.Community))) || bytes.Contains(data, []byte("private fixture description")) ||
		bytes.Contains(data, []byte("192.0.2.99")) {
		t.Fatal("unexpected sensitive data in queued observation")
	}
}

func TestSNMPTrapRejectsWrongCommunityVersionPDUAndOID(t *testing.T) {
	profile := fixtureTrapProfile("v2c")
	source := netip.MustParseAddr(profile.SourceIP)
	for name, modify := range map[string]func(*gosnmp.SnmpPacket){
		"community": func(packet *gosnmp.SnmpPacket) { packet.Community = "another-fixture" },
		"version":   func(packet *gosnmp.SnmpPacket) { packet.Version = gosnmp.Version1 },
		"inform":    func(packet *gosnmp.SnmpPacket) { packet.PDUType = gosnmp.InformRequest },
		"order": func(packet *gosnmp.SnmpPacket) {
			packet.Variables[0], packet.Variables[1] = packet.Variables[1], packet.Variables[0]
		},
		"oid": func(packet *gosnmp.SnmpPacket) { packet.Variables[1].Value = ".1.3.6.1.6.3.1.1.5.4" },
	} {
		t.Run(name, func(t *testing.T) {
			packet := fixtureTrap(t, profile)
			modify(packet)
			if _, ok := trapObservation(trapWire(t, packet), source, profile, time.Now()); ok {
				t.Fatal("unexpected accepted trap")
			}
		})
	}
}

func TestSNMPTrapV1EnterpriseOID(t *testing.T) {
	profile := fixtureTrapProfile("v1")
	profile.OIDs = []string{"1.3.6.1.4.1.123.0.77"}
	packet := fixtureTrap(t, profile)
	packet.GenericTrap, packet.SpecificTrap = 6, 77
	observation, ok := trapObservation(trapWire(t, packet), netip.MustParseAddr(profile.SourceIP), profile, time.Now())
	if !ok || observation.Payload["snmp_trap_v1"].(map[string]any)["oid"] != profile.OIDs[0] {
		t.Fatal("wrong enterprise-specific trap OID")
	}
}
