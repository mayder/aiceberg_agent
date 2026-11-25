package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
)

const SSHSource = "ssh"

var (
	sshReference = regexp.MustCompile(`^ssh_probe_profile:[1-9][0-9]{0,8}$`)
	sshMetricID  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,38}$`)
	errSSH       = errors.New("invalid_ssh_selection")
)

type SSHSelection struct {
	CredentialRef string
	Metrics       []SSHMetric
}

type SSHMetric struct {
	ID     string
	Preset string
}

func ValidSSHReference(reference string) bool { return sshReference.MatchString(reference) }

func SSHSelectors(config map[string]any) (SSHSelection, error) {
	reference, _ := config["credential_ref"].(string)
	metrics, ok := config["metrics"].([]any)
	if len(config) != 2 || !ValidSSHReference(reference) || !ok || len(metrics) < 1 || len(metrics) > 32 {
		return SSHSelection{}, errSSH
	}
	selection := SSHSelection{CredentialRef: reference, Metrics: make([]SSHMetric, 0, len(metrics))}
	ids := make(map[string]bool, len(metrics))
	for _, value := range metrics {
		raw, ok := value.(map[string]any)
		id, _ := raw["id"].(string)
		preset, _ := raw["preset"].(string)
		if !ok || len(raw) != 2 || !sshMetricID.MatchString(id) || ids[id] || sshUnit(preset) == "" {
			return SSHSelection{}, errSSH
		}
		ids[id] = true
		selection.Metrics = append(selection.Metrics, SSHMetric{ID: id, Preset: preset})
	}
	return selection, nil
}

func sshUnit(preset string) string {
	switch preset {
	case "load_1", "load_5", "load_15":
		return "none"
	case "uptime_seconds":
		return "seconds"
	case "memory_total_bytes", "memory_available_bytes":
		return "bytes"
	default:
		return ""
	}
}

func (s SSHSelection) selector(metric SSHMetric) NumericSelector {
	sum := sha256.Sum256([]byte(s.CredentialRef + "\x00" + metric.Preset))
	return NumericSelector{ID: metric.ID, Fingerprint: hex.EncodeToString(sum[:]), Unit: sshUnit(metric.Preset)}
}

func (s SSHSelection) Reference(metric SSHMetric) string { return s.selector(metric).Reference() }

func ProjectSSH(scope Scope, selection SSHSelection, observation Observation, observed, freshness int64) (Batch, error) {
	series := make([]NumericSelector, 0, len(selection.Metrics))
	for _, metric := range selection.Metrics {
		series = append(series, selection.selector(metric))
	}
	return projectNumericProbe(scope, SSHSource, series, observation, observed, freshness)
}
