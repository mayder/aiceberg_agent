//go:build !windows

package oslogs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKernelStorageWaitSurvivesErrorMinimumWithOriginalTimestamp(t *testing.T) {
	c := collector{enrich: true, maxBytes: 4096, cursor: map[string]int64{}}
	line := "2026-11-23T16:23:57.100140-03:00 srv-ainalise-prod kernel: megaraid_sas 0000:02:00.0: [ 0]waiting for 98 commands to complete for scsi0"
	ev := c.buildEvent("/var/log/kern.log", "srv-ainalise-prod", line)
	if ev.App != "kernel" || ev.TimestampUTC != "2026-11-23T19:23:57.10014Z" || ev.Level != "critical" || shouldDropLogEvent(ev, "", "", "error") {
		t.Fatalf("kernel wait dropped or time corrupted %+v", ev)
	}
	if ev.Attributes["storage_rule_id"] != "infra.storage.controller-wait.v1" {
		t.Fatal("missing provenance")
	}
	for _, line := range []string{"2026-11-23T16:25:33-03:00 host kernel: megaraid_sas 0000:02:00.0: IO is completed, no OCR is required", "2026-11-23T16:23:57-03:00 host other: megaraid_sas waiting for 98 commands to complete"} {
		ev := c.buildEvent("/var/log/syslog", "host", line)
		if ev.Level == "critical" {
			t.Fatal("healthy or non-kernel elevated")
		}
	}
}

func TestStorageFileCursorDoesNotSkipBufferedReadAhead(t *testing.T) {
	line := "2026-11-23T16:23:57-03:00 host kernel: megaraid_sas 0000:02:00.0: waiting for 98 commands to complete for scsi0\n"
	path := filepath.Join(t.TempDir(), "kern.log")
	if err := os.WriteFile(path, []byte(strings.Repeat(line, 120)), 0600); err != nil {
		t.Fatal(err)
	}
	c := collector{enrich: true, maxBytes: 4096, batchLines: 1, cursor: map[string]int64{}}
	for i := 1; i <= 120; i++ {
		events, _ := c.readFile(path, "host")
		if len(events) != 1 || c.cursor[path] != int64(i*len(line)) {
			t.Fatalf("lost buffered event at%d, cursor%d", i, c.cursor[path])
		}
	}
	events, _ := c.readFile(path, "host")
	if len(events) != 0 {
		t.Fatal("duplicate after EOF")
	}
}
