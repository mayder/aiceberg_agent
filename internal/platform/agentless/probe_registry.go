package agentless

import (
	"context"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type probeClassification struct{ outcome, reason string }

var (
	probeConfiguration = probeClassification{"failed", "configuration"}
	probeConnection    = probeClassification{"failed", "connection"}
	probeResponse      = probeClassification{"failed", "response"}
	probeCancelled     = probeClassification{"cancelled", "cancelled"}
	probeDeadline      = probeClassification{"timeout", "deadline"}
	probeOverBudget    = probeClassification{"timeout", "budget"}
)

// Protocol-specific stages stay with the adapter. The dispatcher supplies
// shared handling for cancellation, scope and the enclosing context budget.
var probeAdapters = map[string]probeAdapter{
	"icmp": regularProbe(runICMP, map[string]probeClassification{"ping_fail": probeConnection}),
	"tcp":  regularProbe(runTCP, map[string]probeClassification{"connect_fail": probeConnection}),
	"dns":  regularProbe(runDNS, map[string]probeClassification{"dns_fail": probeConnection}),
	"tls": regularProbe(runTLS, map[string]probeClassification{
		"tls_fail": probeConnection, "cert_expired": probeResponse,
	}),
	"http": regularProbe(func(ctx context.Context, job entities.AgentlessJob) Result {
		return runHTTP(ctx, job, false)
	}, map[string]probeClassification{
		"invalid_url": probeConfiguration, "http_request": probeConfiguration,
		"http_fail": probeConnection, "http_status": probeResponse,
	}),
	"openmetrics": regularProbe(runOpenMetrics, exporterProbeFailures("openmetrics")),
	"jolokia":     regularProbe(runJolokia, exporterProbeFailures("jolokia")),
	"postgresql": regularProbe(runPostgres, map[string]probeClassification{
		"invalid_postgresql_config": probeConfiguration, "postgresql_credential_unavailable": probeConfiguration,
		"postgresql_access_denied": probeConfiguration, "postgresql_connection_failed": probeConnection,
		"postgresql_read_failed": probeResponse, "postgresql_statement_timeout": probeDeadline,
	}),
	"mysql": regularProbe(runMySQL, map[string]probeClassification{
		"invalid_mysql_config": probeConfiguration, "mysql_credential_unavailable": probeConfiguration,
		"mysql_access_denied": probeConfiguration, "mysql_connection_failed": probeConnection,
		"mysql_auth_plugin_unsupported": probeConfiguration,
		"mysql_read_failed":             probeResponse, "invalid_mysql_response": probeResponse,
	}),
	"ipmi": regularProbe(runIPMI, map[string]probeClassification{
		"invalid_ipmi_config": probeConfiguration, "ipmi_credential_unavailable": probeConfiguration,
		"ipmi_tool_unavailable": probeConfiguration, "ipmi_read_failed": probeResponse,
		"invalid_ipmi_response": probeResponse,
	}),
	"vcenter": regularProbe(runVCenter, map[string]probeClassification{
		"invalid_vcenter_config": probeConfiguration, "vcenter_credential_unavailable": probeConfiguration,
		"vcenter_access_denied": probeConfiguration, "vcenter_scope_mismatch": probeConfiguration,
		"vcenter_connection_failed": probeConnection, "invalid_vcenter_response": probeResponse,
		"vcenter_query_timeout": probeOverBudget,
	}),
	"ssh": regularProbe(runSSHProbe, map[string]probeClassification{
		"invalid_ssh_config": probeConfiguration, "ssh_credential_unavailable": probeConfiguration,
		"ssh_host_key_mismatch": probeConfiguration, "ssh_access_denied": probeConfiguration,
		"ssh_connection_failed": probeConnection,
		"ssh_read_failed":       probeResponse, "invalid_ssh_response": probeResponse,
	}),
	"telnet": regularProbe(runTelnetProbe, map[string]probeClassification{
		"invalid_telnet_config": probeConfiguration, "telnet_target_not_allowed": probeConfiguration,
		"telnet_worker_failed": probeConfiguration, "telnet_connection_failed": probeConnection,
		"telnet_unrecognized_protocol": probeResponse, "telnet_response_limit": probeResponse,
		"telnet_read_failed": probeResponse,
	}),
	"jmx": regularProbe(runJMXProbe, map[string]probeClassification{
		"invalid_jmx_config": probeConfiguration, "jmx_invalid_profile": probeConfiguration,
		"jmx_worker_unavailable": probeConfiguration, "jmx_credential_unavailable": probeConfiguration,
		"jmx_access_denied": probeConfiguration, "jmx_endpoint_not_allowed": probeConfiguration,
		"jmx_tls_failed": probeConfiguration, "jmx_unavailable": probeConnection,
		"jmx_response_rejected": probeResponse, "jmx_timeout": probeOverBudget,
	}),
	"browser": regularProbe(runBrowserProbe, map[string]probeClassification{
		"invalid_browser_config": probeConfiguration, "browser_invalid_recipe": probeConfiguration,
		"browser_not_allowed": probeConfiguration, "browser_runner_unavailable": probeConfiguration,
		"browser_busy": probeConfiguration, "browser_step_failed": probeResponse,
		"browser_resource_limit": probeResponse, "browser_cancelled": probeCancelled,
		"browser_timeout": probeOverBudget,
	}),
	"discovery": regularProbe(runDiscoveryAssisted, map[string]probeClassification{
		"discovery_policy_invalid": probeConfiguration, "discovery_canceled": probeCancelled,
	}),
	"snmp": {
		collect:       collectSNMPProbe,
		flushPartials: shouldFlushSNMPPartials,
		cost:          snmpObservationCost,
		failures: map[string]probeClassification{
			"snmp_no_profile": probeConfiguration, "snmp_auth": probeConfiguration,
			"snmp_refused": probeConnection, "snmp_timeout": probeDeadline,
			"snmp_time_budget_exceeded": probeOverBudget, "snmp_cancelled": probeCancelled,
			"snmp_canceled": probeCancelled, "snmp_unsupported_version": {"unsupported", "version"},
		},
	},
}

func exporterProbeFailures(protocol string) map[string]probeClassification {
	return map[string]probeClassification{
		"invalid_" + protocol + "_config":   probeConfiguration,
		"invalid_" + protocol + "_response": probeResponse,
		httpCredentialUnavailable:           probeConfiguration,
		httpAccessDenied:                    probeConfiguration,
		"http_fail":                         probeConnection,
		"http_status":                       probeResponse,
	}
}

func regularProbe(run func(context.Context, entities.AgentlessJob) Result, failures map[string]probeClassification) probeAdapter {
	return probeAdapter{
		collect: func(ctx context.Context, job entities.AgentlessJob, _ probeSegmentSink) Result {
			return run(ctx, job)
		},
		failures: failures,
	}
}

func collectSNMPProbe(ctx context.Context, job entities.AgentlessJob, sink probeSegmentSink) Result {
	if sink == nil {
		return runSNMP(ctx, job)
	}
	return runSNMPWithPartials(ctx, job, func(partial snmpPartialResult) {
		sink(probeSegment(partial))
	})
}
