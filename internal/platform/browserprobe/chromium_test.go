package browserprobe

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBrowserSandboxRejectsUnsafeEffectiveArguments(t *testing.T) {
	for _, flag := range []string{"--no-sandbox", "--no-sandbox=false", "--disable-seccomp-filter-sandbox", "--single-process"} {
		if allowsBrowserSandbox([]string{"/fixture/chromium", "--headless", flag}) {
			t.Fatalf("effective arguments bypassed the sandbox check: %s", flag)
		}
	}
	if allowsBrowserSandbox(nil) || !allowsBrowserSandbox([]string{"/fixture/chromium", "--headless"}) {
		t.Fatal("sandbox check did not distinguish absent and allowed arguments")
	}
}

func TestChromiumRejectsWrapperThatDisablesSandboxBeforeLoadingPage(t *testing.T) {
	path := chromiumFixture(t)
	var requests atomic.Int64
	app := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer app.Close()
	wrapper := filepath.Join(t.TempDir(), "chromium-fixture")
	quotedPath := "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nexec "+quotedPath+" --no-sandbox \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	receipt := Run(context.Background(), wrapper, testRecipe(app.URL))
	if receipt.Outcome != "runner_unavailable" || receipt.Completed != 0 || requests.Load() != 0 {
		t.Fatalf("an unsandboxed browser reached the fixture: receipt=%+v requests=%d", receipt, requests.Load())
	}
}

func TestChromiumRunsJourneyAndKeepsNetworkOnReviewedOrigin(t *testing.T) {
	path := chromiumFixture(t)
	var foreignRequests, writes, reads atomic.Int64
	foreign := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		foreignRequests.Add(1)
	}))
	defer foreign.Close()
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<!doctype html><meta charset="UTF-8"><title>Journey fixture</title>
<button data-testid="refresh">Refresh</button><output data-testid="state">Waiting</output>
<script src="/application.js"></script><img src="%s/pixel.png" alt="">`, foreign.URL)
		case "/application.js":
			w.Header().Set("Content-Type", "application/javascript")
			fmt.Fprint(w, `document.querySelector("button").onclick = async () => {
				try { await fetch('/update', {method: 'POST'}); } catch {}
				const response = await fetch('/status');
				document.querySelector('output').innerText = await response.text();
			};`)
		case "/update":
			writes.Add(1)
		case "/status":
			reads.Add(1)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "Reading updated fixture-private-page-content")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer app.Close()
	got := Run(context.Background(), path, testRecipe(app.URL))
	if got.Outcome != "ok" || got.Completed != 3 || got.CheckID != 90 {
		t.Fatalf("Chrome did not complete the real steps: %+v", got)
	}
	if reads.Load() != 1 || foreignRequests.Load() != 0 || writes.Load() != 0 {
		t.Fatalf("egress or action mismatch: reads=%d foreign=%d writes=%d", reads.Load(), foreignRequests.Load(), writes.Load())
	}
	wire, err := json.Marshal(got)
	if err != nil || strings.Contains(string(wire), "fixture-private") || strings.Contains(string(wire), app.URL) {
		t.Fatal("private page metadata escaped the runner")
	}
}

func TestChromiumRejectsExternalNavigationAndLimitsResponseSize(t *testing.T) {
	path := chromiumFixture(t)
	var foreignRequests atomic.Int64
	foreign := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		foreignRequests.Add(1)
	}))
	defer foreign.Close()
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/offsite":
			http.Redirect(w, r, foreign.URL, http.StatusFound)
		case "/large":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, strings.Repeat("a", maxResponseBody+1))
		}
	}))
	defer app.Close()
	for _, fixture := range []struct{ path, outcome string }{{"/offsite", "step_failed"}, {"/large", "resource_limit"}} {
		t.Run(fixture.path, func(t *testing.T) {
			recipe := testRecipe(app.URL)
			recipe.Steps[0].Path = fixture.path
			got := Run(context.Background(), path, recipe)
			if got.Outcome != fixture.outcome || got.Completed != 0 {
				t.Fatalf("invalid navigation succeeded: %+v", got)
			}
		})
	}
	if foreignRequests.Load() != 0 {
		t.Fatalf("foreign redirect reached the server: %d", foreignRequests.Load())
	}
}

func TestChromiumCancelsAnActiveRequest(t *testing.T) {
	path := chromiumFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	stopped := make(chan struct{})
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		cancel()
		<-r.Context().Done()
		close(stopped)
	}))
	defer app.Close()
	got := Run(ctx, path, testRecipe(app.URL))
	if got.Outcome != "cancelled" || got.Completed != 0 {
		t.Fatalf("expected cancellation with no completed step: %+v", got)
	}
	<-started
	<-stopped
}

func TestChromiumJourneyRequiresTrustedCertificateForApprovedOrigin(t *testing.T) {
	chromium := chromiumFixture(t)
	var pages atomic.Int64
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		pages.Add(1)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><button data-testid="refresh"
onclick="document.querySelector('output').innerText='Reading updated'">Refresh</button>
<output data-testid="state">Waiting</output>`)
	})
	trusted := httptest.NewTLSServer(app)
	defer trusted.Close()
	wrongName := wrongNameTLSServer(t, app)
	defer wrongName.Close()
	for _, scenario := range []struct {
		name      string
		server    *httptest.Server
		trust     bool
		completed int
		outcome   string
	}{
		{"trusted origin", trusted, true, 3, "ok"},
		{"untrusted certificate", trusted, false, 0, "step_failed"},
		{"trusted certificate for another host", wrongName, true, 0, "step_failed"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			roots := x509.NewCertPool()
			if scenario.trust {
				roots.AddCert(scenario.server.Certificate())
				if _, err := scenario.server.Certificate().Verify(x509.VerifyOptions{Roots: roots}); err != nil {
					t.Fatalf("fixture certificate must be trusted before testing its origin: %v", err)
				}
			}
			got := run(context.Background(), chromium, testRecipe(scenario.server.URL), roots)
			if got.Outcome != scenario.outcome || got.Completed != scenario.completed {
				t.Fatalf("unexpected HTTPS journey result: %+v", got)
			}
		})
	}
	if pages.Load() != 1 {
		t.Fatalf("unexpected number of HTTPS page requests: %d", pages.Load())
	}
}

func wrongNameTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{"other.fixture.invalid"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	return server
}

func chromiumFixture(t *testing.T) string {
	t.Helper()
	path := os.Getenv("AICEBERG_BROWSER_FIXTURE_CHROMIUM")
	if path == "" {
		t.Skip("requires AICEBERG_BROWSER_FIXTURE_CHROMIUM pointing to an existing browser")
	}
	if !browserSupported(path) {
		t.Fatal("fixture browser requires a non-root Linux/macOS account and an executable absolute path")
	}
	return path
}
