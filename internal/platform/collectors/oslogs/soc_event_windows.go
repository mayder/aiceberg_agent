//go:build windows
// +build windows

package oslogs

func eventChannel(ev logEvent) string { return ev.Channel }

func eventProvider(ev logEvent) string { return ev.Provider }

func eventID(ev logEvent) uint64 { return ev.EventID }

func eventPath(ev logEvent) string { return ev.Path }
