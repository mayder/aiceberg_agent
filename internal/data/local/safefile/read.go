package safefile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

var ErrInvalidFile = errors.New("invalid local configuration state file")

// Read resolves the same protected directory chain as Replace, with no creation.
func Read(path string, limit int) ([]byte, error) {
	absolute, err := CanonicalPath(path)
	if err != nil {
		return nil, err
	}
	return readFile(absolute, limit, false)
}

// ReadPrivate requires a regular owner-only file on a supported platform.
func ReadPrivate(path string, limit int) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, ErrInvalidFile
	}
	return readFile(systemPath(filepath.Clean(path)), limit, true)
}

func readBounded(file *os.File, limit int, private bool) ([]byte, error) {
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > int64(limit) || (private && !privateFile(info)) {
		return nil, ErrInvalidFile
	}
	raw, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > limit {
		return nil, ErrInvalidFile
	}
	return raw, nil
}

// CanonicalPath resolves absolute names and fixed OS aliases, never configured links.
func CanonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return systemPath(absolute), nil
}
