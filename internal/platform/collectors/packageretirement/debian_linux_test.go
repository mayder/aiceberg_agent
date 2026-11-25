package packageretirement

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestDebianRetirementUsesDPKGInDisposableRoot(t *testing.T) {
	if os.Getenv("AICEBERG_TEST_DISPOSABLE_DPKG") != "1" {
		t.Skip("requires disposable Debian environment")
	}
	root := t.TempDir()
	admindir := filepath.Join(root, "var/lib/dpkg")
	if err := os.MkdirAll(admindir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(admindir, "status"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	fixture := buildDebianFixturePackage(t, "aiceberg-discovery-fixture", true)
	anchor := buildDebianFixturePackage(t, "aiceberg-discovery-anchor", false)
	dpkg := func(args ...string) {
		runDebianFixtureCommand(t, "dpkg", append([]string{"--root=" + root}, args...)...)
	}
	dpkg("--install", anchor, fixture)
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	var sent []monitoring.DiscoveryEvent
	tracker := packageRetirementTracker{source: "dpkg", client: 7, asset: 17, publish: func(_ context.Context, events []monitoring.DiscoveryEvent) error {
		sent = append(sent, events...)
		return nil
	}}
	observe := func() {
		inventory, err := readDebianInventory(ctx, "dpkg-query", admindir)
		if err != nil {
			t.Fatal(err)
		}
		if err := tracker.observe(ctx, inventory, now); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Second)
	}
	observe()
	dpkg("--remove", "aiceberg-discovery-fixture")
	observe()
	dpkg("--install", fixture)
	observe()
	dpkg("--purge", "aiceberg-discovery-fixture")
	observe()
	if len(sent) != 1 {
		t.Fatalf("purge must wait for another complete inventory: %+v", sent)
	}
	observe()
	want, err := monitoring.NewDiscoveryEvent(7, 17, "applications", []string{"aiceberg-discovery-fixture", "", "dpkg"},
		"tombstone", 100, 300, time.Unix(1800000001, 0))
	if err != nil || len(sent) != 2 || !slices.Equal(sent[:1], []monitoring.DiscoveryEvent{want}) ||
		sent[1].Fingerprint != want.Fingerprint || sent[1].ObservedMS <= want.ObservedMS {
		t.Fatalf("unexpected native dpkg retirement events: %+v, %v", sent, err)
	}
}

func buildDebianFixturePackage(t *testing.T, name string, keepConfig bool) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{"DEBIAN/control": "Package: " + name + "\nVersion: 1.0\nArchitecture: all\nMaintainer: Fixture <fixture@example.invalid>\nDescription: Discovery fixture\n"}
	if keepConfig {
		files["DEBIAN/conffiles"] = "/etc/" + name + "\n"
		files["etc/"+name] = "fixture\n"
	}
	for relative, contents := range files {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(t.TempDir(), name+".deb")
	runDebianFixtureCommand(t, "dpkg-deb", "--build", root, archive)
	return archive
}

func runDebianFixtureCommand(t *testing.T, command string, args ...string) {
	t.Helper()
	if output, err := exec.Command(command, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", command, err, output)
	}
}
