package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

const IPMISource = "ipmi"

var (
	ipmiReference = regexp.MustCompile(`^ipmi_profile:[1-9][0-9]{0,8}$`)
	ipmiMetricID  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,38}$`)
	ipmiSensor    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9 _./():+\-]{0,63}$`)
	errIPMI       = errors.New("invalid_ipmi_selection")
)

type IPMISelection struct {
	CredentialRef string
	Sensors       []IPMISensor
}

type IPMISensor struct {
	ID   string
	Name string
	Unit string
}

func ValidIPMIReference(reference string) bool { return ipmiReference.MatchString(reference) }

func IPMISelectors(config map[string]any) (IPMISelection, error) {
	ref, _ := config["credential_ref"].(string)
	values, ok := config["metrics"].([]any)
	if len(config) != 2 || !ok || len(values) < 1 || len(values) > 32 || !ValidIPMIReference(ref) {
		return IPMISelection{}, errIPMI
	}
	selection := IPMISelection{CredentialRef: ref, Sensors: make([]IPMISensor, 0, len(values))}
	ids := make(map[string]bool, len(values))
	for _, value := range values {
		raw, ok := value.(map[string]any)
		id, _ := raw["id"].(string)
		name, _ := raw["sensor"].(string)
		unit, _ := raw["unit"].(string)
		if !ok || len(raw) != 3 || !ipmiMetricID.MatchString(id) || ids[id] || !ipmiSensor.MatchString(name) ||
			strings.TrimSpace(name) != name || !definitionEnum(unit, "celsius", "rpm", "volts", "amps", "watts", "percent") {
			return IPMISelection{}, errIPMI
		}
		ids[id] = true
		selection.Sensors = append(selection.Sensors, IPMISensor{ID: id, Name: name, Unit: unit})
	}
	return selection, nil
}

func (s IPMISelection) selector(metric IPMISensor) NumericSelector {
	sum := sha256.Sum256([]byte(strings.Join([]string{s.CredentialRef, metric.Name, metric.Unit}, "\x00")))
	return NumericSelector{ID: metric.ID, Fingerprint: hex.EncodeToString(sum[:]), Unit: metric.Unit}
}

func (s IPMISelection) Reference(metric IPMISensor) string { return s.selector(metric).Reference() }

func ProjectIPMI(scope Scope, selection IPMISelection, observation Observation, observed, freshness int64) (Batch, error) {
	series := make([]NumericSelector, 0, len(selection.Sensors))
	for _, metric := range selection.Sensors {
		series = append(series, selection.selector(metric))
	}
	return projectNumericProbe(scope, IPMISource, series, observation, observed, freshness)
}
