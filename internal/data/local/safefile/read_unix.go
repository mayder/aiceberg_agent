//go:build darwin || linux

package safefile

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func readFile(path string, limit int) ([]byte, error) {
	parent, err := openDirectoryMode(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer unix.Close(parent)
	descriptor, err := unix.Openat(parent, filepath.Base(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	return readBounded(os.NewFile(uintptr(descriptor), filepath.Base(path)), limit)
}
