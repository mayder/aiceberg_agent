//go:build darwin || linux

package safefile

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedDirectoryCannotBeRedirectedByAncestorSymlink(t *testing.T) {
	root := systemPath(t.TempDir())
	original := filepath.Join(root, "original")
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "prefs.json"), []byte("outside preserved"), 0600); err != nil {
		t.Fatal(err)
	}
	descriptor, err := openDirectory(original)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(descriptor)
	moved := filepath.Join(root, "moved")
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, original); err != nil {
		t.Fatal(err)
	}
	if err := replaceInDirectory(descriptor, "prefs.json", []byte("new config")); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(moved, "prefs.json"))
	if string(raw) != "new config" {
		t.Fatal("pinned destination lost")
	}
	raw, _ = os.ReadFile(filepath.Join(outside, "prefs.json"))
	if string(raw) != "outside preserved" {
		t.Fatal("symlink redirected write")
	}
	writer := &Writer{}
	if err := writer.Replace(filepath.Join(original, "missing", "prefs.json"), []byte("bad")); err == nil {
		t.Fatal("unexpected symlink accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "missing")); !os.IsNotExist(err) {
		t.Fatal("created directory through untrusted ancestor")
	}
}
