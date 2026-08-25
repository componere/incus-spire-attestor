package host

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/lxc/incus/v7/shared/api"
)

// initialRetryDelay is the first wait between retryable mutations.
const initialRetryDelay = 25 * time.Millisecond

// maxRetryDelay is the capped wait between retryable mutations.
const maxRetryDelay = 250 * time.Millisecond

// retryBackoffFactor doubles the wait between retryable mutations.
const retryBackoffFactor = 2

// waitFunc waits delay or until ctx is done.
type waitFunc func(ctx context.Context, delay time.Duration) error

// timeoutError is an error that reports a timeout through Timeout().
type timeoutError interface {
	// Timeout reports whether the failure is a timeout.
	Timeout() bool
}

// temporaryError is an error that reports a transient failure through Temporary().
type temporaryError interface {
	// Temporary reports whether the failure is transient.
	Temporary() bool
}

// waitDuration waits delay or returns ctx.Err() when ctx is done.
func waitDuration(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// nextRetryDelay doubles delay and caps it at maxRetryDelay.
func nextRetryDelay(delay time.Duration) time.Duration {
	next := delay * retryBackoffFactor
	if next > maxRetryDelay {
		return maxRetryDelay
	}
	return next
}

// isContextError reports whether err is cancellation or a deadline.
func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// isConflict reports whether err is an ETag precondition failure.
func isConflict(err error) bool {
	return api.StatusErrorCheck(err, http.StatusPreconditionFailed)
}

// isRetryable reports whether err should be retried while ctx is live.
func isRetryable(err error) bool {
	if err == nil || isContextError(err) {
		return false
	}
	if isConflict(err) {
		return true
	}
	if api.StatusErrorCheck(err,
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	) {
		return true
	}
	var timeout timeoutError
	if errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}
	var temporary temporaryError
	if errors.As(err, &temporary) && temporary.Temporary() {
		return true
	}
	var op *net.OpError
	return errors.As(err, &op)
}
