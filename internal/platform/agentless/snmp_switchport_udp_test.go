package agentless

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestSNMPSwitchportWireOverUDP(t *testing.T) {
	cases := []struct {
		name    string
		trunk   bool
		maxRows int
	}{
		{name: "access", maxRows: 10},
		{name: "truncated", trunk: true, maxRows: 1},
		{name: "trunk", trunk: true, maxRows: 10},
	}
	wire := make(map[string]map[string]any, len(cases))
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- serveSwitchportFixture(conn, switchportFixtureNext(fixture.trunk)) }()
			t.Cleanup(func() {
				_ = conn.Close()
				if err := <-done; err != nil {
					t.Error(err)
				}
			})
			job := newSNMPJob()
			job.Endpoint.Endereco = "127.0.0.1"
			job.SNMP.Version, job.SNMP.Community = "v1", "fixture-switchport"
			job.SNMP.Port = conn.LocalAddr().(*net.UDPAddr).Port
			job.Config["groups"] = []string{"interfaces_status", "vlan"}
			job.Config["fetch_mode"] = "walk_only"
			job.Config["snmp_max_rows"] = fixture.maxRows
			result := RunJob(context.Background(), job)
			complete, _ := result.Payload["port_tables_complete_v1"].(map[string]any)
			ifaces, _ := result.Payload["ifaces"].([]any)
			tables, _ := result.Payload["tables"].(map[string]any)
			egress, _ := tables[switchportEgressOID].([]any)
			if result.Status != "ok" || complete[switchportEgressOID] != (fixture.maxRows > 1) ||
				complete[switchportUntaggedOID] != true || len(ifaces) != 1 || len(egress) == 0 {
				t.Fatalf("SNMP fixture did not deliver switchport data: %s, %+v", result.Status, complete)
			}
			if first := egress[0].(map[string]any); first["index"] != "0.56" || first["value"] != "0x80" {
				t.Fatalf("unexpected bitmap on the wire: %+v", first)
			}
			wire[fixture.name] = result.Payload
		})
	}
	if output := os.Getenv("AICEBERG_RELAY_SWITCHPORT_WIRE_FIXTURE"); output != "" && !t.Failed() {
		exportSwitchportWire(t, output, wire)
	}
}

func switchportFixtureNext(trunk bool) map[string]gosnmp.SnmpPDU {
	variables := make(map[string]gosnmp.SnmpPDU)
	for _, root := range snmpGroupDefs["interfaces_status"].Tables {
		pdu := gosnmp.SnmpPDU{Name: "." + root + ".9", Type: gosnmp.Integer, Value: 1}
		switch root {
		case "1.3.6.1.2.1.2.2.1.2", "1.3.6.1.2.1.31.1.1.1.1":
			pdu.Type, pdu.Value = gosnmp.OctetString, []byte("eth9")
		case "1.3.6.1.2.1.31.1.1.1.18":
			pdu.Type, pdu.Value = gosnmp.OctetString, []byte("fixture-link")
		case "1.3.6.1.2.1.2.2.1.5":
			pdu.Type, pdu.Value = gosnmp.Gauge32, uint32(1000000000)
		}
		variables[root] = pdu
	}
	const bridge = "1.3.6.1.2.1.17.1.4.1.2"
	const pvid = "1.3.6.1.2.1.17.7.1.4.5.1.1"
	const static = "1.3.6.1.2.1.17.7.1.4.3.1.1"
	variables[bridge] = gosnmp.SnmpPDU{Name: "." + bridge + ".1", Type: gosnmp.Integer, Value: 9}
	variables[pvid] = gosnmp.SnmpPDU{Name: "." + pvid + ".1", Type: gosnmp.Integer, Value: 56}
	variables[static] = gosnmp.SnmpPDU{Name: "." + static + ".56", Type: gosnmp.OctetString, Value: []byte("fixture-vlan")}
	for _, root := range []string{switchportEgressOID, switchportUntaggedOID} {
		variables[root] = gosnmp.SnmpPDU{Name: "." + root + ".0.56", Type: gosnmp.OctetString, Value: []byte{0x80}}
	}
	if trunk {
		variables[switchportEgressOID+".0.56"] = gosnmp.SnmpPDU{
			Name: "." + switchportEgressOID + ".0.57", Type: gosnmp.OctetString, Value: []byte{0x80},
		}
	}
	return variables
}

func serveSwitchportFixture(conn net.PacketConn, next map[string]gosnmp.SnmpPDU) error {
	frame := make([]byte, 8192)
	decoder := *gosnmp.Default
	for {
		n, peer, err := conn.ReadFrom(frame)
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		request, err := decoder.SnmpDecodePacket(frame[:n])
		if err != nil || request.Version != gosnmp.Version1 || request.Community != "fixture-switchport" ||
			request.PDUType != gosnmp.GetNextRequest || len(request.Variables) != 1 {
			return fmt.Errorf("unexpected SNMPv1 switchport request: %v", err)
		}
		reply := &gosnmp.SnmpPacket{Version: request.Version, Community: request.Community,
			PDUType: gosnmp.GetResponse, RequestID: request.RequestID}
		if variable, exists := next[strings.TrimPrefix(request.Variables[0].Name, ".")]; exists {
			reply.Variables = []gosnmp.SnmpPDU{variable}
		} else {
			reply.Error, reply.ErrorIndex, reply.Variables = gosnmp.NoSuchName, 1, request.Variables
		}
		raw, err := reply.MarshalMsg()
		if err != nil {
			return err
		}
		if _, err := conn.WriteTo(raw, peer); err != nil {
			return err
		}
	}
}

func exportSwitchportWire(t *testing.T, output string, wire map[string]map[string]any) {
	t.Helper()
	parent := filepath.Dir(output)
	if filepath.Base(output) != "switchport-wire.json" || filepath.Dir(parent) != "/tmp" ||
		!strings.HasPrefix(filepath.Base(parent), "aiceberg-assignment-test.") {
		t.Fatal("switchport output requires the disposable Runtime fixture")
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(wire); err != nil {
		t.Fatal(err)
	}
}
