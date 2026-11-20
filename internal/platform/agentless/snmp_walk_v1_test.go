package agentless

import (
	"strings"
	"testing"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
)

func TestSNMPV1WalkRejectsLoopAndPreservesLeafGet(t *testing.T) {
	job := newSNMPJob()
	plan := buildSNMPPlan(job, "127.0.0.1")
	plan.SNMPVersion = "v1"
	root := "1.3.6.1.4.1.99999.1"
	client := &fakeSNMPClient{getNextFn: func(_ []string) (*gosnmp.SnmpPacket, error) {
		return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: root + ".1", Type: gosnmp.Integer, Value: 25}}}, nil
	}}
	c := snmpCollector{plan: plan, payload: newSNMPPayload(plan), client: client, started: time.Now(), budget: time.Second}
	count := 0
	if err := c.walkV1(root, func(_ gosnmp.SnmpPDU) error { count++; return nil }); err == nil || !strings.Contains(err.Error(), "non_increasing") || count != 1 {
		t.Fatalf("loop accepted: %d %v", count, err)
	}
	client.getNextFn = func(_ []string) (*gosnmp.SnmpPacket, error) {
		return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: "1.3.6.1.4.1.99999.2", Type: gosnmp.Integer, Value: 26}}}, nil
	}
	client.getFn = func(_ []string) (*gosnmp.SnmpPacket, error) {
		return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: root, Type: gosnmp.Integer, Value: 25}}}, nil
	}
	count = 0
	if err := c.walkV1(root, func(v gosnmp.SnmpPDU) error {
		count++
		if v.Value != 25 {
			t.Fatal("leaf value changed")
		}
		return nil
	}); err != nil || count != 1 {
		t.Fatalf("leaf GET failed: %d %v", count, err)
	}
}

func TestSNMPOIDOrderingUsesNumericArcsAndRejectsMalformedInput(t *testing.T) {
	for _, c := range []struct {
		next, current string
		want          bool
	}{
		{"1.3.10", "1.3.9", true}, {"1.3.9", "1.3.10", false}, {"1.3.1", "1.3.1", false},
		{"1.3.1.1", "1.3.1", true}, {"1.3.2.bad", "1.3.1", false}, {"1.3.4294967296", "1.3.1", false},
	} {
		if got := snmpOIDFollows(c.next, c.current); got != c.want {
			t.Fatalf("OID order %s/%s got %v", c.next, c.current, got)
		}
	}
}
