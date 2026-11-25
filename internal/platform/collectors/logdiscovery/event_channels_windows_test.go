package logdiscovery

import "testing"

func TestNativeEventLogChannelProbe(t *testing.T) {
	for _, tc := range []struct {
		channel string
		want    channelPresence
	}{
		{"System", channelRegistered},
		{"AIceberg-Discovery-Test-8FBA387A93264BD69F75F02F06F41498/Operational", channelMissing},
	} {
		if got, err := probeEventChannel(tc.channel); err != nil || got != tc.want {
			t.Fatalf("probe %q returned %v, %v, want %v", tc.channel, got, err, tc.want)
		}
	}
}
