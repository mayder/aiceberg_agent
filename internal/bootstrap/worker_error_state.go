package app

import (
	"sync"
	"time"
)

type workerErrorTransitionState struct {
	confirmedRecovery     string
	confirmedAt           time.Time
	generation            uint64
	lastAttemptRecovery   string
	lastAttemptAt         time.Time
	lastAttemptGeneration uint64
	inFlight              bool
}

type workerErrorStateMachine struct {
	mu               sync.Mutex
	states           map[string]workerErrorTransitionState
	now              func() time.Time
	openCooldown     time.Duration
	recoveryCooldown time.Duration
}

func newWorkerErrorStateMachine(now func() time.Time) *workerErrorStateMachine {
	if now == nil {
		now = time.Now
	}
	return &workerErrorStateMachine{states: make(map[string]workerErrorTransitionState), now: now, openCooldown: 30 * time.Minute, recoveryCooldown: 5 * time.Minute}
}

func (m *workerErrorStateMachine) report(fingerprint, recovery string, send func(time.Time) error) (bool, error) {
	now := m.now()
	if !m.begin(fingerprint, recovery, now) {
		return false, nil
	}
	err := send(now)
	m.complete(fingerprint, recovery, m.now(), err == nil)
	return true, err
}

func (m *workerErrorStateMachine) begin(fingerprint, recovery string, now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.states[fingerprint]
	if state.inFlight {
		return false
	}
	if recovery == "recovered" && state.confirmedRecovery != "open" && state.confirmedRecovery != "monitoring" {
		return false
	}
	cooldown := m.openCooldown
	if recovery == "recovered" {
		cooldown = m.recoveryCooldown
	}
	if state.lastAttemptRecovery == recovery && state.lastAttemptGeneration == state.generation && !state.lastAttemptAt.IsZero() && now.Sub(state.lastAttemptAt) < cooldown {
		return false
	}
	if state.confirmedRecovery == recovery && !state.confirmedAt.IsZero() && now.Sub(state.confirmedAt) < cooldown {
		return false
	}
	state.inFlight = true
	state.lastAttemptRecovery = recovery
	state.lastAttemptAt = now
	state.lastAttemptGeneration = state.generation
	m.states[fingerprint] = state
	return true
}

func (m *workerErrorStateMachine) complete(fingerprint, recovery string, now time.Time, delivered bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.states[fingerprint]
	state.inFlight = false
	if delivered {
		if state.confirmedRecovery != recovery {
			state.generation++
		}
		state.confirmedRecovery = recovery
		state.confirmedAt = now
		state.lastAttemptGeneration = state.generation
	}
	m.states[fingerprint] = state
}
