//go:build !windows

package oslogs

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var storageWaitPattern = regexp.MustCompile(`(?i)\bmegaraid_sas\b[^\r\n]*\bwaiting for [1-9][0-9]* commands to complete\b`)

// ISO rsyslog timestamps contain colons: remove the timestamp/host before
// parsing the program header, otherwise the time itself becomes the program.
func unixISOPrefix(line string) (string, string) {
	f := strings.SplitN(strings.TrimSpace(line), " ", 3)
	if len(f) != 3 {
		return line, ""
	}
	at, err := time.Parse(time.RFC3339Nano, f[0])
	if err != nil {
		return line, ""
	}
	return f[2], at.UTC().Format(time.RFC3339Nano)
}

func preserveCriticalStorageSignal(ev logEvent) logEvent {
	kernel := ev.App == "kernel" && (ev.File == "journald" || filepath.Base(ev.File) == "syslog" || filepath.Base(ev.File) == "kern.log")
	if !kernel || !storageWaitPattern.MatchString(ev.Message) {
		return ev
	}
	if ev.Attributes == nil {
		ev.Attributes = map[string]any{}
	}
	ev.Attributes["original_severity"] = ev.Severity
	ev.Attributes["storage_rule_id"] = "infra.storage.controller-wait.v1"
	ev.Level = "critical"
	ev.Severity = "critical"
	ev.EventCode = "storage.controller_wait"
	return ev
}
