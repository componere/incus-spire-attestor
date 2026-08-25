package guest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// secretSafeError preserves an inspectable cause without copying its text.
type secretSafeError struct {
	// message is the public error text, with no config key or value.
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

// transientError is a retryable guest transport or service failure.
type transientError struct {
	// message is the public error text, with no config key or value.
	message string
	// cause is the wrapped inspectable error.
	cause error
	// timeout reports a deadline-like failure.
	timeout bool
}

// Error returns the public message.
func (e transientError) Error() string {
	return e.message
}

// Unwrap returns the inspectable cause.
func (e transientError) Unwrap() error {
	return e.cause
}

// Temporary reports that the failure may succeed on retry.
func (e transientError) Temporary() bool {
	return true
}

// Timeout reports whether the failure is deadline-like.
func (e transientError) Timeout() bool {
	return e.timeout
}

// isContextError reports whether err is cancellation or a deadline.
func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// wrapContext preserves a context cause without copying URL or secret text.
func wrapContext(op string, err error) error {
	return secretSafeError{message: op, cause: err}
}

// transportError classifies a failed dial or HTTP round trip as retryable.
func transportError(err error) error {
	if isContextError(err) {
		return err
	}
	timeout := false
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		timeout = true
	}
	return transientError{
		message: "guest transport failed",
		cause:   err,
		timeout: timeout,
	}
}

// statusError classifies an HTTP status as transient or permanent.
func statusError(status int) error {
	switch {
	case status == http.StatusRequestTimeout:
		return transientError{message: "guest request timeout", timeout: true}
	case status == http.StatusTooManyRequests || status >= 500:
		return transientError{message: "guest service unavailable"}
	default:
		return fmt.Errorf("guest status %d", status)
	}
}
