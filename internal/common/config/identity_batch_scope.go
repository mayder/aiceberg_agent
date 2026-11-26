package config

import (
	"bytes"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"
	"strings"
)

// VerifiedIdentityBatchScope never creates a replacement claim. Only existing,
// independently valid claims with exactly the same signed subject may share a request.
// Unknown/legacy identities keep their original forwarding boundary.
func VerifiedIdentityBatchScope(header, auth string, now int64) (string, int64, bool) {
	parts := strings.Fields(auth)
	if len(header) > 4096 || len(parts) != 2 || parts[0] != "Token" {
		return "", 0, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil {
		return "", 0, false
	}
	var claim struct {
		Schema         string `json:"schema_version"`
		ClientID       int64  `json:"cliente_id"`
		AgentID        int64  `json:"agente_id"`
		InstallationID string `json:"installation_id"`
		HostGUID       string `json:"host_guid"`
		IssuedAt       int64  `json:"issued_at"`
		Nonce          string `json:"nonce"`
		Signature      string `json:"signature"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&claim) != nil || decoder.Decode(new(any)) != io.EOF || claim.Schema != AgentIdentitySchemaVersion ||
		claim.ClientID <= 0 || claim.AgentID <= 0 || claim.IssuedAt < now-604800 || claim.IssuedAt > now+604800 ||
		len(claim.InstallationID) > 120 || len(claim.HostGUID) > 120 || len(claim.Nonce) == 0 || len(claim.Nonce) > 80 || len(claim.Signature) != 64 {
		return "", 0, false
	}
	values := map[string]any{"schema_version": claim.Schema, "cliente_id": claim.ClientID, "agente_id": claim.AgentID,
		"installation_id": claim.InstallationID, "host_guid": claim.HostGUID, "issued_at": claim.IssuedAt, "nonce": claim.Nonce}
	expected := signAgentIdentity(values, parts[1])
	if !hmac.Equal([]byte(expected), []byte(strings.ToLower(claim.Signature))) {
		return "", 0, false
	}
	scope, err := json.Marshal([]string{claim.Schema, strconv.FormatInt(claim.ClientID, 10), strconv.FormatInt(claim.AgentID, 10),
		strings.TrimSpace(claim.InstallationID), strings.TrimSpace(claim.HostGUID)})
	return string(scope), claim.IssuedAt, err == nil
}
