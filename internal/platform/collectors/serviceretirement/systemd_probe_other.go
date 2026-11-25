//go:build !linux

package serviceretirement

import (
	"context"
	"errors"
)

func SystemdAbsenceProbe() (func(context.Context, string) (bool, error), error) {
	return nil, errors.New("systemd is unavailable")
}
