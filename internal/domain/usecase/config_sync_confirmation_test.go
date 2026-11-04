package usecase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
)

func TestConfigSyncRetriesFullConfirmationAfterReportFailure(t *testing.T) {
	var reports, fullRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/agent/config":
			if r.URL.Query().Get("version") == "41" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			fullRequests.Add(1)
			_, _ = w.Write([]byte(`{"version":"41","collect":{}}`))
		case "/v1/agent/config-report":
			if reports.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	uc := NewConfigSync(config.Config{APIBaseURL: server.URL}, &fakeLogger{}, store, nil)
	for attempt := 0; attempt < 3; attempt++ {
		if err := uc.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if reports.Load() != 2 || fullRequests.Load() != 2 {
		t.Fatalf("full requests=%d reports=%d", fullRequests.Load(), reports.Load())
	}
	if store.Get().Version != "41" {
		t.Fatal("confirmation retry lost applied preferences")
	}
}
