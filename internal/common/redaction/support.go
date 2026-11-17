package redaction

import (
	"regexp"
	"strings"
)

// SupportText omits credential-bearing lines regardless of their log format.
var sensitiveSupportLine = regexp.MustCompile(`(?i)(password|passwd|\bpwd\b|token|secret|authorization|cookie|api[_-]?key|private[_ -]?key|credential|://[^\s/]+:[^\s/]+@)`)

func SupportText(value string) string {
	lines := strings.Split(strings.ToValidUTF8(value, ""), "\n")
	for i, line := range lines {
		if sensitiveSupportLine.MatchString(line) {
			lines[i] = "[redacted sensitive line]"
		}
	}
	return strings.Join(lines, "\n")
}
