package agentless

import (
	"context"
	"net"
	"testing"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
)

// Actual local UDP traffic with synthetic replies, never physical acceptance.
func TestBasicProbeRealUDPWireEvidence(t *testing.T) {
	for _, version := range []string{"v1", "v2c"} {
		t.Run(version, func(t *testing.T) {
			conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			done := make(chan error, 1)
			go func() { done <- serveBasicFixture(conn) }()
			job := newSNMPJob()
			job.Endpoint.Endereco = "127.0.0.1"
			job.SNMP.Version, job.SNMP.Port, job.SNMP.Community = version, conn.LocalAddr().(*net.UDPAddr).Port, "fixture-basic-private"
			job.SNMP.Retries = 7
			job.Config["snmp_basic_probe"], job.Config["snmp_basic_oid"] = true, "1.3.6.1.2.1.1.3.0"
			result := runSNMP(context.Background(), job)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if result.Status != "ok" {
				t.Fatalf("probe failed: %s", result.Code)
			}
			report := result.Payload["basic_probe"].(map[string]any)
			if report["sent_datagrams"] != float64(1) || report["received_datagrams"] != float64(1) || report["response_received"] != true || report["response_version"] != version {
				t.Fatalf("wire evidence lost: %+v", report)
			}
			// No second GET, retry, GETNEXT or GETBULK is allowed after the reply.
			_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			if _, _, err = conn.ReadFrom(make([]byte, 2048)); err == nil {
				t.Fatal("unexpected additional request")
			}
		})
	}
}

func serveBasicFixture(conn net.PacketConn) error {
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buffer := make([]byte, 2048)
	n, peer, err := conn.ReadFrom(buffer)
	if err != nil {
		return err
	}
	decoder := *gosnmp.Default
	packet, err := decoder.SnmpDecodePacket(buffer[:n])
	if err != nil {
		return err
	}
	if packet.PDUType != gosnmp.GetRequest || len(packet.Variables) != 1 || packet.Community != "fixture-basic-private" {
		return &basicFixtureError{}
	}
	response := &gosnmp.SnmpPacket{Version: packet.Version, Community: packet.Community, PDUType: gosnmp.GetResponse, RequestID: packet.RequestID,
		Variables: []gosnmp.SnmpPDU{{Name: packet.Variables[0].Name, Type: gosnmp.TimeTicks, Value: uint32(12345)}}}
	encoded, err := response.MarshalMsg()
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(encoded, peer)
	return err
}

type basicFixtureError struct{}

func (*basicFixtureError) Error() string { return "unexpected basic probe request" }

func TestBasicProbeUDPTimeoutDoesNotClaimAuthenticationOrResponse(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	job := newSNMPJob()
	job.Endpoint.Endereco = "127.0.0.1"
	job.SNMP.Version, job.SNMP.Port = "v1", conn.LocalAddr().(*net.UDPAddr).Port
	job.SNMP.Retries = 9
	job.Config["snmp_basic_probe"], job.Config["snmp_basic_oid"] = true, "1.3.6.1.2.1.1.3.0"
	started := time.Now()
	result := runSNMP(context.Background(), job)
	report := result.Payload["basic_probe"].(map[string]any)
	if result.Code != "snmp_timeout" || report["response_received"] != false || report["sent_datagrams"] != float64(1) || report["received_datagrams"] != float64(0) {
		t.Fatalf("timeout evidence incorrect: %+v", report)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("unexpected retries or budget exceeded")
	}
}
