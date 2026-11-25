package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func AgentlessVCenterProfilesFile() string {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return ""
	}
	path := strings.TrimSpace(os.Getenv("AICEBERG_VCENTER_PROFILES_FILE"))
	if !filepath.IsAbs(path) {
		return ""
	}
	return path
}
