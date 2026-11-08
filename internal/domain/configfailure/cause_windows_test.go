//go:build windows

package configfailure

import (
	"fmt"
	"golang.org/x/sys/windows"
	"testing"
)

func TestNativeWindowsFilesystemCauses(t *testing.T) {
	for _, item := range []struct {
		err        error
		code, kind string
	}{
		{windows.ERROR_ACCESS_DENIED, "permission_denied", "persistent"},
		{windows.ERROR_DISK_FULL, "disk_full", "persistent"},
		{windows.ERROR_HANDLE_DISK_FULL, "disk_full", "persistent"},
		{windows.ERROR_WRITE_PROTECT, "read_only", "persistent"},
		{windows.ERROR_DIRECTORY, "path_conflict", "persistent"},
		{windows.ERROR_SHARING_VIOLATION, "concurrent_change", "transient"},
		{windows.ERROR_LOCK_VIOLATION, "concurrent_change", "transient"},
	} {
		cause := Classify(fmt.Errorf("fixture: %w", item.err))
		if cause.Code != item.code || cause.Kind != item.kind {
			t.Fatalf("native mapping: got %+v", cause)
		}
	}
}
