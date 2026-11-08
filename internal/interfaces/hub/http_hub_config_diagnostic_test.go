package hub

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestHubPreservesConfigDiagnosticBodyAndRelayIdentity(t *testing.T) {
	body := `{"status":"applied","config_version":"2","diagnostic":{"schema_version":1,"phase":"recovered","report_id":"receipt","count":1000}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != body || r.Header.Get("Authorization") != "Token relay-fixture" || r.Header.Get("X-Agent-Identity") != "relay-identity" {
			t.Error("diagnostic or authenticated identity changed")
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()
	handler := NewHandler(config.Config{APIBaseURL: upstream.URL}, nil, testHubLogger{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/agent/config-report", strings.NewReader(body))
	request.Header.Set("Authorization", "Token relay-fixture")
	request.Header.Set("X-Agent-Identity", "relay-identity")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d", response.Code)
	}
}
