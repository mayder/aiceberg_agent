package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
	"github.com/you/aiceberg_agent/internal/domain/configfailure"
)

func TestConfigDiagnosticWireAcceptedByActualPHPConsumer(t *testing.T) {
	root := os.Getenv("AICEBERG_CONFIG_FAILURE_WEB_ROOT")
	if root == "" {
		t.Skip("explicit local Web checkout required; no production access")
	}
	if filepath.Base(root) != "aiceberg_web" {
		t.Fatal("explicit Web checkout root required")
	}
	script := `require $argv[1].'/common/tests/_bootstrap.php';$b=json_decode(stream_get_contents(STDIN),true,32,JSON_THROW_ON_ERROR);echo json_encode(common\services\agent\AgentConfigFailureDiagnostic::normalize($b['diagnostic'],$b['status']),JSON_THROW_ON_ERROR);`
	phases := make(map[string]bool)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		command := exec.Command("php", "-r", script, root)
		command.Stdin = bytes.NewReader(raw)
		result, err := command.CombinedOutput()
		if err != nil {
			t.Errorf("PHP consumer rejected wire: %s", result)
			w.WriteHeader(400)
			return
		}
		var diagnostic configfailure.Report
		if err := json.Unmarshal(result, &diagnostic); err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		phases[diagnostic.Phase] = true
		if r.Header.Get("Authorization") != "Token fixture" {
			t.Error("identity header lost")
		}
	}))
	defer server.Close()
	uc := NewConfigSync(config.Config{APIBaseURL: server.URL, Agent: config.AgentCfg{Token: "fixture"}}, &fakeLogger{}, prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json")), nil)
	state := configfailure.NewState()
	now := int64(1794140000000)
	digest := strings.Repeat("a", 64)
	state.Failure("17", "2", digest, digest, configfailure.Cause{Code: "disk_full", Kind: "persistent"}, now, now+60000, true)
	state.Repeated(now + configfailure.SummaryInterval.Milliseconds())
	state.Failure("17", "2", digest, digest, configfailure.Cause{Code: "permission_denied", Kind: "persistent"}, now+1900000, now+1960000, true)
	state.Recover(now+2000000, true)
	for _, item := range state.Pending {
		status := "apply_failed"
		if item.Diagnostic.Phase == "recovered" {
			status = "applied"
		}
		if err := uc.reportConfigApply(context.Background(), item.Version, item.ConfigHash, status, "controlled", item.Diagnostic); err != nil {
			t.Fatal(err)
		}
	}
	for _, phase := range []string{"first", "summary", "changed", "recovered"} {
		if !phases[phase] {
			t.Fatalf("phase %s not qualified", phase)
		}
	}
}
