package config

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestRelaySNMPAssignmentRequiresOptInAndPinnedCoordinator(t *testing.T) {
	cfg := Config{AgentMode: "relay", AgentClientID: 3, AgentID: 424}
	public := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public().(ed25519.PublicKey)
	t.Setenv("RELAY_SNMP_ASSIGNMENT_ENABLED", "")
	t.Setenv("RELAY_SNMP_ASSIGNMENT_HUB_ID", "84")
	t.Setenv("RELAY_SNMP_ASSIGNMENT_PUBLIC_KEY", hex.EncodeToString(public))
	if _, enabled := cfg.RelaySNMPAssignmentSettings(); enabled {
		t.Fatal("delegation enabled by default")
	}
	t.Setenv("RELAY_SNMP_ASSIGNMENT_ENABLED", "true")
	if settings, enabled := cfg.RelaySNMPAssignmentSettings(); !enabled || settings.HubID != 84 || !settings.PublicKey.Equal(public) {
		t.Fatal("pinned coordinator was not loaded")
	}
	t.Setenv("RELAY_SNMP_ASSIGNMENT_PUBLIC_KEY", "unconfigured")
	if _, enabled := cfg.RelaySNMPAssignmentSettings(); enabled {
		t.Fatal("untrusted coordinator enabled execution")
	}
}
