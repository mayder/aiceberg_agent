package monitoring

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var recordEventID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
var recordEventCode = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func validRecordValue(kind string, raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > MaxRecordBytes || bytes.Equal(raw, []byte("null")) || !recordJSONValid(raw) {
		return false
	}
	switch kind {
	case "number", "integer":
		number, err := strconv.ParseFloat(string(raw), 64)
		if !json.Valid(raw) || err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return false
		}
		mantissa := strings.SplitN(strings.ToLower(string(raw)), "e", 2)[0]
		if number == 0 && strings.Trim(mantissa, "-+.0") != "" {
			return false
		}
		return kind == "number" || (math.Abs(number) <= 9007199254740991 && math.Trunc(number) == number)
	case "boolean":
		return bytes.Equal(raw, []byte("true")) || bytes.Equal(raw, []byte("false"))
	case "text":
		var value string
		return json.Unmarshal(raw, &value) == nil && recordText(value, 4096)
	case "event":
		_, ok := decodeRecordEvent(raw)
		return ok
	}
	return false
}

func decodeRecordEvent(raw []byte) (RecordEvent, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || !definitionFields(fields, "event_id", "code", "message") {
		return RecordEvent{}, false
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return RecordEvent{}, false
		}
	}
	var value RecordEvent
	if json.Unmarshal(raw, &value) != nil || !recordEventID.MatchString(value.EventID) || !recordEventCode.MatchString(value.Code) || !recordText(value.Message, 1024) {
		return RecordEvent{}, false
	}
	return value, true
}

func recordText(value string, maxBytes int) bool {
	if len(value) > maxBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
