package hub

import (
	"encoding/json"
	"sync"
)

type PendingConfigStore struct {
	mu     sync.Mutex
	byAuth map[string]json.RawMessage
}

func NewPendingConfigStore() *PendingConfigStore {
	return &PendingConfigStore{
		byAuth: make(map[string]json.RawMessage),
	}
}

func (s *PendingConfigStore) Set(auth string, cfg json.RawMessage) {
	if auth == "" || len(cfg) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byAuth[auth] = cfg
}

func (s *PendingConfigStore) Pop(auth string) (json.RawMessage, bool) {
	if auth == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, ok := s.byAuth[auth]
	if !ok {
		return nil, false
	}
	delete(s.byAuth, auth)
	return cfg, true
}
