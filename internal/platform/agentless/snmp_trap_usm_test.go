package agentless

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func fixtureUSM() SNMPTrapUSMProfile {
	return SNMPTrapUSMProfile{EngineID: "80001f880102030405", User: "fixture-traps",
		AuthProtocol: "sha256", AuthPassword: "fixture-auth-only", PrivPassword: "fixture-priv-only"}
}

func fixtureV3TrapWire(t *testing.T, settings SNMPTrapUSMProfile, flags gosnmp.SnmpV3MsgFlags) []byte {
	t.Helper()
	engine, err := hex.DecodeString(settings.EngineID)
	if err != nil {
		t.Fatal(err)
	}
	protocol, _, _ := trapAuthAlgorithm(settings.AuthProtocol)
	security := &gosnmp.UsmSecurityParameters{
		AuthoritativeEngineID: string(engine), AuthoritativeEngineBoots: 7, AuthoritativeEngineTime: 302,
		UserName: settings.User, AuthenticationProtocol: protocol, PrivacyProtocol: gosnmp.AES,
		AuthenticationPassphrase: settings.AuthPassword, PrivacyPassphrase: settings.PrivPassword,
		PrivacyParameters: []byte{11, 12, 13, 14, 15, 16, 17, 18},
	}
	if err := security.InitSecurityKeys(); err != nil {
		t.Fatal(err)
	}
	packet := fixtureTrap(t, fixtureTrapProfile("v2c"))
	packet.Version, packet.MsgFlags, packet.SecurityModel = gosnmp.Version3, flags, gosnmp.UserSecurityModel
	packet.SecurityParameters, packet.ContextEngineID, packet.ContextName = security, string(engine), settings.Context
	packet.MsgID, packet.MsgMaxSize = 0x12345678, 65507
	return trapWire(t, packet)
}

func TestSNMPv3TrapAuthPrivAndRedaction(t *testing.T) {
	for _, protocol := range []string{"sha", "sha224", "sha256", "sha384", "sha512"} {
		t.Run(protocol, func(t *testing.T) {
			settings := fixtureUSM()
			settings.AuthProtocol = protocol
			usm, err := newTrapUSM(&settings)
			if err != nil {
				t.Fatal(err)
			}
			wire := fixtureV3TrapWire(t, settings, gosnmp.AuthPriv)
			profile := fixtureTrapProfile("v3")
			now := time.Unix(1780000000, 0)
			peer := trapPeer{profile: profile, usm: usm}
			event, proof, ok := peer.observation(wire, netip.MustParseAddr(profile.SourceIP), now)
			if !ok || proof == nil || proof.Boots != 7 || proof.EngineTime != 302 || proof.ReceivedAt != now.Unix() {
				t.Fatal("valid USM notification not decoded")
			}
			checkTrapObservation(t, profile, event)
			raw, err := json.Marshal(event)
			if err != nil || bytes.Contains(raw, []byte(settings.AuthPassword)) || bytes.Contains(raw, []byte(settings.PrivPassword)) ||
				bytes.Contains(raw, []byte(settings.User)) || bytes.Contains(raw, []byte(settings.EngineID)) {
				t.Fatal("USM configuration was exposed in the event")
			}
		})
	}
}

func TestSNMPv3TrapRequiresConfiguredIdentityAndBothSecurityServices(t *testing.T) {
	settings := fixtureUSM()
	receiver, err := newTrapUSM(&settings)
	if err != nil {
		t.Fatal(err)
	}
	for name, modify := range map[string]func(*SNMPTrapUSMProfile) gosnmp.SnmpV3MsgFlags{
		"other user": func(s *SNMPTrapUSMProfile) gosnmp.SnmpV3MsgFlags { s.User = "other-fixture"; return gosnmp.AuthPriv },
		"other engine": func(s *SNMPTrapUSMProfile) gosnmp.SnmpV3MsgFlags {
			s.EngineID = "80001f880101010101"
			return gosnmp.AuthPriv
		},
		"other auth": func(s *SNMPTrapUSMProfile) gosnmp.SnmpV3MsgFlags {
			s.AuthPassword = "different-auth"
			return gosnmp.AuthPriv
		},
		"other priv": func(s *SNMPTrapUSMProfile) gosnmp.SnmpV3MsgFlags {
			s.PrivPassword = "different-priv"
			return gosnmp.AuthPriv
		},
		"other context": func(s *SNMPTrapUSMProfile) gosnmp.SnmpV3MsgFlags {
			s.Context = "other-context"
			return gosnmp.AuthPriv
		},
		"auth only": func(*SNMPTrapUSMProfile) gosnmp.SnmpV3MsgFlags { return gosnmp.AuthNoPriv },
		"plaintext": func(*SNMPTrapUSMProfile) gosnmp.SnmpV3MsgFlags { return gosnmp.NoAuthNoPriv },
	} {
		t.Run(name, func(t *testing.T) {
			other := settings
			flags := modify(&other)
			if packet, proof := receiver.decode(fixtureV3TrapWire(t, other, flags), time.Now()); packet != nil || proof != nil {
				t.Fatal("unexpected permission to ingest notification")
			}
		})
	}
}

func TestSNMPv3TrapFromNetSNMP(t *testing.T) {
	path := os.Getenv("AICEBERG_SNMPTRAP_BIN")
	if path == "" {
		t.Skip("set AICEBERG_SNMPTRAP_BIN to a Net-SNMP sender")
	}
	settings := fixtureUSM()
	settings.AuthProtocol = "sha"
	profile := fixtureTrapProfile("v3")
	profile.USM, profile.Community = &settings, ""
	receiver, err := ListenSNMPTraps("127.0.0.1:0", []SNMPTrapProfile{profile})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	done := make(chan error, 1)
	observations := make(chan entities.AgentlessObservation, 1)
	go func() {
		done <- receiver.Serve(ctx, func() bool { return true }, func(event entities.AgentlessObservation, proof *entities.SNMPTrapReplayProof) error {
			if proof == nil {
				t.Error("external trap has no authenticated replay proof")
			}
			observations <- event
			return nil
		}, func() {})
	}()
	addr := receiver.conn.LocalAddr().(*net.UDPAddr)
	cmd := exec.CommandContext(ctx, path, "-v", "3", "-l", "authPriv", "-u", settings.User,
		"-a", "SHA", "-A", settings.AuthPassword, "-x", "AES", "-X", settings.PrivPassword,
		"-e", "0x"+settings.EngineID, "-E", "0x"+settings.EngineID, "-Z", "7,302",
		"udp:"+addr.String(), "42", "."+fixtureTrapOID)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Net-SNMP fixture could not deliver: %v %s", err, output)
	}
	select {
	case observation := <-observations:
		checkTrapObservation(t, profile, observation)
	case <-ctx.Done():
		t.Fatal("no trap accepted from Net-SNMP")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
