package safefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBoundedReadRejectsLinksAndDoesNotCreateMissingDirectories(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "state.json")
	if err := os.WriteFile(file, []byte("valid"), 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := Read(file, 5)
	if err != nil || string(raw) != "valid" {
		t.Fatal("read failed", err)
	}
	if _, err := Read(file, 4); err == nil {
		t.Fatal("oversize read accepted")
	}
	missing := filepath.Join(root, "absent", "state.json")
	if _, err := Read(missing, 100); !os.IsNotExist(err) {
		t.Fatal("missing path not reported", err)
	}
	if _, err := os.Stat(filepath.Dir(missing)); !os.IsNotExist(err) {
		t.Fatal("read created a directory")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := Read(filepath.Join(link, "state.json"), 100); err == nil {
		t.Fatal("directory symlink read followed")
	}
}
