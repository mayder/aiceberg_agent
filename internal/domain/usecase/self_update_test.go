package usecase

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

func TestSelfUpdate_RequiresSignatureFromRemotePolicyPrefs(t *testing.T) {
	pkg := []byte("unsigned package")
	sum := sha256.Sum256(pkg)
	versionTarget := testUpdateVersion()
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
	uc := NewSelfUpdate(cfg, &fakeLogger{}, func() config.CollectPrefs {
		return config.CollectPrefs{AutoUpdateTrustRequired: true}
	})
	payload := &UpdatePayload{
		Version: versionTarget,
		URL:     srv.URL + "/pkg.bin",
		SHA256:  hex.EncodeToString(sum[:]),
	}

	err := uc.Execute(context.Background(), payload)
	if err == nil || !strings.Contains(err.Error(), "artifact trust public key missing") {
		t.Fatalf("expected trust failure from remote policy, got %v", err)
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

	command := `test -f "$AICEBERG_UPDATE_FILE" && test -n "$AICEBERG_AGENT_BIN" && test "$AICEBERG_UPDATE_BIN_DST" = "$AICEBERG_AGENT_BIN"`
	if runtime.GOOS == "windows" {
		command = `if (!(Test-Path $env:AICEBERG_UPDATE_FILE)) { exit 1 }; if ([string]::IsNullOrWhiteSpace($env:AICEBERG_AGENT_BIN)) { exit 1 }; if ($env:AICEBERG_UPDATE_BIN_DST -ne $env:AICEBERG_AGENT_BIN) { exit 1 }`
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

func TestSelfUpdateRestartGuardHelpers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX restart guard is not used on Windows")
	}
	updateDir := filepath.Join(t.TempDir(), "data", "updates")
	pidFile := inferAgentPIDFile(updateDir)
	if !strings.HasSuffix(pidFile, filepath.Join("data", "agent.pid")) {
		t.Fatalf("expected pid file beside updates dir, got %s", pidFile)
	}
	logFile := inferAgentRestartLogFile(pidFile)
	if !strings.HasSuffix(logFile, filepath.Join("logs", "agent.stderr.log")) {
		t.Fatalf("expected default restart log in logs dir, got %s", logFile)
	}
	if !looksLikeRestartingUpdateCommand("sudo -n /usr/local/sbin/aiceberg-agent-update-launcher.sh") {
		t.Fatalf("expected official launcher to enable restart guard")
	}
	if looksLikeRestartingUpdateCommand(`test -f "$AICEBERG_UPDATE_FILE"`) {
		t.Fatalf("expected non-restarting command to skip restart guard")
	}
	if !strings.Contains(restartGuardScript(), "has_agent_process") {
		t.Fatalf("expected restart guard script body")
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

func TestSelfUpdate_DownloadTimeoutDoesNotFinalizePartialFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("late package bytes"))
	}))
	defer srv.Close()

	cfg := config.Config{
		AutoUpdateEnabled: true,
		AutoUpdateDir:     t.TempDir(),
		AutoUpdateTimeout: 50 * time.Millisecond,
		AutoUpdateMaxMB:   5,
	}
	uc := NewSelfUpdate(cfg, &fakeLogger{})
	targetVersion := testUpdateVersion()
	payload := &UpdatePayload{
		Version: targetVersion,
		URL:     srv.URL + "/pkg.bin",
	}

	err := uc.Execute(context.Background(), payload)
	if err == nil || !strings.Contains(err.Error(), "download body") {
		t.Fatalf("expected download timeout error, got %v", err)
	}
	finalPath := filepath.Join(cfg.AutoUpdateDir, targetVersion, "pkg.bin")
	if _, statErr := os.Stat(finalPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no finalized artifact at %s, got %v", finalPath, statErr)
	}
}

func TestSelfUpdate_DownloadResumesPartialFileWithRange(t *testing.T) {
	pkg := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	sum := sha256.Sum256(pkg)
	half := len(pkg) / 2
	var calls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		if call == 1 {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pkg)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(pkg[:half])
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		expectedRange := fmt.Sprintf("bytes=%d-", half)
		if got := r.Header.Get("Range"); got != expectedRange {
			http.Error(w, "missing resume range", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", half, len(pkg)-1, len(pkg)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(pkg[half:])
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfg := config.Config{
		AutoUpdateEnabled: true,
		AutoUpdateDir:     dir,
		AutoUpdateTimeout: 2 * time.Second,
		AutoUpdateMaxMB:   5,
	}
	uc := NewSelfUpdate(cfg, &fakeLogger{})
	payload := &UpdatePayload{
		Version: testUpdateVersion(),
		URL:     srv.URL + "/pkg.bin",
		SHA256:  hex.EncodeToString(sum[:]),
	}
	opts := uc.effectiveOptions()

	if artifact, err := uc.download(context.Background(), payload, opts); err == nil {
		t.Fatalf("expected first partial download to fail, got artifact %#v", artifact)
	}
	tmpPath := filepath.Join(dir, payload.Version, "pkg.bin.part")
	if info, err := os.Stat(tmpPath); err != nil || info.Size() != int64(half) {
		t.Fatalf("expected partial file with %d bytes, info=%#v err=%v", half, info, err)
	}

	artifact, err := uc.download(context.Background(), payload, opts)
	if err != nil {
		t.Fatalf("expected resumed download to succeed: %v", err)
	}
	if !artifact.Resumed || artifact.ResumeOffset != int64(half) {
		t.Fatalf("expected resumed artifact from offset %d, got %#v", half, artifact)
	}
	raw, err := os.ReadFile(artifact.FilePath)
	if err != nil {
		t.Fatalf("read final artifact: %v", err)
	}
	if string(raw) != string(pkg) {
		t.Fatalf("unexpected final artifact content: %q", string(raw))
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("expected partial file to be finalized, got %v", err)
	}
}

func TestSelfUpdate_DownloadRestartsWhenPartialRangeIsNotSatisfiable(t *testing.T) {
	pkg := []byte("fresh package after invalid partial")
	sum := sha256.Sum256(pkg)
	var calls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		if call == 1 {
			if got := r.Header.Get("Range"); got == "" {
				t.Errorf("expected first request to try Range resume")
			}
			http.Error(w, "range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if got := r.Header.Get("Range"); got != "" {
			t.Errorf("expected retry without Range, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pkg)
	}))
	defer srv.Close()

	dir := t.TempDir()
	targetVersion := testUpdateVersion()
	targetDir := filepath.Join(dir, targetVersion)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	tmpPath := filepath.Join(targetDir, "pkg.bin.part")
	if err := os.WriteFile(tmpPath, []byte("stale partial content larger than remote"), 0o644); err != nil {
		t.Fatalf("write stale partial: %v", err)
	}

	cfg := config.Config{
		AutoUpdateEnabled: true,
		AutoUpdateDir:     dir,
		AutoUpdateTimeout: 2 * time.Second,
		AutoUpdateMaxMB:   5,
	}
	uc := NewSelfUpdate(cfg, &fakeLogger{})
	payload := &UpdatePayload{
		Version: targetVersion,
		URL:     srv.URL + "/pkg.bin",
		SHA256:  hex.EncodeToString(sum[:]),
	}

	artifact, err := uc.download(context.Background(), payload, uc.effectiveOptions())
	if err != nil {
		t.Fatalf("expected clean retry after 416 to succeed: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
	if artifact.Resumed || artifact.ResumeOffset != 0 {
		t.Fatalf("expected clean download after rejected range, got %#v", artifact)
	}
	raw, err := os.ReadFile(artifact.FilePath)
	if err != nil {
		t.Fatalf("read final artifact: %v", err)
	}
	if string(raw) != string(pkg) {
		t.Fatalf("unexpected final artifact content: %q", string(raw))
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale partial to be removed/finalized, got %v", err)
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
	var precheckReport map[string]any
	for _, item := range received {
		if status, _ := item["status"].(string); status == "precheck_ok" {
			precheckReport = item
			break
		}
	}
	if precheckReport == nil {
		t.Fatalf("expected precheck_ok report, got %#v", received)
	}
	precheckUpdate, ok := precheckReport["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected precheck update metadata")
	}
	preflight, ok := precheckUpdate["preflight"].(map[string]any)
	if !ok {
		t.Fatalf("expected preflight metadata: %#v", precheckUpdate)
	}
	checks, ok := preflight["checks"].(map[string]any)
	if !ok {
		t.Fatalf("expected preflight checks: %#v", preflight)
	}
	for _, key := range []string{"staging_dir", "staging_free_space", "service_manager", "binary_lock", "install_dir", "install_dir_mode", "install_dir_writable", "executable_mode"} {
		if strings.TrimSpace(fmt.Sprint(checks[key])) == "" {
			t.Fatalf("expected preflight check %s in %#v", key, checks)
		}
	}
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
	reportFingerprint, _ := report["report_fingerprint"].(string)
	updateFingerprint, _ := update["report_fingerprint"].(string)
	if strings.TrimSpace(reportFingerprint) == "" {
		t.Fatalf("expected report_fingerprint")
	}
	if reportFingerprint != updateFingerprint {
		t.Fatalf("expected matching fingerprints, got report=%q update=%q", reportFingerprint, updateFingerprint)
	}
	delete(update, "report_fingerprint")
	reportStatus, _ := report["status"].(string)
	reportReason, _ := report["reason_code"].(string)
	reportCurrentVersion, _ := report["current_version"].(string)
	expected := updateReportFingerprint(payload, reportStatus, reportReason, reportCurrentVersion, update)
	if reportFingerprint != expected {
		t.Fatalf("expected stable fingerprint %q, got %q", expected, reportFingerprint)
	}
}

func TestSelfUpdate_PreflightFailsWhenStagingIsNotWritable(t *testing.T) {
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

	dir := t.TempDir()
	blockingFile := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}

	uc := NewSelfUpdate(config.Config{
		Agent:             config.AgentCfg{Token: "agent-token"},
		APIBaseURL:        srv.URL,
		AutoUpdateEnabled: true,
		AutoUpdateDir:     filepath.Join(blockingFile, "updates"),
		AutoUpdateTimeout: time.Second,
	}, &fakeLogger{})

	err := uc.Execute(context.Background(), &UpdatePayload{
		Version: "0.9.0",
		URL:     srv.URL + "/pkg.bin",
	})
	if err == nil {
		t.Fatalf("expected preflight error")
	}
	if len(received) != 1 {
		t.Fatalf("expected one preflight report, got %d", len(received))
	}
	report := received[0]
	if status, _ := report["status"].(string); status != "preflight_failed" {
		t.Fatalf("expected preflight_failed, got %v", report["status"])
	}
	update, ok := report["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected update metadata")
	}
	preflight, ok := update["preflight"].(map[string]any)
	if !ok {
		t.Fatalf("expected preflight metadata: %#v", update)
	}
	if okValue, _ := preflight["ok"].(bool); okValue {
		t.Fatalf("expected preflight ok=false: %#v", preflight)
	}
	if code, _ := preflight["failure_code"].(string); code != "staging_not_writable" {
		t.Fatalf("expected staging_not_writable, got %v", preflight["failure_code"])
	}
}

func TestSelfUpdate_PreflightFailsWhenStagingHasLowSpace(t *testing.T) {
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

	uc := NewSelfUpdate(config.Config{
		Agent:             config.AgentCfg{Token: "agent-token"},
		APIBaseURL:        srv.URL,
		AutoUpdateEnabled: true,
		AutoUpdateDir:     t.TempDir(),
		AutoUpdateTimeout: time.Second,
		AutoUpdateMaxMB:   100_000_000,
	}, &fakeLogger{})

	err := uc.Execute(context.Background(), &UpdatePayload{
		Version: "0.9.0",
		URL:     srv.URL + "/pkg.bin",
	})
	if err == nil {
		t.Fatalf("expected low space preflight error")
	}
	if len(received) != 1 {
		t.Fatalf("expected one preflight report, got %d", len(received))
	}
	report := received[0]
	if status, _ := report["status"].(string); status != "preflight_failed" {
		t.Fatalf("expected preflight_failed, got %v", report["status"])
	}
	update, ok := report["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected update metadata")
	}
	preflight, ok := update["preflight"].(map[string]any)
	if !ok {
		t.Fatalf("expected preflight metadata: %#v", update)
	}
	if code, _ := preflight["failure_code"].(string); code != "staging_low_space" {
		t.Fatalf("expected staging_low_space, got %v", preflight["failure_code"])
	}
	checks, ok := preflight["checks"].(map[string]any)
	if !ok {
		t.Fatalf("expected preflight checks: %#v", preflight)
	}
	if value, _ := checks["staging_free_space"].(string); !strings.Contains(value, "low:") {
		t.Fatalf("expected low staging_free_space check, got %v", checks["staging_free_space"])
	}
}

func TestSelfUpdate_PreflightFailsWhenInstallDirIsNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not reliable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can write to read-only test directories")
	}
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

	installDir := filepath.Join(t.TempDir(), "install")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("create install dir: %v", err)
	}
	exePath := filepath.Join(installDir, "aiceberg-agent")
	if err := os.WriteFile(exePath, []byte("agent"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	if err := os.Chmod(installDir, 0o555); err != nil {
		t.Fatalf("chmod install dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(installDir, 0o755)
	})
	oldExecutablePath := currentExecutablePath
	currentExecutablePath = func() (string, error) { return exePath, nil }
	t.Cleanup(func() {
		currentExecutablePath = oldExecutablePath
	})

	uc := NewSelfUpdate(config.Config{
		Agent:             config.AgentCfg{Token: "agent-token"},
		APIBaseURL:        srv.URL,
		AutoUpdateEnabled: true,
		AutoUpdateDir:     t.TempDir(),
		AutoUpdateTimeout: time.Second,
		AutoUpdateMaxMB:   1,
	}, &fakeLogger{})

	err := uc.Execute(context.Background(), &UpdatePayload{
		Version: "0.9.0",
		URL:     srv.URL + "/pkg.bin",
	})
	if err == nil {
		t.Fatalf("expected install dir preflight error")
	}
	if len(received) != 1 {
		t.Fatalf("expected one preflight report, got %d", len(received))
	}
	report := received[0]
	if status, _ := report["status"].(string); status != "preflight_failed" {
		t.Fatalf("expected preflight_failed, got %v", report["status"])
	}
	update, ok := report["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected update metadata")
	}
	preflight, ok := update["preflight"].(map[string]any)
	if !ok {
		t.Fatalf("expected preflight metadata: %#v", update)
	}
	if code, _ := preflight["failure_code"].(string); code != "install_dir_not_writable" {
		t.Fatalf("expected install_dir_not_writable, got %v", preflight["failure_code"])
	}
	checks, ok := preflight["checks"].(map[string]any)
	if !ok {
		t.Fatalf("expected preflight checks: %#v", preflight)
	}
	if value, _ := checks["install_dir_writable"].(string); strings.TrimSpace(value) == "" || value == "ok" {
		t.Fatalf("expected install_dir_writable failure, got %v", value)
	}
}

func TestSelfUpdate_ApplyFailureReportsExitCodeCooldownAndClearsPending(t *testing.T) {
	pkg := []byte("payload")
	sum := sha256.Sum256(pkg)
	var received []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/agent/update-report" {
			raw, _ := io.ReadAll(r.Body)
			payload := map[string]any{}
			_ = json.Unmarshal(raw, &payload)
			received = append(received, payload)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pkg)
	}))
	defer srv.Close()

	command := "exit 42"
	if runtime.GOOS == "windows" {
		command = "exit 42"
	}
	cfg := config.Config{
		Agent:                   config.AgentCfg{Token: "agent-token"},
		APIBaseURL:              srv.URL,
		AutoUpdateEnabled:       true,
		AutoUpdateDir:           t.TempDir(),
		AutoUpdateTimeout:       2 * time.Second,
		AutoUpdateMaxMB:         5,
		AutoUpdateRetryInterval: time.Hour,
		AutoUpdateCommand:       command,
	}
	uc := NewSelfUpdate(cfg, &fakeLogger{})
	payload := &UpdatePayload{
		Version: testUpdateVersion(),
		URL:     srv.URL + "/pkg.bin",
		SHA256:  hex.EncodeToString(sum[:]),
	}

	err := uc.Execute(context.Background(), payload)
	if err == nil {
		t.Fatalf("expected apply command failure")
	}
	var applyFailed map[string]any
	for _, report := range received {
		if status, _ := report["status"].(string); status == "apply_failed" {
			applyFailed = report
			break
		}
	}
	if applyFailed == nil {
		t.Fatalf("expected apply_failed report, got %#v", received)
	}
	update, ok := applyFailed["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected update metadata")
	}
	if stage, _ := update["stage"].(string); stage != "apply" {
		t.Fatalf("expected apply stage, got %v", update["stage"])
	}
	if exitCode, ok := update["launcher_exit_code"].(float64); !ok || int(exitCode) != 42 {
		t.Fatalf("expected launcher_exit_code=42, got %#v", update["launcher_exit_code"])
	}
	if attempt, ok := update["attempt_count"].(float64); !ok || int(attempt) != 1 {
		t.Fatalf("expected cooldown attempt_count=1, got %#v", update["attempt_count"])
	}
	if reason, _ := update["last_reason_code"].(string); reason == "" {
		t.Fatalf("expected cooldown last_reason_code in update metadata: %#v", update)
	}
	if pending, pendingErr := uc.loadPendingState(cfg.AutoUpdateDir); pendingErr != nil || pending != nil {
		t.Fatalf("expected pending state cleared after apply failure, pending=%#v err=%v", pending, pendingErr)
	}
	if cooldown, ok := uc.loadUpdateCooldown(cfg.AutoUpdateDir); !ok || cooldown.LastReasonCode == "" {
		t.Fatalf("expected cooldown persisted after apply failure, got %#v ok=%v", cooldown, ok)
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
	if rollbackAvailable, _ := pending["rollback_available"].(bool); !rollbackAvailable {
		t.Fatalf("expected rollback_available in pending_state: %#v", pending)
	}
	if rollbackVersion, _ := pending["rollback_version"].(string); rollbackVersion != version.Version {
		t.Fatalf("unexpected rollback_version: %v", pending["rollback_version"])
	}
}

func TestSelfUpdate_SnapshotClearsCurrentVersionPendingState(t *testing.T) {
	cfg := config.Config{
		AutoUpdateEnabled: true,
		AutoUpdateDir:     t.TempDir(),
		AutoUpdateCommand: "echo ok",
	}
	uc := NewSelfUpdate(cfg, &fakeLogger{})
	opts := uc.effectiveOptions()
	artifact := downloadedArtifact{
		FilePath:  filepath.Join(cfg.AutoUpdateDir, version.Version, "pkg.bin"),
		DirPath:   filepath.Join(cfg.AutoUpdateDir, version.Version),
		SHA256:    strings.Repeat("b", 64),
		SizeBytes: 42,
		Source:    "pending_state",
	}
	if err := uc.savePendingState(opts.dir, version.Version, "old-version", artifact, opts); err != nil {
		t.Fatalf("savePendingState failed: %v", err)
	}

	snap := uc.Snapshot()
	if _, ok := snap["pending_state"]; ok {
		t.Fatalf("did not expect pending_state for current version: %#v", snap)
	}
	if pending, err := uc.loadPendingState(cfg.AutoUpdateDir); err != nil || pending != nil {
		t.Fatalf("expected current-version pending state cleared, pending=%#v err=%v", pending, err)
	}
}

func TestSelfUpdate_ClearsCurrentVersionPendingStateBeforeNewUpdate(t *testing.T) {
	pkg := []byte("next package")
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
	opts := uc.effectiveOptions()
	staleArtifact := downloadedArtifact{
		FilePath:  filepath.Join(cfg.AutoUpdateDir, version.Version, "old.bin"),
		DirPath:   filepath.Join(cfg.AutoUpdateDir, version.Version),
		SHA256:    strings.Repeat("a", 64),
		SizeBytes: 12,
		Source:    "pending_state",
	}
	if err := uc.savePendingState(opts.dir, version.Version, "0.8.previous", staleArtifact, opts); err != nil {
		t.Fatalf("savePendingState failed: %v", err)
	}

	payload := &UpdatePayload{
		Version: testUpdateVersion(),
		URL:     srv.URL + "/pkg.bin",
		SHA256:  hex.EncodeToString(sum[:]),
	}
	if err := uc.Execute(context.Background(), payload); err != nil {
		t.Fatalf("expected update to continue after stale pending cleanup, got %v", err)
	}
	if pending, err := uc.loadPendingState(cfg.AutoUpdateDir); err != nil || pending != nil {
		t.Fatalf("expected current-version pending state cleared, pending=%#v err=%v", pending, err)
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

func TestSelfUpdate_ReportPendingResultMarksVersionMismatchAfterRollback(t *testing.T) {
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
	targetVersion := version.Version + "-rollback-target"
	artifact := downloadedArtifact{
		FilePath:  filepath.Join(cfg.AutoUpdateDir, targetVersion, "pkg.bin"),
		DirPath:   filepath.Join(cfg.AutoUpdateDir, targetVersion),
		SHA256:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		SizeBytes: 456,
		Source:    "pending_state",
	}
	opts := uc.effectiveOptions()
	if err := uc.savePendingState(opts.dir, targetVersion, "old-version", artifact, opts); err != nil {
		t.Fatalf("savePendingState failed: %v", err)
	}

	if err := uc.ReportPendingResult(context.Background()); err != nil {
		t.Fatalf("ReportPendingResult failed: %v", err)
	}
	if len(received) != 2 {
		t.Fatalf("expected reconnect and failed confirmation reports, got %d", len(received))
	}
	if status, _ := received[0]["status"].(string); status != "reconnected" {
		t.Fatalf("expected reconnected status, got %v", received[0]["status"])
	}
	failed := received[1]
	if status, _ := failed["status"].(string); status != "rolled_back" {
		t.Fatalf("expected rolled_back status, got %v", failed["status"])
	}
	if code, _ := failed["reason_code"].(string); code != "version_mismatch_after_restart" {
		t.Fatalf("expected version_mismatch_after_restart, got %v", failed["reason_code"])
	}
	update, ok := failed["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected update metadata")
	}
	if rollbackAvailable, _ := update["rollback_available"].(bool); !rollbackAvailable {
		t.Fatalf("expected rollback_available in report metadata: %#v", update)
	}
	if rollbackVersion, _ := update["rollback_version"].(string); rollbackVersion != "old-version" {
		t.Fatalf("expected rollback_version old-version, got %#v", update["rollback_version"])
	}
	if stage, _ := update["stage"].(string); stage != "rolled_back" {
		t.Fatalf("expected rolled_back stage, got %v", update["stage"])
	}
	if failureClass, _ := update["failure_class"].(string); failureClass != "reconexao" {
		t.Fatalf("expected reconexao failure class, got %v", update["failure_class"])
	}
	if current, _ := failed["current_version"].(string); current != version.Version {
		t.Fatalf("expected current version to remain previous runtime version, got %v", failed["current_version"])
	}
	if pending, err := uc.loadPendingState(opts.dir); err != nil || pending != nil {
		t.Fatalf("expected pending state cleared after mismatch report, pending=%#v err=%v", pending, err)
	}
}

func TestSelfUpdate_PersistsCooldownAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	opts := effectiveAutoUpdateOptions{
		dir:   dir,
		retry: time.Hour,
	}
	first := NewSelfUpdate(config.Config{}, &fakeLogger{})
	state := first.recordUpdateCooldown(opts, "0.9.0", "download_failed", errors.New("download timeout token=secret"))

	if state.AttemptCount != 1 || state.CooldownUntilUnix <= time.Now().Unix() {
		t.Fatalf("unexpected cooldown state: %#v", state)
	}
	if state.LastErrorFingerprint == "" || strings.Contains(state.LastErrorFingerprint, "secret") {
		t.Fatalf("unexpected error fingerprint: %#v", state)
	}

	second := NewSelfUpdate(config.Config{}, &fakeLogger{})
	skipped, loaded := second.shouldSkip("0.9.0", opts)
	if !skipped {
		t.Fatalf("expected persisted cooldown to skip update")
	}
	if loaded.AttemptCount != 1 || loaded.LastReasonCode != "download_failed" {
		t.Fatalf("unexpected loaded cooldown: %#v", loaded)
	}
}

func TestSelfUpdate_CleanupOldUpdateStagingKeepsCurrentAndMetadata(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "0.8.current")
	old := filepath.Join(dir, "0.8.old")
	recent := filepath.Join(dir, "0.8.recent")
	meta := filepath.Join(dir, ".pending_update.json")
	for _, path := range []string{current, old, recent} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	if err := os.WriteFile(meta, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}

	uc := NewSelfUpdate(config.Config{}, &fakeLogger{})
	uc.cleanupOldUpdateStaging(dir, "0.8.current", 7*24*time.Hour)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("expected old staging removed, got %v", err)
	}
	for _, path := range []string{current, recent, meta} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s preserved: %v", path, err)
		}
	}
}
