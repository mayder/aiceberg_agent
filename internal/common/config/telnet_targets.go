package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func AgentlessTelnetTargetsFile() string {
	path := strings.TrimSpace(os.Getenv("AICEBERG_TELNET_TARGETS_FILE"))
	if (runtime.GOOS != "linux" && runtime.GOOS != "darwin") || !filepath.IsAbs(path) {
		return ""
	}
	return path
}
