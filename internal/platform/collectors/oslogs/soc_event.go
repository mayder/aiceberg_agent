package oslogs

import (
	"strings"

	"github.com/you/aiceberg_agent/internal/common/soclog"
)

func enrichSOCEvent(ev logEvent) logEvent {
	contract := soclog.Build(soclog.Hints{
		Transport:      ev.Transport,
		SourceTool:     ev.SourceTool,
		SourceCategory: ev.SourceCategory,
		Path:           eventPath(ev),
		Channel:        eventChannel(ev),
		Provider:       eventProvider(ev),
		EventID:        eventID(ev),
		Level:          firstNonEmptyString(ev.Level, ev.Severity),
		Message:        ev.Message,
		Attributes:     ev.Attributes,
	})
	return applySOCContract(ev, contract)
}

func applySOCContract(ev logEvent, contract soclog.Contract) logEvent {
	ev.AicebergTransport = contract.Transport
	ev.AicebergToolOrigin = contract.ToolOrigin
	ev.AicebergSourceCategory = contract.SourceCategory
	ev.AicebergSOCSourceType = contract.SOCSourceType
	ev.AicebergSOCEligible = contract.SOCEligible
	ev.AicebergOriginConfidence = contract.OriginConfidence
	ev.AicebergRouteReason = contract.RouteReason
	for key, value := range contract.Promoted {
		switch key {
		case "event_code":
			ev.EventCode = value
		case "vendor":
			ev.Vendor = value
		case "product":
			ev.Product = value
		case "src_ip":
			ev.SrcIP = value
		case "dst_ip":
			ev.DstIP = value
		case "src_host":
			ev.SrcHost = value
		case "dst_host":
			ev.DstHost = value
		case "username":
			ev.Username = value
		case "process_name":
			ev.ProcessName = value
		case "command_line":
			ev.CommandLine = value
		case "file_hash":
			ev.FileHash = value
		case "domain":
			ev.Domain = value
		case "url":
			ev.URL = value
		case "action":
			ev.Action = value
		case "rule_name":
			ev.RuleName = value
		case "technique_id":
			ev.TechniqueID = value
		case "alert_id":
			ev.AlertID = value
		}
	}
	return ev
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
