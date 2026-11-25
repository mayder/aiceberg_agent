//go:build !windows

package logdiscovery

func probeEventChannel(string) (channelPresence, error) {
	return channelUnknown, nil
}
