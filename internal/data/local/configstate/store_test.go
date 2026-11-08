package configstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/configfailure"
)

func stateFixture() configfailure.State {
	s := configfailure.NewState()
	s.Failure("agent", "2", strings.Repeat("a", 64), strings.Repeat("b", 64), configfailure.Cause{Code: "disk_full", Kind: "persistent"}, 1000, 61000, true)
	return s
}

func TestConfigStatePersistsUnacknowledgedReceiptAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "independent", "state.json")
	store := New(path)
	state, err := store.Load()
	if err != nil || len(state.Entries) != 0 {
		t.Fatal(err)
	}
	state = stateFixture()
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	restored, err := New(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if restored.Pending[0] != state.Pending[0] || restored.Active != state.Active {
		t.Fatal("receipt changed on reboot")
	}
	restored.Acknowledge(state.Pending[0].Diagnostic.ReportID)
	if err := store.Save(restored); err != nil {
		t.Fatal(err)
	}
	again, err := New(path).Load()
	if err != nil || len(again.Pending) != 0 {
		t.Fatal("ACK not persisted", err)
	}
}

func TestConfigStatePreservesInvalidOversizedAndSymlinkFiles(t *testing.T) {
	for _, content := range []string{`{"schema_version":77}`, strings.Repeat("x", MaxBytes+1)} {
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		store := New(path)
		if _, err := store.Load(); err == nil {
			t.Fatal("invalid state accepted")
		}
		if err := store.Save(stateFixture()); err == nil {
			t.Fatal("invalid evidence overwritten")
		}
		raw, _ := os.ReadFile(path)
		if string(raw) != content {
			t.Fatal("invalid evidence changed")
		}
	}
	root := t.TempDir()
	target := filepath.Join(root, "original")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "state.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	store := New(link)
	if _, err := store.Load(); err == nil {
		t.Fatal("symlink loaded")
	}
	if err := store.Save(stateFixture()); err == nil {
		t.Fatal("symlink overwritten")
	}
}

func TestDiagnosticPathCannotOverwritePreferencesOrOutbox(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "prefs.json")
	store := New(protected, protected)
	if _, err := store.Load(); err == nil {
		t.Fatal("protected nonexistent path admitted")
	}
	if err := store.Save(stateFixture()); err == nil {
		t.Fatal("protected path overwritten")
	}
	if _, err := os.Stat(protected); !os.IsNotExist(err) {
		t.Fatal("protected path created")
	}
	original := `{"version":"good"}`
	if err := os.WriteFile(protected, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "hardlink.json")
	if err := os.Link(protected, link); err != nil {
		t.Skip("hardlinks unavailable")
	}
	store = New(link, protected)
	if _, err := store.Load(); err == nil {
		t.Fatal("protected inode admitted")
	}
	if err := store.Save(stateFixture()); err == nil {
		t.Fatal("protected inode overwritten")
	}
	raw, _ := os.ReadFile(protected)
	if string(raw) != original {
		t.Fatal("preferences changed")
	}
}
