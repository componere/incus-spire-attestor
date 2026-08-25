package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/componere/incus-spire-attestor/internal/attest"
	"github.com/componere/incus-spire-attestor/internal/wire"
)

// Service is the guest attestation application.
type Service struct {
	// evidence reads guest claims and challenged config values.
	evidence GuestEvidence
	// pollTimeout bounds config reads and waits after a valid challenge.
	pollTimeout time.Duration
	// wait waits between polls; tests may replace it.
	wait waitFunc
}

// New constructs a Service that polls evidence until pollTimeout.
//
// New rejects a nil evidence port and a non-positive timeout.
func New(evidence GuestEvidence, pollTimeout time.Duration) (*Service, error) {
	return newService(evidence, pollTimeout, waitDuration)
}

// newService constructs a Service with an injectable wait function.
func newService(evidence GuestEvidence, pollTimeout time.Duration, wait waitFunc) (*Service, error) {
	if evidence == nil {
		return nil, fmt.Errorf("evidence is required")
	}
	if pollTimeout <= 0 {
		return nil, fmt.Errorf("poll timeout must be positive")
	}
	if wait == nil {
		wait = waitDuration
	}
	return &Service{
		evidence:    evidence,
		pollTimeout: pollTimeout,
		wait:        wait,
	}, nil
}

// Attest runs one guest attestation against exchange.
//
// It reads and validates VM claims, sends the payload, receives one
// challenge, polls the challenged config value until pollTimeout, and
// sends the nonce response. Config is not read until the challenge is a
// valid exact v1 key. pollTimeout bounds only config reads and waits.
func (s *Service) Attest(ctx context.Context, exchange Exchange) error {
	if exchange == nil {
		return fmt.Errorf("exchange is required")
	}

	claims, err := s.evidence.Claims(ctx)
	if err != nil {
		return fmt.Errorf("read guest claims: %w", err)
	}
	if err := attest.ValidateClaims(claims); err != nil {
		return fmt.Errorf("validate guest claims: %w", err)
	}

	payload, err := wire.EncodePayload(claims)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}
	if err := exchange.SendPayload(ctx, payload); err != nil {
		return fmt.Errorf("send payload: %w", err)
	}

	challenge, err := exchange.ReceiveChallenge(ctx)
	if err != nil {
		return fmt.Errorf("receive challenge: %w", err)
	}
	key, err := wire.DecodeChallenge(challenge)
	if err != nil {
		return fmt.Errorf("decode challenge: %w", err)
	}

	value, err := s.pollConfig(ctx, key)
	if err != nil {
		return err
	}
	nonce, err := wire.ParseNonce(value)
	if err != nil {
		return fmt.Errorf("parse nonce: %w", err)
	}

	response, err := wire.EncodeResponse(nonce)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	if err := exchange.SendResponse(ctx, response); err != nil {
		return fmt.Errorf("send response: %w", err)
	}
	return nil
}
