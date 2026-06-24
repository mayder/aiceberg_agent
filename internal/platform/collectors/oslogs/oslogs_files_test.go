//go:build !windows
// +build !windows

package oslogs

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

func TestCollectorCollectDisabled(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "sys.log")
	if err := os.WriteFile(logFile, []byte("test line\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   256,
		OSLogInterval:   1 * time.Second,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: false}
	}

	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if data != nil {
		t.Fatalf("expected nil data when disabled")
	}
}

func TestCollectorDiagNoNewEventsDoesNotReturnError(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "sys.log")
	if err := os.WriteFile(logFile, []byte("Jun 20 10:00:00 host app[1]: first error\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   256,
		OSLogInterval:   time.Second,
		OSLogDiag:       true,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogDiag: true}
	}
	c := New(cfg, prefs)

	first, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("first collect: %v", err)
	}
	if first == nil {
		t.Fatalf("expected first payload")
	}
	second, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("expected no diagnostic error for no new events, got %v", err)
	}
	var secondPayload struct {
		Events          []map[string]any `json:"events"`
		LogSourceHealth []map[string]any `json:"log_source_health"`
	}
	if err := json.Unmarshal(second, &secondPayload); err != nil {
		t.Fatalf("invalid second payload: %v", err)
	}
	if len(secondPayload.Events) != 0 || len(secondPayload.LogSourceHealth) != 1 {
		t.Fatalf("expected only source health for no new events, got %s", string(second))
	}
	if secondPayload.LogSourceHealth[0]["status"] != "no_new_events" {
		t.Fatalf("expected no_new_events health, got %#v", secondPayload.LogSourceHealth[0])
	}
}

func TestCollectorDiagMissingFileStillReturnsError(t *testing.T) {
	tmp := t.TempDir()
	missingFile := filepath.Join(tmp, "missing.log")
	cfg := config.Config{
		OSLogFiles:      []string{missingFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   256,
		OSLogInterval:   time.Second,
		OSLogDiag:       true,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogDiag: true}
	}
	c := New(cfg, prefs)

	if _, err := c.Collect(context.Background()); err == nil {
		t.Fatalf("expected diagnostic error for missing file")
	}
}

func TestSeverityFilterDropsUnknownWhenMinimumConfigured(t *testing.T) {
	if !shouldDropLogEvent(logEvent{Message: "regular cron line"}, "", "", "error") {
		t.Fatalf("expected unknown level to be dropped when min severity is configured")
	}
	if shouldDropLogEvent(logEvent{Level: "error", Message: "failed login"}, "", "", "error") {
		t.Fatalf("expected error level to pass min severity filter")
	}
	if shouldDropLogEvent(logEvent{Message: "Invalid user admin from 10.0.0.1"}, "", "", "error") {
		t.Fatalf("expected security signal without explicit level to pass min severity filter")
	}
}

func TestSecuritySignalsPassMinimumSeverityWithoutExplicitLevel(t *testing.T) {
	signals := []string{
		"sshd[10]: Failed password for invalid user root from 10.0.0.5",
		"postfix/smtpd[20]: warning: unknown[10.0.0.6]: SASL LOGIN authentication failed",
		"sudo: pam_unix(sudo:auth): authentication failure",
		"nginx: [error] upstream timed out while reading response header",
		"apache2: [error] client denied by server configuration",
		"kernel: Out of memory: Killed process 42",
	}
	for _, msg := range signals {
		if shouldDropLogEvent(logEvent{Message: msg}, "", "", "error") {
			t.Fatalf("expected security/operational signal to pass min severity: %s", msg)
		}
	}
}

func TestHealthStatusForMissingWindowsChannelAndDroppedEvents(t *testing.T) {
	channelHealth := newLogSourceHealth("windows_eventlog", "Microsoft-Windows-Sysmon/Operational")
	channelHealth.PermissionStatus = "missing"
	channelHealth.LastError = "falha wevtutil Microsoft-Windows-Sysmon/Operational"
	channelHealth.Gaps = appendUniqueHealthGap(channelHealth.Gaps, "channel_or_permission_error")

	finalChannel := finalizeLogSourceHealth(channelHealth)
	if finalChannel.Status != "channel_missing" || finalChannel.Confidence != "high" {
		t.Fatalf("expected missing Windows channel health, got %#v", finalChannel)
	}

	droppedHealth := newLogSourceHealth("file", "/var/log/auth.log")
	droppedHealth.PermissionStatus = "ok"
	droppedHealth.ReadLines = 3
	droppedHealth.DroppedEvents = 3
	droppedHealth.DropReason = "severity_or_processor"

	finalDropped := finalizeLogSourceHealth(droppedHealth)
	if finalDropped.Status != "dropped_by_severity" || finalDropped.Confidence != "medium" {
		t.Fatalf("expected dropped_by_severity health, got %#v", finalDropped)
	}
}

func TestCollectorMinSeverityUsesJSONSeverity(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "app.log")
	content := strings.Join([]string{
		`{"severity":"info","message":"health ok"}`,
		`{"severity":"error","message":"payment failed"}`,
		`{"message":"no level"}`,
		"",
	}, "\n")
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:       []string{logFile},
		OSLogCursorPath:  filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines:  10,
		OSLogMaxBytes:    512,
		OSLogInterval:    time.Second,
		OSLogMinSeverity: "error",
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true}
	}
	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	var payload struct {
		Events          []map[string]any `json:"events"`
		DroppedCount    int              `json:"dropped_count"`
		LogSourceHealth []map[string]any `json:"log_source_health"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected only error event, got %#v", payload.Events)
	}
	if got := payload.Events[0]["level"]; got != "error" {
		t.Fatalf("expected level error, got %#v", got)
	}
	if payload.DroppedCount != 2 {
		t.Fatalf("expected 2 dropped events, got %d", payload.DroppedCount)
	}
	if len(payload.LogSourceHealth) != 1 {
		t.Fatalf("expected source health, got %#v", payload.LogSourceHealth)
	}
	if payload.LogSourceHealth[0]["status"] != "delivering" || payload.LogSourceHealth[0]["accepted_events"] != float64(1) {
		t.Fatalf("unexpected source health %#v", payload.LogSourceHealth[0])
	}
}

func TestCollectorClassifiesGraylogGELFAndLinuxAuthAndAppFormats(t *testing.T) {
	tmp := t.TempDir()
	graylogFile := filepath.Join(tmp, "graylog.log")
	authFile := filepath.Join(tmp, "auth.log")
	secureFile := filepath.Join(tmp, "secure")
	appFile := filepath.Join(tmp, "app.log")
	textFile := filepath.Join(tmp, "plain.log")
	if err := os.WriteFile(graylogFile, []byte(`{"version":"1.1","host":"winhost","short_message":"failed logon","level":3,"_gl2_source_input":"gelf-tcp","_app":"ad"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write graylog file: %v", err)
	}
	if err := os.WriteFile(authFile, []byte("Jan  1 00:00:01 host sshd[123]: Failed password for invalid user root from 10.0.0.5\n"), 0o644); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if err := os.WriteFile(secureFile, []byte("Jan  1 00:00:02 host sshd[124]: Failed password for invalid user admin from 10.0.0.6\n"), 0o644); err != nil {
		t.Fatalf("write secure file: %v", err)
	}
	if err := os.WriteFile(appFile, []byte(`{"severity":"error","message":"payment failed","service":"checkout","token":"secret"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write app file: %v", err)
	}
	if err := os.WriteFile(textFile, []byte("plain text warning line\n"), 0o644); err != nil {
		t.Fatalf("write text file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{graylogFile, authFile, secureFile, appFile, textFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   1024,
		OSLogInterval:   time.Second,
		OSLogEnrich:     true,
		OSLogDetections: true,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogEnrich: true, OSLogDetections: true}
	}

	payload := collectLogPayload(t, New(cfg, prefs))
	if len(payload.Events) != 5 {
		t.Fatalf("expected five events, got %#v", payload.Events)
	}
	byFile := map[string]logEvent{}
	for _, event := range payload.Events {
		byFile[event.File] = event
	}
	if got := byFile[graylogFile]; got.SourceTool != "graylog_gelf" || got.Message != "failed logon" || got.Service != "ad" || got.Level != "error" {
		t.Fatalf("unexpected graylog event: %#v", got)
	}
	if got := byFile[graylogFile]; got.AicebergToolOrigin != "graylog_gelf" || got.AicebergSOCEligible != "conditional" || got.AicebergRouteReason != "graylog_unknown_origin" {
		t.Fatalf("unexpected graylog SOC contract: %#v", got)
	}
	if got := byFile[authFile]; got.SourceTool != "linux_auth" || got.SourceCategory != "security" || got.Category != "auth_fail" {
		t.Fatalf("unexpected linux auth event: %#v", got)
	}
	if got := byFile[authFile]; got.AicebergSourceCategory != "soc" || got.AicebergSOCSourceType != "linux_security" || got.AicebergSOCEligible != "yes" {
		t.Fatalf("unexpected linux auth SOC contract: %#v", got)
	}
	if got := byFile[secureFile]; got.SourceTool != "linux_auth" || got.SourceCategory != "security" || got.Category != "auth_fail" {
		t.Fatalf("unexpected linux secure event: %#v", got)
	}
	if got := byFile[appFile]; got.Service != "checkout" || got.Level != "error" || strings.Contains(got.Message, "secret") {
		t.Fatalf("unexpected app json event: %#v", got)
	}
	if got := byFile[appFile]; got.AicebergToolOrigin != "application" || got.AicebergSourceCategory != "conditional" || got.AicebergSOCEligible != "conditional" {
		t.Fatalf("unexpected app SOC contract: %#v", got)
	}
	if got := byFile[textFile]; got.SourceTool != "file" || got.Message != "plain text warning line" {
		t.Fatalf("unexpected text file event: %#v", got)
	}
}

func TestCollectorCollectsLocalUDPAndTCPLogs(t *testing.T) {
	cfg := config.Config{
		OSLogFiles:      []string{filepath.Join(t.TempDir(), "missing.log")},
		OSLogCursorPath: filepath.Join(t.TempDir(), "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   512,
		OSLogInterval:   time.Second,
		OSLogUDPAddr:    "127.0.0.1:0",
		OSLogTCPAddr:    "127.0.0.1:0",
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true}
	}
	c := New(cfg, prefs)
	collector, ok := c.(*collector)
	if !ok || collector.local == nil {
		t.Fatalf("expected local receiver")
	}
	defer collector.local.Close()

	udpConn, err := net.Dial("udp", collector.local.udpLocalAddr())
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	if _, err := fmt.Fprintln(udpConn, "udp auth token=secret"); err != nil {
		t.Fatalf("write udp: %v", err)
	}
	_ = udpConn.Close()

	tcpConn, err := net.Dial("tcp", collector.local.tcpLocalAddr())
	if err != nil {
		t.Fatalf("dial tcp: %v", err)
	}
	if _, err := fmt.Fprintln(tcpConn, `{"level":"info","message":"tcp ok","password":"secret"}`); err != nil {
		t.Fatalf("write tcp: %v", err)
	}
	_ = tcpConn.Close()

	transports := map[string]bool{}
	var payload struct {
		Events []map[string]any `json:"events"`
	}
	for i := 0; i < 20; i++ {
		data, err := c.Collect(context.Background())
		if err != nil {
			t.Fatalf("collect: %v", err)
		}
		if len(data) > 0 {
			var current struct {
				Events []map[string]any `json:"events"`
			}
			if err := json.Unmarshal(data, &current); err != nil {
				t.Fatalf("invalid payload: %v", err)
			}
			payload.Events = append(payload.Events, current.Events...)
			for _, event := range current.Events {
				transport, _ := event["transport"].(string)
				transports[transport] = true
			}
			if transports["agent_udp"] && transports["agent_tcp"] {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(payload.Events) < 2 {
		t.Fatalf("expected udp and tcp events, got %#v", payload.Events)
	}
	for _, event := range payload.Events {
		transport, _ := event["transport"].(string)
		transports[transport] = true
		msg, _ := event["message"].(string)
		if strings.Contains(msg, "secret") {
			t.Fatalf("local log leaked secret: %q", msg)
		}
		if event["source_category"] != "log" {
			t.Fatalf("expected source_category log, got %#v", event)
		}
	}
	if !transports["agent_udp"] || !transports["agent_tcp"] {
		t.Fatalf("expected agent_udp and agent_tcp transports, got %#v", transports)
	}
}

func TestCollectorCollectsJournaldWithUnitAndPriorityFilters(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Config{
		OSLogFiles:              []string{filepath.Join(tmp, "missing.log")},
		OSLogCursorPath:         filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines:         10,
		OSLogMaxBytes:           512,
		OSLogInterval:           time.Second,
		OSLogJournaldEnabled:    true,
		OSLogJournaldUnits:      []string{"nginx.service", "bad unit;rm -rf /"},
		OSLogJournaldPriorities: []string{"warn", "invalid"},
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true}
	}
	c := New(cfg, prefs)
	collector, ok := c.(*collector)
	if !ok {
		t.Fatalf("expected POSIX collector")
	}
	var args []string
	collector.journald.run = func(_ context.Context, got []string) ([]byte, error) {
		args = append([]string{}, got...)
		return []byte(strings.Join([]string{
			`{"MESSAGE":"accepted password=secret","PRIORITY":"4","_SYSTEMD_UNIT":"nginx.service","SYSLOG_IDENTIFIER":"nginx","_PID":"123","_HOSTNAME":"host","_SOURCE_REALTIME_TIMESTAMP":"1780000000000000"}`,
			`{"MESSAGE":"ignored debug","PRIORITY":"7","_SYSTEMD_UNIT":"nginx.service","_SOURCE_REALTIME_TIMESTAMP":"1780000000000001"}`,
		}, "\n")), nil
	}

	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if !containsAllArgs(args, "-u", "nginx.service") {
		t.Fatalf("expected sanitized unit arg, got %#v", args)
	}
	if containsAny(strings.Join(args, " "), "bad unit", "rm -rf") {
		t.Fatalf("unsafe journald unit reached args: %#v", args)
	}
	var payload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected one filtered journald event, got %d: %#v", len(payload.Events), payload.Events)
	}
	event := payload.Events[0]
	if event["transport"] != "agent_journald" || event["source_tool"] != "journald" {
		t.Fatalf("expected journald metadata, got %#v", event)
	}
	msg, _ := event["message"].(string)
	if strings.Contains(msg, "secret") {
		t.Fatalf("journald event leaked secret: %q", msg)
	}
	if event["service"] != "nginx.service" || event["level"] != "warning" {
		t.Fatalf("expected service and level, got %#v", event)
	}
	if event["aiceberg_transport"] != "agent_journald" || event["aiceberg_tool_origin"] != "journald" || event["aiceberg_source_category"] != "observability" {
		t.Fatalf("expected journald SOC contract, got %#v", event)
	}
}

func TestCollectorAppliesConfiguredProcessors(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "app.log")
	content := strings.Join([]string{
		`{"message":"charge card=4111111111111111","app":"billing","severity":"error","tenant":"gold"}`,
		`{"message":"health check ok","app":"billing","severity":"info","tenant":"gold"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   1024,
		OSLogInterval:   time.Second,
		OSLogProcessors: []config.LogProcessorConfig{
			{Type: "parse"},
			{Type: "remap", Field: "service", Key: "app"},
			{Type: "remap", Field: "level", Key: "severity"},
			{Type: "drop", Pattern: "health check"},
			{Type: "mask", Pattern: `card=\d+`, Replacement: "card=[redacted]"},
			{Type: "route", Value: "security"},
			{Type: "sample", Rate: 1},
			{Type: "enrich", Key: "pipeline", Value: "local"},
		},
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true}
	}

	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	var payload struct {
		Events       []map[string]any `json:"events"`
		DroppedCount int              `json:"dropped_count"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.Events) != 1 || payload.DroppedCount != 1 {
		t.Fatalf("expected one kept and one dropped event, got events=%d dropped=%d", len(payload.Events), payload.DroppedCount)
	}
	event := payload.Events[0]
	if event["service"] != "billing" || event["level"] != "error" || event["source_category"] != "security" {
		t.Fatalf("expected remap and route processors, got %#v", event)
	}
	if event["aiceberg_source_category"] != "soc" {
		t.Fatalf("expected SOC contract recalculated after route processor, got %#v", event)
	}
	msg, _ := event["message"].(string)
	if strings.Contains(msg, "411111") || !strings.Contains(msg, "card=[redacted]") {
		t.Fatalf("expected masked card in message, got %q", msg)
	}
	attrs, ok := event["attributes"].(map[string]any)
	if !ok || attrs["pipeline"] != "local" {
		t.Fatalf("expected enrichment attribute, got %#v", event["attributes"])
	}
}

func TestCollectorCollectReadsFile(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "sys.log")
	content := "Jan  1 00:00:01 host app[123]: hello world\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   256,
		OSLogInterval:   1 * time.Second,
		OSLogEnrich:     true,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogEnrich: true}
	}

	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected payload")
	}
	var payload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.Events) == 0 {
		t.Fatalf("expected events")
	}
	if payload.Events[0]["file"] == "" {
		t.Fatalf("expected file in event")
	}
	if payload.Events[0]["message"] == "" {
		t.Fatalf("expected message in event")
	}
	if payload.Events[0]["schema_version"] != float64(1) {
		t.Fatalf("expected schema_version in event, got %#v", payload.Events[0])
	}
	if payload.Events[0]["redaction_status"] == "" {
		t.Fatalf("expected redaction_status in event")
	}
}

func TestCollectorRedactsSensitiveValuesAndExtractsJSONAttributes(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "app.log")
	content := `{"level":"info","token":"abc123","message":"Authorization: Bearer secret-token password=hunter2"}`
	if err := os.WriteFile(logFile, []byte(content+"\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   512,
		OSLogInterval:   1 * time.Second,
		OSLogEnrich:     true,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogEnrich: true}
	}

	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	var payload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(payload.Events))
	}
	msg, _ := payload.Events[0]["message"].(string)
	if msg == "" || containsAny(msg, "abc123", "secret-token", "hunter2") {
		t.Fatalf("message leaked secret: %q", msg)
	}
	if payload.Events[0]["redaction_status"] != "redacted" {
		t.Fatalf("expected redacted status, got %#v", payload.Events[0]["redaction_status"])
	}
	attrs, ok := payload.Events[0]["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("expected attributes, got %#v", payload.Events[0])
	}
	if attrs["token"] != "[redacted]" {
		t.Fatalf("expected redacted token attribute, got %#v", attrs)
	}
}

func TestCollectorCombinesMultilineStackTrace(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "app.log")
	content := strings.Join([]string{
		"Jan  1 00:00:01 host app[123]: panic: boom",
		"goroutine 1 [running]:",
		"main.main()",
		"Jan  1 00:00:02 host app[123]: recovered",
	}, "\n") + "\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   1024,
		OSLogInterval:   1 * time.Second,
		OSLogEnrich:     true,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogEnrich: true}
	}

	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	var payload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.Events) != 2 {
		t.Fatalf("expected 2 multiline events, got %d: %#v", len(payload.Events), payload.Events)
	}
	msg, _ := payload.Events[0]["message"].(string)
	if !strings.Contains(msg, "goroutine 1") || !strings.Contains(msg, "main.main()") {
		t.Fatalf("expected stack trace in first event, got %q", msg)
	}
}

func TestCollectorPersistsCursorAcrossRestartAndHandlesRotation(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "app.log")
	cursorPath := filepath.Join(tmp, "cursor.json")
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: cursorPath,
		OSLogBatchLines: 10,
		OSLogMaxBytes:   1024,
		OSLogInterval:   time.Second,
		OSLogEnrich:     true,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogEnrich: true}
	}

	if err := os.WriteFile(logFile, []byte("Jan  1 00:00:01 host app[123]: first error\n"), 0o644); err != nil {
		t.Fatalf("write initial log: %v", err)
	}
	firstCollector := New(cfg, prefs)
	firstPayload := collectLogPayload(t, firstCollector)
	if got := eventMessages(firstPayload.Events); len(got) != 1 || !strings.Contains(got[0], "first error") {
		t.Fatalf("expected first event, got %#v", got)
	}

	restartedCollector := New(cfg, prefs)
	restartPayload := collectLogPayload(t, restartedCollector)
	if len(restartPayload.Events) != 0 || len(restartPayload.LogSourceHealth) != 1 {
		t.Fatalf("expected restart without duplicate and with source health, got %#v", restartPayload)
	}
	if restartPayload.LogSourceHealth[0].Status != "no_new_events" {
		t.Fatalf("expected no_new_events after restart, got %#v", restartPayload.LogSourceHealth[0])
	}

	if err := os.Truncate(logFile, 0); err != nil {
		t.Fatalf("truncate log: %v", err)
	}
	if err := os.WriteFile(logFile, []byte("Jan  1 00:00:02 host app[123]: after truncate error\n"), 0o644); err != nil {
		t.Fatalf("write truncated log: %v", err)
	}
	truncatedCollector := New(cfg, prefs)
	truncatedPayload := collectLogPayload(t, truncatedCollector)
	if got := eventMessages(truncatedPayload.Events); len(got) != 1 || !strings.Contains(got[0], "after truncate error") {
		t.Fatalf("expected truncated event, got %#v", got)
	}

	rotatedPath := filepath.Join(tmp, "app.log.1")
	if err := os.Rename(logFile, rotatedPath); err != nil {
		t.Fatalf("rotate log: %v", err)
	}
	rotatedContent := strings.Repeat("x", 256)
	rotatedContent += "\nJan  1 00:00:03 host app[123]: after rotation error\n"
	if err := os.WriteFile(logFile, []byte(rotatedContent), 0o644); err != nil {
		t.Fatalf("write rotated log: %v", err)
	}
	rotatedCollector := New(cfg, prefs)
	rotatedPayload := collectLogPayload(t, rotatedCollector)
	if got := eventMessages(rotatedPayload.Events); len(got) == 0 || !strings.Contains(strings.Join(got, "\n"), "after rotation error") {
		t.Fatalf("expected rotated event, got %#v", got)
	}
}

func TestCollectorReportsMissingAndPermissionDeniedHealth(t *testing.T) {
	tmp := t.TempDir()
	missingFile := filepath.Join(tmp, "missing.log")
	protectedFile := filepath.Join(tmp, "protected.log")
	if err := os.WriteFile(protectedFile, []byte("Jan  1 00:00:01 host app[123]: hidden error\n"), 0o600); err != nil {
		t.Fatalf("write protected log: %v", err)
	}
	if err := os.Chmod(protectedFile, 0); err != nil {
		t.Fatalf("chmod protected log: %v", err)
	}
	defer func() { _ = os.Chmod(protectedFile, 0o600) }()

	cfg := config.Config{
		OSLogFiles:      []string{missingFile, protectedFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   1024,
		OSLogInterval:   time.Second,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true}
	}

	got := collectLogPayload(t, New(cfg, prefs))
	if len(got.Events) != 0 || len(got.LogSourceHealth) != 2 {
		t.Fatalf("expected only health for inaccessible sources, got %#v", got)
	}
	byPath := healthByPath(got.LogSourceHealth)
	if health := byPath[missingFile]; health.Status != "file_missing" || health.LastError != "file_missing" {
		t.Fatalf("expected file_missing health, got %#v", health)
	}
	if os.Geteuid() == 0 {
		t.Skip("root can still read chmod 000 files")
	}
	if health := byPath[protectedFile]; health.Status != "permission_denied" || health.LastError != "permission_denied" {
		t.Fatalf("expected permission_denied health, got %#v", health)
	}
}

func TestCollectorResetsMisalignedCursor(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "app.log")
	cursorPath := filepath.Join(tmp, "cursor.json")
	content := "Jan  1 00:00:01 host app[123]: first error\nJan  1 00:00:02 host app[123]: second error\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cursor := map[string]int64{logFile: 5}
	if err := saveCursor(cursorPath, cursor); err != nil {
		t.Fatalf("save cursor: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: cursorPath,
		OSLogBatchLines: 10,
		OSLogMaxBytes:   1024,
		OSLogInterval:   time.Second,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true}
	}

	got := collectLogPayload(t, New(cfg, prefs))
	if len(got.Events) != 2 {
		t.Fatalf("expected cursor reset to reread full file, got %#v", got.Events)
	}
	if len(got.LogSourceHealth) != 1 || !hasHealthGap(got.LogSourceHealth[0], "cursor_not_line_boundary") {
		t.Fatalf("expected cursor_not_line_boundary health gap, got %#v", got.LogSourceHealth)
	}
}

func TestCollectorAppliesFiltersWithoutPersistingDroppedContent(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "app.log")
	content := strings.Join([]string{
		"Jan  1 00:00:01 host app[123]: health check ok",
		"Jan  1 00:00:02 host app[123]: failed login password=secret",
	}, "\n") + "\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:        []string{logFile},
		OSLogCursorPath:   filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines:   10,
		OSLogMaxBytes:     1024,
		OSLogInterval:     1 * time.Second,
		OSLogEnrich:       true,
		OSLogExcludeRegex: "health check",
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogEnrich: true}
	}

	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	var payload struct {
		Events       []map[string]any `json:"events"`
		DroppedCount int              `json:"dropped_count"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected 1 event after filter, got %d: %#v", len(payload.Events), payload.Events)
	}
	if payload.DroppedCount != 1 {
		t.Fatalf("expected dropped_count=1, got %d", payload.DroppedCount)
	}
	msg, _ := payload.Events[0]["message"].(string)
	if strings.Contains(msg, "secret") || strings.Contains(msg, "health check") {
		t.Fatalf("unexpected leaked or dropped content: %q", msg)
	}
}

func collectLogPayload(t *testing.T, c ports.Collector) payload {
	t.Helper()
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected payload")
	}
	var got payload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	return got
}

func eventMessages(events []logEvent) []string {
	messages := make([]string, 0, len(events))
	for _, event := range events {
		messages = append(messages, event.Message)
	}
	return messages
}

func healthByPath(rows []logSourceHealth) map[string]logSourceHealth {
	out := make(map[string]logSourceHealth, len(rows))
	for _, row := range rows {
		out[row.Path] = row
	}
	return out
}

func hasHealthGap(row logSourceHealth, gap string) bool {
	for _, current := range row.Gaps {
		if current == gap {
			return true
		}
	}
	return false
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func containsAllArgs(args []string, values ...string) bool {
	for _, value := range values {
		found := false
		for _, arg := range args {
			if arg == value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
