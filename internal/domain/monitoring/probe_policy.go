package monitoring

import (
	"errors"
	"strconv"
)

const ProbePolicyContract = "monitoring.probe-policy.v1"

var ErrProbePolicy = errors.New("invalid_monitoring_probe_policy")

type ProbePolicy struct {
	Contract      string `json:"contract"`
	BudgetMS      int64  `json:"budget_ms"`
	FreshnessSec  int64  `json:"freshness_sec"`
	CredentialRef string `json:"credential_ref,omitempty"`
}

// ProbeFreshness uses the persisted check interval, before any segment override.
func ProbeFreshness(intervalSec int64) int64 {
	return max(int64(20), min(intervalSec, 201600)) * 3
}

// SimpleProbePolicy mirrors the connection timeouts already enforced by the
// legacy ICMP, TCP, HTTP, DNS and TLS implementations.
func SimpleProbePolicy(protocol string, timeoutMS, intervalSec int64) (ProbePolicy, bool) {
	var minimum int64
	switch protocol {
	case "icmp", "tcp":
		minimum = 1000
	case "dns":
		minimum = 2000
	case "http", "tls":
		minimum = 5000
	default:
		return ProbePolicy{}, false
	}
	p := ProbePolicy{Contract: ProbePolicyContract, BudgetMS: max(timeoutMS, minimum), FreshnessSec: ProbeFreshness(intervalSec)}
	return p, p.Validate(protocol, 0) == nil
}

// A reference may identify the resolved profile only. The reference never
// selects a credential or overrides the already authorized job envelope.
func (p ProbePolicy) Validate(protocol string, resolvedProfileID int64) error {
	if p.Contract != ProbePolicyContract || p.BudgetMS < 0 || p.BudgetMS > 86400000 ||
		p.FreshnessSec < 60 || p.FreshnessSec > 604800 {
		return ErrProbePolicy
	}
	if protocol == "snmp" {
		if resolvedProfileID < 1 || resolvedProfileID > 2147483647 ||
			p.CredentialRef != "snmp_profile:"+strconv.FormatInt(resolvedProfileID, 10) {
			return ErrProbePolicy
		}
		return nil
	}
	if protocol == JMXSource {
		if resolvedProfileID != 0 || !ValidJMXReference(p.CredentialRef) || p.BudgetMS < 1000 || p.BudgetMS > 20000 {
			return ErrProbePolicy
		}
		return nil
	}
	if protocol == BrowserSource {
		if resolvedProfileID != 0 || p.CredentialRef != "" || p.BudgetMS < 1000 || p.BudgetMS > 30000 {
			return ErrProbePolicy
		}
		return nil
	}
	if protocol == TelnetSource {
		if resolvedProfileID != 0 || p.CredentialRef != "" || p.BudgetMS < 1 || p.BudgetMS > 5000 {
			return ErrProbePolicy
		}
		return nil
	}
	if protocol == MySQLSource {
		if resolvedProfileID != 0 || !ValidMySQLReference(p.CredentialRef) || p.BudgetMS < 1 || p.BudgetMS > 10000 {
			return ErrProbePolicy
		}
		return nil
	}
	if protocol == SSHSource {
		if resolvedProfileID != 0 || !ValidSSHReference(p.CredentialRef) || p.BudgetMS < 1 || p.BudgetMS > 10000 {
			return ErrProbePolicy
		}
		return nil
	}
	if protocol == VCenterSource {
		if resolvedProfileID != 0 || !ValidVCenterReference(p.CredentialRef) || p.BudgetMS < 1 || p.BudgetMS > 20000 {
			return ErrProbePolicy
		}
		return nil
	}
	if protocol == IPMISource {
		if resolvedProfileID != 0 || !ValidIPMIReference(p.CredentialRef) || p.BudgetMS < 1 || p.BudgetMS > 20000 {
			return ErrProbePolicy
		}
		return nil
	}
	if protocol == PostgresSource {
		if resolvedProfileID != 0 || !ValidPostgresReference(p.CredentialRef) || p.BudgetMS < 1 || p.BudgetMS > 10000 {
			return ErrProbePolicy
		}
		return nil
	}
	if protocol == OpenMetricsSource || protocol == JolokiaSource {
		if resolvedProfileID != 0 || p.BudgetMS < 1 || p.BudgetMS > 10000 ||
			(p.CredentialRef != "" && !ValidHTTPProbeReference(p.CredentialRef)) {
			return ErrProbePolicy
		}
		return nil
	}
	if p.CredentialRef != "" || p.BudgetMS < 1 || resolvedProfileID != 0 {
		return ErrProbePolicy
	}
	switch protocol {
	case "icmp", "tcp", "http", "dns", "tls":
		return nil
	default:
		return ErrProbePolicy
	}
}
