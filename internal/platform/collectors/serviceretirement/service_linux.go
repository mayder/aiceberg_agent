package serviceretirement

import (
	"context"
	"errors"
	"io"
	"os/exec"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const systemdOutputLimit = 512 * 1024

func Run(ctx context.Context, client, asset int64, publish func(context.Context, []monitoring.DiscoveryEvent) error) error {
	if client < 1 || asset < 1 || publish == nil {
		return errors.New("invalid service discovery scope")
	}
	inventory, err := localSystemdInventory()
	if err != nil {
		return err
	}
	tracker := retirementTracker{client: client, asset: asset, inventory: inventory, publish: publish, caseSensitive: true}
	return tracker.watch(ctx)
}

func SystemdAbsenceProbe() (func(context.Context, string) (bool, error), error) {
	inventory, err := localSystemdInventory()
	if err != nil {
		return nil, err
	}
	return inventory.missing, nil
}

func localSystemdInventory() (systemdServiceInventory, error) {
	path, err := exec.LookPath("systemctl")
	if err != nil {
		return systemdServiceInventory{}, err
	}
	return systemdServiceInventory{run: func(ctx context.Context, args ...string) (string, int, error) {
		return readSystemd(ctx, path, args)
	}}, nil
}

func readSystemd(ctx context.Context, path string, args []string) (string, int, error) {
	cmd := exec.CommandContext(ctx, path, append([]string{"--system", "--no-pager"}, args...)...)
	cmd.Env = append(cmd.Environ(), "LC_ALL=C", "SYSTEMD_COLORS=0")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", -1, err
	}
	if err := cmd.Start(); err != nil {
		return "", -1, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, systemdOutputLimit+1))
	if readErr != nil || len(output) > systemdOutputLimit {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", -1, errors.New("systemd output incomplete or oversized")
	}
	if err := cmd.Wait(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && ctx.Err() == nil {
			return string(output), exit.ExitCode(), nil
		}
		return "", -1, err
	}
	return string(output), 0, nil
}
