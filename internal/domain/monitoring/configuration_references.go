package monitoring

import "regexp"

func configurationReferences(values []string, pattern *regexp.Regexp) bool {
	if values == nil {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !pattern.MatchString(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}
