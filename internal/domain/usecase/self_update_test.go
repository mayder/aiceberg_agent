package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"runtime"
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
