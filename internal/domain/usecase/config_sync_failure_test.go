package usecase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/metrics"
	"github.com/you/aiceberg_agent/internal/data/local/configstate"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
	"github.com/you/aiceberg_agent/internal/domain/configfailure"
)

func TestConfigApplyFailureThousandCyclesSummaryRecoveryAndGoodPrefs(t *testing.T) {
	metricsBefore := metrics.ConfigFailureSnapshot()
	root := t.TempDir()
	path := filepath.Join(root, "data", "prefs.json")
	store := prefs.NewStore(path)
	good := store.Get()
	good.Version = "1"
	if err := store.Update(good); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Dir(path), filepath.Join(root, "good-data")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Dir(path), []byte("conflict evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	var reports []map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/agent/config" {
			_, _ = w.Write([]byte(`{"version":"2","collect":{"cpu":false}}`))
			return
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		reports = append(reports, body)
	}))
	defer srv.Close()
	cfg := config.Config{APIBaseURL: srv.URL, AgentID: 17, ConfigFailureStatePath: filepath.Join(root, "state", "failure.json")}
	uc := NewConfigSync(cfg, &fakeLogger{}, store, nil)
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	uc.now = func() time.Time { return now }
	if err := uc.Execute(context.Background()); err == nil {
		t.Fatal("missing application failure")
	}
	if store.Get().Version != "1" {
		t.Fatal("lost good preferences")
	}
	for i := 0; i < 1000; i++ {
		now = now.Add(time.Millisecond)
		if err := uc.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(reports) != 1 {
		t.Fatalf("repeated reports=%d", len(reports))
	}
	metricsAfter := metrics.ConfigFailureSnapshot()
	if metricsAfter["attempts"]-metricsBefore["attempts"] != 1 || metricsAfter["suppressed"]-metricsBefore["suppressed"] != 1000 || metricsAfter["report_bytes"] <= metricsBefore["report_bytes"] {
		t.Fatal("operational counters do not reflect actual flow")
	}
	first := uc.failureState.Entries[0].Diagnostic
	if first.Code != "path_conflict" || first.Count != 1001 || first.Attempt != 1 {
		t.Fatalf("bad diagnostic %+v", first)
	}
	raw, _ := json.Marshal(reports[0])
	if containsPrivatePath(raw, root) {
		t.Fatal("private path in report")
	}
	now = now.Add(31 * time.Minute)
	if err := uc.Execute(context.Background()); err == nil {
		t.Fatal("failure disappeared")
	}
	if len(reports) != 2 {
		t.Fatalf("summary count=%d", len(reports))
	}
	if err := os.Rename(filepath.Dir(path), filepath.Join(root, "conflict.evidence")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "good-data"), filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.Get().Version != "2" || store.Get().CPU {
		t.Fatal("new configuration not applied")
	}
	if len(reports) != 3 {
		t.Fatalf("recovery reports=%d", len(reports))
	}
	var recovered configfailure.Report
	_ = json.Unmarshal(reports[2]["diagnostic"], &recovered)
	if recovered.Phase != "recovered" || recovered.NextRetry != 0 {
		t.Fatalf("bad recovery %+v", recovered)
	}
}

func containsPrivatePath(raw []byte, path string) bool {
	var decoded any
	_ = json.Unmarshal(raw, &decoded)
	text, _ := json.Marshal(decoded)
	// JSON encoding may escape separators; compare the encoded path as well.
	encoded, _ := json.Marshal(path)
	return len(encoded) > 2 && bytesContains(text, encoded[1:len(encoded)-1])
}

func bytesContains(raw, part []byte) bool { return strings.Contains(string(raw), string(part)) }

func TestConfigDiagnosticReportRetryAndRestartKeepReceiptAndDoNotReapply(t *testing.T) {
	root := t.TempDir()
	conflict := filepath.Join(root, "conflict")
	_ = os.WriteFile(conflict, []byte("preserve"), 0600)
	var ids []string
	accept := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/agent/config" {
			_, _ = w.Write([]byte(`{"version":"2","collect":{}}`))
			return
		}
		var body struct {
			Diagnostic configfailure.Report `json:"diagnostic"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		ids = append(ids, body.Diagnostic.ReportID)
		if !accept {
			w.WriteHeader(503)
		}
	}))
	defer srv.Close()
	cfg := config.Config{APIBaseURL: srv.URL, AgentID: 18, ConfigFailureStatePath: filepath.Join(root, "state.json")}
	store := prefs.NewStore(filepath.Join(conflict, "prefs.json"))
	uc := NewConfigSync(cfg, &fakeLogger{}, store, nil)
	if err := uc.Execute(context.Background()); err == nil {
		t.Fatal("missing failure")
	}
	if len(ids) != 1 || ids[0] == "" {
		t.Fatal("missing first receipt")
	}
	accept = true
	restarted := NewConfigSync(cfg, &fakeLogger{}, store, nil)
	if err := restarted.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != ids[1] {
		t.Fatalf("retry identity changed %v", ids)
	}
	if restarted.failureState.Entries[0].Diagnostic.Attempt != 1 {
		t.Fatal("report retry reapplied config")
	}
}

func TestNewConfigBypassesApplyWaitAndConcurrentIngestIsSerialized(t *testing.T) {
	root := t.TempDir()
	store := prefs.NewStore(filepath.Join(root, "prefs.json"))
	uc := NewConfigSync(config.Config{}, &fakeLogger{}, store, nil)
	uc.applyDigest = "old-digest"
	uc.applyUntil = time.Now().Add(time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := uc.ApplyIncomingConfig(ConfigPayload{Version: "3"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if store.Get().Version != "3" {
		t.Fatal("new configuration blocked")
	}
}

func TestConfigSignatureCorrectionIsImmediateAndNeverExecutesRejectedCommands(t *testing.T) {
	root := t.TempDir()
	store := prefs.NewStore(filepath.Join(root, "prefs.json"))
	good := store.Get()
	good.Version = "1"
	if err := store.Update(good); err != nil {
		t.Fatal(err)
	}
	commands := make(chan ControlCommand, 8)
	uc := NewConfigSync(config.Config{RemoteConfigSignatureRequired: true, RemoteConfigSignatureSecret: "fixture-secret"}, &fakeLogger{}, store, commands)
	collectNow := []string{"cpu"}
	payload := ConfigPayload{Version: "2", CollectNow: &collectNow}
	if _, _, err := uc.ApplyIncomingConfig(payload); err == nil {
		t.Fatal("unsigned command accepted")
	}
	if store.Get().Version != "1" || len(commands) != 0 {
		t.Fatal("rejected configuration had effects")
	}
	payload.Signature = PayloadSignature{Algorithm: "hmac-sha256", SignedAt: "2026-11-08T12:00:00Z"}
	signature, err := SignConfigPayload(payload, "fixture-secret")
	if err != nil {
		t.Fatal(err)
	}
	payload.Signature.Value = signature
	if _, applied, err := uc.ApplyIncomingConfig(payload); err != nil || !applied {
		t.Fatal("valid correction blocked", err)
	}
	if store.Get().Version != "2" || len(commands) != 1 {
		t.Fatal("valid configuration not applied once")
	}
}

func TestConfigSignatureRefreshDoesNotCreateAnotherFilesystemCondition(t *testing.T) {
	root := t.TempDir()
	conflict := filepath.Join(root, "conflict")
	if err := os.WriteFile(conflict, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	uc := NewConfigSync(config.Config{ConfigFailureStatePath: filepath.Join(root, "state.json"), RemoteConfigSignatureRequired: true, RemoteConfigSignatureSecret: "fixture-secret"}, &fakeLogger{}, prefs.NewStore(filepath.Join(conflict, "prefs.json")), nil)
	payload := ConfigPayload{Version: "2"}
	for _, timestamp := range []string{"2026-11-08T12:00:00Z", "2026-11-08T12:00:01Z"} {
		payload.Signature = PayloadSignature{Algorithm: "hmac-sha256", SignedAt: timestamp}
		signature, err := SignConfigPayload(payload, "fixture-secret")
		if err != nil {
			t.Fatal(err)
		}
		payload.Signature.Value = signature
		_, _, _ = uc.ApplyIncomingConfig(payload)
	}
	if len(uc.failureState.Entries) != 1 || uc.failureState.Entries[0].Diagnostic.Attempt != 1 || len(uc.failureState.Pending) != 1 {
		t.Fatal("signature refresh bypassed apply wait")
	}
}

func TestConfigUnwritableDiagnosticStateUsesMemoryWithoutLosingGoodConfiguration(t *testing.T) {
	root := t.TempDir()
	conflict := filepath.Join(root, "conflict")
	if err := os.WriteFile(conflict, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	uc := NewConfigSync(config.Config{ConfigFailureStatePath: filepath.Join(conflict, "state.json")}, &fakeLogger{}, prefs.NewStore(filepath.Join(conflict, "prefs.json")), nil)
	if _, _, err := uc.ApplyIncomingConfig(ConfigPayload{Version: "2"}); err == nil {
		t.Fatal("missing apply failure")
	}
	if uc.durable || len(uc.failureState.Pending) != 1 || uc.failureState.Pending[0].Diagnostic.Durable {
		t.Fatal("memory fallback not explicit")
	}
	if uc.store.Get().Version != "" {
		t.Fatal("failed configuration replaced known prefs")
	}
}

func TestConfigReportRetryKeepsContentWhenDurabilityLaterDegrades(t *testing.T) {
	root := t.TempDir()
	conflict := filepath.Join(root, "conflict")
	if err := os.WriteFile(conflict, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	uc := NewConfigSync(config.Config{APIBaseURL: srv.URL, ConfigFailureStatePath: filepath.Join(root, "state.json")}, &fakeLogger{}, prefs.NewStore(filepath.Join(conflict, "prefs.json")), nil)
	_, _, _ = uc.ApplyIncomingConfig(ConfigPayload{Version: "2"})
	original := uc.failureState.Pending[0].Diagnostic
	uc.flushConfigDiagnostics(context.Background())
	uc.failureStore = configstate.New(filepath.Join(conflict, "cannot-write.json"))
	uc.persistConfigDiagnostics()
	if uc.failureState.Pending[0].Diagnostic != original {
		t.Fatal("retry changed an attempted receipt")
	}
	if uc.durable {
		t.Fatal("durability loss hidden")
	}
}

func TestConfigSyncRejectsOversizeOrTrailingBodyBeforeEffects(t *testing.T) {
	for _, body := range []string{`{"version":"2","collect":{}} trailing`, `{"version":"2","collect":{}}` + strings.Repeat(" ", 1<<20)} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
		uc := NewConfigSync(config.Config{APIBaseURL: srv.URL}, &fakeLogger{}, store, nil)
		if err := uc.Execute(context.Background()); err == nil {
			t.Fatal("invalid response accepted")
		}
		if store.Get().Version != "" {
			t.Fatal("invalid response changed configuration")
		}
		srv.Close()
	}
}

func TestNewConfigurationCanRecoverBeforeOldRetryWithRealFilesystem(t *testing.T) {
	root := t.TempDir()
	conflict := filepath.Join(root, "data")
	if err := os.WriteFile(conflict, []byte("fixture conflict"), 0600); err != nil {
		t.Fatal(err)
	}
	store := prefs.NewStore(filepath.Join(conflict, "prefs.json"))
	uc := NewConfigSync(config.Config{ConfigFailureStatePath: filepath.Join(root, "state.json")}, &fakeLogger{}, store, nil)
	if _, _, err := uc.ApplyIncomingConfig(ConfigPayload{Version: "2"}); err == nil {
		t.Fatal("fixture failure not reproduced")
	}
	if !time.Now().Before(uc.applyUntil) {
		t.Fatal("fixture did not schedule wait")
	}
	if err := os.Rename(conflict, filepath.Join(root, "conflict.evidence")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(conflict, 0700); err != nil {
		t.Fatal(err)
	}
	if _, applied, err := uc.ApplyIncomingConfig(ConfigPayload{Version: "3"}); err != nil || !applied {
		t.Fatal("new configuration blocked by old failure", err)
	}
	if store.Get().Version != "3" || uc.failureState.Active != "" {
		t.Fatal("recovery did not preserve current version")
	}
	last := uc.failureState.Pending[len(uc.failureState.Pending)-1]
	if last.Diagnostic.Phase != "recovered" || last.Version != "3" {
		t.Fatal("recovery confirmed wrong configuration")
	}
}
