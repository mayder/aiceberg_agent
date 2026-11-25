//go:build !linux && !windows

package interfaceretirement

import (
	"context"
	"errors"
)

func watchRemovals(context.Context, func([]removal)) error {
	return errors.New("interface retirement events are supported on Linux and Windows")
}
