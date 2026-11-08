package prefs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFailedReplacementPreservesLastGoodFileAndMemory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("POSIX permission fixture; Windows maps native errors separately")
	}
	root := t.TempDir()
	path := filepath.Join(root, "prefs.json")
	store := NewStore(path)
	good := store.Get()
	good.Version = "good"
	if err := store.Update(good); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0500); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(root, 0700); err != nil {
			t.Error(err)
		}
	}()
	changed := good
	changed.Version = "candidate"
	err = store.Update(changed)
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("expected controlled permission failure, got %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("last good file changed", err)
	}
	if store.Get().Version != "good" {
		t.Fatal("last good memory changed")
	}
}
