package agent

import (
	"context"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// GuestEvidence reads guest claims and one challenged config value.
type GuestEvidence interface {
	// Claims returns the guest instance locators for this attestation.
	Claims(ctx context.Context) (attest.Claims, error)
	// ReadConfig reads the challenged config value.
	//
	// found is false when the key is not yet visible. A non-nil error is
	// classified before found: context errors fail immediately, and only
	// Timeout() or Temporary() true errors are retried. Returned errors
	// must not include the config key or value.
	ReadConfig(ctx context.Context, key attest.ConfigKey) (value string, found bool, err error)
}

// Exchange preserves the payload, challenge, and response sequence.
type Exchange interface {
	// SendPayload sends the encoded guest-claims payload.
	SendPayload(ctx context.Context, payload []byte) error
	// ReceiveChallenge receives the one challenge message.
	ReceiveChallenge(ctx context.Context) ([]byte, error)
	// SendResponse sends the encoded nonce response. Implementations must not
	// include response bytes in returned errors.
	SendResponse(ctx context.Context, response []byte) error
}
