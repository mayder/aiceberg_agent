package config

import "testing"

func TestLoadAgentlessDeliveryPriorityDefaultsOffAndExplicitOptIn(t *testing.T) {
	t.Setenv("AGENT_TOKEN", "fixture-token")
	t.Setenv("AGENT_ENV_FILE", "")
	t.Setenv("AGENTLESS_DELIVERY_PRIORITY", "")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentlessDeliveryPriority {
		t.Fatal("priority must require opt-in")
	}
	t.Setenv("AGENTLESS_DELIVERY_PRIORITY", "true")
	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AgentlessDeliveryPriority {
		t.Fatal("priority opt-in ignored")
	}
	t.Setenv("AGENTLESS_DELIVERY_PRIORITY", "false")
	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentlessDeliveryPriority {
		t.Fatal("rollback ignored")
	}
}
