package agentless

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	bolt "go.etcd.io/bbolt"
)

const (
	snmpTrapTimeWindow  = 150
	maxReplayEngines    = 1024
	maxReplayIdentities = 256
)

var snmpTrapReplayBucket = []byte("snmp_trap_replay_v1")

type trapReplayState struct {
	Boots         uint32           `json:"boots"`
	EngineTime    uint32           `json:"engine_time"`
	ReceivedAt    int64            `json:"received_at"`
	MinEngineTime int64            `json:"min_engine_time"`
	Recent        []trapReplayMark `json:"recent"`
}

type trapReplayMark struct {
	At     uint32 `json:"t"`
	Digest string `json:"d"`
}

// PushSNMPTrap commits the USM time/replay state and the outbox record together.
// A suppressed replay is a successful no-op.
func (b *BoltStore) PushSNMPTrap(event entities.AgentlessObservation, proof *entities.SNMPTrapReplayProof) error {
	return b.push(event, proof)
}

func admitSNMPTrap(tx *bolt.Tx, proof entities.SNMPTrapReplayProof) (bool, error) {
	if proof.ReceivedAt <= 0 || proof.Boots >= math.MaxInt32 || proof.EngineTime > math.MaxInt32 {
		return false, nil
	}
	bucket, err := tx.CreateBucketIfNotExists(snmpTrapReplayBucket)
	if err != nil {
		return false, err
	}
	state, err := readSNMPTrapReplayState(bucket, proof.Engine[:])
	if err != nil {
		return false, err
	}
	if !state.advance(proof) {
		return false, nil
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return false, err
	}
	return true, bucket.Put(proof.Engine[:], raw)
}

func readSNMPTrapReplayState(bucket *bolt.Bucket, key []byte) (trapReplayState, error) {
	var state trapReplayState
	if raw := bucket.Get(key); raw != nil {
		if json.Unmarshal(raw, &state) != nil || state.ReceivedAt <= 0 || len(state.Recent) > maxReplayIdentities {
			return trapReplayState{}, errors.New("invalid SNMP trap replay state")
		}
	} else if bucket.Stats().KeyN >= maxReplayEngines {
		return trapReplayState{}, errors.New("SNMP trap replay state capacity reached")
	}
	return state, nil
}

func (s *trapReplayState) advance(proof entities.SNMPTrapReplayProof) bool {
	if s.ReceivedAt == 0 || proof.Boots > s.Boots {
		*s = trapReplayState{Boots: proof.Boots, EngineTime: proof.EngineTime, ReceivedAt: proof.ReceivedAt}
	}
	if proof.Boots < s.Boots {
		return false
	}
	elapsed := max(0, proof.ReceivedAt-s.ReceivedAt)
	earliest := max(s.MinEngineTime, int64(s.EngineTime)+elapsed-snmpTrapTimeWindow)
	if int64(proof.EngineTime) < earliest {
		return false
	}
	if proof.EngineTime > s.EngineTime {
		s.EngineTime = proof.EngineTime
		s.ReceivedAt = max(s.ReceivedAt, proof.ReceivedAt)
		earliest = max(earliest, int64(proof.EngineTime)-snmpTrapTimeWindow)
	}
	s.MinEngineTime = earliest
	s.Recent = slices.DeleteFunc(s.Recent, func(mark trapReplayMark) bool { return int64(mark.At) < earliest })
	digest := hex.EncodeToString(proof.Digest[:])
	if len(s.Recent) >= maxReplayIdentities || slices.ContainsFunc(s.Recent, func(mark trapReplayMark) bool { return mark.Digest == digest }) {
		return false
	}
	s.Recent = append(s.Recent, trapReplayMark{At: proof.EngineTime, Digest: digest})
	return true
}
