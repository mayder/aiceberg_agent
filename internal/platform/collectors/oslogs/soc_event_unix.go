//go:build !windows
// +build !windows

package oslogs

func eventChannel(logEvent) string { return "" }

func eventProvider(logEvent) string { return "" }

func eventID(logEvent) uint64 { return 0 }

func eventPath(ev logEvent) string { return firstNonEmptyString(ev.Path, ev.File) }
