package monitoring

import (
	"errors"
	"time"
)

const DiscoveryEventContract = "monitoring.discovery-event.v1"

type DiscoveryEvent struct {
	Contract    string `json:"contract"`
	ClientID    int64  `json:"client_id"`
	AssetID     int64  `json:"asset_id"`
	Source      string `json:"source"`
	Fingerprint string `json:"fingerprint"`
	State       string `json:"state"`
	Confidence  int    `json:"confidence"`
	ObservedMS  int64  `json:"observed_at_ms"`
	Freshness   int    `json:"freshness_sec"`
}

// NewDiscoveryEvent uses canonical inventory identity parts, which stay local.
func NewDiscoveryEvent(client, asset int64, source string, identity []string, state string, confidence, freshness int, observed time.Time) (DiscoveryEvent, error) {
	if client < 1 || asset < 1 || (state != "observed" && state != "tombstone") ||
		confidence < 0 || confidence > 100 || freshness < 1 || freshness > 86400 || observed.UnixMilli() < 1 {
		return DiscoveryEvent{}, errors.New("invalid discovery event")
	}
	fingerprint, ok := discoveryFingerprint(client, asset, source, identity)
	if !ok {
		return DiscoveryEvent{}, errors.New("invalid discovery identity")
	}
	return DiscoveryEvent{
		Contract: DiscoveryEventContract, ClientID: client, AssetID: asset, Source: source,
		Fingerprint: fingerprint, State: state, Confidence: confidence,
		ObservedMS: observed.UnixMilli(), Freshness: freshness,
	}, nil
}
