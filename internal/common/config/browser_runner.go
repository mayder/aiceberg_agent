package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func AgentlessBrowserRunnerSocket() string {
	path := strings.TrimSpace(os.Getenv("AICEBERG_BROWSER_RUNNER_SOCKET"))
	if (runtime.GOOS != "linux" && runtime.GOOS != "darwin") || !filepath.IsAbs(path) ||
		filepath.Clean(path) != path || len(path) > 100 {
		return ""
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm()&0o007 != 0 {
		return ""
	}
	return path
}
