package agentless

import (
	gosnmp "github.com/gosnmp/gosnmp"
	"sync/atomic"
)

type snmpBasicWire struct {
	sent     atomic.Int64
	received atomic.Int64
}

func attachSNMPBasicWire(client *gosnmp.GoSNMP, enabled bool) *snmpBasicWire {
	if !enabled {
		return nil
	}
	wire := &snmpBasicWire{}
	// gosnmp1.38 invokes OnSent after a successful write and OnRecv after read,
	// before decoding. A received datagram alone does not prove a valid SNMP PDU.
	client.OnSent = func(*gosnmp.GoSNMP) { wire.sent.Add(1) }
	client.OnRecv = func(*gosnmp.GoSNMP) { wire.received.Add(1) }
	return wire
}

func (c *goSNMPClient) basicWireCounts() (int64, int64) {
	if c.basicWire == nil {
		return 0, 0
	}
	return c.basicWire.sent.Load(), c.basicWire.received.Load()
}
