package outbox

import "time"

type PruneOptions struct {
	MaxPerAgent int
	MaxAge      time.Duration
	Now         time.Time
}

func (o PruneOptions) effectiveNow() time.Time {
	if o.Now.IsZero() {
		return time.Now()
	}
	return o.Now
}
