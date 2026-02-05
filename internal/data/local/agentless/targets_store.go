package agentless

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type TargetsStore struct {
	path string
}

func NewTargetsStore(path string) *TargetsStore {
	return &TargetsStore{path: path}
}

func (s *TargetsStore) Load() ([]entities.AgentlessJob, error) {
	if s == nil || s.path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil, nil
	}
	var jobs []entities.AgentlessJob
	if err := json.Unmarshal(b, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (s *TargetsStore) Save(jobs []entities.AgentlessJob) error {
	if s == nil || s.path == "" {
		return nil
	}
	raw, _ := json.Marshal(jobs)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o600)
}

func (s *TargetsStore) Clear() error {
	if s == nil || s.path == "" {
		return nil
	}
	_ = os.Remove(s.path)
	return nil
}
