package config

import (
	"os"
	"runtime"
	"strings"
)

func AgentlessHTTPProbeProfilesFile() string {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return ""
	}
	return strings.TrimSpace(os.Getenv("AICEBERG_HTTP_PROBE_PROFILES_FILE"))
}
