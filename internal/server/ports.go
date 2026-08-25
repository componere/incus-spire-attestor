package server

import (
	"context"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// Incus is the host-side lookup and nonce-mutation port.
type Incus interface {
	// Lookup returns the named instance in project when it exists.
	Lookup(ctx context.Context, project attest.ProjectName, name attest.InstanceName) (instance attest.Instance, found bool, err error)
	// SetNonce stores nonce under key on instance.
	SetNonce(ctx context.Context, instance attest.Instance, key attest.ConfigKey, nonce string) error
	// UnsetNonce removes key from instance.
	UnsetNonce(ctx context.Context, instance attest.Instance, key attest.ConfigKey) error
}

// Exchange is the server-side attestation stream.
type Exchange interface {
	// ReceivePayload reads the guest claims payload.
	ReceivePayload(ctx context.Context) ([]byte, error)
	// SendChallenge writes the config-nonce challenge.
	SendChallenge(ctx context.Context, challenge []byte) error
	// ReceiveResponse reads the guest nonce response.
	ReceiveResponse(ctx context.Context) ([]byte, error)
	// SendAttributes writes the terminal SPIRE attributes.
	SendAttributes(ctx context.Context, attrs attest.Attributes) error
}
