package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
)

const TelnetSource = "telnet"

var (
	telnetMetricID     = regexp.MustCompile(`^[a-z][a-z0-9_]{0,38}$`)
	errTelnetSelection = errors.New("invalid_telnet_selection")
)

type TelnetSelection struct {
	BannerID string
}

func TelnetSelectors(config map[string]any) (TelnetSelection, error) {
	enabled, _ := config["legacy_opt_in"].(bool)
	metrics, ok := config["metrics"].([]any)
	if len(config) != 2 || !enabled || !ok || len(metrics) != 1 {
		return TelnetSelection{}, errTelnetSelection
	}
	metric, _ := metrics[0].(map[string]any)
	id, _ := metric["id"].(string)
	if len(metric) != 2 || !telnetMetricID.MatchString(id) || metric["preset"] != "banner_present" {
		return TelnetSelection{}, errTelnetSelection
	}
	return TelnetSelection{BannerID: id}, nil
}

func (s TelnetSelection) selector() NumericSelector {
	fingerprint := sha256.Sum256([]byte("telnet_banner_present_v1"))
	return NumericSelector{ID: s.BannerID, Fingerprint: hex.EncodeToString(fingerprint[:]), Unit: "none"}
}

func (s TelnetSelection) Reference() string { return s.selector().Reference() }

func ProjectTelnet(scope Scope, selection TelnetSelection, observation Observation, observed, freshness int64) (Batch, error) {
	selector := selection.selector()
	value, ok := NumericProbeValue(NumericProbeValues(observation.Payload, TelnetSource)[selector.Reference()])
	if !ok || (value != 0 && value != 1) {
		observation.Payload = nil
	}
	return projectNumericProbe(scope, TelnetSource, []NumericSelector{selector}, observation, observed, freshness)
}
