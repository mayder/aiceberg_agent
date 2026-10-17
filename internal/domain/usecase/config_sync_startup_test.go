package usecase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
)

func TestStartupConfigSyncRestoresPolicyBeforeConditionalPolling(t *testing.T) {
	var versions []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/agent/config-report" {
			w.WriteHeader(http.StatusOK)
			return
		}
		versions = append(versions, r.URL.Query().Get("version"))
		if len(versions) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "12", "collect": map[string]any{}, "auto_update": map[string]any{"enabled": true}})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	p := store.Get()
	p.Version = "12"
	if err := store.Update(p); err != nil {
		t.Fatal(err)
	}
	commands := make(chan ControlCommand, 4)
	uc := NewStartupConfigSync(config.Config{APIBaseURL: srv.URL, Agent: config.AgentCfg{Token: "fixture"}}, &fakeLogger{}, store, commands)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case c := <-commands:
		if c.Name != "self_update_policy" || c.AutoUpdate == nil || c.AutoUpdate.Enabled == nil || !*c.AutoUpdate.Enabled {
			t.Fatalf("policy was not restored: %+v", c)
		}
	default:
		t.Fatal("remote policy was not dispatched after restart")
	}
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0] != "" || versions[1] != "12" {
		t.Fatalf("requests=%v", versions)
	}
}
