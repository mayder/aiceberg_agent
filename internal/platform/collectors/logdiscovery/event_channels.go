package logdiscovery

import (
	"context"
	"runtime"
	"strings"
	"unicode"

	"github.com/you/aiceberg_agent/internal/common/config"
)

type channelPresence uint8

const (
	channelUnknown channelPresence = iota
	channelRegistered
	channelMissing
)

const sysmonChannel = "Microsoft-Windows-Sysmon/Operational"
const maxDiscoveryEventChannels = 64

func (c *Collector) discoverStructuredChannels(ctx context.Context, prefs config.CollectPrefs, gaps *[]gap) []Candidate {
	if runtime.GOOS != "windows" {
		return nil
	}
	channels := c.cfg.OSLogWinChannels
	if strings.TrimSpace(prefs.Version) != "" {
		channels = prefs.OSLogWinChList
	}
	rows, unavailable := discoverEventChannels(ctx, channels, probeEventChannel)
	if unavailable {
		*gaps = append(*gaps, gap{
			Code: "eventlog_channel_unavailable", Message: "não foi possível verificar todos os canais do Windows Event Log", Scope: "windows_eventlog",
		})
	}
	return rows
}

func discoverEventChannels(ctx context.Context, configured []string, probe func(string) (channelPresence, error)) ([]Candidate, bool) {
	defaults := []string{"Security", "System", "Application", sysmonChannel}
	seen := make(map[string]bool, len(defaults))
	var rows []Candidate
	unavailable := false
	for _, group := range [][]string{defaults, configured} {
		for _, raw := range group {
			channel := strings.TrimSpace(raw)
			key := strings.ToLower(channel)
			if !validEventChannel(channel) || seen[key] {
				continue
			}
			if ctx.Err() != nil || len(seen) >= maxDiscoveryEventChannels {
				return rows, true
			}
			seen[key] = true
			presence, err := probe(channel)
			if err != nil || presence == channelUnknown {
				unavailable = true
			} else if presence == channelRegistered {
				rows = append(rows, eventChannelCandidate(channel))
			}
		}
	}
	return rows, unavailable
}

func validEventChannel(channel string) bool {
	return channel != "" && len(channel) <= 1024 && strings.IndexFunc(channel, unicode.IsControl) < 0
}

func eventChannelCandidate(channel string) Candidate {
	row := baseCandidate("eventlog_channel", "windows_eventlog")
	row.Channel = channel
	row.ServiceName = "windows_eventlog"
	row.Confidence = 0.92
	row.Evidence = []string{"windows_eventlog_channel:" + channel}
	assignCategory(&row, "observability")
	row.SOCEligible = "conditional"
	row.SOCSourceType = "none"
	if strings.EqualFold(channel, "Security") || strings.EqualFold(channel, sysmonChannel) {
		assignCategory(&row, "soc")
		row.SOCEligible = "yes"
		row.SOCSourceType = "windows_security"
	}
	row.Fingerprint = fingerprint(row)
	return row
}
