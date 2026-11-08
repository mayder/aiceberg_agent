//go:build windows

package configfailure

import (
	"errors"
	"golang.org/x/sys/windows"
)

func platformCause(err error) (Cause, bool) {
	switch {
	case errors.Is(err, windows.ERROR_DISK_FULL), errors.Is(err, windows.ERROR_HANDLE_DISK_FULL):
		return Cause{"disk_full", "persistent"}, true
	case errors.Is(err, windows.ERROR_WRITE_PROTECT):
		return Cause{"read_only", "persistent"}, true
	case errors.Is(err, windows.ERROR_SHARING_VIOLATION), errors.Is(err, windows.ERROR_LOCK_VIOLATION):
		return Cause{"concurrent_change", "transient"}, true
	case errors.Is(err, windows.ERROR_DIRECTORY):
		return Cause{"path_conflict", "persistent"}, true
	default:
		return Cause{}, false
	}
}
