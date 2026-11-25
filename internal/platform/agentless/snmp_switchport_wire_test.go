package agentless

import (
	"context"
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestSNMPSwitchportWirePreservesPrintableBitmapsAndCollectionCompleteness(t *testing.T) {
	job := newSNMPJob()
	job.Config["fetch_mode"] = "walk_only"
	job.Config["groups"] = []any{"vlan"}
	job.Config["snmp_max_rows"] = 1
	withFakeSNMPClient(t, &fakeSNMPClient{bulkWalkFn: func(root string, read gosnmp.WalkFunc) error {
		value := gosnmp.SnmpPDU{Name: "." + root + ".0.57", Type: gosnmp.OctetString, Value: []byte("80")}
		if err := read(value); err != nil {
			return err
		}
		if root == switchportUntaggedOID {
			value.Name = "." + root + ".0.58"
			return read(value)
		}
		return nil
	}})
	res := runSNMP(context.Background(), job)
	tables, _ := res.Payload["tables"].(map[string]any)
	rows, _ := tables[switchportEgressOID].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["value"] != "0x3830" || rows[0].(map[string]any)["index"] != "0.57" {
		t.Fatalf("Q-BRIDGE port bitmap changed its bytes: %+v", rows)
	}
	complete, _ := res.Payload["port_tables_complete_v1"].(map[string]any)
	if complete[switchportEgressOID] != true || complete[switchportUntaggedOID] != false {
		t.Fatalf("truncated VLAN walk marked as complete: %+v", complete)
	}
}
