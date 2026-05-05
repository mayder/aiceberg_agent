package config

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const AgentIdentitySchemaVersion = "agent_identity.v1"

// AgentIdentityClaim builds a signed operational identity hint for the backend.
// Authorization still comes from the agent token; this claim only proves that
// the declared client/agent metadata was provisioned with the same signing secret.
func (c Config) AgentIdentityClaim(hostGUID string) map[string]any {
	secret := strings.TrimSpace(c.AgentIdentitySecret)
	if secret == "" {
		secret = strings.TrimSpace(c.Agent.Token)
	}
	if secret == "" {
		return nil
	}

	installationID := strings.TrimSpace(c.AgentInstallationID)
	hostGUID = strings.TrimSpace(hostGUID)
	if c.AgentClientID <= 0 && c.AgentID <= 0 && installationID == "" && hostGUID == "" {
		return nil
	}

	claim := map[string]any{
		"schema_version": AgentIdentitySchemaVersion,
		"issued_at":      time.Now().Unix(),
		"nonce":          nonce(),
	}
	if c.AgentClientID > 0 {
		claim["cliente_id"] = c.AgentClientID
	}
	if c.AgentID > 0 {
		claim["agente_id"] = c.AgentID
	}
	if installationID != "" {
		claim["installation_id"] = installationID
	}
	if hostGUID != "" {
		claim["host_guid"] = hostGUID
	}
	claim["signature"] = signAgentIdentity(claim, secret)
	return claim
}

func (c Config) AgentIdentityHeader(hostGUID string) string {
	claim := c.AgentIdentityClaim(hostGUID)
	if len(claim) == 0 {
		return ""
	}
	raw, err := json.Marshal(claim)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func signAgentIdentity(claim map[string]any, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(agentIdentityPayload(claim)))
	return hex.EncodeToString(mac.Sum(nil))
}

func agentIdentityPayload(claim map[string]any) string {
	return strings.Join([]string{
		AgentIdentitySchemaVersion,
		intField(claim, "cliente_id"),
		intField(claim, "agente_id"),
		stringField(claim, "installation_id"),
		stringField(claim, "host_guid"),
		intField(claim, "issued_at"),
		stringField(claim, "nonce"),
	}, "|")
}

func intField(claim map[string]any, key string) string {
	switch v := claim[key].(type) {
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func stringField(claim map[string]any, key string) string {
	if v, ok := claim[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func nonce() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
