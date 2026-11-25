package config

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type JMXProbeSettings struct {
	Java     string
	JAR      string
	Profiles string
	Digest   [32]byte
}

func AgentlessJMXSettings() (JMXProbeSettings, bool) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return JMXProbeSettings{}, false
	}
	settings := JMXProbeSettings{
		Java:     strings.TrimSpace(os.Getenv("AICEBERG_JMX_JAVA")),
		JAR:      strings.TrimSpace(os.Getenv("AICEBERG_JMX_WORKER_JAR")),
		Profiles: strings.TrimSpace(os.Getenv("AICEBERG_JMX_PROBE_PROFILES_FILE")),
	}
	if !filepath.IsAbs(settings.Java) || !filepath.IsAbs(settings.JAR) || !filepath.IsAbs(settings.Profiles) {
		return JMXProbeSettings{}, false
	}
	digest, err := hex.DecodeString(strings.TrimSpace(os.Getenv("AICEBERG_JMX_WORKER_SHA256")))
	if err != nil || len(digest) != len(settings.Digest) {
		return JMXProbeSettings{}, false
	}
	copy(settings.Digest[:], digest)
	return settings, true
}
