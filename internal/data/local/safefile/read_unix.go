//go:build darwin || linux

package safefile

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"syscall"
)

func readFile(path string, limit int, private bool) ([]byte, error) {
	parent, err := openDirectoryMode(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer unix.Close(parent)
	descriptor, err := unix.Openat(parent, filepath.Base(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	return readBounded(os.NewFile(uintptr(descriptor), filepath.Base(path)), limit, private)
}

func privateFile(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) && info.Mode().Perm()&0o077 == 0
}
