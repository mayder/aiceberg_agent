package agentless

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

const (
	maxTrapBytes = 8192
	trapOIDName  = "1.3.6.1.6.3.1.1.4.1.0"
	uptimeName   = "1.3.6.1.2.1.1.3.0"
	trapCode     = "snmp_trap"
)

type SNMPTrapProfile struct {
	SourceIP   string              `json:"source_ip"`
	Version    string              `json:"version"`
	Community  string              `json:"community,omitempty"`
	USM        *SNMPTrapUSMProfile `json:"usm,omitempty"`
	ClientID   int                 `json:"client_id"`
	AssetID    int                 `json:"asset_id"`
	CheckID    int                 `json:"check_id"`
	EndpointID int                 `json:"endpoint_id"`
	OIDs       []string            `json:"trap_oids"`
}

func validTrapOID(oid string) bool {
	if len(oid) > 128 {
		return false
	}
	parts := strings.Split(oid, ".")
	if len(parts) < 2 || (parts[0] != "0" && parts[0] != "1" && parts[0] != "2") {
		return false
	}
	for index, part := range parts[1:] {
		value, err := strconv.ParseUint(part, 10, 32)
		if err != nil || strconv.FormatUint(value, 10) != part ||
			(index == 0 && parts[0] != "2" && value > 39) {
			return false
		}
	}
	return true
}

func trapObservation(raw []byte, source netip.Addr, profile SNMPTrapProfile, observed time.Time) (entities.AgentlessObservation, bool) {
	version := gosnmp.Version1
	pdu := gosnmp.Trap
	if profile.Version == "v2c" {
		version = gosnmp.Version2c
		pdu = gosnmp.SNMPv2Trap
	}
	if !matchesTrapEnvelope(raw, version, pdu, profile.Community) {
		return entities.AgentlessObservation{}, false
	}
	decoder := gosnmp.GoSNMP{Version: version}
	packet, err := decoder.SnmpDecodePacket(raw)
	if err != nil {
		return entities.AgentlessObservation{}, false
	}
	return newTrapObservation(packet, source, profile, observed)
}

func newTrapObservation(packet *gosnmp.SnmpPacket, source netip.Addr, profile SNMPTrapProfile, observed time.Time) (entities.AgentlessObservation, bool) {
	oid, uptime, ok := trapFields(packet)
	if !ok || !containsTrapOID(profile.OIDs, oid) {
		return entities.AgentlessObservation{}, false
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return entities.AgentlessObservation{}, false
	}
	eventID := hex.EncodeToString(id[:])
	return entities.AgentlessObservation{
		ID: "obs_" + eventID, CheckID: profile.CheckID, EndpointID: &profile.EndpointID,
		Status: "ok", Code: trapCode, ObservedAt: observed, CreatedAt: observed, SegmentID: trapCode, IsFinal: true,
		SegmentStartedAt: &observed,
		DedupeKey:        fmt.Sprintf("trap:%d:%d:%s", profile.ClientID, profile.CheckID, eventID),
		Payload: map[string]any{"snmp_trap_v1": map[string]any{
			"contract": "monitoring.snmp-trap.v1", "event_id": eventID, "source_ip": source.String(),
			"version": profile.Version, "oid": oid, "uptime_ticks": uptime,
			"client_id": profile.ClientID, "asset_id": profile.AssetID,
		}},
	}, true
}

// Check the routing fields and community before handing a packet to the SNMP decoder.
func matchesTrapEnvelope(raw []byte, version gosnmp.SnmpVersion, pdu gosnmp.PDUType, community string) bool {
	var envelope asn1.RawValue
	trailing, err := asn1.Unmarshal(raw, &envelope)
	if err != nil || len(trailing) != 0 || !isTrapSequence(envelope) {
		return false
	}
	var wireVersion int
	tail, err := asn1.Unmarshal(envelope.Bytes, &wireVersion)
	if err != nil || wireVersion != int(version) {
		return false
	}
	var wireCommunity []byte
	tail, err = asn1.Unmarshal(tail, &wireCommunity)
	if err != nil || subtle.ConstantTimeCompare(wireCommunity, []byte(community)) != 1 {
		return false
	}
	var wirePDU asn1.RawValue
	tail, err = asn1.Unmarshal(tail, &wirePDU)
	return err == nil && len(tail) == 0 && wirePDU.IsCompound &&
		wirePDU.Class == asn1.ClassContextSpecific && wirePDU.Tag == int(pdu&0x1f)
}

func trapFields(packet *gosnmp.SnmpPacket) (string, uint32, bool) {
	if packet.Version == gosnmp.Version1 {
		if packet.Timestamp > 4294967295 || packet.GenericTrap < 0 || packet.GenericTrap > 6 {
			return "", 0, false
		}
		if packet.GenericTrap < 6 {
			return "1.3.6.1.6.3.1.1.5." + strconv.Itoa(packet.GenericTrap+1), uint32(packet.Timestamp), true
		}
		if packet.SpecificTrap < 0 || uint64(packet.SpecificTrap) > 2147483647 {
			return "", 0, false
		}
		oid := strings.TrimPrefix(packet.Enterprise, ".") + ".0." + strconv.Itoa(packet.SpecificTrap)
		return oid, uint32(packet.Timestamp), validTrapOID(oid)
	}
	if len(packet.Variables) < 2 || packet.Error != gosnmp.NoError || packet.ErrorIndex != 0 {
		return "", 0, false
	}
	uptime, oid := packet.Variables[0], packet.Variables[1]
	if strings.TrimPrefix(uptime.Name, ".") != uptimeName || uptime.Type != gosnmp.TimeTicks ||
		strings.TrimPrefix(oid.Name, ".") != trapOIDName || oid.Type != gosnmp.ObjectIdentifier {
		return "", 0, false
	}
	ticks, ticksOK := uptime.Value.(uint32)
	value, oidOK := oid.Value.(string)
	value = strings.TrimPrefix(value, ".")
	return value, ticks, ticksOK && oidOK && validTrapOID(value)
}

func containsTrapOID(oids []string, oid string) bool {
	for _, allowed := range oids {
		if allowed == oid {
			return true
		}
	}
	return false
}
