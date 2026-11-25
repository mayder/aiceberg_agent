package monitoring

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

func discoveryFingerprint(client, asset int64, source string, identity []string) (string, bool) {
	width := 1
	switch source {
	case "applications":
		width = 3
	case "interfaces", "disks", "sensors", "services", "log_sources":
	default:
		return "", false
	}
	if len(identity) != width {
		return "", false
	}
	parts := slices.Clone(identity)
	for i, part := range parts {
		parts[i] = strings.Trim(part, " \t\n\r\x00\x0b")
		if len(parts[i]) > 1020 || !utf8.ValidString(parts[i]) {
			return "", false
		}
	}
	if parts[0] == "" {
		return "", false
	}
	if source == "interfaces" {
		address, ok := discoveryInterfaceIdentity(parts[0])
		if !ok {
			return "", false
		}
		parts[0] = address
	}
	var identityJSON bytes.Buffer
	encoder := json.NewEncoder(&identityJSON)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(parts) != nil {
		return "", false
	}
	return hashDiscoveryJSON(client, asset, source, strings.TrimSuffix(identityJSON.String(), "\n")), true
}

func discoveryInterfaceIdentity(value string) (string, bool) {
	value = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(value, ":", ""), "-", ""))
	if len(value) != 12 || value == "000000000000" {
		return "", false
	}
	address, err := hex.DecodeString(value)
	return value, err == nil && address[0]&1 == 0
}

// The Web's approved identities use PHP's default json_encode byte format.
// Escape slash and Unicode here so existing approvals keep the same digest.
func hashDiscoveryJSON(client, asset int64, source, identity string) string {
	var wire strings.Builder
	wire.WriteByte('[')
	wire.WriteString(strconv.FormatInt(client, 10))
	wire.WriteByte(',')
	wire.WriteString(strconv.FormatInt(asset, 10))
	wire.WriteString(`,"`)
	wire.WriteString(source)
	wire.WriteString(`",`)
	for _, char := range identity {
		switch {
		case char == '/':
			wire.WriteString(`\/`)
		case char <= 0x7f:
			wire.WriteByte(byte(char))
		case char <= 0xffff:
			fmt.Fprintf(&wire, `\u%04x`, char)
		default:
			first, second := utf16.EncodeRune(char)
			fmt.Fprintf(&wire, `\u%04x\u%04x`, first, second)
		}
	}
	wire.WriteByte(']')
	digest := sha256.Sum256([]byte(wire.String()))
	return hex.EncodeToString(digest[:])
}
