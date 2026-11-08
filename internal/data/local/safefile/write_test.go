package safefile

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestReplacementPreservesConflictAndDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	writer := &Writer{}
	if err := writer.Replace(path, []byte("good")); err != nil {
		t.Fatal(err)
	}
	conflict := filepath.Join(dir, "conflict")
	if err := os.WriteFile(conflict, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writer.Replace(filepath.Join(conflict, "prefs.json"), []byte("bad")); err == nil {
		t.Fatal("file treated as directory")
	}
	raw, _ := os.ReadFile(conflict)
	if string(raw) != "preserve" {
		t.Fatal("conflict overwritten")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Skip("symlink creation unavailable")
	}
	if err := writer.Replace(link, []byte("bad")); err == nil {
		t.Fatal("symlink followed")
	}
	raw, _ = os.ReadFile(path)
	if string(raw) != "good" {
		t.Fatal("target overwritten")
	}
	dirLink := filepath.Join(dir, "dir-link")
	if err := os.Symlink(dir, dirLink); err != nil {
		t.Fatal(err)
	}
	if err := writer.Replace(filepath.Join(dirLink, "prefs.json"), []byte("bad")); err == nil {
		t.Fatal("directory symlink followed")
	}
}

func TestConcurrentCreationAndWritesRemainComplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "prefs.json")
	writer := &Writer{}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := writer.Replace(path, []byte("complete")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "complete" {
		t.Fatal("partial write", err)
	}
	items, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(items) != 1 {
		t.Fatal("temporary file leaked", err)
	}
}
