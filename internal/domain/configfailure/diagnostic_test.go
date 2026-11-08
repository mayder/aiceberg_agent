package configfailure

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"syscall"
	"testing"
)

func TestTypedDiagnosticsDoNotInferCauseFromText(t *testing.T) {
	cases := []struct {
		err        error
		code, kind string
	}{
		{fs.ErrPermission, "permission_denied", "persistent"},
		{fs.ErrExist, "path_conflict", "persistent"},
		{syscall.ENOTDIR, "path_conflict", "persistent"},
		{syscall.ENOSPC, "disk_full", "persistent"},
		{syscall.EROFS, "read_only", "persistent"},
		{syscall.EBUSY, "concurrent_change", "transient"},
		{syscall.EAGAIN, "concurrent_change", "transient"},
		{syscall.ETIMEDOUT, "io_temporary", "transient"},
		{errors.New("permission denied: /private/customer/secret"), "unknown", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.code+tc.err.Error(), func(t *testing.T) {
			got := Classify(fmt.Errorf("apply: %w", &fs.PathError{Op: "write", Path: "/private/customer/secret", Err: tc.err}))
			if got.Code != tc.code || got.Kind != tc.kind {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestFingerprintIsStableScopedAndUnambiguous(t *testing.T) {
	base := Fingerprint("agent-A", "apply", "config-1", "disk_full")
	if len(base) != 64 || base != Fingerprint("agent-A", "apply", "config-1", "disk_full") {
		t.Fatal("unstable digest")
	}
	for _, changed := range []string{
		Fingerprint("agent-B", "apply", "config-1", "disk_full"),
		Fingerprint("agent-A", "apply", "config-2", "disk_full"),
		Fingerprint("agent-A", "apply", "config-1", "permission_denied"),
		Fingerprint("agent-A", "report", "config-1", "disk_full"),
	} {
		if changed == base {
			t.Fatal("scope collision")
		}
	}
	if Fingerprint("a", "bc", "d", "e") == Fingerprint("ab", "c", "d", "e") {
		t.Fatal("ambiguous encoding")
	}
	if strings.Contains(base, "agent") {
		t.Fatal("raw identity leaked")
	}
}

func TestReportRetryIDRemainsStableAndChangesWithMaterialReport(t *testing.T) {
	r := Report{Fingerprint: Fingerprint("a", "apply", "c", "unknown"), Phase: "first", LastSeen: 1000, Count: 1}
	same := r
	if same.ID() != r.ID() {
		t.Fatal("retry ID changed")
	}
	same.Phase = "recovered"
	if same.ID() == r.ID() {
		t.Fatal("recovery ID reused")
	}
}
