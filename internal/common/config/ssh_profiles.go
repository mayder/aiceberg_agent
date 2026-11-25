package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func AgentlessSSHProfilesFile() string {
	path := strings.TrimSpace(os.Getenv("AICEBERG_SSH_PROBE_PROFILES_FILE"))
	if (runtime.GOOS != "linux" && runtime.GOOS != "darwin") || !filepath.IsAbs(path) {
		return ""
	}
	return path
}
