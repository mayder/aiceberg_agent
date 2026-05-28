package retry

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type statusErr struct{ code int }

func (e statusErr) Error() string   { return http.StatusText(e.code) }
func (e statusErr) StatusCode() int { return e.code }

func TestClassifyErrorSeparatesTransientAndPermanentHTTP(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusInternalServerError} {
		if got := ClassifyError(statusErr{code: code}); got != ErrorKindTransient {
			t.Fatalf("expected %d transient, got %s", code, got)
		}
	}
	for _, code := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		if got := ClassifyError(statusErr{code: code}); got != ErrorKindPermanent {
			t.Fatalf("expected %d permanent, got %s", code, got)
		}
	}
}

func TestClassifyErrorDetectsTimeoutAndConnectionFailures(t *testing.T) {
	if got := ClassifyError(context.DeadlineExceeded); got != ErrorKindTransient {
		t.Fatalf("expected deadline transient, got %s", got)
	}
	if got := ClassifyError(context.Canceled); got != ErrorKindPermanent {
		t.Fatalf("expected canceled permanent, got %s", got)
	}
}

func TestBackoffDelayUsesExponentialJitterAndMax(t *testing.T) {
	delay := BackoffDelay(1, 2*time.Second, time.Minute, 0.20, 0.40, func() float64 { return 0 })
	if delay != 2400*time.Millisecond {
		t.Fatalf("expected first delay 2.4s, got %s", delay)
	}
	delay = BackoffDelay(10, 2*time.Second, time.Minute, 0.20, 0.40, func() float64 { return 1 })
	if delay != time.Minute {
		t.Fatalf("expected capped delay 60s, got %s", delay)
	}
}
