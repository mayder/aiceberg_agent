package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func AgentlessIPMISettings() (binary, profiles string) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return "", ""
	}
	binary = strings.TrimSpace(os.Getenv("AICEBERG_IPMITOOL_PATH"))
	profiles = strings.TrimSpace(os.Getenv("AICEBERG_IPMI_PROFILES_FILE"))
	if !filepath.IsAbs(binary) {
		binary = ""
	}
	if !filepath.IsAbs(profiles) {
		profiles = ""
	}
	return binary, profiles
}
