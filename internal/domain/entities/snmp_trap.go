package entities

// SNMPTrapReplayProof is local-only metadata for an authenticated SNMPv3 notification.
type SNMPTrapReplayProof struct {
	Engine     [32]byte
	Boots      uint32
	EngineTime uint32
	Digest     [32]byte
	ReceivedAt int64
}
