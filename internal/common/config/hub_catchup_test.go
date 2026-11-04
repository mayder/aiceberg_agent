package config

import "testing"

func TestLoadHubOutboxCatchupDefaultAndRollback(t *testing.T) {
	t.Setenv("AGENT_TOKEN", "fixture-token")
	for _, tc := range []struct {
		value string
		want  bool
	}{{"", true}, {"true", true}, {"false", false}} {
		t.Setenv("HUB_OUTBOX_CATCHUP_ENABLED", tc.value)
		cfg, err := Load("")
		if err != nil || cfg.HubOutboxCatchupEnabled != tc.want {
			t.Fatalf("value=%q enabled=%v want=%v err=%v", tc.value, cfg.HubOutboxCatchupEnabled, tc.want, err)
		}
	}
}

func TestLoadHubOutboxAsyncDefaultAndRollback(t *testing.T) {
	t.Setenv("AGENT_TOKEN", "fixture-token")
	for _, tc := range []struct {
		value string
		want  bool
	}{{"", true}, {"true", true}, {"false", false}} {
		t.Setenv("HUB_OUTBOX_ASYNC_ENABLED", tc.value)
		cfg, err := Load("")
		if err != nil || cfg.HubOutboxAsyncEnabled != tc.want {
			t.Fatalf("value=%q enabled=%v want=%v err=%v", tc.value, cfg.HubOutboxAsyncEnabled, tc.want, err)
		}
	}
}
