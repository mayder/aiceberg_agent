//go:build windows
// +build windows

package oslogs

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestParseEventBlock_NormalizesLocalizedWindowsLevel(t *testing.T) {
	block := "Event[0]\n" +
		"  Log Name: Application\n" +
		"  Source: AIcebergAgent\n" +
		"  Date: 2026-06-19T13:17:48.3150000Z\n" +
		"  Event ID: 6902\n" +
		"  Level: Erro\x00\n" +
		"  Computer: govi\n" +
		"  Description:\n" +
		"controlled error\n"

	ev := parseEventBlock(block, "Application", "govi", 4096)

	if ev.Level != "error" {
		t.Fatalf("expected localized level to normalize to error, got %q", ev.Level)
	}
	if ev.Severity != "error" {
		t.Fatalf("expected severity to normalize to error, got %q", ev.Severity)
	}
}

func TestParseEventXMLBlock_ExtractsRecordAndWindowsSeverity(t *testing.T) {
	block := []byte(`<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='AIcebergAgent'/><EventID Qualifiers='0'>6906</EventID><Level>2</Level><TimeCreated SystemTime='2026-06-19T16:30:02.1653349Z'/><EventRecordID>8063</EventRecordID><Channel>Application</Channel><Computer>govi</Computer></System><EventData><Data>controlled error</Data></EventData></Event>`)

	ev := parseEventXMLBlock(block, "Application", "govi", 4096)

	if ev.RecordID != 8063 {
		t.Fatalf("expected record id 8063, got %d", ev.RecordID)
	}
	if ev.Level != "error" || ev.Severity != "error" {
		t.Fatalf("expected error severity, got level=%q severity=%q", ev.Level, ev.Severity)
	}
	if ev.Message != "controlled error" {
		t.Fatalf("unexpected message %q", ev.Message)
	}
	if ev.AicebergSourceCategory != "observability" || ev.AicebergSOCEligible != "no" {
		t.Fatalf("expected operational Windows Application contract, got %#v", ev)
	}
}

func TestParseEventXMLBlock_ClassifiesSecurityAndSysmonChannels(t *testing.T) {
	securityBlock := []byte(`<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Microsoft-Windows-Security-Auditing'/><EventID>4625</EventID><Level>0</Level><TimeCreated SystemTime='2026-06-19T16:30:02Z'/><EventRecordID>100</EventRecordID><Channel>Security</Channel><Computer>dc01</Computer></System><EventData><Data>failed logon</Data></EventData></Event>`)
	sysmonBlock := []byte(`<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Microsoft-Windows-Sysmon'/><EventID>1</EventID><Level>4</Level><TimeCreated SystemTime='2026-06-19T16:31:02Z'/><EventRecordID>101</EventRecordID><Channel>Microsoft-Windows-Sysmon/Operational</Channel><Computer>srv01</Computer></System><EventData><Data>process created</Data></EventData></Event>`)

	security := parseEventXMLBlock(securityBlock, "Security", "fallback", 4096)
	sysmon := parseEventXMLBlock(sysmonBlock, "Microsoft-Windows-Sysmon/Operational", "fallback", 4096)

	if security.SourceCategory != "security" || security.Provider != "Microsoft-Windows-Security-Auditing" || security.EventID != 4625 {
		t.Fatalf("unexpected security event: %#v", security)
	}
	if security.AicebergToolOrigin != "ad_security" || security.AicebergSourceCategory != "soc" || security.AicebergSOCEligible != "yes" {
		t.Fatalf("unexpected security SOC contract: %#v", security)
	}
	if sysmon.SourceCategory != "security" || sysmon.Provider != "Microsoft-Windows-Sysmon" || sysmon.Channel != "Microsoft-Windows-Sysmon/Operational" {
		t.Fatalf("unexpected sysmon event: %#v", sysmon)
	}
	if sysmon.AicebergSourceCategory != "soc" || sysmon.AicebergSOCEligible != "yes" || sysmon.AicebergRouteReason != "sysmon_security_telemetry" {
		t.Fatalf("unexpected sysmon SOC contract: %#v", sysmon)
	}
}

func TestParseEventXMLBlock_DoesNotPromoteDistributedCOMToSOC(t *testing.T) {
	block := []byte(`<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Microsoft-Windows-DistributedCOM'/><EventID>10028</EventID><Level>2</Level><TimeCreated SystemTime='2026-06-19T16:30:02Z'/><EventRecordID>200</EventRecordID><Channel>System</Channel><Computer>srv01</Computer></System><EventData><Data>DistributedCOM remote activation failed</Data></EventData></Event>`)

	ev := parseEventXMLBlock(block, "System", "srv01", 4096)

	if ev.AicebergSourceCategory != "observability" || ev.AicebergSOCEligible != "no" || ev.AicebergRouteReason != "windows_distributedcom_operational" {
		t.Fatalf("DistributedCOM must remain operational, got %#v", ev)
	}
}

func TestWindowsEventQuery_AddsSeverityPredicate(t *testing.T) {
	query := windowsEventQuery(42, nil, nil, "error")

	if want := "EventRecordID>42"; !strings.Contains(query, want) {
		t.Fatalf("expected query to contain %q, got %q", want, query)
	}
	if want := "Level<=2"; !strings.Contains(query, want) {
		t.Fatalf("expected query to contain %q, got %q", want, query)
	}
}

func TestManualWindowsCollect(t *testing.T) {
	if os.Getenv("AICEBERG_OSLOG_MANUAL") != "1" {
		t.Skip("manual Windows collector diagnostic")
	}
	cfg, err := config.Load(os.Getenv("AICEBERG_OSLOG_CONFIG"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	prefs := config.CollectPrefs{
		Logs:             true,
		OSLogWinChannels: true,
		OSLogWinChList:   cfg.OSLogWinChannels,
		OSLogMinSeverity: cfg.OSLogMinSeverity,
		OSLogBatchLines:  cfg.OSLogBatchLines,
		OSLogMaxBytes:    cfg.OSLogMaxBytes,
		OSLogDiag:        true,
	}
	collector := New(cfg, func() config.CollectPrefs { return prefs })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	data, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected collected payload")
	}
	t.Logf("payload=%s", string(data))
}
