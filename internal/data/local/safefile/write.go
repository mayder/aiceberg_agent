// Package safefile writes local configuration state without truncating the last good file.
package safefile

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Writer serializes writes owned by one store. It never changes existing permissions
// or removes a conflicting path. Callers retain their in-memory state on errors.
type Writer struct{ mu sync.Mutex }

func (w *Writer) Replace(path string, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absolute = systemPath(absolute)
	return replaceFile(absolute, data)
}

// Darwin exposes these fixed system aliases in temporary/configured paths.
// Resolve only the OS aliases; configured directory symlinks remain rejected.
func systemPath(path string) string {
	if runtime.GOOS == "darwin" {
		for _, prefix := range []string{"/var", "/tmp"} {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return "/private" + path
			}
		}
	}
	return path
}
