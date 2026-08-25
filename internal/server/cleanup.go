package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// cleanup removes a previously armed nonce key exactly once.
type cleanup struct {
	// client is the Incus port used to unset the key.
	client Incus
	// rpc is the caller RPC context.
	rpc context.Context
	// timeout bounds the detached cleanup attempt.
	timeout time.Duration
	// instance is the retained API snapshot.
	instance attest.Instance
	// key is the exact attempt key.
	key attest.ConfigKey
	// once ensures UnsetNonce runs at most once.
	once sync.Once
	// err is the first cleanup result.
	err error
}

// run unsets the armed nonce key at most once.
func (c *cleanup) run() error {
	c.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.rpc), c.timeout)
		defer cancel()
		c.err = c.client.UnsetNonce(ctx, c.instance, c.key)
	})
	return c.err
}

// annotateCleanup preserves a primary inspectable class and sanitizes cleanup.
func annotateCleanup(primary, cleanErr error) error {
	if cleanErr == nil {
		return primary
	}
	if primary == nil {
		return fmt.Errorf("cleanup nonce: %w", cleanErr)
	}
	return fmt.Errorf("%w (cleanup failed)", primary)
}
