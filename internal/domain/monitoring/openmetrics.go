package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const OpenMetricsSource = "openmetrics"

var (
	openMetricsID           = regexp.MustCompile(`^[a-z][a-z0-9_]{0,38}$`)
	openMetricsName         = regexp.MustCompile(`^[A-Za-z_:][A-Za-z0-9_:]{0,127}$`)
	openMetricsLabel        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	errOpenMetricsSelection = errors.New("invalid_openmetrics_selection")
)

type OpenMetricsSelector struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Labels        map[string]string `json:"labels"`
	credentialRef string
}

func OpenMetricsSelectors(config map[string]any) ([]OpenMetricsSelector, error) {
	raw, ok := config["metrics"].([]any)
	credentialRef, valid := HTTPProbeCredential(config)
	if !valid || !ok || len(raw) < 1 || len(raw) > 32 {
		return nil, errOpenMetricsSelection
	}
	result := make([]OpenMetricsSelector, 0, len(raw))
	ids := make(map[string]bool, len(raw))
	for _, item := range raw {
		selector, err := parseOpenMetricsSelector(item)
		if err != nil || ids[selector.ID] {
			return nil, errOpenMetricsSelection
		}
		selector.credentialRef = credentialRef
		ids[selector.ID] = true
		result = append(result, selector)
	}
	return result, nil
}

func parseOpenMetricsSelector(raw any) (OpenMetricsSelector, error) {
	item, ok := raw.(map[string]any)
	if !ok || len(item) != 3 {
		return OpenMetricsSelector{}, errOpenMetricsSelection
	}
	id, _ := item["id"].(string)
	name, _ := item["name"].(string)
	labels, ok := item["labels"].(map[string]any)
	if !openMetricsID.MatchString(id) || !openMetricsName.MatchString(name) || !ok || len(labels) > 8 {
		return OpenMetricsSelector{}, errOpenMetricsSelection
	}
	selector := OpenMetricsSelector{ID: id, Name: name, Labels: make(map[string]string, len(labels))}
	for label, rawValue := range labels {
		value, ok := rawValue.(string)
		if !openMetricsLabel.MatchString(label) || strings.HasPrefix(label, "__") || !ok ||
			len(value) > 120 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return OpenMetricsSelector{}, errOpenMetricsSelection
		}
		selector.Labels[label] = value
	}
	return selector, nil
}

// Reference binds a value to the exact configured series independently of label order.
func (s OpenMetricsSelector) numericSelector() NumericSelector {
	input := []string{s.Name}
	keys := make([]string, 0, len(s.Labels))
	for key := range s.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		input = append(input, key, s.Labels[key])
	}
	if s.credentialRef != "" {
		input = append(input, s.credentialRef)
	}
	sum := sha256.Sum256([]byte(strings.Join(input, "\x00")))
	return NumericSelector{ID: s.ID, Fingerprint: hex.EncodeToString(sum[:])}
}

func (s OpenMetricsSelector) Reference() string { return s.numericSelector().Reference() }

func ProjectOpenMetrics(scope Scope, selectors []OpenMetricsSelector, observation Observation, observed, freshness int64) (Batch, error) {
	series := make([]NumericSelector, 0, len(selectors))
	for _, selector := range selectors {
		series = append(series, selector.numericSelector())
	}
	return projectNumericProbe(scope, OpenMetricsSource, series, observation, observed, freshness)
}
