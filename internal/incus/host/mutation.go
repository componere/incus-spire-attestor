package host

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/lxc/incus/v7/shared/api"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// setAction stores a nonce under a challenge key.
const setAction = "set nonce"

// unsetAction removes a challenge key.
const unsetAction = "unset nonce"

// errReplacedTarget reports that the resolved instance is no longer the same VM.
var errReplacedTarget = errors.New("instance was replaced")

// secretSafeError preserves an inspectable cause without copying its text.
type secretSafeError struct {
	// message is the public error text, with no nonce.
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

// wrapMutation wraps err without including nonce text.
func wrapMutation(action string, err error) error {
	if err == nil {
		return nil
	}
	if isContextError(err) {
		return fmt.Errorf("%s: %w", action, err)
	}
	return secretSafeError{message: action, cause: err}
}

// SetNonce stores nonce under key on instance.
//
// Returned errors never include nonce. A failed update or wait is an unknown
// set outcome.
func (c *Client) SetNonce(ctx context.Context, instance attest.Instance, key attest.ConfigKey, nonce string) error {
	return wrapMutation(setAction, c.mutate(ctx, instance, key, &nonce))
}

// UnsetNonce removes key from instance.
//
// Absence of the key, a 404 refetch, or a replaced target succeeds without
// mutation.
func (c *Client) UnsetNonce(ctx context.Context, instance attest.Instance, key attest.ConfigKey) error {
	return wrapMutation(unsetAction, c.mutate(ctx, instance, key, nil))
}

// mutate applies one exact-key set or unset with ETag-safe retries.
func (c *Client) mutate(ctx context.Context, target attest.Instance, key attest.ConfigKey, nonce *string) error {
	delay := initialRetryDelay
	for {
		err := c.mutateOnce(ctx, target, key, nonce)
		if err == nil {
			return nil
		}
		if errors.Is(err, errReplacedTarget) || !isRetryable(err) {
			return err
		}
		if waitErr := c.wait(ctx, delay); waitErr != nil {
			return waitErr
		}
		delay = nextRetryDelay(delay)
	}
}

// mutateOnce refetches, revalidates, and applies one exact-key change.
func (c *Client) mutateOnce(ctx context.Context, target attest.Instance, key attest.ConfigKey, nonce *string) error {
	server, err := c.scoped(ctx, target.Project)
	if err != nil {
		return err
	}
	current, etag, err := server.GetInstance(string(target.Name))
	if err != nil {
		if nonce == nil && api.StatusErrorCheck(err, http.StatusNotFound) {
			return nil
		}
		return err
	}
	if err := matchResolvedTarget(target, current); err != nil {
		if nonce == nil && errors.Is(err, errReplacedTarget) {
			return nil
		}
		return err
	}
	writable := current.Writable()
	config := copyConfig(writable.Config)
	if nonce == nil {
		if _, found := config[string(key)]; !found {
			return nil
		}
		delete(config, string(key))
	} else {
		config[string(key)] = *nonce
	}
	writable.Config = config
	op, err := server.UpdateInstance(string(target.Name), writable, etag)
	if err != nil {
		return err
	}
	if op == nil {
		return errors.New("update instance returned no operation")
	}
	return op.WaitContext(ctx)
}

// matchResolvedTarget reports whether current still identifies target.
func matchResolvedTarget(target attest.Instance, current *api.Instance) error {
	if current == nil {
		return errReplacedTarget
	}
	if current.Project != "" && current.Project != string(target.Project) {
		return errReplacedTarget
	}
	if current.Name != "" && current.Name != string(target.Name) {
		return errReplacedTarget
	}
	if current.Type != string(attest.InstanceTypeVirtualMachine) {
		return errReplacedTarget
	}
	got, err := attest.NewInstanceUUID(configValue(current.ExpandedConfig, volatileUUIDKey))
	if err != nil || got != target.UUID {
		return errReplacedTarget
	}
	return nil
}
