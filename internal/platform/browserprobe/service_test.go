package browserprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/browserjourney"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

func TestBrowserServiceRunsTheApprovedRecipeFromTheHub(t *testing.T) {
	chromium := chromiumFixture(t)
	var hits atomic.Int64
	var stall atomic.Bool
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			hits.Add(1)
			if stall.Load() {
				<-r.Context().Done()
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><button data-testid="refresh"
onclick="document.querySelector('output').innerText='Reading updated'">Refresh</button>
<output data-testid="state">Pending</output>`))
		}
	}))
	defer app.Close()
	dir := serviceFixtureDir(t)
	socket := filepath.Join(dir, "runner.sock")
	recipe := testRecipe(app.URL)
	digest := writeServiceFixture(t, dir, recipe)
	t.Setenv("AICEBERG_BROWSER_RUNNER_SOCKET", socket)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- Serve(ctx, socket, dir, chromium) }()
	t.Cleanup(func() {
		cancel()
		if err := <-stopped; err != nil {
			t.Errorf("runner shutdown: %v", err)
		}
		if _, err := os.Lstat(socket); !os.IsNotExist(err) {
			t.Errorf("runner socket was not removed: %v", err)
		}
	})
	waitForRunnerSocket(t, socket)

	job := entities.AgentlessJob{
		Tipo: "browser", ClienteID: 3, AtivoID: 42, CheckID: 90, BaseIntervalSec: 60,
		Endpoint: &entities.AgentlessEndpoint{Tipo: "url", ID: 12, Endereco: app.URL},
		Config: entities.AgentlessConfig{"recipe_sha256": digest, "metrics": []any{
			map[string]any{"id": "steps", "preset": "steps_completed"},
			map[string]any{"id": "duration", "preset": "duration_ms"},
		}},
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, BudgetMS: 12000, FreshnessSec: 180},
	}
	observation := agentless.RunJob(context.Background(), job)
	if observation.Status != "ok" || observation.ProbeReport == nil || observation.ProbeReport.Protocol != "browser" ||
		observation.MonitoringSamples == nil || len(observation.MonitoringSamples.Samples) != 2 {
		t.Fatalf("HUB did not receive the real browser result: %s", observation.Code)
	}
	steps, duration := observation.MonitoringSamples.Samples[0], observation.MonitoringSamples.Samples[1]
	if steps.Value == nil || *steps.Value != 3 || duration.Value == nil || *duration.Value <= 0 || duration.Unit != "milliseconds" {
		t.Fatalf("unexpected browser series: %+v %+v", steps, duration)
	}

	job.ClienteID++
	if got := agentless.RunJob(context.Background(), job); got.Code != "browser_not_allowed" {
		t.Fatalf("foreign scope reached the browser: %s", got.Code)
	}
	job.ClienteID--
	job.Config["recipe_sha256"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if got := agentless.RunJob(context.Background(), job); got.Code != "browser_not_allowed" {
		t.Fatalf("unreviewed recipe reached the browser: %s", got.Code)
	}
	if hits.Load() != 1 {
		t.Fatalf("unexpected re-execution of fixture: %d hits", hits.Load())
	}

	stall.Store(true)
	recipe.BudgetMS = 1000
	job.Config["recipe_sha256"] = writeServiceFixture(t, dir, recipe)
	timeout := agentless.RunJob(context.Background(), job)
	if timeout.Code != "browser_timeout" || timeout.ProbeReport == nil ||
		timeout.ProbeReport.Outcome != "timeout" || timeout.ProbeReport.Reason != "budget" {
		t.Fatalf("runner budget timeout lost its canonical classification: code=%s", timeout.Code)
	}
}

func TestBrowserServiceRefusesUnapprovedOriginAndBackpressureKeepsScope(t *testing.T) {
	const origin = "http://127.0.0.1:8080"
	dir := serviceFixtureDir(t)
	recipe := testRecipe(origin)
	digest := writeServiceFixture(t, dir, recipe)
	runner := service{recipes: dir, slot: make(chan struct{}, 1), run: func(context.Context, Journey) Receipt {
		t.Fatal("unapproved or busy probe reached browser execution")
		return Receipt{}
	}}
	request := browserjourney.Request{
		Contract: browserjourney.RunContract, Scope: recipe.scope(), Origin: "http://127.0.0.1:8081", Digest: digest, BudgetMS: 10000,
	}
	if got := runner.handle(context.Background(), request); got.Outcome != "not_allowed" || !got.Matches(request) {
		t.Fatalf("unexpected origin decision: %+v", got)
	}
	request.Origin = origin
	runner.slot <- struct{}{}
	if got := runner.handle(context.Background(), request); got.Outcome != "busy" || !got.Matches(request) {
		t.Fatalf("unexpected backpressure receipt: %+v", got)
	}
}

func serviceFixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ab-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func writeServiceFixture(t *testing.T, dir string, journey Journey) string {
	t.Helper()
	raw, err := json.Marshal(journey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "3-42-90-12.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func waitForRunnerSocket(t *testing.T, socket string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		info, err := os.Lstat(socket)
		if err == nil && info.Mode()&os.ModeSocket != 0 && info.Mode().Perm() == 0o660 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("runner socket was not ready")
}
