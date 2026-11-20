package agentless

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	gosnmp "github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

const basicProbeKind = "system_get"

type snmpBasicProbeReport struct {
	Kind              string `json:"kind"`
	OID               string `json:"oid"`
	Phase             string `json:"phase"`
	Outcome           string `json:"outcome"`
	GetAttempts       int    `json:"get_attempts"`
	ResponseReceived  bool   `json:"response_received"`
	ResponseVersion   string `json:"response_version,omitempty"`
	PDUError          string `json:"pdu_error,omitempty"`
	SentDatagrams     *int64 `json:"sent_datagrams"`
	ReceivedDatagrams *int64 `json:"received_datagrams"`
}

func runSNMPBasicProbe(ctx context.Context, job entities.AgentlessJob, host, version string) Result {
	started := time.Now()
	plan := buildSNMPPlan(job, host)
	plan.SNMPVersion, plan.DiagnosticProbe = version, true
	plan.Groups, plan.TimeBudgetMs = []string{"custom"}, 5000
	payload := newSNMPPayload(plan)
	report := &snmpBasicProbeReport{Kind: basicProbeKind, Phase: "validate", Outcome: "blocked"}
	payload.BasicProbe = report
	oid, _ := job.Config["snmp_basic_oid"].(string)
	if !basicProbeOID(oid) || (version != "v1" && version != "v2c") {
		return finishBasicProbe(payload, started, "blocked", "snmp_fail")
	}
	report.OID = oid
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client, err := basicProbeClient(ctx, job, host)
	if err != nil || client == nil {
		return finishBasicProbe(payload, started, "transport_error", "snmp_fail")
	}
	defer func() { _ = client.Close() }()
	report.Phase = "connect"
	if err = client.Connect(); err != nil {
		return basicProbeFailure(payload, started, ctx, err)
	}
	return executeBasicProbe(ctx, client, payload, started, oid, version)
}

func executeBasicProbe(ctx context.Context, client snmpClient, payload *snmpPayload, started time.Time, oid, version string) Result {
	report := payload.BasicProbe
	report.Phase, report.GetAttempts = "get", 1
	packet, err := client.Get([]string{oid})
	if wire, ok := client.(interface{ basicWireCounts() (int64, int64) }); ok {
		sent, received := wire.basicWireCounts()
		report.SentDatagrams, report.ReceivedDatagrams = &sent, &received
	}
	if err != nil {
		return basicProbeFailure(payload, started, ctx, err)
	}
	expected, _, _ := configuredSNMPVersion(version)
	if packet == nil || packet.Version != expected || packet.PDUType != gosnmp.GetResponse {
		return finishBasicProbe(payload, started, "invalid_response", "snmp_fail")
	}
	report.ResponseReceived, report.ResponseVersion, report.Phase = true, version, "response"
	if packet.Error != gosnmp.NoError {
		report.PDUError = basicPDUError(packet.Error)
		return finishBasicProbe(payload, started, "pdu_error", "snmp_no_data")
	}
	for _, variable := range packet.Variables {
		if strings.TrimPrefix(variable.Name, ".") != oid {
			continue
		}
		value, ok := basicProbeValue(oid, variable)
		if !ok {
			return finishBasicProbe(payload, started, "unsupported_value", "snmp_no_data")
		}
		payload.Sys[oid], payload.Scalars[oid], payload.OIDs[oid] = value, value, value
		payload.addGroupStats("custom", snmpGroupStats{GetAttempts: 1, GetSuccess: 1})
		return finishBasicProbe(payload, started, "value", "")
	}
	return finishBasicProbe(payload, started, "missing_value", "snmp_no_data")
}

func finishBasicProbe(payload *snmpPayload, started time.Time, outcome, code string) Result {
	payload.BasicProbe.Outcome = outcome
	status, message := "fail", "Consulta SNMP diagnóstica sem valor válido"
	if code == "" {
		status, message = "ok", "Consulta SNMP diagnóstica concluída"
	}
	return Result{Status: status, Code: code, Message: message, LatencyMs: int(time.Since(started).Milliseconds()), Payload: payload.toMap()}
}

func basicProbeFailure(payload *snmpPayload, started time.Time, ctx context.Context, err error) Result {
	// Never return arbitrary transport errors: they may contain endpoint or credentials.
	if errors.Is(ctx.Err(), context.Canceled) {
		return finishBasicProbe(payload, started, "cancelled", "snmp_fail")
	}
	text := strings.ToLower(err.Error())
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || strings.Contains(text, "timeout") || strings.Contains(text, "deadline") {
		return finishBasicProbe(payload, started, "timeout", "snmp_timeout")
	}
	if strings.Contains(text, "refused") {
		return finishBasicProbe(payload, started, "refused", "snmp_refused")
	}
	return finishBasicProbe(payload, started, "transport_error", "snmp_fail")
}

func basicProbeOID(oid string) bool {
	return oid == "1.3.6.1.2.1.1.1.0" || oid == "1.3.6.1.2.1.1.2.0" || oid == "1.3.6.1.2.1.1.3.0"
}

func basicProbeValue(oid string, variable gosnmp.SnmpPDU) (any, bool) {
	if oid == "1.3.6.1.2.1.1.3.0" {
		return basicUptimeValue(variable)
	}
	if oid == "1.3.6.1.2.1.1.2.0" && variable.Type != gosnmp.ObjectIdentifier {
		return nil, false
	}
	if oid == "1.3.6.1.2.1.1.1.0" && variable.Type != gosnmp.OctetString {
		return nil, false
	}
	var value string
	switch v := variable.Value.(type) {
	case string:
		value = v
	case []byte:
		value = string(v)
	default:
		return nil, false
	}
	if len(value) > 256 || strings.TrimSpace(value) == "" || !utf8.ValidString(value) {
		return nil, false
	}
	if oid == "1.3.6.1.2.1.1.2.0" && !basicObjectID(value) {
		return nil, false
	}
	return value, true
}

func basicPDUError(code gosnmp.SNMPError) string {
	switch code {
	case gosnmp.NoSuchName:
		return "noSuchName"
	case gosnmp.TooBig:
		return "tooBig"
	case gosnmp.BadValue:
		return "badValue"
	case gosnmp.ReadOnly:
		return "readOnly"
	case gosnmp.GenErr:
		return "genErr"
	default:
		return "protocol_error"
	}
}

func basicUptimeValue(variable gosnmp.SnmpPDU) (any, bool) {
	if variable.Type != gosnmp.TimeTicks {
		return nil, false
	}
	switch n := variable.Value.(type) {
	case uint32:
		return n, true
	case uint:
		if uint64(n) <= 4294967295 {
			return n, true
		}
	case uint64:
		if n <= 4294967295 {
			return n, true
		}
	case int:
		if n >= 0 && uint64(n) <= 4294967295 {
			return n, true
		}
	case int64:
		if n >= 0 && n <= 4294967295 {
			return n, true
		}
	}
	return nil, false
}

func basicObjectID(value string) bool {
	parts := strings.Split(strings.TrimPrefix(value, "."), ".")
	if len(parts) < 2 || len(parts) > 128 {
		return false
	}
	for i, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil || (i == 0 && n > 2) {
			return false
		}
		if i == 1 && parts[0] != "2" && n > 39 {
			return false
		}
	}
	return true
}

func basicProbeClient(ctx context.Context, job entities.AgentlessJob, host string) (snmpClient, error) {
	bounded := job
	bounded.TimeoutMs, bounded.Retries = 1500, 0
	profile := *job.SNMP
	profile.TimeoutMs, profile.Retries, profile.TimeBudgetMs = 1500, 0, 5000
	bounded.SNMP = &profile
	factory := defaultSNMPClient
	if factory == nil {
		factory = defaultSNMPClientFactory
	}
	return factory(ctx, bounded, host)
}
