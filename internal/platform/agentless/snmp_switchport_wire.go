package agentless

import (
	"encoding/hex"
	"strings"

	"github.com/gosnmp/gosnmp"
)

const (
	switchportEgressOID   = "1.3.6.1.2.1.17.7.1.4.2.1.4"
	switchportUntaggedOID = "1.3.6.1.2.1.17.7.1.4.2.1.5"
)

func snmpTableValue(rootOID string, variable gosnmp.SnmpPDU) any {
	switch strings.TrimPrefix(rootOID, ".") {
	case switchportEgressOID, switchportUntaggedOID:
		if raw, ok := variable.Value.([]byte); variable.Type == gosnmp.OctetString && ok {
			// These bytes represent bits even when they happen to be printable ASCII.
			return "0x" + hex.EncodeToString(raw)
		}
		return nil
	default:
		return snmpValueToAny(variable)
	}
}

func (p *snmpPayload) storeTable(rootOID string, rows []map[string]any, truncated bool) {
	p.Tables[rootOID] = rows
	oid := strings.TrimPrefix(rootOID, ".")
	switch oid {
	case switchportEgressOID, switchportUntaggedOID,
		"1.3.6.1.2.1.17.1.4.1.2", "1.3.6.1.2.1.17.7.1.4.5.1.1",
		"1.3.6.1.2.1.2.2.1.5", "1.3.6.1.2.1.2.2.1.7", "1.3.6.1.2.1.31.1.1.1.15":
		if p.CompletePortTables == nil {
			p.CompletePortTables = make(map[string]bool)
		}
		p.CompletePortTables[oid] = !truncated && completePortTable(oid, rows)
	}
}

func completePortTable(oid string, rows []map[string]any) bool {
	if oid == switchportEgressOID || oid == switchportUntaggedOID {
		for _, row := range rows {
			value, _ := row["value"].(string)
			if len(value) <= 2 {
				return false
			}
		}
	}
	return len(rows) > 0
}
