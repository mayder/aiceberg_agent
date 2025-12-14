//go:build windows

package sysmetrics

import (
	"context"
	"encoding/json"
	"os/exec"
)

// collectWindowsHotfixes usa PowerShell Get-HotFix e retorna lista de hotfixes.
func collectWindowsHotfixes(ctx context.Context) []winHotfix {
	path, err := exec.LookPath("powershell")
	if err != nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, path, "-Command", "Get-HotFix | Select-Object HotFixID,InstalledOn,Description | ConvertTo-Json")
	raw, err := cmd.Output()
	if err != nil || len(raw) == 0 {
		return nil
	}
	var out []struct {
		HotFixID    string `json:"HotFixID"`
		InstalledOn string `json:"InstalledOn"`
		Description string `json:"Description"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	var res []winHotfix
	for _, h := range out {
		res = append(res, winHotfix{ID: h.HotFixID, InstalledOn: h.InstalledOn, Description: h.Description})
	}
	return res
}

// collectWindowsApps usa PowerShell para ler programas instalados do registry (Uninstall).
func collectWindowsApps(ctx context.Context) []winApp {
	path, err := exec.LookPath("powershell")
	if err != nil {
		return nil
	}
	script := `Get-ItemProperty 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*' | Select-Object DisplayName,DisplayVersion,Publisher,InstallDate | ConvertTo-Json`
	cmd := exec.CommandContext(ctx, path, "-Command", script)
	raw, err := cmd.Output()
	if err != nil || len(raw) == 0 {
		return nil
	}
	var out []struct {
		Name        string `json:"DisplayName"`
		Version     string `json:"DisplayVersion"`
		Publisher   string `json:"Publisher"`
		InstallDate string `json:"InstallDate"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	var res []winApp
	for _, a := range out {
		if a.Name == "" {
			continue
		}
		res = append(res, winApp{
			Name:    a.Name,
			Version: a.Version,
			Vendor:  a.Publisher,
			Install: a.InstallDate,
			Source:  "registry",
		})
	}
	return res
}
