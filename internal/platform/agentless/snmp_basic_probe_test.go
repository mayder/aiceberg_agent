package agentless

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	gosnmp "github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestBasicProbeSingleGETAndImmutableBounds(t *testing.T) {
	for _, version := range []string{"v1", "v2c"} {
		t.Run(version, func(t *testing.T) {
			job := newSNMPJob()
			job.SNMP.Version, job.SNMP.TimeoutMs, job.SNMP.Retries = version, 9000, 7
			original := *job.SNMP
			job.Config["snmp_basic_probe"], job.Config["snmp_basic_oid"] = true, "1.3.6.1.2.1.1.3.0"
			calls := 0
			fake := &fakeSNMPClient{getFn: func(oids []string) (*gosnmp.SnmpPacket, error) {
				calls++
				if !reflect.DeepEqual(oids, []string{"1.3.6.1.2.1.1.3.0"}) {
					t.Fatal("unexpected OID")
				}
				v, _, _ := configuredSNMPVersion(version)
				return &gosnmp.SnmpPacket{Version: v, PDUType: gosnmp.GetResponse, Variables: []gosnmp.SnmpPDU{{Name: oids[0], Type: gosnmp.TimeTicks, Value: uint32(0)}}}, nil
			}, walkFn: func(string, gosnmp.WalkFunc) error { t.Fatal("WALK forbidden"); return nil }, bulkWalkFn: func(string, gosnmp.WalkFunc) error { t.Fatal("BULK forbidden"); return nil }}
			prev := defaultSNMPClient
			defaultSNMPClient = func(ctx context.Context, bounded entities.AgentlessJob, host string) (snmpClient, error) {
				if bounded.TimeoutMs != 1500 || bounded.Retries != 0 || bounded.SNMP.TimeoutMs != 1500 || bounded.SNMP.Retries != 0 || bounded.SNMP.TimeBudgetMs != 5000 {
					t.Fatal("unbounded probe")
				}
				return fake, nil
			}
			defer func() { defaultSNMPClient = prev }()
			result := runSNMP(context.Background(), job)
			if result.Status != "ok" || calls != 1 || *job.SNMP != original {
				t.Fatalf("invalid result or profile mutation: %s", result.Status)
			}
			report := result.Payload["basic_probe"].(map[string]any)
			if report["response_received"] != true || report["outcome"] != "value" || report["get_attempts"] != float64(1) {
				t.Fatalf("bad report: %+v", report)
			}
			if result.Payload["diagnostic_probe"] != true {
				t.Fatal("missing diagnostic isolation")
			}

		})
	}
}

func TestBasicProbeRejectsBadPacketsAndNeverLeaksTransportErrors(t *testing.T) {
	cases := []struct {
		name      string
		packet    *gosnmp.SnmpPacket
		err       error
		outcome   string
		responded bool
	}{
		{"nil", nil, nil, "invalid_response", false},
		{"version", &gosnmp.SnmpPacket{Version: gosnmp.Version1, PDUType: gosnmp.GetResponse}, nil, "invalid_response", false},
		{"pdu", &gosnmp.SnmpPacket{Version: gosnmp.Version2c, PDUType: gosnmp.GetRequest}, nil, "invalid_response", false},
		{"noSuchName", &gosnmp.SnmpPacket{Version: gosnmp.Version2c, PDUType: gosnmp.GetResponse, Error: gosnmp.NoSuchName}, nil, "pdu_error", true},
		{"missing", &gosnmp.SnmpPacket{Version: gosnmp.Version2c, PDUType: gosnmp.GetResponse}, nil, "missing_value", true},
		{"wrongType", &gosnmp.SnmpPacket{Version: gosnmp.Version2c, PDUType: gosnmp.GetResponse, Variables: []gosnmp.SnmpPDU{{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.OctetString, Value: []byte("secret")}}}, nil, "unsupported_value", true},
		{"timeout", nil, errors.New("fixture-secret timeout"), "timeout", false},
		{"transport", nil, errors.New("fixture-secret transport failed"), "transport_error", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := newSNMPJob()
			job.Config["snmp_basic_probe"], job.Config["snmp_basic_oid"] = true, "1.3.6.1.2.1.1.3.0"
			withFakeSNMPClient(t, &fakeSNMPClient{getFn: func([]string) (*gosnmp.SnmpPacket, error) { return tc.packet, tc.err }})
			result := runSNMP(context.Background(), job)
			data, _ := json.Marshal(result)
			report := result.Payload["basic_probe"].(map[string]any)
			if result.Status != "fail" || report["outcome"] != tc.outcome || report["response_received"] != tc.responded || strings.Contains(string(data), "fixture-secret") {
				t.Fatalf("unsafe outcome: %s", data)
			}
		})
	}
}

func TestBasicProbeRejectsUnapprovedOIDBeforeNetwork(t *testing.T) {
	job := newSNMPJob()
	job.Config["snmp_basic_probe"], job.Config["snmp_basic_oid"] = true, "1.3.6.1.4.1.99999.0"
	prev := defaultSNMPClient
	defer func() { defaultSNMPClient = prev }()
	defaultSNMPClient = func(context.Context, entities.AgentlessJob, string) (snmpClient, error) {
		t.Fatal("unexpected network")
		return nil, nil
	}
	if result := runSNMP(context.Background(), job); result.Status != "fail" {
		t.Fatal("unapproved OID accepted")
	}
}

func TestBasicScalarValidation(t *testing.T) {
	for _, bad := range []any{int(-1), uint64(4294967296), float64(10), "10"} {
		if _, ok := basicUptimeValue(gosnmp.SnmpPDU{Type: gosnmp.TimeTicks, Value: bad}); ok {
			t.Fatalf("bad uptime: %v", bad)
		}
	}
	for _, bad := range []string{".", "1..3", "3.1", "1.40", "2.4294967296", "2.-1"} {
		if basicObjectID(bad) {
			t.Fatalf("bad object ID: %s", bad)
		}
	}
	for _, good := range []string{"1.3.6.1.4.1.9", ".2.999.1"} {
		if !basicObjectID(good) {
			t.Fatal("valid object ID rejected")
		}
	}
	for _, bad := range []string{"", strings.Repeat("a", 257), string([]byte{255})} {
		if _, ok := basicProbeValue("1.3.6.1.2.1.1.1.0", gosnmp.SnmpPDU{Type: gosnmp.OctetString, Value: bad}); ok {
			t.Fatal("bad description accepted")
		}
	}
}
