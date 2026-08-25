package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/componere/incus-spire-attestor/internal/attest"
	"github.com/componere/incus-spire-attestor/internal/config"
	"github.com/componere/incus-spire-attestor/internal/server/mocks"
	"github.com/componere/incus-spire-attestor/internal/wire"
)

const (
	testTrustDomain  = "example.org"
	testInstanceName = "web-01"
	testLocation     = "node-a"
	testCloudInitID  = "i-abc123"
	testProject      = "default"
	testOpsProject   = "ops"
	canonicalUUID    = "550e8400-e29b-41d4-a716-446655440000"
	attemptFill      = byte(0x11)
	nonceFill        = byte(0x22)
)

type attestEnv struct {
	t        *testing.T
	incus    *mocks.MockIncus
	exchange *mocks.MockExchange
	service  *Service
}

func validServerConfig() config.Server {
	return config.Server{
		IncusEndpoint:            "https://incus.example.invalid:8443",
		TLSCAPath:                "/no/such/incus/ca.pem",
		TLSCertPath:              "/no/such/incus/client.pem",
		TLSKeyPath:               "/no/such/incus/client-key.pem",
		Projects:                 []attest.ProjectName{testProject, testOpsProject},
		UserSelectors:            []string{"user.role"},
		IncusTimeout:             5 * time.Second,
		ChallengeResponseTimeout: 10 * time.Second,
		CleanupTimeout:           5 * time.Second,
	}
}

func validClaims() attest.Claims {
	return attest.Claims{
		Name:        attest.InstanceName(testInstanceName),
		UUID:        attest.InstanceUUID(canonicalUUID),
		Type:        attest.InstanceTypeVirtualMachine,
		Location:    testLocation,
		CloudInitID: testCloudInitID,
	}
}

func hintedClaims() attest.Claims {
	claims := validClaims()
	claims.Project = testProject
	return claims
}

func validInstance() attest.Instance {
	return attest.Instance{
		Project:     testProject,
		Name:        attest.InstanceName(testInstanceName),
		UUID:        attest.InstanceUUID(canonicalUUID),
		Type:        attest.InstanceTypeVirtualMachine,
		Location:    testLocation,
		CloudInitID: testCloudInitID,
		Profiles:    []string{"default"},
		ExpandedConfig: map[string]string{
			"user.role": "app",
		},
	}
}

func fill16(fill byte) [16]byte {
	var out [16]byte
	for i := range out {
		out[i] = fill
	}
	return out
}

// fillBytes returns 16 bytes initialized to fill.
func fillBytes(fill byte) []byte {
	value := fill16(fill)
	return value[:]
}

func pairBytes(attempt, nonce byte) []byte {
	buf := make([]byte, 32)
	copy(buf[:16], fillBytes(attempt))
	copy(buf[16:], fillBytes(nonce))
	return buf
}

func pairReader() io.Reader {
	return bytes.NewReader(pairBytes(attemptFill, nonceFill))
}

func pairValues() (attest.ConfigKey, string, attest.Nonce) {
	id := fill16(attemptFill)
	raw := fill16(nonceFill)
	nonce, err := attest.NewNonce(raw[:])
	if err != nil {
		panic(err)
	}
	return attest.NewConfigKeyFromAttemptID(id), base64.RawURLEncoding.EncodeToString(nonce[:]), nonce
}

func mustPayload(t *testing.T, claims attest.Claims) []byte {
	t.Helper()
	raw, err := wire.EncodePayload(claims)
	require.NoError(t, err)
	return raw
}

func mustResponse(t *testing.T, nonce attest.Nonce) []byte {
	t.Helper()
	raw, err := wire.EncodeResponse(nonce)
	require.NoError(t, err)
	return raw
}

func mustChallenge(t *testing.T, key attest.ConfigKey) []byte {
	t.Helper()
	raw, err := wire.EncodeChallenge(key)
	require.NoError(t, err)
	return raw
}

func matchTimeout(d time.Duration) any {
	return mock.MatchedBy(func(ctx context.Context) bool {
		deadline, ok := ctx.Deadline()
		if !ok {
			return false
		}
		remaining := time.Until(deadline)
		const slack = 500 * time.Millisecond
		return remaining > 0 && remaining <= d && remaining >= d-slack
	})
}

func incusCtx() any {
	return matchTimeout(validServerConfig().IncusTimeout)
}

func challengeCtx() any {
	return matchTimeout(validServerConfig().ChallengeResponseTimeout)
}

func detachedCleanupCtx() any {
	return mock.MatchedBy(func(ctx context.Context) bool {
		if ctx.Err() != nil {
			return false
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			return false
		}
		remaining := time.Until(deadline)
		const slack = 500 * time.Millisecond
		d := validServerConfig().CleanupTimeout
		return remaining > 0 && remaining <= d && remaining >= d-slack
	})
}

func newEnv(t *testing.T, cfg config.Server, random io.Reader) *attestEnv {
	t.Helper()
	incus := mocks.NewMockIncus(t)
	exchange := mocks.NewMockExchange(t)
	service, err := New(incus, cfg, testTrustDomain, random)
	require.NoError(t, err)
	return &attestEnv{t: t, incus: incus, exchange: exchange, service: service}
}

func (e *attestEnv) expectPayload(claims attest.Claims) {
	e.t.Helper()
	e.exchange.EXPECT().ReceivePayload(mock.Anything).Return(mustPayload(e.t, claims), nil).Once()
}

func (e *attestEnv) expectHintedLookup(instance attest.Instance) {
	e.t.Helper()
	e.incus.EXPECT().
		Lookup(incusCtx(), instance.Project, instance.Name).
		Return(instance, true, nil).
		Once()
}

func (e *attestEnv) expectChallengeFlow(
	instance attest.Instance,
	recv []byte,
	recvErr error,
) (attest.ConfigKey, string) {
	e.t.Helper()
	key, stored, nonceVal := pairValues()
	e.incus.EXPECT().SetNonce(incusCtx(), instance, key, stored).Return(nil).Once()
	e.exchange.EXPECT().SendChallenge(mock.Anything, mustChallenge(e.t, key)).Return(nil).Once()
	if recv == nil && recvErr == nil {
		recv = mustResponse(e.t, nonceVal)
	}
	e.exchange.EXPECT().ReceiveResponse(challengeCtx()).Return(recv, recvErr).Once()
	return key, stored
}

func (e *attestEnv) expectCleanup(instance attest.Instance, key attest.ConfigKey, err error) {
	e.t.Helper()
	e.incus.EXPECT().UnsetNonce(mock.Anything, instance, key).Return(err).Once()
}

func (e *attestEnv) expectAttributes(instance attest.Instance, selectors []string) {
	e.t.Helper()
	want, err := attest.BuildAttributes(testTrustDomain, instance, selectors)
	require.NoError(e.t, err)
	e.exchange.EXPECT().SendAttributes(mock.Anything, want).Return(nil).Once()
}

func TestNewRejectsInvalidDependencies(t *testing.T) {
	t.Parallel()

	cfg := validServerConfig()
	random := pairReader()
	client := mocks.NewMockIncus(t)

	tests := []struct {
		name   string
		client Incus
		cfg    config.Server
		domain string
		random io.Reader
	}{
		{name: "nil client", cfg: cfg, domain: testTrustDomain, random: random},
		{name: "nil random", client: client, cfg: cfg, domain: testTrustDomain},
		{name: "invalid config", client: client, domain: testTrustDomain, random: random},
		{name: "empty trust domain", client: client, cfg: cfg, random: random},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := New(tt.client, tt.cfg, tt.domain, tt.random)
			require.Error(t, err)
			assert.Nil(t, got)
		})
	}
}

func TestNewCopiesProjectsAndSelectors(t *testing.T) {
	t.Parallel()

	cfg := validServerConfig()
	cfg.Projects = []attest.ProjectName{testProject}
	cfg.UserSelectors = []string{"user.role"}
	env := newEnv(t, cfg, pairReader())

	cfg.Projects[0] = "mutated"
	cfg.UserSelectors[0] = "user.other"

	claims := hintedClaims()
	instance := validInstance()
	env.expectPayload(claims)
	env.expectHintedLookup(instance)
	key, _ := env.expectChallengeFlow(instance, nil, nil)
	env.expectCleanup(instance, key, nil)
	env.expectAttributes(instance, []string{"user.role"})

	require.NoError(t, env.service.Attest(context.Background(), env.exchange))
}

func TestAttestHintedLookupSucceeds(t *testing.T) {
	t.Parallel()

	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	instance := validInstance()
	env.expectPayload(claims)
	env.expectHintedLookup(instance)
	key, stored := env.expectChallengeFlow(instance, nil, nil)
	env.expectCleanup(instance, key, nil)
	env.expectAttributes(instance, validServerConfig().UserSelectors)

	require.NoError(t, env.service.Attest(context.Background(), env.exchange))
	assert.Equal(t, attest.NewConfigKeyFromAttemptID(fill16(attemptFill)), key)
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(fillBytes(nonceFill)), stored)
}

func TestAttestDeniesHintedProjectOutsideAllowlist(t *testing.T) {
	t.Parallel()

	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	claims.Project = "forbidden"
	env.expectPayload(claims)

	err := env.service.Attest(context.Background(), env.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, attest.ErrDenied)
	env.incus.AssertNotCalled(t, "Lookup", mock.Anything, mock.Anything, mock.Anything)
	env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
}

func TestAttestDeniesHintedInstanceNotFound(t *testing.T) {
	t.Parallel()

	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	env.expectPayload(claims)
	env.incus.EXPECT().Lookup(incusCtx(), claims.Project, claims.Name).
		Return(attest.Instance{}, false, nil).Once()

	err := env.service.Attest(context.Background(), env.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, attest.ErrDenied)
	env.incus.AssertNotCalled(t, "SetNonce", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
}

func TestAttestDeniesHintedClaimMismatch(t *testing.T) {
	t.Parallel()

	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	instance := validInstance()
	instance.Location = "other-node"
	env.expectPayload(claims)
	env.expectHintedLookup(instance)

	err := env.service.Attest(context.Background(), env.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, attest.ErrDenied)
	env.incus.AssertNotCalled(t, "SetNonce", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
}

func TestAttestRejectsGuestClaimsBeforeLookup(t *testing.T) {
	t.Parallel()

	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	claims.Type = "container"
	env.expectPayload(claims)

	err := env.service.Attest(context.Background(), env.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, attest.ErrDenied)
	env.incus.AssertNotCalled(t, "Lookup", mock.Anything, mock.Anything, mock.Anything)
	env.incus.AssertNotCalled(t, "SetNonce", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
}

func TestAttestNoHintSearchesEveryProjectIncludingNonmatchingFound(t *testing.T) {
	t.Parallel()

	env := newEnv(t, validServerConfig(), pairReader())
	claims := validClaims()
	mismatch := validInstance()
	mismatch.Project = testProject
	mismatch.UUID = attest.InstanceUUID("11111111-1111-1111-1111-111111111111")
	match := validInstance()
	match.Project = testOpsProject

	env.expectPayload(claims)
	env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testProject), claims.Name).
		Return(mismatch, true, nil).Once()
	env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testOpsProject), claims.Name).
		Return(match, true, nil).Once()
	key, _ := env.expectChallengeFlow(match, nil, nil)
	env.expectCleanup(match, key, nil)
	env.expectAttributes(match, validServerConfig().UserSelectors)

	require.NoError(t, env.service.Attest(context.Background(), env.exchange))
}

func TestAttestNoHintContinuesThroughNotFound(t *testing.T) {
	t.Parallel()

	env := newEnv(t, validServerConfig(), pairReader())
	claims := validClaims()
	match := validInstance()
	match.Project = testOpsProject

	env.expectPayload(claims)
	env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testProject), claims.Name).
		Return(attest.Instance{}, false, nil).Once()
	env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testOpsProject), claims.Name).
		Return(match, true, nil).Once()
	key, _ := env.expectChallengeFlow(match, nil, nil)
	env.expectCleanup(match, key, nil)
	env.expectAttributes(match, validServerConfig().UserSelectors)

	require.NoError(t, env.service.Attest(context.Background(), env.exchange))
}

func TestAttestNoHintCompletesSearchAfterMatch(t *testing.T) {
	t.Parallel()

	env := newEnv(t, validServerConfig(), pairReader())
	claims := validClaims()
	match := validInstance()

	env.expectPayload(claims)
	env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testProject), claims.Name).
		Return(match, true, nil).Once()
	env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testOpsProject), claims.Name).
		Return(attest.Instance{}, false, nil).Once()
	key, _ := env.expectChallengeFlow(match, nil, nil)
	env.expectCleanup(match, key, nil)
	env.expectAttributes(match, validServerConfig().UserSelectors)

	require.NoError(t, env.service.Attest(context.Background(), env.exchange))
}

func TestAttestNoHintAbortsOperationalLookupError(t *testing.T) {
	t.Parallel()

	opErr := errors.New("unauthorized")
	env := newEnv(t, validServerConfig(), pairReader())
	claims := validClaims()
	env.expectPayload(claims)
	env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testProject), claims.Name).
		Return(attest.Instance{}, false, opErr).Once()

	err := env.service.Attest(context.Background(), env.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, opErr)
	require.NotErrorIs(t, err, attest.ErrDenied)
	env.incus.AssertNotCalled(t, "Lookup", mock.Anything, attest.ProjectName(testOpsProject), mock.Anything)
	env.incus.AssertNotCalled(t, "SetNonce", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
}

func TestAttestNoHintDeniesZeroAndMultipleMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		first  attest.Instance
		ok1    bool
		second attest.Instance
		ok2    bool
	}{
		{name: "zero matches", ok1: false, ok2: false},
		{
			name:   "multiple matches",
			first:  validInstance(),
			ok1:    true,
			second: func() attest.Instance { inst := validInstance(); inst.Project = testOpsProject; return inst }(),
			ok2:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newEnv(t, validServerConfig(), pairReader())
			claims := validClaims()
			env.expectPayload(claims)
			env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testProject), claims.Name).
				Return(tt.first, tt.ok1, nil).Once()
			env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testOpsProject), claims.Name).
				Return(tt.second, tt.ok2, nil).Once()

			err := env.service.Attest(context.Background(), env.exchange)
			require.Error(t, err)
			require.ErrorIs(t, err, attest.ErrDenied)
			env.incus.AssertNotCalled(t, "SetNonce", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
		})
	}
}

func TestAttestServerAuthoredDenialsWrapErrDenied(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T) error
	}{
		{
			name: "allowlist",
			setup: func(t *testing.T) error {
				env := newEnv(t, validServerConfig(), pairReader())
				claims := hintedClaims()
				claims.Project = "forbidden"
				env.expectPayload(claims)
				return env.service.Attest(context.Background(), env.exchange)
			},
		},
		{
			name: "hinted not found",
			setup: func(t *testing.T) error {
				env := newEnv(t, validServerConfig(), pairReader())
				claims := hintedClaims()
				env.expectPayload(claims)
				env.incus.EXPECT().Lookup(incusCtx(), claims.Project, claims.Name).
					Return(attest.Instance{}, false, nil)
				return env.service.Attest(context.Background(), env.exchange)
			},
		},
		{
			name: "zero match",
			setup: func(t *testing.T) error {
				env := newEnv(t, validServerConfig(), pairReader())
				claims := validClaims()
				env.expectPayload(claims)
				env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testProject), claims.Name).
					Return(attest.Instance{}, false, nil)
				env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testOpsProject), claims.Name).
					Return(attest.Instance{}, false, nil)
				return env.service.Attest(context.Background(), env.exchange)
			},
		},
		{
			name: "multiple match",
			setup: func(t *testing.T) error {
				env := newEnv(t, validServerConfig(), pairReader())
				claims := validClaims()
				first := validInstance()
				second := validInstance()
				second.Project = testOpsProject
				env.expectPayload(claims)
				env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testProject), claims.Name).
					Return(first, true, nil)
				env.incus.EXPECT().Lookup(incusCtx(), attest.ProjectName(testOpsProject), claims.Name).
					Return(second, true, nil)
				return env.service.Attest(context.Background(), env.exchange)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.setup(t)
			require.Error(t, err)
			require.ErrorIs(t, err, attest.ErrDenied)
		})
	}
}

func TestAttestFailsOnShortRandomReads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
	}{
		{name: "short attempt id", n: 10},
		{name: "short nonce", n: 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newEnv(t, validServerConfig(), bytes.NewReader(make([]byte, tt.n)))
			claims := hintedClaims()
			instance := validInstance()
			env.expectPayload(claims)
			env.expectHintedLookup(instance)

			err := env.service.Attest(context.Background(), env.exchange)
			require.Error(t, err)
			require.NotErrorIs(t, err, attest.ErrDenied)
			env.incus.AssertNotCalled(t, "SetNonce", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
		})
	}
}

func TestAttestCleansUpAfterSetNonceFailure(t *testing.T) {
	t.Parallel()

	setErr := errors.New("transport interrupted")
	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	instance := validInstance()
	key, stored, _ := pairValues()
	env.expectPayload(claims)
	env.expectHintedLookup(instance)
	env.incus.EXPECT().SetNonce(incusCtx(), instance, key, stored).Return(setErr).Once()
	env.expectCleanup(instance, key, nil)

	err := env.service.Attest(context.Background(), env.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, setErr)
	assert.NotContains(t, err.Error(), stored)
	env.exchange.AssertNotCalled(t, "SendChallenge", mock.Anything, mock.Anything)
	env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
}

func TestAttestCleansUpUncertainSetOutcomeOnce(t *testing.T) {
	t.Parallel()

	setErr := errors.New("write outcome unknown")
	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	instance := validInstance()
	key, stored, _ := pairValues()
	env.expectPayload(claims)
	env.expectHintedLookup(instance)
	env.incus.EXPECT().SetNonce(incusCtx(), instance, key, stored).Return(setErr).Once()
	env.expectCleanup(instance, key, nil)

	err := env.service.Attest(context.Background(), env.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, setErr)
	env.incus.AssertNumberOfCalls(t, "UnsetNonce", 1)
	env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
}

func TestAttestCleansUpExactlyOnceAfterCancellation(t *testing.T) {
	t.Parallel()

	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	instance := validInstance()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	env.expectPayload(claims)
	env.expectHintedLookup(instance)
	key, stored, _ := pairValues()
	env.incus.EXPECT().SetNonce(incusCtx(), instance, key, stored).Return(nil).Once()
	env.exchange.EXPECT().SendChallenge(mock.Anything, mustChallenge(t, key)).
		Run(func(context.Context, []byte) { cancel() }).
		Return(nil).Once()
	env.exchange.EXPECT().ReceiveResponse(challengeCtx()).Return(nil, context.Canceled).Once()
	env.incus.EXPECT().UnsetNonce(detachedCleanupCtx(), instance, key).Return(nil).Once()

	err := env.service.Attest(ctx, env.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	env.incus.AssertNumberOfCalls(t, "UnsetNonce", 1)
	env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
}

func TestAttestCleansUpExactlyOnceOnSuccess(t *testing.T) {
	t.Parallel()

	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	instance := validInstance()
	env.expectPayload(claims)
	env.expectHintedLookup(instance)
	key, _ := env.expectChallengeFlow(instance, nil, nil)
	env.expectCleanup(instance, key, nil)
	env.expectAttributes(instance, validServerConfig().UserSelectors)

	require.NoError(t, env.service.Attest(context.Background(), env.exchange))
	env.incus.AssertNumberOfCalls(t, "UnsetNonce", 1)
	env.exchange.AssertNumberOfCalls(t, "SendAttributes", 1)
}

func TestAttestRejectsChallengeFailures(t *testing.T) {
	t.Parallel()

	wrong, err := attest.NewNonce(fillBytes(0xff))
	require.NoError(t, err)

	tests := []struct {
		name    string
		recv    []byte
		recvErr error
		want    error
	}{
		{name: "timeout", recvErr: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "closure", recvErr: io.ErrUnexpectedEOF, want: io.ErrUnexpectedEOF},
		{name: "malformed", recv: []byte(`{"version":1}`), want: wire.ErrInvalid},
		{name: "mismatch", recv: mustResponse(t, wrong), want: attest.ErrDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newEnv(t, validServerConfig(), pairReader())
			claims := hintedClaims()
			instance := validInstance()
			env.expectPayload(claims)
			env.expectHintedLookup(instance)
			key, stored := env.expectChallengeFlow(instance, tt.recv, tt.recvErr)
			env.expectCleanup(instance, key, nil)

			err := env.service.Attest(context.Background(), env.exchange)
			require.Error(t, err)
			require.ErrorIs(t, err, tt.want)
			assert.NotContains(t, err.Error(), stored)
			env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
		})
	}
}

func TestAttestPrimaryClassWinsOverCleanupFailure(t *testing.T) {
	t.Parallel()

	cleanupErr := errors.New("unset failed")
	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	instance := validInstance()
	env.expectPayload(claims)
	env.expectHintedLookup(instance)
	key, stored := env.expectChallengeFlow(instance, nil, context.DeadlineExceeded)
	env.expectCleanup(instance, key, cleanupErr)

	err := env.service.Attest(context.Background(), env.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, cleanupErr)
	assert.Contains(t, err.Error(), cleanupErr.Error())
	assert.NotContains(t, err.Error(), stored)
	env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
}

func TestAttestCleanupFailurePreventsAttributes(t *testing.T) {
	t.Parallel()

	cleanupErr := errors.New("still present")
	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	instance := validInstance()
	env.expectPayload(claims)
	env.expectHintedLookup(instance)
	key, stored := env.expectChallengeFlow(instance, nil, nil)
	env.expectCleanup(instance, key, cleanupErr)

	err := env.service.Attest(context.Background(), env.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, cleanupErr)
	assert.NotContains(t, err.Error(), stored)
	env.exchange.AssertNotCalled(t, "SendAttributes", mock.Anything, mock.Anything)
	env.incus.AssertNumberOfCalls(t, "UnsetNonce", 1)
}

func TestAttestUsesExactKeyAndAPIAttributes(t *testing.T) {
	t.Parallel()

	env := newEnv(t, validServerConfig(), pairReader())
	claims := hintedClaims()
	instance := validInstance()
	instance.Profiles = []string{"default", "web"}
	instance.ExpandedConfig = map[string]string{"user.role": "edge"}
	env.expectPayload(claims)
	env.expectHintedLookup(instance)
	key, stored := env.expectChallengeFlow(instance, nil, nil)
	env.expectCleanup(instance, key, nil)
	env.expectAttributes(instance, validServerConfig().UserSelectors)

	require.NoError(t, env.service.Attest(context.Background(), env.exchange))
	assert.Equal(t, "user.spire.attestor.nonce.11111111111111111111111111111111", string(key))
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(fillBytes(nonceFill)), stored)
}

func TestAttestConcurrentAttemptsUseDistinctKeysAndNonces(t *testing.T) {
	t.Parallel()

	cfg := validServerConfig()
	incus := mocks.NewMockIncus(t)
	service, err := New(
		incus,
		cfg,
		testTrustDomain,
		bytes.NewReader(append(pairBytes(0x01, 0x02), pairBytes(0x03, 0x04)...)),
	)
	require.NoError(t, err)

	claims := hintedClaims()
	instance := validInstance()
	payload := mustPayload(t, claims)
	wantAttrs, err := attest.BuildAttributes(testTrustDomain, instance, cfg.UserSelectors)
	require.NoError(t, err)

	keyA := attest.NewConfigKeyFromAttemptID(fill16(0x01))
	keyB := attest.NewConfigKeyFromAttemptID(fill16(0x03))
	nonceA, err := attest.NewNonce(fillBytes(0x02))
	require.NoError(t, err)
	nonceB, err := attest.NewNonce(fillBytes(0x04))
	require.NoError(t, err)
	storedA := base64.RawURLEncoding.EncodeToString(nonceA[:])
	storedB := base64.RawURLEncoding.EncodeToString(nonceB[:])
	wantPairs := map[attest.ConfigKey]string{keyA: storedA, keyB: storedB}
	respByKey := map[attest.ConfigKey][]byte{
		keyA: mustResponse(t, nonceA),
		keyB: mustResponse(t, nonceB),
	}

	incus.EXPECT().Lookup(incusCtx(), claims.Project, claims.Name).
		Return(instance, true, nil).Times(2)

	var (
		mu         sync.Mutex
		nonceByKey = map[attest.ConfigKey]string{}
	)
	incus.EXPECT().
		SetNonce(incusCtx(), instance, mock.AnythingOfType("attest.ConfigKey"), mock.AnythingOfType("string")).
		Run(func(_ context.Context, _ attest.Instance, key attest.ConfigKey, nonce string) {
			mu.Lock()
			defer mu.Unlock()
			nonceByKey[key] = nonce
		}).
		Return(nil).
		Times(2)
	incus.EXPECT().UnsetNonce(mock.Anything, instance, mock.AnythingOfType("attest.ConfigKey")).
		Return(nil).Times(2)

	run := func() error {
		exchange := mocks.NewMockExchange(t)
		var attemptKey attest.ConfigKey
		var decodeErr error
		exchange.EXPECT().ReceivePayload(mock.Anything).Return(payload, nil).Once()
		exchange.EXPECT().SendChallenge(mock.Anything, mock.Anything).
			Run(func(_ context.Context, raw []byte) {
				attemptKey, decodeErr = wire.DecodeChallenge(raw)
			}).Return(nil).Once()
		exchange.EXPECT().ReceiveResponse(challengeCtx()).
			RunAndReturn(func(context.Context) ([]byte, error) {
				if decodeErr != nil {
					return nil, decodeErr
				}
				return respByKey[attemptKey], nil
			}).Once()
		exchange.EXPECT().SendAttributes(mock.Anything, wantAttrs).Return(nil).Once()
		return service.Attest(context.Background(), exchange)
	}

	var (
		wg      sync.WaitGroup
		runErrs []error
	)
	wg.Add(2)
	for range 2 {
		go func() {
			defer wg.Done()
			err := run()
			mu.Lock()
			runErrs = append(runErrs, err)
			mu.Unlock()
		}()
	}
	wg.Wait()

	require.Len(t, runErrs, 2)
	for _, runErr := range runErrs {
		require.NoError(t, runErr)
	}
	assert.Equal(t, wantPairs, nonceByKey)
}
