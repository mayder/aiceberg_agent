package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

const VCenterSource = "vcenter"

var (
	vcenterReference = regexp.MustCompile(`^vcenter_profile:[1-9][0-9]{0,8}$`)
	vcenterObjectID  = regexp.MustCompile(`^(vm-)?[1-9][0-9]{0,17}$`)
	vcenterUUID      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	vcenterMetricID  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,38}$`)
	errVCenter       = errors.New("invalid_vcenter_selection")
)

type VCenterSelection struct {
	CredentialRef string
	VMID          string
	VMUUID        string
	Metrics       []VCenterMetric
}

type VCenterMetric struct {
	ID     string
	Preset string
}

func ValidVCenterReference(reference string) bool { return vcenterReference.MatchString(reference) }

func VCenterSelectors(config map[string]any) (VCenterSelection, error) {
	ref, _ := config["credential_ref"].(string)
	id, _ := config["vm_id"].(string)
	uuid, _ := config["vm_uuid"].(string)
	values, ok := config["metrics"].([]any)
	if len(config) != 4 || !ValidVCenterReference(ref) || !vcenterObjectID.MatchString(id) ||
		!vcenterUUID.MatchString(uuid) || !ok || len(values) < 1 || len(values) > 32 {
		return VCenterSelection{}, errVCenter
	}
	selection := VCenterSelection{CredentialRef: ref, VMID: id, VMUUID: uuid, Metrics: make([]VCenterMetric, 0, len(values))}
	ids := make(map[string]bool, len(values))
	for _, value := range values {
		raw, ok := value.(map[string]any)
		id, _ := raw["id"].(string)
		preset, _ := raw["preset"].(string)
		if !ok || len(raw) != 2 || !vcenterMetricID.MatchString(id) || ids[id] || vcenterUnit(preset) == "" {
			return VCenterSelection{}, errVCenter
		}
		ids[id] = true
		selection.Metrics = append(selection.Metrics, VCenterMetric{ID: id, Preset: preset})
	}
	return selection, nil
}

func vcenterUnit(preset string) string {
	switch preset {
	case "powered_on":
		return "none"
	case "cpu_usage_mhz":
		return "megahertz"
	case "active_memory_mb":
		return "megabytes"
	case "uptime_seconds":
		return "seconds"
	case "committed_storage_bytes":
		return "bytes"
	default:
		return ""
	}
}

func (s VCenterSelection) selector(metric VCenterMetric) NumericSelector {
	sum := sha256.Sum256([]byte(strings.Join([]string{s.CredentialRef, s.VMID, s.VMUUID, metric.Preset}, "\x00")))
	return NumericSelector{ID: metric.ID, Fingerprint: hex.EncodeToString(sum[:]), Unit: vcenterUnit(metric.Preset)}
}

func (s VCenterSelection) Reference(metric VCenterMetric) string {
	return s.selector(metric).Reference()
}

func ProjectVCenter(scope Scope, selection VCenterSelection, observation Observation, observed, freshness int64) (Batch, error) {
	series := make([]NumericSelector, 0, len(selection.Metrics))
	for _, metric := range selection.Metrics {
		series = append(series, selection.selector(metric))
	}
	return projectNumericProbe(scope, VCenterSource, series, observation, observed, freshness)
}
