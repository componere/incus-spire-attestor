package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/componere/incus-spire-attestor/internal/attest"
	"github.com/componere/incus-spire-attestor/internal/config"
	"github.com/componere/incus-spire-attestor/internal/wire"
)

// randomPairBytes is the number of random bytes in one attempt-ID or nonce.
const randomPairBytes = 16

// Service attests a guest against Incus and emits SPIRE attributes.
type Service struct {
	// client looks up instances and mutates nonce keys.
	client Incus
	// projects is the copied allowlist of Incus projects.
	projects []attest.ProjectName
	// userSelectors is the copied list of expanded_config selector keys.
	userSelectors []string
	// trustDomain is the SPIRE trust domain used for the agent ID.
	trustDomain string
	// incusTimeout bounds Incus lookup and SetNonce work.
	incusTimeout time.Duration
	// challengeTimeout bounds waiting for the guest nonce response.
	challengeTimeout time.Duration
	// cleanupTimeout bounds post-attempt nonce-key removal.
	cleanupTimeout time.Duration
	// random supplies attempt-ID and nonce bytes.
	random io.Reader
	// randomMu serializes consecutive 16-byte reads from random.
	randomMu sync.Mutex
}

// New constructs a Service from a validated server configuration.
func New(client Incus, cfg config.Server, trustDomain string, random io.Reader) (*Service, error) {
	if client == nil {
		return nil, fmt.Errorf("incus client is required")
	}
	if random == nil {
		return nil, fmt.Errorf("random reader is required")
	}
	if err := config.ValidateServer(cfg, trustDomain); err != nil {
		return nil, err
	}

	return &Service{
		client:           client,
		projects:         append([]attest.ProjectName(nil), cfg.Projects...),
		userSelectors:    append([]string(nil), cfg.UserSelectors...),
		trustDomain:      trustDomain,
		incusTimeout:     cfg.IncusTimeout,
		challengeTimeout: cfg.ChallengeResponseTimeout,
		cleanupTimeout:   cfg.CleanupTimeout,
		random:           random,
	}, nil
}

// Attest resolves a guest VM, challenges it with a config nonce, and emits attributes.
func (s *Service) Attest(ctx context.Context, exchange Exchange) error {
	raw, err := exchange.ReceivePayload(ctx)
	if err != nil {
		return fmt.Errorf("receive payload: %w", err)
	}
	claims, err := wire.DecodePayload(raw)
	if err != nil {
		return err
	}
	if err := attest.ValidateClaims(claims); err != nil {
		return err
	}

	instance, err := s.resolveInstance(ctx, claims)
	if err != nil {
		return err
	}

	attemptID, nonce, err := s.readRandomPair()
	if err != nil {
		return err
	}
	key := attest.NewConfigKeyFromAttemptID(attemptID)
	stored := base64.RawURLEncoding.EncodeToString(nonce[:])

	armed := &cleanup{
		client:   s.client,
		rpc:      ctx,
		timeout:  s.cleanupTimeout,
		instance: instance,
		key:      key,
	}
	defer func() { _ = armed.run() }()

	setCtx, cancelSet := context.WithTimeout(ctx, s.incusTimeout)
	err = s.client.SetNonce(setCtx, instance, key, stored)
	cancelSet()
	if err != nil {
		return annotateCleanup(fmt.Errorf("set nonce: %w", err), armed.run())
	}

	challenge, err := wire.EncodeChallenge(key)
	if err != nil {
		return annotateCleanup(err, armed.run())
	}
	if err := exchange.SendChallenge(ctx, challenge); err != nil {
		return annotateCleanup(fmt.Errorf("send challenge: %w", err), armed.run())
	}

	recvCtx, cancelRecv := context.WithTimeout(ctx, s.challengeTimeout)
	rawResp, err := exchange.ReceiveResponse(recvCtx)
	cancelRecv()
	if err != nil {
		return annotateCleanup(fmt.Errorf("receive response: %w", err), armed.run())
	}
	got, err := wire.DecodeResponse(rawResp)
	if err != nil {
		return annotateCleanup(err, armed.run())
	}
	if err := attest.VerifyNonce(nonce, got[:]); err != nil {
		return annotateCleanup(err, armed.run())
	}

	if err := armed.run(); err != nil {
		return annotateCleanup(nil, err)
	}

	attrs, err := attest.BuildAttributes(s.trustDomain, instance, s.userSelectors)
	if err != nil {
		return err
	}
	if err := exchange.SendAttributes(ctx, attrs); err != nil {
		return fmt.Errorf("send attributes: %w", err)
	}
	return nil
}

// readRandomPair returns independent attempt-ID and nonce values.
func (s *Service) readRandomPair() (attemptID [randomPairBytes]byte, nonce attest.Nonce, err error) {
	s.randomMu.Lock()
	defer s.randomMu.Unlock()

	if _, err = io.ReadFull(s.random, attemptID[:]); err != nil {
		return attemptID, nonce, fmt.Errorf("read attempt id: %w", err)
	}
	var raw [randomPairBytes]byte
	if _, err = io.ReadFull(s.random, raw[:]); err != nil {
		return attemptID, nonce, fmt.Errorf("read nonce: %w", err)
	}
	nonce, err = attest.NewNonce(raw[:])
	if err != nil {
		return attemptID, nonce, fmt.Errorf("read nonce: %w", err)
	}
	return attemptID, nonce, nil
}
