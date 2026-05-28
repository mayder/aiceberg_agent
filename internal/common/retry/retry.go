package retry

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	DefaultInitialBackoff = 2 * time.Second
	DefaultMaxBackoff     = 60 * time.Second
	DefaultMinJitter      = 0.20
	DefaultMaxJitter      = 0.40
)

type HTTPStatus interface {
	StatusCode() int
}

type ErrorKind string

const (
	ErrorKindNone      ErrorKind = "none"
	ErrorKindTransient ErrorKind = "transient"
	ErrorKindPermanent ErrorKind = "permanent"
)

type Backoff struct {
	initial   time.Duration
	max       time.Duration
	minJitter float64
	maxJitter float64
	randFloat func() float64
	now       func() time.Time

	mu       sync.Mutex
	failures int
	until    time.Time
}

func NewBackoff() *Backoff {
	return NewBackoffWithOptions(DefaultInitialBackoff, DefaultMaxBackoff, DefaultMinJitter, DefaultMaxJitter)
}

func NewBackoffWithOptions(initial, max time.Duration, minJitter, maxJitter float64) *Backoff {
	if initial <= 0 {
		initial = DefaultInitialBackoff
	}
	if max <= 0 {
		max = DefaultMaxBackoff
	}
	if minJitter < 0 {
		minJitter = 0
	}
	if maxJitter < minJitter {
		maxJitter = minJitter
	}
	return &Backoff{
		initial:   initial,
		max:       max,
		minJitter: minJitter,
		maxJitter: maxJitter,
		randFloat: rand.Float64,
		now:       time.Now,
	}
}

func (b *Backoff) Active() (time.Time, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.until.IsZero() || !b.now().Before(b.until) {
		return time.Time{}, false
	}
	return b.until, true
}

func (b *Backoff) Failure(err error) (time.Duration, ErrorKind) {
	kind := ClassifyError(err)
	if kind != ErrorKindTransient {
		return 0, kind
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	delay := BackoffDelay(b.failures, b.initial, b.max, b.minJitter, b.maxJitter, b.randFloat)
	b.until = b.now().Add(delay)
	return delay, kind
}

func (b *Backoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.until = time.Time{}
}

func (b *Backoff) Cooldown(delay time.Duration) time.Duration {
	if delay <= 0 {
		delay = DefaultMaxBackoff
	}
	if delay > b.max {
		delay = b.max
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.until = b.now().Add(delay)
	return delay
}

func (b *Backoff) Failures() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failures
}

func BackoffDelay(failures int, initial, max time.Duration, minJitter, maxJitter float64, randFloat func() float64) time.Duration {
	if failures < 1 {
		failures = 1
	}
	if initial <= 0 {
		initial = DefaultInitialBackoff
	}
	if max <= 0 {
		max = DefaultMaxBackoff
	}
	if randFloat == nil {
		randFloat = rand.Float64
	}

	pow := math.Pow(2, float64(failures-1))
	base := time.Duration(float64(initial) * pow)
	if base > max {
		base = max
	}
	jitterRange := maxJitter - minJitter
	if jitterRange < 0 {
		jitterRange = 0
	}
	jitterFactor := minJitter + randFloat()*jitterRange
	delay := base + time.Duration(float64(base)*jitterFactor)
	if delay > max {
		return max
	}
	return delay
}

func ClassifyError(err error) ErrorKind {
	if err == nil {
		return ErrorKindNone
	}
	if errors.Is(err, context.Canceled) {
		return ErrorKindPermanent
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorKindTransient
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrorKindTransient
		}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTemporary || dnsErr.IsTimeout {
			return ErrorKindTransient
		}
	}
	var statusErr HTTPStatus
	if errors.As(err, &statusErr) {
		code := statusErr.StatusCode()
		switch code {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return ErrorKindTransient
		case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			return ErrorKindPermanent
		}
		if code >= 500 {
			return ErrorKindTransient
		}
		if code >= 400 {
			return ErrorKindPermanent
		}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "connection reset by peer"),
		strings.Contains(msg, "temporary"),
		strings.Contains(msg, "temporary failure"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "server misbehaving"),
		strings.Contains(msg, "network is unreachable"):
		return ErrorKindTransient
	}
	return ErrorKindPermanent
}
