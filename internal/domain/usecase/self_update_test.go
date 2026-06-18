package usecase

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/version"
)

func testUpdateVersion() string {
	return version.Version + "-test"
}

func TestSelfUpdate_DownloadAndVerifyChecksum(t *testing.T) {
	pkg := []byte("agent package bytes")
	sum := sha256.Sum256(pkg)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pkg)
	}))
	defer srv.Close()

	cfg := config.Config{
		AutoUpdateEnabled: true,
		AutoUpdateDir:     t.TempDir(),
		AutoUpdateTimeout: 2 * time.Second,
		AutoUpdateMaxMB:   5,
	}

	uc := NewSelfUpdate(cfg, &fakeLogger{})
	targetVersion := testUpdateVersion()
	payload := &UpdatePayload{
		Version: targetVersion,
		URL:     srv.URL + "/aiceberg-agent-linux-amd64.tar.gz",
		SHA256:  hex.EncodeToString(sum[:]),
	}

	if err := uc.Execute(context.Background(), payload); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	path := filepath.Join(cfg.AutoUpdateDir, targetVersion, "aiceberg-agent-linux-amd64.tar.gz")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected downloaded file at %s: %v", path, err)
	}
}

func TestSelfUpdate_ChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("corrupted"))
	}))
	defer srv.Close()

	cfg := config.Config{
		AutoUpdateEnabled: true,
		AutoUpdateDir:     t.TempDir(),
		AutoUpdateTimeout: 2 * time.Second,
		AutoUpdateMaxMB:   5,
	}

	uc := NewSelfUpdate(cfg, &fakeLogger{})
	payload := &UpdatePayload{
		Version: testUpdateVersion(),
		URL:     srv.URL + "/pkg.bin",
		SHA256:  "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}

	if err := uc.Execute(context.Background(), payload); err == nil {
		t.Fatalf("expected checksum error")
	}
}

func TestSelfUpdate_VerifiesTrustedArtifactSignature(t *testing.T) {
	pkg := []byte("trusted package")
	sum := sha256.Sum256(pkg)
	versionTarget := testUpdateVersion()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	message := artifactTrustMessage(versionTarget, hex.EncodeToString(sum[:]), "fleet-key-v1")
	signature := ed25519.Sign(privateKey, []byte(message))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pkg)
	}))
	defer srv.Close()

	cfg := config.Config{
		AutoUpdateEnabled:        true,
		AutoUpdateDir:            t.TempDir(),
		AutoUpdateTimeout:        2 * time.Second,
		AutoUpdateMaxMB:          5,
		AutoUpdateTrustRequired:  true,
		AutoUpdateTrustPublicKey: hex.EncodeToString(publicKey),
	}
	uc := NewSelfUpdate(cfg, &fakeLogger{})
	payload := &UpdatePayload{
		Version:            versionTarget,
		URL:                srv.URL + "/pkg.bin",
		SHA256:             hex.EncodeToString(sum[:]),
		SignatureAlgorithm: "ed25519-sha256",
		Signature:          hex.EncodeToString(signature),
		SigningKeyID:       "fleet-key-v1",
	}

	if err := uc.Execute(context.Background(), payload); err != nil {
		t.Fatalf("expected trusted update, got %v", err)
	}
}

func TestSelfUpdate_RejectsInvalidTrustedArtifactSignature(t *testing.T) {
	pkg := []byte("trusted package")
	sum := sha256.Sum256(pkg)
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pkg)
	}))
	defer srv.Close()

	cfg := config.Config{
		AutoUpdateEnabled:        true,
		AutoUpdateDir:            t.TempDir(),
		AutoUpdateTimeout:        2 * time.Second,
		AutoUpdateMaxMB:          5,
		AutoUpdateTrustRequired:  true,
		AutoUpdateTrustPublicKey: hex.EncodeToString(publicKey),
	}
	uc := NewSelfUpdate(cfg, &fakeLogger{})
	payload := &UpdatePayload{
		Version:      testUpdateVersion(),
		URL:          srv.URL + "/pkg.bin",
		SHA256:       hex.EncodeToString(sum[:]),
		Signature:    hex.EncodeToString(make([]byte, ed25519.SignatureSize)),
		SigningKeyID: "fleet-key-v1",
	}

	if err := uc.Execute(context.Background(), payload); err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected signature verification failure, got %v", err)
	}
}

func TestSelfUpdate_RunCommand(t *testing.T) {
	pkg := []byte("payload")
	sum := sha256.Sum256(pkg)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pkg)
	}))
	defer srv.Close()

	command := `test -f "$AICEBERG_UPDATE_FILE"`
	if runtime.GOOS == "windows" {
		command = `if (!(Test-Path $env:AICEBERG_UPDATE_FILE)) { exit 1 }`
	}

	cfg := config.Config{
		AutoUpdateEnabled: true,
		AutoUpdateDir:     t.TempDir(),
		AutoUpdateTimeout: 2 * time.Second,
		AutoUpdateMaxMB:   5,
		AutoUpdateCommand: command,
	}

	uc := NewSelfUpdate(cfg, &fakeLogger{})
	payload := &UpdatePayload{
		Version: testUpdateVersion(),
		URL:     srv.URL + "/pkg.bin",
		SHA256:  hex.EncodeToString(sum[:]),
	}

	if err := uc.Execute(context.Background(), payload); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestSelfUpdateFailureReasonCodePrefersSpecificFailureClass(t *testing.T) {
	if got := updateFailureReasonCode("command_failed", "sudoers"); got != "sudoers" {
		t.Fatalf("expected sudoers, got %s", got)
	}
	if got := updateFailureReasonCode("sha_mismatch", "pacote"); got != "sha_mismatch" {
		t.Fatalf("expected explicit reason code to be preserved, got %s", got)
	}
}

func TestSelfUpdate_RelayDownloadsViaHubProxy(t *testing.T) {
	pkg := []byte("relay package bytes")
	sum := sha256.Sum256(pkg)

	var directHits int32
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&directHits, 1)
		http.Error(w, "should not call direct first", http.StatusBadGateway)
	}))
	defer direct.Close()

	relayToken := "relay-token"
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/update/download" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Token "+relayToken {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		if got := r.URL.Query().Get("url"); got != direct.URL+"/pkg.bin" {
			http.Error(w, "invalid source url", http.StatusBadRequest)
			return
		}
		if got := r.URL.Query().Get("use_agent_auth"); got != "1" {
			http.Error(w, "missing use_agent_auth", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pkg)
	}))
	defer hub.Close()

	cfg := config.Config{
		Agent:                  config.AgentCfg{Token: relayToken},
		AgentMode:              "relay",
		HubURL:                 hub.URL,
		AutoUpdateEnabled:      true,
		AutoUpdateDir:          t.TempDir(),
		AutoUpdateTimeout:      2 * time.Second,
		AutoUpdateMaxMB:        5,
		AutoUpdateUseAgentAuth: true,
	}

	uc := NewSelfUpdate(cfg, &fakeLogger{})
	targetVersion := testUpdateVersion()
	payload := &UpdatePayload{
		Version: targetVersion,
		URL:     direct.URL + "/pkg.bin",
		SHA256:  hex.EncodeToString(sum[:]),
	}

	if err := uc.Execute(context.Background(), payload); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if got := atomic.LoadInt32(&directHits); got != 0 {
		t.Fatalf("expected direct source to not be hit, got %d", got)
	}

	path := filepath.Join(cfg.AutoUpdateDir, targetVersion, "pkg.bin")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected downloaded file at %s: %v", path, err)
	}

	proxyURL := buildHubUpdateProxyURL(hub.URL, direct.URL+"/pkg.bin", true)
	parsed, err := neturl.Parse(proxyURL)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	if parsed.Path != "/v1/agent/update/download" {
		t.Fatalf("unexpected proxy path: %s", parsed.Path)
	}
}

func TestSelfUpdate_RelayDownloadDoesNotFallbackToDirect(t *testing.T) {
	pkg := []byte("relay package bytes")
	sum := sha256.Sum256(pkg)

	var directHits int32
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&directHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pkg)
	}))
	defer direct.Close()

	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "hub unavailable", http.StatusBadGateway)
	}))
	defer hub.Close()

	cfg := config.Config{
		Agent:             config.AgentCfg{Token: "relay-token"},
		AgentMode:         "relay",
		HubURL:            hub.URL,
		AutoUpdateEnabled: true,
		AutoUpdateDir:     t.TempDir(),
		AutoUpdateTimeout: 2 * time.Second,
		AutoUpdateMaxMB:   5,
	}

	uc := NewSelfUpdate(cfg, &fakeLogger{})
	payload := &UpdatePayload{
		Version: testUpdateVersion(),
		URL:     direct.URL + "/pkg.bin",
		SHA256:  hex.EncodeToString(sum[:]),
	}

	if err := uc.Execute(context.Background(), payload); err == nil {
		t.Fatalf("expected hub proxy failure")
	}
	if got := atomic.LoadInt32(&directHits); got != 0 {
		t.Fatalf("relay download must not call direct source, calls=%d", got)
	}
}

func TestSelfUpdate_ApplyRemoteConfigResetsOverridesWhenPayloadIsEmpty(t *testing.T) {
	cfg := config.Config{
		AutoUpdateEnabled: true,
		AutoUpdateDir:     "/tmp/default-updates",
		AutoUpdateCommand: "/bin/echo default",
	}
	uc := NewSelfUpdate(cfg, &fakeLogger{})

	enabled := false
	command := "/bin/echo override"
	uc.ApplyRemoteConfig(&AutoUpdatePayload{
		Enabled: &enabled,
		Command: &command,
	})

	opts := uc.effectiveOptions()
	if opts.enabled {
		t.Fatalf("expected runtime enabled=false after override")
	}
	if opts.command != command {
		t.Fatalf("expected runtime command override, got %q", opts.command)
	}

	uc.ApplyRemoteConfig(&AutoUpdatePayload{})

	opts = uc.effectiveOptions()
	if !opts.enabled {
		t.Fatalf("expected enabled fallback to config default=true")
	}
	if opts.command != cfg.AutoUpdateCommand {
		t.Fatalf("expected command fallback to config default, got %q", opts.command)
	}
	if opts.dir != cfg.AutoUpdateDir {
		t.Fatalf("expected dir fallback to config default, got %q", opts.dir)
	}
}

func TestSelfUpdate_ReportIncludesDownloadMetadata(t *testing.T) {
	pkg := []byte("pkg-report-metadata")
	sum := sha256.Sum256(pkg)
	var (
		mu        sync.Mutex
		received  []map[string]any
		reqCount  int
		targetURL string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pkg.bin":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(pkg)
			return
		case "/v1/agent/update-report":
			reqCount++
			raw, _ := io.ReadAll(r.Body)
			payload := map[string]any{}
			_ = json.Unmarshal(raw, &payload)
			mu.Lock()
			received = append(received, payload)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	targetURL = srv.URL + "/pkg.bin"

	cfg := config.Config{
		Agent:             config.AgentCfg{Token: "agent-token"},
		APIBaseURL:        srv.URL,
		AutoUpdateEnabled: true,
		AutoUpdateDir:     t.TempDir(),
		AutoUpdateTimeout: 2 * time.Second,
		AutoUpdateMaxMB:   5,
	}
	uc := NewSelfUpdate(cfg, &fakeLogger{})
	payload := &UpdatePayload{
		Version: testUpdateVersion(),
		URL:     targetURL,
		SHA256:  hex.EncodeToString(sum[:]),
	}

	if err := uc.Execute(context.Background(), payload); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if reqCount == 0 {
		t.Fatalf("expected update-report request")
	}
	mu.Lock()
	defer mu.Unlock()
	report := received[len(received)-1]
	if status, _ := report["status"].(string); status != "download_ok" {
		t.Fatalf("expected status download_ok, got %v", report["status"])
	}
	update, ok := report["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected update metadata map")
	}
	if stage, _ := update["stage"].(string); stage != "download" {
		t.Fatalf("expected download stage in final report, got %v", update["stage"])
	}
	if steps, ok := update["handshake_steps"].([]any); !ok || len(steps) == 0 {
		t.Fatalf("expected handshake_steps in update metadata")
	}
	filePath, _ := update["download_file"].(string)
	if strings.TrimSpace(filePath) == "" {
		t.Fatalf("expected download_file in update metadata")
	}
	if !strings.Contains(filePath, payload.Version) {
		t.Fatalf("expected download path to include version dir, got %s", filePath)
	}
	if sha, _ := update["download_sha256"].(string); sha == "" {
		t.Fatalf("expected download_sha256")
	}
}

func TestSelfUpdate_SnapshotIncludesPendingStateMetadata(t *testing.T) {
	cfg := config.Config{
		AutoUpdateEnabled: true,
		AutoUpdateDir:     t.TempDir(),
		AutoUpdateTimeout: 2 * time.Second,
		AutoUpdateMaxMB:   5,
		AutoUpdateCommand: "echo ok",
	}
	uc := NewSelfUpdate(cfg, &fakeLogger{})
	const expectedSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	artifact := downloadedArtifact{
		FilePath:  filepath.Join(cfg.AutoUpdateDir, "v-test", "pkg.bin"),
		DirPath:   filepath.Join(cfg.AutoUpdateDir, "v-test"),
		SHA256:    expectedSHA,
		SizeBytes: 321,
		Source:    "direct",
	}
	opts := effectiveAutoUpdateOptions{
		enabled: true,
		dir:     cfg.AutoUpdateDir,
		maxMB:   cfg.AutoUpdateMaxMB,
		timeout: cfg.AutoUpdateTimeout,
		retry:   30 * time.Minute,
		useAuth: false,
		command: cfg.AutoUpdateCommand,
		workDir: cfg.AutoUpdateDir,
	}
	if err := uc.savePendingState(opts.dir, "v-test", version.Version, artifact, opts); err != nil {
		t.Fatalf("savePendingState failed: %v", err)
	}
	snap := uc.Snapshot()
	pending, ok := snap["pending_state"].(map[string]any)
	if !ok {
		t.Fatalf("expected pending_state snapshot")
	}
	if got, _ := pending["target_version"].(string); got != "v-test" {
		t.Fatalf("unexpected target_version: %v", got)
	}
	if got, _ := pending["download_file"].(string); got == "" {
		t.Fatalf("expected download_file in pending_state")
	}
	if got, _ := pending["download_sha256"].(string); got != expectedSHA {
		t.Fatalf("unexpected download_sha256: %v", got)
	}
}

func TestSelfUpdate_ReportPendingResultConfirmsVersionAfterReconnect(t *testing.T) {
	var received []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/update-report" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		payload := map[string]any{}
		_ = json.Unmarshal(raw, &payload)
		received = append(received, payload)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	cfg := config.Config{
		Agent:             config.AgentCfg{Token: "agent-token"},
		APIBaseURL:        srv.URL,
		AutoUpdateEnabled: true,
		AutoUpdateDir:     t.TempDir(),
		AutoUpdateCommand: "echo ok",
	}
	uc := NewSelfUpdate(cfg, &fakeLogger{})
	artifact := downloadedArtifact{
		FilePath:  filepath.Join(cfg.AutoUpdateDir, version.Version, "pkg.bin"),
		DirPath:   filepath.Join(cfg.AutoUpdateDir, version.Version),
		SHA256:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SizeBytes: 123,
		Source:    "pending_state",
	}
	opts := uc.effectiveOptions()
	if err := uc.savePendingState(opts.dir, version.Version, "old-version", artifact, opts); err != nil {
		t.Fatalf("savePendingState failed: %v", err)
	}

	if err := uc.ReportPendingResult(context.Background()); err != nil {
		t.Fatalf("ReportPendingResult failed: %v", err)
	}
	if len(received) != 2 {
		t.Fatalf("expected reconnect and version confirmation reports, got %d", len(received))
	}
	if status, _ := received[0]["status"].(string); status != "reconnected" {
		t.Fatalf("expected reconnected status, got %v", received[0]["status"])
	}
	if status, _ := received[1]["status"].(string); status != "version_confirmed" {
		t.Fatalf("expected version_confirmed status, got %v", received[1]["status"])
	}
	update, ok := received[1]["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected update metadata")
	}
	if stage, _ := update["stage"].(string); stage != "version_confirmed" {
		t.Fatalf("expected version_confirmed stage, got %v", update["stage"])
	}
}
