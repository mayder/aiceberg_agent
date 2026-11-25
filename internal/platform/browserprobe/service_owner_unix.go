//go:build linux || darwin

package browserprobe

import (
	"os"
	"syscall"
)

func currentOwner(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}
