package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/you/aiceberg_agent/internal/common/config"
)

type Store struct {
	path string
	cur  atomic.Value
}

func NewStore(path string) *Store {
	s := &Store{path: path}
	s.cur.Store(Default())
	return s
}

func Default() config.CollectPrefs {
	return config.CollectPrefs{
		CPU:       true,
		Memory:    true,
		Disk:      true,
		Network:   true,
		NetActive: true,
		Host:      true,
		Sensors:   true,
		Power:     true,
		Sanity:    true,
		GPU:       true,
		Services:  true,
		TimeSync:  true,
		Logs:      true,
		Updates:   true,
		Agent:     true,
		Processes: true,
	}
}

func (s *Store) Load() (config.CollectPrefs, error) {
	if b, err := os.ReadFile(s.path); err == nil {
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
	raw, _ := json.MarshalIndent(p, "", "  ")
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.path, raw, 0o600); err != nil {
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
