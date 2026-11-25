package packageretirement

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func newRPMProbe(ctx context.Context) (packageProbe, error) {
	path, err := exec.LookPath("rpm")
	if err != nil {
		return packageProbe{}, err
	}
	read, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := readPackageOutput(exec.CommandContext(read, path, "--eval", "%{_dbpath}"))
	if err != nil {
		return packageProbe{}, err
	}
	dir := filepath.Clean(strings.TrimSpace(output))
	if dir != "/var/lib/rpm" && dir != "/usr/lib/sysimage/rpm" {
		return packageProbe{}, errors.New("application retirement requires the local RPM database")
	}
	return packageProbe{source: "rpm", read: func(ctx context.Context) (map[packageAddress]packageRecord, error) {
		return readRPMInventory(ctx, path, dir)
	}}, nil
}

func readRPMInventory(ctx context.Context, path, dir string) (map[packageAddress]packageRecord, error) {
	for _, filename := range []string{"rpmdb.sqlite", "Packages.db", "Packages"} {
		status := filepath.Join(dir, filename)
		before, err := os.Stat(status)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !before.Mode().IsRegular() || before.Size() == 0 {
			return nil, errors.New("local RPM database unavailable")
		}
		cmd := exec.CommandContext(ctx, path, "--dbpath", dir, "-qa", "--qf", `%{NAME}\t%{ARCH}\n`)
		cmd.Env = append(cmd.Environ(), "LC_ALL=C")
		output, err := readPackageOutput(cmd)
		if err != nil {
			return nil, err
		}
		if err := verifyPackageDatabase(status, before); err != nil {
			return nil, err
		}
		return parseRPMInventory(output)
	}
	return nil, errors.New("local RPM database unavailable")
}
