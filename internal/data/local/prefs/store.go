package prefs

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/safefile"
)

type Store struct {
	mu     sync.Mutex
	path   string
	cur    atomic.Value
	writer safefile.Writer
}

func NewStore(path string) *Store {
	s := &Store{path: path}
	s.cur.Store(Default())
	return s
}

func Default() config.CollectPrefs {
	return config.CollectPrefs{
		Paused:                       false,
		CPU:                          true,
		Memory:                       true,
		Disk:                         true,
		Network:                      true,
		NetActive:                    true,
		Host:                         true,
		Sensors:                      true,
		Power:                        true,
		Sanity:                       true,
		GPU:                          true,
		Services:                     true,
		TimeSync:                     true,
		Logs:                         true,
		Updates:                      true,
		Agent:                        true,
		Processes:                    true,
		Vulns:                        true,
		Inventory:                    false,
		OSLogEnrich:                  true,
		OSLogDetections:              true,
		OSLogDiag:                    false,
		OSLogWinChannels:             true,
		OSLogFiles:                   true,
		LogDiscoveryEnabled:          true,
		LogDiscoveryIntervalSec:      300,
		LogDiscoveryMaxCandidates:    200,
		LogDiscoveryMaxEvidenceBytes: 2048,
		AgentlessEnabled:             false,
		NetworkPassiveMode:           "auto",
		NetworkPCAPEnabled:           false,
		NetworkExternalMaxRecords:    1000,
	}
}

func (s *Store) Load() (config.CollectPrefs, error) {
	if b, err := safefile.Read(s.path, 2<<20); err == nil {
		var p config.CollectPrefs
		if err := json.Unmarshal(b, &p); err == nil {
			s.cur.Store(p)
			return p, nil
		}
	}
	def := Default()
	s.cur.Store(def)
	return def, nil
}

func (s *Store) Update(p config.CollectPrefs) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := s.writer.Replace(s.path, raw); err != nil {
		return err
	}
	s.cur.Store(p)
	return nil
}

func (s *Store) Get() config.CollectPrefs {
	v := s.cur.Load()
	if v == nil {
		return Default()
	}
	return v.(config.CollectPrefs)
}

// Path is the configured preferences path; callers must not log its raw value.
func (s *Store) Path() string { return s.path }
