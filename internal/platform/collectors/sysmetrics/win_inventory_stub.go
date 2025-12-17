//go:build !windows

package sysmetrics

import "context"

func collectWindowsHotfixes(ctx context.Context) []winHotfix { return nil }
func collectWindowsApps(ctx context.Context) []winApp        { return nil }
func collectWindowsFeatures(ctx context.Context) []winFeature {
	return nil
}
