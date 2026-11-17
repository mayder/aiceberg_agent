package support

import (
	"github.com/you/aiceberg_agent/internal/common/redaction"
	"unicode/utf8"
)

func redact(value string) string {
	out := redaction.SupportText(value)
	if len(out) > MaxBytes {
		out = out[:MaxBytes]
	}
	for !utf8.ValidString(out) {
		out = out[:len(out)-1]
	}
	return out
}
