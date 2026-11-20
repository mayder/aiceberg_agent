package agentless

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
)

// A real UDP exchange with synthetic values. It is not a physical-sensor test.
func TestSNMPV1RealUDPGetAndGetNextWithUnsupportedOID(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	requests := make(chan gosnmp.PDUType, 32)
	root := "1.3.6.1.4.1.99999.1"
	scalar := root + ".0"
	missing := "1.3.6.1.4.1.99999.9.0"
	go func() { done <- serveV1Fixture(conn, requests, root, scalar) }()
	t.Cleanup(func() {
		_ = conn.Close()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	job := newSNMPJob()
	job.SNMP.Version = "v1"
	job.SNMP.Port = conn.LocalAddr().(*net.UDPAddr).Port
	job.SNMP.Community = "fixture-v1-private"
	job.SNMP.TimeoutMs = 250
	job.TimeoutMs = 250
	job.Endpoint.Endereco = "127.0.0.1"
	job.Config["groups"] = []string{"system"}
	job.Config["exclude_groups"] = []string{"system"}
	job.Config["fetch_mode"] = "auto"
	job.Config["custom"] = map[string]any{"get": []any{map[string]any{"oid": missing}, map[string]any{"oid": scalar, "metric": "temperature_raw", "unit": "raw"}}, "walk": []any{map[string]any{"oid": root, "unit": "raw"}}}
	result := runSNMP(context.Background(), job)
	if result.Status != "ok" {
		t.Fatalf("UDP collection failed: %+v", result)
	}
	applied, _ := result.Payload["runtime_applied"].(map[string]any)
	if applied["snmp_version"] != "v1" {
		t.Fatalf("effective version absent: %+v", applied)
	}
	success, _ := result.Payload["oids_success"].([]any)
	failed, _ := result.Payload["oids_failed"].([]any)
	if len(success) != 2 || len(failed) != 1 {
		t.Fatalf("OID validity lost: success=%+v failed=%+v", success, failed)
	}
	custom, _ := result.Payload["custom"].(map[string]any)
	get, _ := custom["get"].([]any)
	var value any
	for _, item := range get {
		row := item.(map[string]any)
		if row["oid"] == scalar {
			value = row["value"]
		}
	}
	if value != float64(253) {
		t.Fatalf("raw value changed or missing: %v", value)
	}
	sawGet, sawNext := false, false
	for len(requests) > 0 {
		switch <-requests {
		case gosnmp.GetRequest:
			sawGet = true
		case gosnmp.GetNextRequest:
			sawNext = true
		default:
			t.Fatal("v1 sent an unsupported PDU")
		}
	}
	if !sawGet || !sawNext {
		t.Fatal("GET and GETNEXT were not both exercised")
	}
}

func serveV1Fixture(conn net.PacketConn, requests chan<- gosnmp.PDUType, root, scalar string) error {
	decoder := *gosnmp.Default
	decoder.Version = gosnmp.Version1
	decoder.Community = "fixture-v1-private"
	buffer := make([]byte, 65535)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, peer, err := conn.ReadFrom(buffer)
		if err != nil {
			if strings.Contains(err.Error(), "closed network connection") {
				return nil
			}
			return err
		}
		packet, err := decoder.SnmpDecodePacket(buffer[:n])
		if err != nil {
			return err
		}
		if packet.Version != gosnmp.Version1 || packet.Community != decoder.Community {
			return fmt.Errorf("wrong SNMP profile on wire")
		}
		requests <- packet.PDUType
		response := &gosnmp.SnmpPacket{Version: packet.Version, Community: packet.Community, PDUType: gosnmp.GetResponse, RequestID: packet.RequestID}
		oid := strings.TrimPrefix(packet.Variables[0].Name, ".")
		switch packet.PDUType {
		case gosnmp.GetRequest:
			switch oid {
			case "1.3.6.1.2.1.1.3.0":
				response.Variables = []gosnmp.SnmpPDU{{Name: "." + oid, Type: gosnmp.TimeTicks, Value: uint32(10000)}}
			case scalar:
				response.Variables = []gosnmp.SnmpPDU{{Name: "." + scalar, Type: gosnmp.Integer, Value: 253}}
			default:
				response.Error = gosnmp.NoSuchName
				response.ErrorIndex = 1
				response.Variables = packet.Variables
			}
		case gosnmp.GetNextRequest:
			if variable, matched := entitySensorFixtureVariable(oid); matched {
				if variable == nil {
					response.Error = gosnmp.NoSuchName
					response.ErrorIndex = 1
					response.Variables = packet.Variables
				} else {
					response.Variables = []gosnmp.SnmpPDU{*variable}
				}
				break
			}
			next := root + ".1"
			if oid == next {
				next = "1.3.6.1.4.1.99999.2.1"
			}
			response.Variables = []gosnmp.SnmpPDU{{Name: "." + next, Type: gosnmp.Integer, Value: 253}}
		default:
			return fmt.Errorf("unsupported v1 PDU")
		}
		encoded, err := response.MarshalMsg()
		if err != nil {
			return err
		}
		if _, err := conn.WriteTo(encoded, peer); err != nil {
			return err
		}
	}
}

func entitySensorFixtureVariable(oid string) (*gosnmp.SnmpPDU, bool) {
	if !strings.HasPrefix(oid, entitySensorRoot) {
		return nil, false
	}
	parts := strings.Split(strings.TrimPrefix(oid, entitySensorRoot), ".")
	if len(parts) != 1 {
		return nil, true
	}
	variable := gosnmp.SnmpPDU{Name: "." + oid + ".1", Type: gosnmp.Integer}
	switch parts[0] {
	case "1":
		variable.Value = 8
	case "2":
		variable.Value = 9
	case "3":
		variable.Value = 1
	case "4":
		variable.Value = 253
	case "5":
		variable.Value = 1
	case "6":
		variable.Type = gosnmp.OctetString
		variable.Value = []byte("Celsius")
	case "7":
		variable.Type = gosnmp.TimeTicks
		variable.Value = uint32(9990)
	case "8":
		variable.Type = gosnmp.Gauge32
		variable.Value = uint32(1000)
	default:
		return nil, true
	}
	return &variable, true
}

func TestSNMPV1RealUDPEntitySensorProducesQualifiedTemperature(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	requests := make(chan gosnmp.PDUType, 64)
	go func() { done <- serveV1Fixture(conn, requests, "1.3.6.1.4.1.99999.1", "1.3.6.1.4.1.99999.1.0") }()
	t.Cleanup(func() {
		_ = conn.Close()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	job := newSNMPJob()
	job.SNMP.Version = "v1"
	job.SNMP.Port = conn.LocalAddr().(*net.UDPAddr).Port
	job.SNMP.Community = "fixture-v1-private"
	job.Endpoint.Endereco = "127.0.0.1"
	job.Config["entity_sensor_probe"] = true
	job.Config["groups"] = []string{"system"}
	job.Config["fetch_mode"] = "auto"
	job.Config["time_budget_ms"] = 5000
	roots := []string{}
	for _, col := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
		roots = append(roots, entitySensorRoot+col)
	}
	job.Config["custom_walk_oids"] = roots
	result := runSNMP(context.Background(), job)
	if result.Payload["diagnostic_probe"] != true {
		t.Fatal("diagnostic marker absent")
	}
	sensors, _ := result.Payload["temperature_sensors"].([]any)
	if result.Status != "ok" || len(sensors) != 1 {
		t.Fatalf("qualified sensor absent: %+v", result)
	}
	sensor := sensors[0].(map[string]any)
	if sensor["state"] != "ok" || sensor["unit"] != "celsius" || sensor["value"] != float64(25.3) || sensor["age_ms"] != float64(100) {
		t.Fatalf("sensor metadata lost: %+v", sensor)
	}
	for len(requests) > 0 {
		if <-requests == gosnmp.GetBulkRequest {
			t.Fatal("v1 emitted GETBULK")
		}
	}
}
