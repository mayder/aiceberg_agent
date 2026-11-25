package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const ProbeReportContract = "monitoring.probe-report.v1"
const MaxProbeReportBytes = 1024

var ErrProbeReport = errors.New("invalid_monitoring_probe_report")

// ProbeReport describes an execution, not the availability or temperature of
// the target. Metric records retain their existing typed monitoring contract.
type ProbeReport struct {
	Contract     string `json:"contract"`
	Scope        Scope  `json:"scope"`
	Protocol     string `json:"protocol"`
	Outcome      string `json:"outcome"`
	Reason       string `json:"reason"`
	ObservedAtMS int64  `json:"observed_at_ms"`
	ElapsedMS    int64  `json:"elapsed_ms"`
}

func (p ProbeReport) Validate(scope Scope, protocol string) error {
	if p.Contract != ProbeReportContract || p.Scope != scope || p.Protocol != protocol ||
		p.ObservedAtMS < 1 || p.ObservedAtMS > 253402300799999 || p.ElapsedMS < 0 || p.ElapsedMS > 86400000 {
		return ErrProbeReport
	}
	for _, id := range []int64{scope.ClientID, scope.AssetID, scope.CheckID} {
		if id < 1 || id > 9007199254740991 {
			return ErrProbeReport
		}
	}
	switch protocol {
	case "icmp", "tcp", "http", "dns", "tls", "snmp", "discovery", "openmetrics", "jolokia", "postgresql", IPMISource, VCenterSource, SSHSource, MySQLSource, TelnetSource, BrowserSource, JMXSource, "unsupported":
	default:
		return ErrProbeReport
	}
	if protocol == "unsupported" && p.Outcome != "unsupported" {
		return ErrProbeReport
	}
	if !validProbeOutcome(p.Outcome, p.Reason) {
		return ErrProbeReport
	}
	return nil
}

func validProbeOutcome(outcome, reason string) bool {
	switch outcome {
	case "ok":
		return reason == "none"
	case "failed":
		return reason == "configuration" || reason == "connection" || reason == "response" || reason == "execution"
	case "timeout":
		return reason == "deadline" || reason == "budget"
	case "cancelled":
		return reason == "cancelled"
	case "unsupported":
		return reason == "protocol" || reason == "version"
	case "partial":
		return reason == "segment"
	default:
		return false
	}
}

// DecodeProbeReport requires check authority obtained independently of the wire.
func DecodeProbeReport(raw []byte, scope Scope, protocol string) (ProbeReport, error) {
	var p ProbeReport
	if len(raw) == 0 || len(raw) > MaxProbeReportBytes || !recordJSONValid(raw) {
		return p, ErrProbeReport
	}
	fields, ok := configurationObject(raw, "contract", "scope", "protocol", "outcome", "reason", "observed_at_ms", "elapsed_ms")
	if !ok {
		return p, ErrProbeReport
	}
	if _, ok := configurationObject(fields["scope"], "client_id", "asset_id", "check_id"); !ok {
		return p, ErrProbeReport
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&p) != nil || decoder.Decode(new(any)) != io.EOF || p.Validate(scope, protocol) != nil {
		return ProbeReport{}, ErrProbeReport
	}
	return p, nil
}
