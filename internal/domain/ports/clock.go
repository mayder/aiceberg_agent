package ports

import "time"

// Clock abstrai o tempo atual para facilitar testes.
type Clock interface {
	Now() time.Time
}
