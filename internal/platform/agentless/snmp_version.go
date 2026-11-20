package agentless

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
)

const maxSNMPv1Requests = 256
const snmpV1RequestInterval = 25 * time.Millisecond

func configuredSNMPVersion(value string) (gosnmp.SnmpVersion, string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "v1", "1":
		return gosnmp.Version1, "v1", nil
	case "", "v2c", "2c":
		return gosnmp.Version2c, "v2c", nil
	case "v3", "3":
		return gosnmp.Version3, "v3", nil
	default:
		return 0, "", errors.New("snmp_unsupported_version")
	}
}

// A missing OID in v1 rejects the entire GET PDU. Single-OID reads preserve the
// supported values, with the existing collection deadline and row/OID budgets.
func (c *snmpCollector) get(oids []string) (*gosnmp.SnmpPacket, error) {
	if c.plan.SNMPVersion != "v1" {
		return c.client.Get(oids)
	}
	response := &gosnmp.SnmpPacket{Version: gosnmp.Version1, Error: gosnmp.NoError}
	for _, oid := range oids {
		if err := c.admitV1Request(); err != nil {
			return nil, err
		}
		packet, err := c.client.Get([]string{oid})
		if err != nil {
			return nil, err
		}
		if packet == nil || packet.Version != gosnmp.Version1 || packet.Error != gosnmp.NoError {
			c.payload.addError(fmt.Sprintf("snmp_v1_oid_unavailable oid=%s", oid))
			continue
		}
		for _, variable := range snmpVariables(packet) {
			if strings.TrimPrefix(variable.Name, ".") != strings.TrimPrefix(oid, ".") || variable.Type == gosnmp.Counter64 {
				continue
			}
			response.Variables = append(response.Variables, variable)
		}
	}
	return response, nil
}

func usefulSNMPVariable(variable gosnmp.SnmpPDU) bool {
	switch variable.Type {
	case gosnmp.Null, gosnmp.NoSuchObject, gosnmp.NoSuchInstance, gosnmp.EndOfMibView:
		return false
	default:
		return variable.Value != nil
	}
}

func (c *snmpCollector) admitV1Request() error {
	if c.v1Requests >= maxSNMPv1Requests {
		c.payload.TimeBudgetExceeded = true
		c.payload.addError("snmp_v1_request_limit")
		return errors.New("snmp_v1_request_limit")
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay := time.Until(c.v1LastGet.Add(snmpV1RequestInterval)); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	if err := c.ensureBudget("v1 get"); err != nil {
		return err
	}
	c.v1Requests++
	c.v1LastGet = time.Now()
	return nil
}
