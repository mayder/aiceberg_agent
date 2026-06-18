//go:build windows
// +build windows

package oslogs

import "strings"

func sourceCategoryForWindows(channel string) string {
	ch := strings.ToLower(channel)
	switch {
	case strings.Contains(ch, "security") || strings.Contains(ch, "sysmon"):
		return "security"
	case strings.Contains(ch, "system") || strings.Contains(ch, "application"):
		return "observability"
	default:
		return "log"
	}
}
