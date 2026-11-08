//go:build !windows

package configfailure

func platformCause(error) (Cause, bool) { return Cause{}, false }
