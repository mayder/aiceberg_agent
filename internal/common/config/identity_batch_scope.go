package config

import (
	"bytes"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
)

// VerifiedIdentityBatchScope never creates a replacement claim. Only existing,
// independently valid claims with exactly the same signed subject may share a request.
// Unknown/legacy identities keep their original forwarding boundary.
func VerifiedIdentityBatchScope(header, auth string, now int64) (string, int64, bool) {
	scope, issued, reason := IdentityBatchScopeEvidence(header, auth, now)
	return scope, issued, reason == "verified"
}

// IdentityBatchScopeEvidence returns only a bounded reason, never credentials or claim values.
func IdentityBatchScopeEvidence(header, auth string, now int64) (string, int64, string) {
	parts := strings.Fields(auth)
	if len(header) > 4096 || len(parts) != 2 || parts[0] != "Token" {
		return "", 0, "unsupported_auth"
	}
	raw, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil {
		return "", 0, "invalid_encoding"
	}
	var claim struct {
		Schema         string `json:"schema_version"`
		ClientID       *int64 `json:"cliente_id"`
		AgentID        *int64 `json:"agente_id"`
		InstallationID string `json:"installation_id"`
		HostGUID       string `json:"host_guid"`
		IssuedAt       int64  `json:"issued_at"`
		Nonce          string `json:"nonce"`
		Signature      string `json:"signature"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&claim) != nil || decoder.Decode(new(any)) != io.EOF || claim.Schema != AgentIdentitySchemaVersion ||
		(claim.ClientID != nil && *claim.ClientID <= 0) || (claim.AgentID != nil && *claim.AgentID <= 0) || claim.IssuedAt < now-604800 || claim.IssuedAt > now+604800 ||
		len(claim.InstallationID) > 120 || len(claim.HostGUID) > 120 || len(claim.Nonce) == 0 || len(claim.Nonce) > 80 || len(claim.Signature) != 64 {
		return "", 0, "invalid_claim"
	}
	values := map[string]any{"schema_version": claim.Schema,
		"installation_id": claim.InstallationID, "host_guid": claim.HostGUID, "issued_at": claim.IssuedAt, "nonce": claim.Nonce}
	if claim.ClientID != nil {
		values["cliente_id"] = *claim.ClientID
	}
	if claim.AgentID != nil {
		values["agente_id"] = *claim.AgentID
	}
	if claim.AgentID == nil && strings.TrimSpace(claim.InstallationID) == "" && strings.TrimSpace(claim.HostGUID) == "" {
		return "", 0, "subject_missing"
	}
	expected := signAgentIdentity(values, parts[1])
	if !hmac.Equal([]byte(expected), []byte(strings.ToLower(claim.Signature))) {
		return "", 0, "signature_invalid"
	}
	scope, err := json.Marshal([]string{claim.Schema, intField(values, "cliente_id"), intField(values, "agente_id"),
		strings.TrimSpace(claim.InstallationID), strings.TrimSpace(claim.HostGUID)})
	if err != nil {
		return "", 0, "invalid_scope"
	}
	return string(scope), claim.IssuedAt, "verified"
}
