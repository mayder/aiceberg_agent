package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestAgentlessCredentialsNegotiateAndStayOutOfCachedJob(t *testing.T) {
	var lookups atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Token test-token" || req.Header.Get("X-Agent-Identity") == "" {
			t.Error("missing agent authentication")
		}
		if req.Method == http.MethodGet {
			if req.URL.Query().Get("credential_mode") != "reference-v1" ||
				req.URL.Query().Get("diagnostic_credential_mode") != "reference-v1" {
				t.Error("missing reference negotiation")
			}
			_, _ = w.Write([]byte(`{"status":"ok","count":1,"jobs":[{"tipo":"snmp","check_id":72,"ativo_id":52,"interval_sec":60,
				"credential_delivery":"reference-v1","probe_policy_v1":{"contract":"monitoring.probe-policy.v1","budget_ms":2000,
				"freshness_sec":180,"credential_ref":"snmp_profile:41"},"endpoint":{"id":17,"endereco":"sensor.fixture"},
				"snmp":{"binding_id":61,"profile_id":41,"version":"v2c","port":161,"timeout_ms":900,"community":"ignore-inline"}}]}`))
			return
		}
		lookup := lookups.Add(1)
		var got snmpCredentialRequest
		want := snmpCredentialRequest{Operation: "resolve_snmp_profile", CheckID: 72, AssetID: 52, EndpointID: 17,
			BindingID: 61, CredentialRef: "snmp_profile:41", Version: "v2c", Address: "sensor.fixture", Port: 161}
		if lookup == 2 {
			want.CommandID = "diagnostic-command-1"
		}
		if err := json.NewDecoder(req.Body).Decode(&got); err != nil || got != want || req.URL.Query().Get("lock") != "0" {
			t.Error("incorrect credential lookup scope")
		}
		response := map[string]any{"status": "ok", "credential_ref": "snmp_profile:41",
			"credential": map[string]string{"community": "resolved-fixture"}}
		if want.CommandID != "" {
			response["command_id"] = want.CommandID
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()
	client := NewAgentlessHubClient(agentlessIdentityTestConfig(srv.URL))
	client.cl = srv.Client()
	jobs, err := client.FetchJobs(context.Background(), 1, true, 60)
	if err != nil || len(jobs) != 1 || lookups.Load() != 0 {
		t.Fatal("job pull unexpectedly resolved the credential")
	}
	profile, err := client.ResolveSNMPCredentials(context.Background(), jobs[0])
	if err != nil || profile.Community != "resolved-fixture" || profile.TimeoutMs != 900 || lookups.Load() != 1 {
		t.Fatal("credential lookup did not preserve scheduling metadata")
	}
	command := jobs[0]
	command.CommandID = "diagnostic-command-1"
	if _, err := client.ResolveSNMPCredentials(context.Background(), command); err != nil || lookups.Load() != 2 {
		t.Fatal("diagnostic command did not resolve its own credential")
	}
	cache, err := json.Marshal(jobs)
	if err != nil || strings.Contains(string(cache), "ignore-inline") || strings.Contains(string(cache), "resolved-fixture") {
		t.Fatal("credential escaped into the job cache")
	}
}

func TestDecodeSNMPCredentialRejectsWrongScopeLegacyResponseAndExtraFields(t *testing.T) {
	for _, wire := range []string{
		`{"status":"ok","credential_ref":"snmp_profile:42","credential":{"community":"fixture"}}`,
		`{"status":"ok","jobs":[{"snmp":{"community":"fixture"}}]}`,
		`{"status":"ok","credential_ref":"snmp_profile:41","credential":{"community":"fixture","password":"unexpected"}}`,
		`{"status":"ok","credential_ref":"snmp_profile:41","credential":{}}`,
		`{"status":"ok","credential_ref":"snmp_profile:41","command_id":"another-command","credential":{"community":"fixture"}}`,
	} {
		profile, err := decodeSNMPCredential(strings.NewReader(wire), entities.AgentlessSnmpProfile{Version: "v2c"}, "snmp_profile:41", "")
		if profile != nil || err != errSNMPCredentialUnavailable {
			t.Fatal("unexpected wire response was accepted")
		}
	}
}
