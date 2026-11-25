package monitoring

import "regexp"

var httpProbeReference = regexp.MustCompile(`^http_probe_profile:[1-9][0-9]{0,15}$`)

func ValidHTTPProbeReference(reference string) bool {
	return httpProbeReference.MatchString(reference)
}

// HTTPProbeCredential accepts the optional reference and rejects unexpected
// fields, including inline credentials. Only numeric HTTP probes use it.
func HTTPProbeCredential(config map[string]any) (string, bool) {
	if len(config) == 1 {
		_, ok := config["metrics"]
		return "", ok
	}
	reference, ok := config["credential_ref"].(string)
	return reference, len(config) == 2 && ok && ValidHTTPProbeReference(reference)
}
