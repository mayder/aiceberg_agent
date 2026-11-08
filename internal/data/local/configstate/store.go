// Package configstate persists bounded diagnostics independently of collection preferences.
package configstate

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"runtime"
	"strings"

	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/domain/configfailure"
)

const MaxBytes = 128 * 1024

type Store struct {
	path    string
	writer  safefile.Writer
	invalid bool
}

func New(path string, protected ...string) *Store {
	store := &Store{path: path}
	candidate, err := safefile.CanonicalPath(path)
	if err != nil {
		store.invalid = true
		return store
	}
	for _, other := range protected {
		if other == "" {
			continue
		}
		target, err := safefile.CanonicalPath(other)
		if err != nil {
			continue
		}
		same := candidate == target
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			same = same || strings.EqualFold(candidate, target)
		}
		left, leftErr := os.Lstat(candidate)
		right, rightErr := os.Lstat(target)
		if same || (leftErr == nil && rightErr == nil && os.SameFile(left, right)) {
			store.invalid = true
		}
	}
	return store
}

func (s *Store) Load() (configfailure.State, error) {
	state := configfailure.NewState()
	if s.invalid {
		return state, errors.New("diagnostic state path is protected or invalid")
	}
	raw, err := safefile.Read(s.path, MaxBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		if errors.Is(err, safefile.ErrInvalidFile) {
			s.invalid = true
		}
		return state, err
	}
	err = json.Unmarshal(raw, &state)
	if err == nil {
		err = state.Validate()
	}
	if err != nil {
		s.invalid = true
		return configfailure.NewState(), errors.New("invalid diagnostic state contents")
	}
	return state, nil
}

func (s *Store) Save(state configfailure.State) error {
	if s.invalid {
		return errors.New("preserving invalid diagnostic state")
	}
	if err := state.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(raw) > MaxBytes {
		return errors.New("diagnostic state capacity exceeded")
	}
	return s.writer.Replace(s.path, raw)
}
