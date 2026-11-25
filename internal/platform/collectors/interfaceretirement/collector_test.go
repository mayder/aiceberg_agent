package interfaceretirement

import (
	"net"
	"testing"
	"time"
)

func TestInterfaceRetirementRequiresExplicitUnambiguousRemoval(t *testing.T) {
	now := time.Unix(1800000000, 0)
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}
	removals := []removal{{mac: mac, observed: now.Add(-time.Second)}, {mac: mac, observed: now}}
	events := removedInterfaces(3, 17, removals, nil, now)
	if len(events) != 1 || events[0].ClientID != 3 || events[0].AssetID != 17 || events[0].State != "tombstone" || len(events[0].Fingerprint) != 64 {
		t.Fatalf("wrong event: %+v", events)
	}
	if got := removedInterfaces(3, 17, removals, []net.Interface{{Name: "renamed", HardwareAddr: mac}}, now); len(got) != 0 {
		t.Fatal("a present interface must not retire")
	}
	if got := removedInterfaces(3, 17, nil, nil, now); len(got) != 0 {
		t.Fatal("an empty inventory must not generate a retirement")
	}
	if got := removedInterfaces(3, 17, removals, nil, now.Add(5*time.Minute)); len(got) != 0 {
		t.Fatal("expired removal must not be queued")
	}
}
