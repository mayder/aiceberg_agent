package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestServiceSourcesAreRestricted(t *testing.T) {
	linux := serviceSources("linux")[0]
	windows := serviceSources("windows")[0]
	if linux.command != "journalctl" || !strings.Contains(strings.Join(linux.args, " "), "--unit=aiceberg-agent.service") {
		t.Fatal(linux)
	}
	if windows.command != "wevtutil" || !strings.Contains(strings.Join(windows.args, " "), "Provider[@Name='AIcebergAgent']") {
		t.Fatal(windows)
	}
	if serviceSources("darwin") != nil {
		t.Fatal("unsupported platform must be explicit")
	}
}

func TestCollectRedactsAndCapsRealOutput(t *testing.T) {
	t.Setenv("AICEBERG_SUPPORT_TEST_HELPER", "1")
	c := &Collector{os: "linux", run: func(ctx context.Context, _ source) ([]byte, bool, error) {
		// Real child process and bounded writer, without reading any host service logs.
		return runCommand(ctx, source{command: os.Args[0], args: []string{"-test.run=^TestSupportOutputHelper$"}})
	}}
	result, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	for _, secret := range []string{"one", "two", "three", "four"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
	row := result["sources"].([]any)[0].(map[string]any)
	if row["truncated"] != true || len(row["text"].(string)) > MaxBytes {
		t.Fatal(row)
	}
	if !strings.Contains(row["text"].(string), "safe") {
		t.Fatal("lost safe evidence")
	}
}

func TestCollectUnavailableNeverClaimsSuccess(t *testing.T) {
	cases := []struct {
		name         string
		err          error
		text, status string
	}{
		{"permission", os.ErrPermission, "secret", "permission_denied"},
		{"missing", exec.ErrNotFound, "secret", "missing"},
		{"failed", errors.New("private error"), "secret", "unavailable"},
		{"empty", nil, "", "empty"},
		{"journal empty", nil, "-- No entries --", "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Collector{os: "linux", run: func(context.Context, source) ([]byte, bool, error) { return []byte(tc.text), false, tc.err }}
			result, err := c.Collect(context.Background())
			if err == nil || result["status"] != "unavailable" {
				t.Fatal(result, err)
			}
			row := result["sources"].([]any)[0].(map[string]any)
			if row["status"] != tc.status {
				t.Fatal(row)
			}
			if tc.err != nil && row["text"] != nil {
				t.Fatal("failed tool output exported")
			}
		})
	}
}

func TestCollectCancellationAndUnsupported(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &Collector{os: "linux", run: func(ctx context.Context, _ source) ([]byte, bool, error) { return nil, false, ctx.Err() }}
	result, err := c.Collect(ctx)
	if err == nil || result["sources"].([]any)[0].(map[string]any)["status"] != "cancelled_or_timeout" {
		t.Fatal(result, err)
	}
	c.os = "darwin"
	if result, err = c.Collect(context.Background()); err == nil || result["status"] != "unsupported" {
		t.Fatal(result, err)
	}
}

func TestSupportOutputHelper(t *testing.T) {
	if os.Getenv("AICEBERG_SUPPORT_TEST_HELPER") != "1" {
		return
	}
	fmt.Print("safe\npassword=one token=two\nAuthorization: Bearer three\nhttps://name:four@example.test\n" + strings.Repeat("x", MaxBytes))
	os.Exit(0)
}
