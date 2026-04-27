package channel

import "testing"

func TestTopologyPreservesRelayThroughHubOnly(t *testing.T) {
	if !ModeConnectsToAiceberg(ModeDirect) {
		t.Fatalf("direct mode must connect outbound to AIceberg")
	}
	if !ModeConnectsToAiceberg(ModeHub) {
		t.Fatalf("hub mode must connect outbound to AIceberg")
	}
	if ModeConnectsToAiceberg(ModeRelay) {
		t.Fatalf("relay mode must not connect directly to AIceberg")
	}

	direct := MustTopology(ModeDirect)
	if direct.Path[0] != "agent" || direct.Path[1] != "aiceberg" || !direct.ConnectsDirectlyAiceberg {
		t.Fatalf("unexpected direct topology: %#v", direct)
	}

	hub := MustTopology(ModeHub)
	if hub.Path[0] != "hub" || hub.Path[1] != "aiceberg" || !hub.AcceptsRelays || !hub.ConnectsDirectlyAiceberg {
		t.Fatalf("unexpected hub topology: %#v", hub)
	}

	topology := MustTopology(ModeRelay)
	if got := len(topology.Path); got != 3 {
		t.Fatalf("expected relay path with 3 hops, got %d", got)
	}
	if topology.Path[0] != "relay" || topology.Path[1] != "hub" || topology.Path[2] != "aiceberg" {
		t.Fatalf("unexpected relay path: %#v", topology.Path)
	}
	if topology.ConnectsDirectlyAiceberg {
		t.Fatalf("relay topology cannot connect directly to AIceberg")
	}
}

func TestLegacyHTTPContractsRemainExplicit(t *testing.T) {
	for _, name := range []string{"ping", "bootstrap", "config", "selfheal", "update-report", "ingest"} {
		if len(LegacyHTTPContracts[name]) == 0 {
			t.Fatalf("missing legacy HTTP contract %s", name)
		}
	}
}

func TestCommandAllowlistBlocksShellAliases(t *testing.T) {
	if !IsAllowedCommandCode(CommandInspectRuntimeConfig) {
		t.Fatalf("inspect_runtime_config must be allowed")
	}
	if !IsAllowedCommandCode(CommandCollectNow) {
		t.Fatalf("collect_now must be allowed")
	}
	if IsAllowedCommandCode("shell") {
		t.Fatalf("shell must not be allowed")
	}
	if !IsShellLikeCommandCode("powershell") {
		t.Fatalf("powershell must be classified as shell-like")
	}
}

func TestValidateCommandEnvelopeRequiresCommandID(t *testing.T) {
	errors := ValidateEnvelope(Envelope{
		ContractID:    ContractID,
		SchemaVersion: SchemaVersion,
		MessageID:     "msg-1",
		Type:          TypeAck,
		TimestampUTC:  "2026-04-26T12:00:00Z",
		AgentID:       "agent-1",
		Mode:          ModeDirect,
	})

	if !contains(errors, "command_id is required for command messages") {
		t.Fatalf("expected command_id error, got %#v", errors)
	}
}

func TestValidateResultEnvelope(t *testing.T) {
	errors := ValidateEnvelope(Envelope{
		ContractID:    ContractID,
		SchemaVersion: SchemaVersion,
		MessageID:     "msg-2",
		Type:          TypeResult,
		TimestampUTC:  "2026-04-26T12:00:01Z",
		AgentID:       "agent-1",
		Mode:          ModeHub,
		CommandID:     "cmd-1",
		CorrelationID: "corr-1",
		Result:        map[string]any{"status": StatusSuccess},
	})

	if len(errors) != 0 {
		t.Fatalf("expected valid envelope, got %#v", errors)
	}
}

func TestValidateOperationalEnvelopeTypes(t *testing.T) {
	for _, env := range []Envelope{
		{
			ContractID:    ContractID,
			SchemaVersion: SchemaVersion,
			MessageID:     "msg-ack",
			Type:          TypeAck,
			TimestampUTC:  "2026-04-26T12:00:01Z",
			AgentID:       "agent-1",
			Mode:          ModeDirect,
			CommandID:     "cmd-ack",
			Payload:       map[string]any{"status": StatusAccepted},
		},
		{
			ContractID:    ContractID,
			SchemaVersion: SchemaVersion,
			MessageID:     "msg-progress",
			Type:          TypeProgress,
			TimestampUTC:  "2026-04-26T12:00:01Z",
			AgentID:       "agent-1",
			Mode:          ModeDirect,
			CommandID:     "cmd-progress",
			Progress:      map[string]any{"stage": "download", "percent": 50},
		},
		{
			ContractID:    ContractID,
			SchemaVersion: SchemaVersion,
			MessageID:     "msg-result",
			Type:          TypeResult,
			TimestampUTC:  "2026-04-26T12:00:01Z",
			AgentID:       "agent-1",
			Mode:          ModeDirect,
			CommandID:     "cmd-result",
			Result:        map[string]any{"status": StatusSuccess},
		},
		{
			ContractID:    ContractID,
			SchemaVersion: SchemaVersion,
			MessageID:     "msg-error",
			Type:          TypeError,
			TimestampUTC:  "2026-04-26T12:00:01Z",
			AgentID:       "agent-1",
			Mode:          ModeDirect,
			CommandID:     "cmd-error",
			Error:         map[string]any{"status": StatusFailed},
		},
		{
			ContractID:    ContractID,
			SchemaVersion: SchemaVersion,
			MessageID:     "msg-timeout",
			Type:          TypeTimeout,
			TimestampUTC:  "2026-04-26T12:00:01Z",
			AgentID:       "agent-1",
			Mode:          ModeDirect,
			CommandID:     "cmd-timeout",
			Payload:       map[string]any{"status": StatusTimeout},
		},
		{
			ContractID:    ContractID,
			SchemaVersion: SchemaVersion,
			MessageID:     "msg-retry",
			Type:          TypeRetry,
			TimestampUTC:  "2026-04-26T12:00:01Z",
			AgentID:       "agent-1",
			Mode:          ModeDirect,
			CommandID:     "cmd-retry",
			Payload:       map[string]any{"status": StatusRetrying},
		},
	} {
		if errors := ValidateEnvelope(env); len(errors) != 0 {
			t.Fatalf("expected valid %s envelope, got %#v", env.Type, errors)
		}
	}
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
