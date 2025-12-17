//go:build windows

package sysmetrics

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

// collectWindowsHotfixes usa PowerShell Get-HotFix e retorna lista de hotfixes.
func collectWindowsHotfixes(ctx context.Context) []winHotfix {
	path, err := exec.LookPath("powershell")
	if err != nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, path, "-Command", "Get-HotFix | Select-Object HotFixID,InstalledOn,Description,InstalledBy | ConvertTo-Json")
	raw, err := cmd.Output()
	if err != nil || len(raw) == 0 {
		return nil
	}
	var out []struct {
		HotFixID    string `json:"HotFixID"`
		InstalledOn string `json:"InstalledOn"`
		Description string `json:"Description"`
		InstalledBy string `json:"InstalledBy"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	var res []winHotfix
	for _, h := range out {
		res = append(res, winHotfix{ID: h.HotFixID, InstalledOn: h.InstalledOn, Description: h.Description, Source: h.InstalledBy})
	}
	return res
}

// collectWindowsApps usa PowerShell para ler programas instalados do registry (Uninstall).
func collectWindowsApps(ctx context.Context) []winApp {
	path, err := exec.LookPath("powershell")
	if err != nil {
		return nil
	}
	script := `
	$paths = @(
	  'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*',
	  'HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*'
	)
	Get-ItemProperty $paths | Select-Object DisplayName,DisplayVersion,Publisher,InstallDate,InstallLocation,InstallSource,UninstallString | ConvertTo-Json
	`
	cmd := exec.CommandContext(ctx, path, "-Command", script)
	raw, err := cmd.Output()
	if err != nil || len(raw) == 0 {
		return nil
	}
	var out []struct {
		Name            string `json:"DisplayName"`
		Version         string `json:"DisplayVersion"`
		Publisher       string `json:"Publisher"`
		InstallDate     string `json:"InstallDate"`
		InstallLocation string `json:"InstallLocation"`
		InstallSource   string `json:"InstallSource"`
		UninstallString string `json:"UninstallString"`
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
			Name:            a.Name,
			Version:         a.Version,
			Vendor:          a.Publisher,
			Install:         a.InstallDate,
			Source:          "registry",
			InstallLocation: a.InstallLocation,
			InstallSource:   a.InstallSource,
			UninstallString: a.UninstallString,
		})
	}
	return res
}

// collectWindowsFeatures lista roles/features instaladas (Windows Server).
func collectWindowsFeatures(ctx context.Context) []winFeature {
	path, err := exec.LookPath("powershell")
	if err != nil {
		return nil
	}
	script := `Get-WindowsFeature | Select-Object Name,DisplayName,Installed | ConvertTo-Json`
	cmd := exec.CommandContext(ctx, path, "-Command", script)
	raw, err := cmd.Output()
	if err != nil || len(raw) == 0 {
		return nil
	}
	// Get-WindowsFeature retorna objeto único quando há apenas 1 linha; normaliza para array.
	if len(raw) > 0 && raw[0] == '{' {
		raw = []byte("[" + strings.TrimSpace(string(raw)) + "]")
	}
	var out []struct {
		Name        string `json:"Name"`
		DisplayName string `json:"DisplayName"`
		Installed   bool   `json:"Installed"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	var res []winFeature
	for _, f := range out {
		res = append(res, winFeature{
			Name:        f.Name,
			DisplayName: f.DisplayName,
			Installed:   f.Installed,
		})
	}
	return res
}
