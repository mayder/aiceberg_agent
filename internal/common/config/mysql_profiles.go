package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func AgentlessMySQLProfilesFile() string {
	path := strings.TrimSpace(os.Getenv("AICEBERG_MYSQL_PROBE_PROFILES_FILE"))
	if (runtime.GOOS != "linux" && runtime.GOOS != "darwin") || !filepath.IsAbs(path) {
		return ""
	}
	return path
}
