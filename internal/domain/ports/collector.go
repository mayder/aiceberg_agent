package ports

import (
	"context"
	"time"
)

// Collector define uma fonte de dados periódica.
// Implementações devem respeitar ctx e retornar erro em falhas recuperáveis.
type Collector interface {
	Name() string
	Interval() time.Duration
	Collect(ctx context.Context) ([]byte, error)
}
