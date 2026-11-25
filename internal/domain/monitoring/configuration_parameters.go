package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
)

var configurationIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
var configurationSecret = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}:[1-9][0-9]{0,15}$`)
var configurationReference = regexp.MustCompile(`^check-config:v1:([1-9][0-9]{0,15}):([1-9][0-9]{0,15}):([1-9][0-9]{0,15}):(icmp|tcp|http|dns|tls|snmp|snmp_trap|ipmi|vcenter|ssh|mysql|telnet|browser|jmx):[a-f0-9]{64}$`)

func configurationParameterType(name string) string {
	switch name {
	case "tipo", "nome", "metric", "source", "kind", "unit", "value_type", "pipeline_ref", "configuration_ref", "formula":
		return "text"
	case "interval_sec", "timeout_ms", "retries", "fail_threshold", "success_threshold", "freshness_sec", "history_days", "trends_days":
		return "integer"
	case "credential_ref":
		return "secret_ref"
	case "enabled":
		return "boolean"
	case "depends_on":
		return "text_list"
	}
	return ""
}

func validateConfigurationParameters(p map[string]ConfigurationValue, a ConfigurationAuthority) error {
	required := []string{"metric", "source", "kind", "unit", "value_type", "freshness_sec", "history_days", "trends_days", "enabled", "depends_on"}
	for _, name := range required {
		if _, ok := p[name]; !ok {
			return errors.New("incomplete_monitoring_configuration")
		}
	}
	if len(p) > 24 {
		return errors.New("invalid_monitoring_configuration")
	}
	for name, v := range p {
		if v.Type == "" || v.Type != configurationParameterType(name) || bytes.Equal(bytes.TrimSpace(v.Value), []byte("null")) || !recordJSONValid(v.Value) {
			return errors.New("invalid_monitoring_configuration")
		}
		if !configurationParameterValid(name, v, a) {
			return errors.New("invalid_monitoring_configuration")
		}
	}
	return nil
}

func configurationParameterValid(name string, v ConfigurationValue, a ConfigurationAuthority) bool {
	switch v.Type {
	case "integer":
		var n int64
		return json.Unmarshal(v.Value, &n) == nil && configurationInteger(name, n)
	case "boolean":
		var b bool
		return json.Unmarshal(v.Value, &b) == nil
	case "text_list":
		var list []string
		if json.Unmarshal(v.Value, &list) != nil || list == nil || len(list) > 16 {
			return false
		}
		seen := map[string]bool{}
		for _, key := range list {
			if !configurationIdentifier.MatchString(key) || seen[key] {
				return false
			}
			seen[key] = true
		}
		return true
	case "text", "secret_ref":
		var s string
		if json.Unmarshal(v.Value, &s) != nil || len(s) > 256 || !recordText(s, 256) {
			return false
		}
		return configurationText(name, s, a)
	}
	return false
}

func configurationInteger(name string, n int64) bool {
	switch name {
	case "interval_sec":
		return definitionInteger(n, 10, 604800)
	case "timeout_ms":
		return definitionInteger(n, 500, 300000)
	case "retries":
		return definitionInteger(n, 0, 5)
	case "fail_threshold", "success_threshold":
		return definitionInteger(n, 1, 100)
	case "freshness_sec":
		return definitionInteger(n, 1, 604800)
	case "history_days":
		return definitionInteger(n, 1, 3650)
	case "trends_days":
		return definitionInteger(n, 0, 3650)
	}
	return false
}

func configurationText(name, s string, a ConfigurationAuthority) bool {
	switch name {
	case "tipo":
		return definitionEnum(s, "icmp", "tcp", "http", "dns", "tls", "snmp", IPMISource, VCenterSource, SSHSource, MySQLSource, TelnetSource, BrowserSource, JMXSource, "snmp_trap")
	case "nome":
		return len(s) > 0 && len(s) <= 120
	case "source":
		return definitionEnum(s, "icmp", "tcp", "http", "dns", "tls", "snmp", "openmetrics", "jolokia", "postgresql", IPMISource, VCenterSource, SSHSource, MySQLSource, TelnetSource, BrowserSource, JMXSource, "snmp_trap", "host", "custom", "derived")
	case "kind":
		return definitionEnum(s, "collected", "dependent", "calculated")
	case "value_type":
		return definitionEnum(s, "number", "integer", "boolean", "text", "event")
	case "unit":
		return definitionUnit.MatchString(s)
	case "metric":
		return definitionMetric.MatchString(s)
	case "formula":
		return formulaTokens(s) != nil
	case "pipeline_ref":
		return configurationIdentifier.MatchString(s) && definitionEnum(s, a.PipelineReferences...)
	case "credential_ref":
		return configurationSecret.MatchString(s) && definitionEnum(s, a.SecretReferences...)
	case "configuration_ref":
		return configurationReference.MatchString(s)
	}
	return false
}
