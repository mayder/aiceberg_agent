package usecase

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/version"
)

const UpdateReadinessSchema = "aiceberg-agent-update-readiness.v1"

// CheckUpdateReadiness probes the existing updater without downloading, applying,
// restarting, changing configuration or clearing pending state. Results contain no
// commands, paths, identity, credentials or raw errors.
func (uc *SelfUpdate) CheckUpdateReadiness(ctx context.Context) map[string]any {
	opts := uc.effectiveOptions()
	configVersion := uc.readinessConfigVersion()
	checks := map[string]bool{
		"enabled": opts.enabled, "signed_updates_required": uc.cfg.AutoUpdateTrustRequired,
		"config_version_known": configVersion != "", "context_active": ctx.Err() == nil,
	}
	if ctx.Err() == nil && opts.enabled {
		result := uc.preflightUpdate(opts)
		checks["local_preflight"] = result.OK
		launcher, valid := officialUpdateLauncher(opts.command, runtime.GOOS)
		checks["official_launcher"] = valid && regularUpdateFile(launcher, runtime.GOOS != "windows")
		apply := "aiceberg-agent-apply-update.sh"
		if runtime.GOOS == "windows" {
			apply = "aiceberg-agent-apply-update.ps1"
		}
		checks["apply_script"] = valid && regularUpdateFile(filepath.Join(filepath.Dir(launcher), apply), runtime.GOOS != "windows")
		checks["service_manager"] = updateServiceManagerAvailable(runtime.GOOS)
		checks["service_authority"] = updateServiceAuthority(ctx, opts.command, runtime.GOOS)
		pending, err := uc.loadPendingState(opts.dir)
		checks["no_pending_update"] = err == nil && pending == nil
	}
	checks["config_unchanged"] = configVersion != "" && configVersion == uc.readinessConfigVersion()
	checks["context_active"] = ctx.Err() == nil
	ready := readinessChecksPassed(checks)
	return map[string]any{
		"schema": UpdateReadinessSchema, "ready": ready, "checks": checks,
		"agent_version": version.Version, "os": runtime.GOOS, "arch": runtime.GOARCH,
		"config_version": configVersion, "checked_at_utc": time.Now().UTC().Format(time.RFC3339Nano),
		"gaps": updatePreflightGaps(),
	}
}

func (uc *SelfUpdate) readinessConfigVersion() string {
	if uc.prefsProvider == nil {
		return ""
	}
	value := strings.TrimSpace(uc.prefsProvider().Version)
	if !regexp.MustCompile(`^[1-9][0-9]{0,9}$`).MatchString(value) {
		return ""
	}
	return value
}

func officialUpdateLauncher(command, goos string) (string, bool) {
	command = strings.TrimSpace(command)
	if goos == "windows" {
		match := regexp.MustCompile(`^&\s+'([A-Za-z]:\\[^']*\\aiceberg-agent-update-launcher\.ps1)'$`).FindStringSubmatch(command)
		if len(match) == 2 {
			return match[1], true
		}
		return "", false
	}
	if goos != "linux" && goos != "darwin" {
		return "", false
	}
	launcher := "/usr/local/sbin/aiceberg-agent-update-launcher.sh"
	return launcher, command == launcher || command == "sudo -n "+launcher
}

func regularUpdateFile(path string, executable bool) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && (!executable || info.Mode().Perm()&0o111 != 0)
}

func updateServiceManagerAvailable(goos string) bool {
	names := map[string][]string{"windows": {"sc.exe"}, "linux": {"systemctl", "service"}, "darwin": {"launchctl"}}
	for _, name := range names[goos] {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

func updateServiceAuthority(ctx context.Context, command, goos string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if goos == "windows" {
		script := `([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)`
		output, err := exec.CommandContext(probeCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
		return err == nil && strings.EqualFold(strings.TrimSpace(string(output)), "true")
	}
	launcher, valid := officialUpdateLauncher(command, goos)
	if !valid {
		return false
	}
	if strings.TrimSpace(command) == launcher {
		return os.Geteuid() == 0
	}
	return probeUpdateLauncherAuthorization(probeCtx, launcher)
}

func readinessChecksPassed(checks map[string]bool) bool {
	required := []string{"enabled", "signed_updates_required", "config_version_known", "context_active",
		"local_preflight", "official_launcher", "apply_script", "service_manager", "service_authority",
		"no_pending_update", "config_unchanged"}
	for _, name := range required {
		if !checks[name] {
			return false
		}
	}
	return true
}

func probeUpdateLauncherAuthorization(ctx context.Context, launcher string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return exec.CommandContext(probeCtx, "sudo", "-n", "-l", launcher).Run() == nil
}
