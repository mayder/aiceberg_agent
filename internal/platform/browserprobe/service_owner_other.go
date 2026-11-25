//go:build !linux && !darwin

package browserprobe

import "os"

func currentOwner(os.FileInfo) bool { return false }
