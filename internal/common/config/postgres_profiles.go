package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// AgentlessPostgresProfilesFile is provisioned by the host operator, outside
// of remote agent configuration and the target cache.
func AgentlessPostgresProfilesFile() string {
	path := os.Getenv("AICEBERG_POSTGRES_PROFILES_FILE")
	if (runtime.GOOS == "linux" || runtime.GOOS == "darwin") && filepath.IsAbs(path) {
		return path
	}
	return ""
}
