//go:build !linux && !windows

package packageretirement

import (
	"context"
	"errors"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func Run(context.Context, int64, int64, func(context.Context, []monitoring.DiscoveryEvent) error) error {
	return errors.New("application retirement requires Linux package inventory or Windows registry")
}
