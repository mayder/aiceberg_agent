package packageretirement

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

func newDebianProbe() (packageProbe, error) {
	const admindir = "/var/lib/dpkg"
	if dir := os.Getenv("DPKG_ADMINDIR"); dir != "" && filepath.Clean(dir) != admindir {
		return packageProbe{}, errors.New("application retirement requires the local dpkg database")
	}
	if root := os.Getenv("DPKG_ROOT"); root != "" && filepath.Clean(root) != "/" {
		return packageProbe{}, errors.New("application retirement requires the local dpkg database")
	}
	path, err := exec.LookPath("dpkg-query")
	if err != nil {
		return packageProbe{}, err
	}
	return packageProbe{source: "dpkg", read: func(ctx context.Context) (map[packageAddress]packageRecord, error) {
		return readDebianInventory(ctx, path, admindir)
	}}, nil
}

func readDebianInventory(ctx context.Context, path, admindir string) (map[packageAddress]packageRecord, error) {
	status := filepath.Join(admindir, "status")
	before, err := os.Stat(status)
	if err != nil || !before.Mode().IsRegular() || before.Size() == 0 {
		return nil, errors.New("local dpkg database unavailable")
	}
	cmd := exec.CommandContext(ctx, path, "--admindir="+admindir, "--show",
		`--showformat=${binary:Package}\t${Package}\t${Architecture}\t${db:Status-Want}\t${db:Status-Status}\n`)
	cmd.Env = append(cmd.Environ(), "LC_ALL=C", "DPKG_PAGER=cat", "DPKG_COLORS=never")
	output, err := readPackageOutput(cmd)
	if err != nil {
		return nil, err
	}
	if err := verifyPackageDatabase(status, before); err != nil {
		return nil, err
	}
	return parseDebianInventory(output)
}
