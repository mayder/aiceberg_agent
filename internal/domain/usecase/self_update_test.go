package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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
