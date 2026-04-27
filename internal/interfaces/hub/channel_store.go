package hub

import (
	"sync"
	"time"
)

type RelayChannelSession struct {
	AuthHash    string
	SessionID   string
	Mode        string
	Hostname    string
	Version     string
	LastAction  string
	ConnectedAt time.Time
	LastSeenAt  time.Time
	ClosedAt    time.Time
}

type RelayChannelStore struct {
	mu       sync.Mutex
	sessions map[string]RelayChannelSession
}

func NewRelayChannelStore() *RelayChannelStore {
	return &RelayChannelStore{
		sessions: make(map[string]RelayChannelSession),
	}
}

func (s *RelayChannelStore) Record(authHash string, event RelayChannelSession) RelayChannelSession {
	now := time.Now().UTC()
	key := event.SessionID
	if key == "" {
		key = authHash
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.sessions[key]
	if current.ConnectedAt.IsZero() {
		current.ConnectedAt = now
	}
	current.AuthHash = authHash
	current.SessionID = event.SessionID
	current.Mode = event.Mode
	current.Hostname = event.Hostname
	current.Version = event.Version
	current.LastAction = event.LastAction
	current.LastSeenAt = now
	if event.LastAction == "close" {
		current.ClosedAt = now
	}
	s.sessions[key] = current
	return current
}

func (s *RelayChannelStore) Snapshot() []RelayChannelSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make([]RelayChannelSession, 0, len(s.sessions))
	for _, row := range s.sessions {
		rows = append(rows, row)
	}
	return rows
}
