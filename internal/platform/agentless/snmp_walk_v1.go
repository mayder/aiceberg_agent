package agentless

import (
	"errors"
	"strconv"
	"strings"

	gosnmp "github.com/gosnmp/gosnmp"
)

// Uses the official codec/GetNext while admitting every v1 packet through the
// same request ceiling, cadence, deadline and cancellation gate as scalar GET.
func (c *snmpCollector) walkV1(root string, callback gosnmp.WalkFunc) error {
	root = strings.TrimPrefix(root, ".")
	current := root
	for {
		if err := c.admitV1Request(); err != nil {
			return err
		}
		packet, err := c.client.GetNext([]string{current})
		if err != nil {
			return err
		}
		if packet == nil || packet.Version != gosnmp.Version1 {
			return errors.New("snmp_v1_invalid_response")
		}
		if packet.Error == gosnmp.NoSuchName {
			return nil
		}
		if packet.Error != gosnmp.NoError {
			return errors.New("snmp_v1_walk_error")
		}
		variables := snmpVariables(packet)
		if len(variables) == 0 {
			return nil
		}
		if len(variables) != 1 {
			return errors.New("snmp_v1_invalid_response")
		}
		variable := variables[0]
		next := strings.TrimPrefix(variable.Name, ".")
		if !strings.HasPrefix(next, root+".") {
			if current == root {
				return c.walkV1Leaf(root, callback)
			}
			return nil
		}
		if !snmpOIDFollows(next, current) {
			return errors.New("snmp_v1_non_increasing_oid")
		}
		if variable.Type != gosnmp.Counter64 {
			if err := callback(variable); err != nil {
				return err
			}
		}
		current = next
	}
}

func (c *snmpCollector) walkV1Leaf(root string, callback gosnmp.WalkFunc) error {
	packet, err := c.get([]string{root})
	if err != nil {
		return err
	}
	for _, variable := range snmpVariables(packet) {
		if err := callback(variable); err != nil {
			return err
		}
	}
	return nil
}

func snmpOIDFollows(next, current string) bool {
	as, bs := strings.Split(next, "."), strings.Split(current, ".")
	if len(as) > 128 || len(bs) > 128 {
		return false
	}
	an, bn := make([]uint64, len(as)), make([]uint64, len(bs))
	for i, part := range as {
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return false
		}
		an[i] = n
	}
	for i, part := range bs {
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return false
		}
		bn[i] = n
	}
	for i := 0; i < min(len(an), len(bn)); i++ {
		if an[i] != bn[i] {
			return an[i] > bn[i]
		}
	}
	return len(an) > len(bn)
}
