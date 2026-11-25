package packageretirement

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func Run(ctx context.Context, client, asset int64, publish func(context.Context, []monitoring.DiscoveryEvent) error) error {
	if err := validatePackageRetirementScope(client, asset, publish); err != nil {
		return err
	}
	debian, debianErr := newDebianProbe()
	rpm, rpmErr := newRPMProbe(ctx)
	if debianErr != nil && rpmErr != nil {
		return errors.Join(debianErr, rpmErr)
	}
	return watchPackageProbes(ctx, client, asset, publish, []packageProbe{debian, rpm})
}

const packageOutputLimit = 2 * 1024 * 1024

type packageDiagnostics struct{ written bool }

func (d *packageDiagnostics) Write(data []byte) (int, error) {
	d.written = d.written || len(data) != 0
	return len(data), nil
}

func readPackageOutput(cmd *exec.Cmd) (string, error) {
	var diagnostics packageDiagnostics
	cmd.Stderr = &diagnostics
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, packageOutputLimit+1))
	if readErr != nil || len(output) > packageOutputLimit {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", errors.New("incomplete or oversized package inventory")
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}
	if diagnostics.written {
		return "", errors.New("package manager reported an inventory diagnostic")
	}
	return string(output), nil
}

func verifyPackageDatabase(status string, before os.FileInfo) error {
	after, err := os.Stat(status)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return errors.New("package database changed during the inventory read")
	}
	return nil
}
