package localchecks

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestRunHTTPCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	metrics, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "http",
		Target:  srv.URL,
		Enabled: true,
	}, 1024)
	if err != nil {
		t.Fatalf("expected http ok, got %v", err)
	}
	if serviceCheck["status"] != "ok" || len(metrics) != 1 {
		t.Fatalf("unexpected result %#v %#v", serviceCheck, metrics)
	}
}

func TestRunOpenMetricsParsesMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("# HELP x\nrequests_total{code=\"200\"} 12\nlatency_seconds 1.5\n"))
	}))
	defer srv.Close()

	metrics, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "openmetrics",
		Target:  srv.URL,
		Enabled: true,
	}, 1024)
	if err != nil {
		t.Fatalf("expected openmetrics ok, got %v", err)
	}
	if serviceCheck["status"] != "ok" || len(metrics) != 2 {
		t.Fatalf("unexpected result %#v %#v", serviceCheck, metrics)
	}
	if metrics[0]["name"] != "requests_total" {
		t.Fatalf("expected metric name without labels, got %#v", metrics[0])
	}
}

func TestRunTCPCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	_, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "redis",
		Target:  ln.Addr().String(),
		Enabled: true,
	}, 1024)
	if err != nil {
		t.Fatalf("expected tcp ok, got %v", err)
	}
	if serviceCheck["status"] != "ok" {
		t.Fatalf("unexpected service check %#v", serviceCheck)
	}
}

func TestDisallowsArbitraryCheckKindAndRedactsResult(t *testing.T) {
	c := &collector{baseEnabled: true, baseChecks: []config.LocalCheckConfig{{
		ID:             "bad",
		Kind:           "shell",
		Target:         "token=abc",
		CredentialsRef: "vault/path",
		Enabled:        true,
	}}, interval: time.Second, maxChecks: 10, maxBytes: 1024}

	raw, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !contains(body, "[redacted]") || contains(body, "abc") || contains(body, "vault/path") {
		t.Fatalf("expected redacted payload, got %s", body)
	}
}

func contains(value, part string) bool {
	return len(part) == 0 || (len(value) >= len(part) && index(value, part) >= 0)
}

func index(value, part string) int {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
