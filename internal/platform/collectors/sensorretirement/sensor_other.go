//go:build !linux

package sensorretirement

import (
	"context"
	"errors"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func Run(context.Context, int64, int64, func(context.Context, []monitoring.DiscoveryEvent) error) error {
	return errors.New("temperature sensor removal notifications are supported on Linux")
}
