package monitoring

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoveryEventKeepsStableIdentityAndDoesNotExportLocalID(t *testing.T) {
	at := time.Date(2026, 11, 24, 14, 0, 0, 0, time.UTC)
	event, err := NewDiscoveryEvent(2, 17, "interfaces", []string{"02:ab:00:00:00:01"}, "observed", 100, 300, at)
	if err != nil || event.Contract != DiscoveryEventContract || len(event.Fingerprint) != 64 {
		t.Fatalf("invalid event: %v", err)
	}
	removed, err := NewDiscoveryEvent(2, 17, "interfaces", []string{"02-AB-00-00-00-01"}, "tombstone", 100, 300, at.Add(time.Minute))
	if err != nil || event.Fingerprint != removed.Fingerprint || event.ObservedMS == removed.ObservedMS {
		t.Fatal("the lifecycle update did not preserve identity and change time")
	}
	wire, err := json.Marshal(event)
	if err != nil || bytes.Contains(wire, []byte("02:ab:00:00:00:01")) {
		t.Fatal("the event exposed the local identifier")
	}
	if _, err := NewDiscoveryEvent(2, 17, "interfaces", []string{"02:ab:00:00:00:01"}, "missing", 100, 300, at); err == nil {
		t.Fatal("absence was accepted as a lifecycle state")
	}
}

func TestDiscoveryInventoryIdentityFixtures(t *testing.T) {
	local, err := os.ReadFile("testdata/discovery-identity-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT"); root != "" {
		peer, err := os.ReadFile(filepath.Join(root, "docs/contracts/fixtures/monitoring-discovery-identity-v1.json"))
		if err != nil || !bytes.Equal(local, peer) {
			t.Fatal("Discovery identity fixture drift", err)
		}
	}
	var cases []struct {
		Name        string   `json:"name"`
		ClientID    int64    `json:"client_id"`
		AssetID     int64    `json:"asset_id"`
		Source      string   `json:"source"`
		Identity    []string `json:"identity"`
		Fingerprint *string  `json:"fingerprint"`
	}
	if err := json.Unmarshal(local, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			event, err := NewDiscoveryEvent(c.ClientID, c.AssetID, c.Source, c.Identity, "observed", 100, 300, time.Unix(1, 0))
			if c.Fingerprint == nil {
				if err == nil {
					t.Fatal("invalid component generated an event")
				}
				return
			}
			if err != nil || event.Fingerprint != *c.Fingerprint {
				t.Fatalf("fingerprint mismatch: %s, %v", event.Fingerprint, err)
			}
		})
	}
}
