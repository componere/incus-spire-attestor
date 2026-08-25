package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// initialPollDelay is the first wait between guest config reads.
const initialPollDelay = 25 * time.Millisecond

// maxPollDelay is the capped wait between guest config reads.
const maxPollDelay = 250 * time.Millisecond

// pollBackoffFactor doubles the wait between guest config reads.
const pollBackoffFactor = 2

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

// secretSafeError preserves an inspectable cause without copying its text.
type secretSafeError struct {
	// message is the public error text, with no config key or nonce.
	message string
	// cause is the inspectable wrapped error.
	cause error
}

// Error returns the public message.
func (e secretSafeError) Error() string {
	return e.message
}

// Unwrap returns the inspectable cause.
func (e secretSafeError) Unwrap() error {
	return e.cause
}

// wrapReadConfig wraps err without including key or value text.
func wrapReadConfig(err error) error {
	return secretSafeError{message: "read challenge config", cause: err}
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

// isContextError reports whether err is cancellation or a deadline.
func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// isRetryable reports whether err should be polled.
//
// An error is retryable only when Timeout() or Temporary() returns true.
// Implementing those methods is not enough.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var timeout timeoutError
	if errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}
	var temporary temporaryError
	if errors.As(err, &temporary) && temporary.Temporary() {
		return true
	}
	return false
}

// nextPollDelay doubles delay and caps it at maxPollDelay.
func nextPollDelay(delay time.Duration) time.Duration {
	next := delay * pollBackoffFactor
	if next > maxPollDelay {
		return maxPollDelay
	}
	return next
}

// pollConfig reads key until it is visible, pollTimeout elapses, or ctx is done.
func (s *Service) pollConfig(ctx context.Context, key attest.ConfigKey) (string, error) {
	pollCtx, cancel := context.WithTimeout(ctx, s.pollTimeout)
	defer cancel()

	delay := initialPollDelay
	for {
		value, found, err := s.evidence.ReadConfig(pollCtx, key)
		if err != nil {
			if isContextError(err) {
				return "", wrapReadConfig(err)
			}
			if !isRetryable(err) {
				return "", wrapReadConfig(err)
			}
		} else if found {
			return value, nil
		}

		if err := s.wait(pollCtx, delay); err != nil {
			if cause := pollCtx.Err(); cause != nil {
				return "", fmt.Errorf("poll config: %w", cause)
			}
			return "", fmt.Errorf("poll config: %w", err)
		}
		delay = nextPollDelay(delay)
	}
}
