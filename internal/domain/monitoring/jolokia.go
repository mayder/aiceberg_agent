package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strings"
)

const JolokiaSource = "jolokia"

var (
	jolokiaID           = regexp.MustCompile(`^[a-z][a-z0-9_]{0,38}$`)
	jolokiaField        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	jolokiaPathPart     = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	jolokiaDomain       = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,96}$`)
	jolokiaProperty     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}=[A-Za-z0-9_. /@+\-]{1,128}$`)
	errJolokiaSelection = errors.New("invalid_jolokia_selection")
)

type JolokiaSelector struct {
	ID, MBean, Attribute, Path string
	credentialRef              string
}

func JolokiaSelectors(config map[string]any) ([]JolokiaSelector, error) {
	raw, ok := config["metrics"].([]any)
	credentialRef, valid := HTTPProbeCredential(config)
	if !valid || !ok || len(raw) < 1 || len(raw) > 32 {
		return nil, errJolokiaSelection
	}
	result := make([]JolokiaSelector, 0, len(raw))
	ids := make(map[string]bool, len(raw))
	for _, item := range raw {
		selector, err := parseJolokiaSelector(item)
		if err != nil || ids[selector.ID] {
			return nil, errJolokiaSelection
		}
		selector.credentialRef = credentialRef
		ids[selector.ID] = true
		result = append(result, selector)
	}
	return result, nil
}

func parseJolokiaSelector(raw any) (JolokiaSelector, error) {
	item, ok := raw.(map[string]any)
	if !ok || len(item) != 4 {
		return JolokiaSelector{}, errJolokiaSelection
	}
	id, _ := item["id"].(string)
	attr, _ := item["attribute"].(string)
	mbean, _ := item["mbean"].(string)
	mbean = CanonicalJolokiaMBean(mbean)
	path, ok := item["path"].(string)
	if !jolokiaID.MatchString(id) || !jolokiaField.MatchString(attr) || mbean == "" || !ok || !validJolokiaPath(path) {
		return JolokiaSelector{}, errJolokiaSelection
	}
	return JolokiaSelector{ID: id, MBean: mbean, Attribute: attr, Path: path}, nil
}

func CanonicalJolokiaMBean(raw string) string {
	domain, rawProperties, ok := strings.Cut(raw, ":")
	if !ok || len(raw) > 256 || !jolokiaDomain.MatchString(domain) {
		return ""
	}
	properties := strings.Split(rawProperties, ",")
	keys := make(map[string]bool, len(properties))
	for _, property := range properties {
		key, _, _ := strings.Cut(property, "=")
		if !jolokiaProperty.MatchString(property) || keys[key] {
			return ""
		}
		keys[key] = true
	}
	sort.Strings(properties)
	return domain + ":" + strings.Join(properties, ",")
}

func validJolokiaPath(path string) bool {
	if path == "" {
		return true
	}
	parts := strings.Split(path, "/")
	if len(parts) > 4 {
		return false
	}
	for _, part := range parts {
		if !jolokiaPathPart.MatchString(part) || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func (s JolokiaSelector) numericSelector() NumericSelector {
	input := []string{s.MBean, s.Attribute, s.Path}
	if s.credentialRef != "" {
		input = append(input, s.credentialRef)
	}
	sum := sha256.Sum256([]byte(strings.Join(input, "\x00")))
	return NumericSelector{ID: s.ID, Fingerprint: hex.EncodeToString(sum[:])}
}

func (s JolokiaSelector) Reference() string { return s.numericSelector().Reference() }

func ProjectJolokia(scope Scope, selectors []JolokiaSelector, observation Observation, observed, freshness int64) (Batch, error) {
	series := make([]NumericSelector, 0, len(selectors))
	for _, selector := range selectors {
		series = append(series, selector.numericSelector())
	}
	return projectNumericProbe(scope, JolokiaSource, series, observation, observed, freshness)
}
