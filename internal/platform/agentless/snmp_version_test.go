package agentless

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
)

func TestSNMPExplicitVersionSelection(t *testing.T) {
	for _, c := range []struct {
		input string
		want  gosnmp.SnmpVersion
	}{{"v1", gosnmp.Version1}, {"1", gosnmp.Version1}, {" V1 ", gosnmp.Version1}, {"v2c", gosnmp.Version2c}, {"2c", gosnmp.Version2c}, {"", gosnmp.Version2c}, {"v3", gosnmp.Version3}, {"3", gosnmp.Version3}} {
		job := newSNMPJob()
		job.SNMP.Version = c.input
		client, err := defaultSNMPClientFactory(context.Background(), job, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		snmp := client.(*goSNMPClient).client
		if snmp.Version != c.want {
			t.Fatalf("version %q: got %v", c.input, snmp.Version)
		}
		if c.want != gosnmp.Version3 && snmp.Community != job.SNMP.Community {
			t.Fatal("community changed")
		}
	}
	job := newSNMPJob()
	job.SNMP.Version = "untrusted-secret-version"
	if _, err := defaultSNMPClientFactory(context.Background(), job, "127.0.0.1"); err == nil || strings.Contains(err.Error(), "untrusted-secret") {
		t.Fatal("unsupported version accepted or disclosed")
	}
	result := runSNMP(context.Background(), job)
	if result.Code != "snmp_unsupported_version" || result.Status != "fail" {
		t.Fatalf("unknown version defaulted: %+v", result)
	}
}

func TestSNMPV1NeverUsesBulkWalkAndKeepsLimits(t *testing.T) {
	for _, version := range []string{"v1", "v2c"} {
		t.Run(version, func(t *testing.T) {
			job := newSNMPJob()
			job.SNMP.Version = version
			plan := buildSNMPPlan(job, "127.0.0.1")
			plan.SNMPVersion = version
			plan.MaxRows = 2
			walk, bulk := 0, 0
			callback := func(root string, fn gosnmp.WalkFunc) error {
				for i := 0; i < 3; i++ {
					if err := fn(gosnmp.SnmpPDU{Name: root + ".1", Type: gosnmp.Integer, Value: 25}); err != nil {
						return err
					}
				}
				return nil
			}
			client := &fakeSNMPClient{getNextFn: func(_ []string) (*gosnmp.SnmpPacket, error) {
				walk++
				return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: fmt.Sprintf("1.3.6.1.4.1.99999.1.%d", walk), Type: gosnmp.Integer, Value: 25}}}, nil
			}, walkFn: func(root string, fn gosnmp.WalkFunc) error { walk++; return callback(root, fn) }, bulkWalkFn: func(root string, fn gosnmp.WalkFunc) error { bulk++; return callback(root, fn) }}
			collector := snmpCollector{job: job, plan: plan, payload: newSNMPPayload(plan), client: client, started: time.Now(), budget: time.Second}
			rows, truncated, err := collector.walkTable("1.3.6.1.4.1.99999.1")
			if err != nil || !truncated || len(rows) != 2 {
				t.Fatalf("limit lost: %v %v %v", rows, truncated, err)
			}
			if version == "v1" && (bulk != 0 || walk != 3) {
				t.Fatal("v1 attempted bulk")
			}
			if version == "v2c" && (bulk != 1 || walk != 0) {
				t.Fatal("v2c bulk regressed")
			}
		})
	}
}

func TestSNMPV1UnknownOIDDoesNotPoisonOtherValuesOrFabricateZero(t *testing.T) {
	job := newSNMPJob()
	job.SNMP.Version = "v1"
	plan := buildSNMPPlan(job, "127.0.0.1")
	plan.SNMPVersion = "v1"
	missing := "1.3.6.1.4.1.99999.1.0"
	found := "1.3.6.1.4.1.99999.2.0"
	client := &fakeSNMPClient{getFn: func(oids []string) (*gosnmp.SnmpPacket, error) {
		if len(oids) != 1 {
			t.Fatal("v1 must isolate unsupported OIDs")
		}
		if oids[0] == missing {
			return &gosnmp.SnmpPacket{Error: gosnmp.NoSuchName, Variables: []gosnmp.SnmpPDU{{Name: missing, Type: gosnmp.Null, Value: nil}}}, nil
		}
		return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: found, Type: gosnmp.Integer, Value: 253}}}, nil
	}}
	collector := snmpCollector{job: job, plan: plan, payload: newSNMPPayload(plan), client: client, started: time.Now(), budget: time.Second}
	packet, err := collector.get([]string{missing, found})
	if err != nil || len(packet.Variables) != 1 || packet.Variables[0].Value != 253 {
		t.Fatalf("lost valid value or invented missing value: %+v %v", packet, err)
	}
	for _, typ := range []gosnmp.Asn1BER{gosnmp.Null, gosnmp.NoSuchObject, gosnmp.NoSuchInstance, gosnmp.EndOfMibView} {
		vars := snmpVariables(&gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: missing, Type: typ, Value: 0}}})
		if len(vars) != 0 {
			t.Fatal("unavailable became numeric zero")
		}
	}
	if len(snmpVariables(&gosnmp.SnmpPacket{Error: gosnmp.NoSuchName, Variables: []gosnmp.SnmpPDU{{Type: gosnmp.Integer, Value: 0}}})) != 0 {
		t.Fatal("SNMP error accepted as value")
	}
}

func TestSNMPV1BudgetAndTimeoutDoNotBecomeSuccess(t *testing.T) {
	job := newSNMPJob()
	job.SNMP.Version = "v1"
	plan := buildSNMPPlan(job, "127.0.0.1")
	plan.SNMPVersion = "v1"
	calls := 0
	client := &fakeSNMPClient{getFn: func(oids []string) (*gosnmp.SnmpPacket, error) { calls++; return nil, errors.New("request timeout") }}
	collector := snmpCollector{job: job, plan: plan, payload: newSNMPPayload(plan), client: client, started: time.Now().Add(-time.Second), budget: time.Millisecond}
	if _, err := collector.get([]string{"1.3.6.1.2.1.1.3.0"}); !errors.Is(err, errSNMPBudgetExceeded) || calls != 0 {
		t.Fatal("budget admitted work")
	}
	withFakeSNMPClient(t, client)
	result := runSNMP(context.Background(), job)
	if result.Status != "fail" || result.Code != "snmp_timeout" {
		t.Fatalf("timeout became success: %+v", result)
	}
	raw, _ := json.Marshal(result.Payload)
	if strings.Contains(string(raw), job.SNMP.Community) {
		t.Fatal("community leaked")
	}
}
