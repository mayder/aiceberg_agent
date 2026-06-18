package httpx

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestNewClientUsesAuthenticatedHTTPProxyFromEnvironment(t *testing.T) {
	expectedProxyAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("proxy-user:proxy-pass"))
	var proxyHits int32

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&proxyHits, 1)
		if got := r.Header.Get("Proxy-Authorization"); got != expectedProxyAuth {
			http.Error(w, "missing proxy auth", http.StatusProxyAuthRequired)
			return
		}
		if got := r.URL.String(); got != "http://updates.example.invalid/pkg.bin" {
			http.Error(w, "unexpected proxy target", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("proxy-ok"))
	}))
	defer proxy.Close()

	proxyURL := strings.Replace(proxy.URL, "http://", "http://proxy-user:proxy-pass@", 1)
	t.Setenv("HTTP_PROXY", proxyURL)
	t.Setenv("http_proxy", proxyURL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	client := NewClient(config.Config{}, 2*time.Second)
	resp, err := client.Get("http://updates.example.invalid/pkg.bin")
	if err != nil {
		t.Fatalf("expected proxied request to succeed, got %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %s", resp.Status)
	}
	if got := atomic.LoadInt32(&proxyHits); got != 1 {
		t.Fatalf("expected one proxy hit, got %d", got)
	}
}
