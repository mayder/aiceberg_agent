package agentless

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestICMPExtractsLatencyWithoutPersistingCommandOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local ping fixture uses a POSIX shell")
	}
	dir := t.TempDir()
	ping := filepath.Join(dir, "ping")
	t.Setenv("PATH", dir)
	job := entities.AgentlessJob{ClienteID: 2, AtivoID: 4, CheckID: 6, Tipo: "icmp",
		Endpoint: &entities.AgentlessEndpoint{Tipo: "ip", Endereco: "127.0.0.1"}}
	for status, script := range map[string]string{"ok": "exit 0", "fail": "exit 1"} {
		if err := os.WriteFile(ping, []byte("#!/bin/sh\nprintf 'private-ping-output time=7.1 ms\\n'\n"+script+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		obs := RunJob(context.Background(), job)
		if obs.Status != status || (status == "ok" && obs.LatencyMs != 7) || (status == "fail" && obs.Code != "ping_fail") {
			t.Fatalf("unexpected ICMP result: status=%s latency=%d code=%s", obs.Status, obs.LatencyMs, obs.Code)
		}
		wire, err := json.Marshal(obs)
		if err != nil || strings.Contains(string(wire), "private-ping-output") || len(obs.Payload) != 0 {
			t.Fatal("raw ping output escaped collection")
		}
	}
}
