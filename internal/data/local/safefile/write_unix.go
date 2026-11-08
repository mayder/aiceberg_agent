//go:build darwin || linux

package safefile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func replaceFile(path string, data []byte) error {
	parent, err := openDirectory(filepath.Dir(path))
	if err != nil {
		return &fs.PathError{Op: "open directory", Path: path, Err: err}
	}
	defer unix.Close(parent)
	return replaceInDirectory(parent, filepath.Base(path), data)
}

func replaceInDirectory(parent int, name string, data []byte) error {
	path := name // Error context contains no ambient directory path.
	mode, err := regularMode(parent, name)
	if err != nil {
		return &fs.PathError{Op: "validate file", Path: path, Err: err}
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	temporary := ".config-state-" + hex.EncodeToString(nonce)
	descriptor, err := unix.Openat(parent, temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return &fs.PathError{Op: "create temporary", Path: path, Err: err}
	}
	defer func() { _ = unix.Unlinkat(parent, temporary, 0) }() // Only this invocation's temporary entry.
	file := os.NewFile(uintptr(descriptor), temporary)
	if err = file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if _, err = regularMode(parent, name); err != nil {
		return &fs.PathError{Op: "validate replacement", Path: path, Err: err}
	}
	if err = unix.Renameat(parent, temporary, parent, name); err != nil {
		return &fs.PathError{Op: "replace", Path: path, Err: err}
	}
	return unix.Fsync(parent)
}

// All components are resolved relative to pinned descriptors with O_NOFOLLOW.
// A concurrent rename cannot redirect subsequent operations through a new symlink.
func openDirectory(path string) (int, error) { return openDirectoryMode(path, true) }

func openDirectoryMode(path string, create bool) (int, error) {
	current, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, component := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if component == "" {
			continue
		}
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if create && errors.Is(openErr, unix.ENOENT) {
			if makeErr := unix.Mkdirat(current, component, 0700); makeErr != nil && !errors.Is(makeErr, unix.EEXIST) {
				openErr = makeErr
			} else {
				next, openErr = unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			}
		}
		_ = unix.Close(current)
		if openErr != nil {
			return -1, openErr
		}
		current = next
	}
	return current, nil
}

func regularMode(parent int, name string) (fs.FileMode, error) {
	var info unix.Stat_t
	if err := unix.Fstatat(parent, name, &info, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return 0600, nil
		}
		return 0, err
	}
	if info.Mode&unix.S_IFMT != unix.S_IFREG {
		return 0, unix.EEXIST
	}
	return fs.FileMode(info.Mode & 0777), nil
}
