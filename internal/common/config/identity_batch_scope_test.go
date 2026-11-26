package config

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func batchClaim(values map[string]any) string {
	values["signature"] = signAgentIdentity(values, "synthetic-secret")
	raw, _ := json.Marshal(values)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TestIdentityBatchScopeRequiresEveryOriginalSignatureAndSubject(t *testing.T) {
	const now = int64(1800000000)
	base := func() map[string]any {
		return map[string]any{"schema_version": AgentIdentitySchemaVersion, "cliente_id": int64(2), "agente_id": int64(84),
			"installation_id": "install", "host_guid": "host", "issued_at": now, "nonce": "first"}
	}
	first, _, valid := VerifiedIdentityBatchScope(batchClaim(base()), "Token synthetic-secret", now)
	if !valid {
		t.Fatal("official signer not accepted")
	}
	for _, field := range []string{"cliente_id", "agente_id", "installation_id", "host_guid"} {
		claim := base()
		if field == "cliente_id" || field == "agente_id" {
			claim[field] = int64(99)
		} else {
			claim[field] = "other"
		}
		scope, _, ok := VerifiedIdentityBatchScope(batchClaim(claim), "Token synthetic-secret", now)
		if !ok || scope == first {
			t.Fatalf("signed subject boundary lost: %s", field)
		}
	}
	changed := base()
	changed["nonce"], changed["issued_at"] = "second", now-30
	if scope, issued, ok := VerifiedIdentityBatchScope(batchClaim(changed), "Token synthetic-secret", now); !ok || scope != first || issued != now-30 {
		t.Fatal("ephemeral fields still fragment the same verified subject")
	}
}

func TestIdentityBatchScopeConservativelyPreservesUnknownOrInvalidClaims(t *testing.T) {
	const now = int64(1800000000)
	for _, tc := range []struct {
		name  string
		field string
		value any
	}{{"expired", "issued_at", now - 604801}, {"future", "issued_at", now + 604801},
		{"missing agent", "agente_id", 0}, {"schema", "schema_version", "future"},
		{"empty nonce", "nonce", ""}, {"long host", "host_guid", strings.Repeat("h", 121)},
		{"unknown signed scope", "new_scope", "not-supported"}} {
		t.Run(tc.name, func(t *testing.T) {
			claim := map[string]any{"schema_version": AgentIdentitySchemaVersion, "cliente_id": 2, "agente_id": 84, "issued_at": now, "nonce": "nonce"}
			claim[tc.field] = tc.value
			if _, _, valid := VerifiedIdentityBatchScope(batchClaim(claim), "Token synthetic-secret", now); valid {
				t.Fatal("unsafe claim allowed to join another request")
			}
		})
	}
	claim := map[string]any{"schema_version": AgentIdentitySchemaVersion, "cliente_id": 2, "agente_id": 84, "issued_at": now, "nonce": "nonce"}
	header := batchClaim(claim)
	for _, auth := range []string{"Token different-secret", "Bearer synthetic-secret", ""} {
		if _, _, valid := VerifiedIdentityBatchScope(header, auth, now); valid {
			t.Fatal("authentication boundary lost")
		}
	}
	for _, raw := range []string{"legacy-opaque", strings.Repeat("a", 4097)} {
		if _, _, valid := VerifiedIdentityBatchScope(raw, "Token synthetic-secret", now); valid {
			t.Fatal("invalid header accepted")
		}
	}
}

func TestIdentityBatchScopeMatchesActualPHPValidator(t *testing.T) {
	if os.Getenv("AICEBERG_WEB_ROOT") == "" {
		t.Skip("requires explicit Web checkout for the actual PHP oracle")
	}
	cfg := Config{Agent: AgentCfg{Token: "synthetic-secret"}, AgentClientID: 7, AgentID: 42, AgentInstallationID: "instalação-1"}
	header := cfg.AgentIdentityHeader("servidor-á")
	raw, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("php", filepath.Join("..", "..", "..", "scripts", "identity_batch_php_oracle.php"))
	command.Stdin = strings.NewReader(string(raw))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("PHP oracle failed: %v: %s", err, output)
	}
	var result struct {
		Trusted  bool `json:"trusted"`
		Rejected bool `json:"changed_subject_rejected"`
	}
	if json.Unmarshal(output, &result) != nil || !result.Trusted || !result.Rejected {
		t.Fatal("actual PHP signature/subject contract differs")
	}
}
