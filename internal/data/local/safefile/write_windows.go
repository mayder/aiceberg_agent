//go:build windows

package safefile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// Holding every directory without FILE_SHARE_DELETE prevents a concurrent rename
// or replacement of a validated ancestor while path-based Windows calls execute.
func replaceFile(path string, data []byte) error {
	pins, err := pinDirectories(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() {
		for _, handle := range pins {
			_ = windows.CloseHandle(handle)
		}
	}()
	if err = regularOrAbsent(path); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-state-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }() // Only this invocation's temporary file.
	if previous, err := os.Lstat(path); err == nil && previous.Mode().IsRegular() {
		if err = file.Chmod(previous.Mode().Perm()); err != nil {
			file.Close()
			return err
		}
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
	if err = regularOrAbsent(path); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func pinDirectories(path string) ([]windows.Handle, error) { return pinDirectoriesMode(path, true) }

func pinDirectoriesMode(path string, create bool) ([]windows.Handle, error) {
	volume := filepath.VolumeName(path)
	current := volume + string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(path, current), string(filepath.Separator))
	paths := []string{current}
	for _, part := range parts {
		if part != "" {
			current = filepath.Join(current, part)
			paths = append(paths, current)
		}
	}
	var pins []windows.Handle
	for _, directory := range paths {
		handle, err := pinDirectory(directory)
		if create && errors.Is(err, fs.ErrNotExist) {
			// The already pinned parent cannot be swapped out while its child is created.
			if makeErr := os.Mkdir(directory, 0700); makeErr != nil && !errors.Is(makeErr, fs.ErrExist) {
				err = makeErr
			} else {
				handle, err = pinDirectory(directory)
			}
		}
		if err != nil {
			for _, pin := range pins {
				_ = windows.CloseHandle(pin)
			}
			return nil, err
		}
		pins = append(pins, handle)
	}
	return pins, nil
}

func pinDirectory(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return windows.InvalidHandle, &fs.PathError{Op: "open directory", Path: path, Err: err}
	}
	var info windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(handle, &info)
	if err == nil && (info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) {
		err = syscall.EEXIST
	}
	if err != nil {
		_ = windows.CloseHandle(handle)
		return windows.InvalidHandle, &fs.PathError{Op: "validate directory", Path: path, Err: err}
	}
	return handle, nil
}

func regularOrAbsent(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return &fs.PathError{Op: "replace", Path: path, Err: syscall.EEXIST}
	}
	return nil
}
