package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/componere/incus-spire-attestor/internal/agent/mocks"
	"github.com/componere/incus-spire-attestor/internal/attest"
	"github.com/componere/incus-spire-attestor/internal/wire"
)

const (
	testPollTimeout = time.Second
	testNonceB64    = "3q2-7wARIjNEVWZ3iJmquw"
	testConfigKey   = "user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"
)

type timeoutTrueError struct{}

func (timeoutTrueError) Error() string { return "guest timeout" }
func (timeoutTrueError) Timeout() bool { return true }

type timeoutFalseError struct{}

func (timeoutFalseError) Error() string { return "guest timeout-shaped" }
func (timeoutFalseError) Timeout() bool { return false }

type temporaryTrueError struct{}

func (temporaryTrueError) Error() string   { return "guest temporary" }
func (temporaryTrueError) Temporary() bool { return true }

type temporaryFalseError struct{}

func (temporaryFalseError) Error() string   { return "guest temporary-shaped" }
func (temporaryFalseError) Temporary() bool { return false }

type canceledTemporaryError struct{}

func (canceledTemporaryError) Error() string   { return "canceled" }
func (canceledTemporaryError) Temporary() bool { return true }
func (canceledTemporaryError) Unwrap() error   { return context.Canceled }

type deadlineTemporaryError struct{}

func (deadlineTemporaryError) Error() string { return "deadline" }
func (deadlineTemporaryError) Timeout() bool { return true }
func (deadlineTemporaryError) Unwrap() error { return context.DeadlineExceeded }

// leakingReadError is a guest read failure whose text includes the key and nonce.
type leakingReadError struct {
	// cause is the inspectable wrapped error.
	cause error
}

func (e leakingReadError) Error() string {
	return testConfigKey + "=" + testNonceB64
}

func (e leakingReadError) Unwrap() error {
	return e.cause
}

type testContext struct {
	evidence *mocks.MockGuestEvidence
	exchange *mocks.MockExchange
	service  *Service
	delays   []time.Duration
}

func validClaims() attest.Claims {
	return attest.Claims{
		Project:     "default",
		Name:        "vm-01",
		UUID:        "550e8400-e29b-41d4-a716-446655440000",
		Type:        attest.InstanceTypeVirtualMachine,
		Location:    "member-01",
		CloudInitID: "i-0123456789abcdef",
	}
}

func validKey() attest.ConfigKey {
	key, err := attest.NewConfigKey(testConfigKey)
	if err != nil {
		panic(err)
	}
	return key
}

func mustChallenge(t *testing.T) []byte {
	t.Helper()
	raw, err := wire.EncodeChallenge(validKey())
	require.NoError(t, err)
	return raw
}

func mustPayload(t *testing.T) []byte {
	t.Helper()
	raw, err := wire.EncodePayload(validClaims())
	require.NoError(t, err)
	return raw
}

func mustResponse(t *testing.T) []byte {
	t.Helper()
	nonce, err := wire.ParseNonce(testNonceB64)
	require.NoError(t, err)
	raw, err := wire.EncodeResponse(nonce)
	require.NoError(t, err)
	return raw
}

func instantWait(ctx context.Context, _ time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func newTestContext(t *testing.T, pollTimeout time.Duration, wait waitFunc) *testContext {
	t.Helper()
	if wait == nil {
		wait = instantWait
	}
	tc := &testContext{
		evidence: mocks.NewMockGuestEvidence(t),
		exchange: mocks.NewMockExchange(t),
	}
	service, err := newService(tc.evidence, pollTimeout, func(ctx context.Context, delay time.Duration) error {
		tc.delays = append(tc.delays, delay)
		return wait(ctx, delay)
	})
	require.NoError(t, err)
	tc.service = service
	return tc
}

func expectHappyClaimsAndChallenge(ctx context.Context, t *testing.T, tc *testContext) {
	t.Helper()
	tc.evidence.EXPECT().Claims(ctx).Return(validClaims(), nil).Once()
	tc.exchange.EXPECT().SendPayload(ctx, mustPayload(t)).Return(nil).Once()
	tc.exchange.EXPECT().ReceiveChallenge(ctx).Return(mustChallenge(t), nil).Once()
}

func TestNewRejectsNilEvidenceAndNonPositiveTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		evidence    GuestEvidence
		pollTimeout time.Duration
	}{
		{name: "nil evidence", pollTimeout: time.Second},
		{name: "zero timeout", evidence: mocks.NewMockGuestEvidence(t), pollTimeout: 0},
		{name: "negative timeout", evidence: mocks.NewMockGuestEvidence(t), pollTimeout: -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := New(tt.evidence, tt.pollTimeout)
			require.Error(t, err)
			assert.Nil(t, got)
		})
	}
}

func TestAttestSendsPayloadBeforeReceivingChallenge(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	tc := newTestContext(t, testPollTimeout, instantWait)
	sent := false
	tc.evidence.EXPECT().Claims(parent).Return(validClaims(), nil).Once()
	tc.exchange.EXPECT().SendPayload(parent, mustPayload(t)).Run(func(context.Context, []byte) {
		sent = true
	}).Return(nil).Once()
	tc.exchange.EXPECT().ReceiveChallenge(parent).RunAndReturn(func(context.Context) ([]byte, error) {
		assert.True(t, sent, "payload must be sent before the challenge is received")
		return mustChallenge(t), nil
	}).Once()
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).Return(testNonceB64, true, nil).Once()
	tc.exchange.EXPECT().SendResponse(parent, mustResponse(t)).Return(nil).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.NoError(t, err)
}

func TestAttestRejectsNonVMBeforeSend(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	tc := newTestContext(t, testPollTimeout, instantWait)
	claims := validClaims()
	claims.Type = "container"
	tc.evidence.EXPECT().Claims(parent).Return(claims, nil).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, attest.ErrDenied)
}

func TestAttestRejectsMalformedChallengeBeforeConfigRead(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	tc := newTestContext(t, testPollTimeout, instantWait)
	tc.evidence.EXPECT().Claims(parent).Return(validClaims(), nil).Once()
	tc.exchange.EXPECT().SendPayload(parent, mustPayload(t)).Return(nil).Once()
	tc.exchange.EXPECT().ReceiveChallenge(parent).Return([]byte("{"), nil).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, wire.ErrInvalid)
}

func TestAttestRejectsWrongChallengeBeforeConfigRead(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	tc := newTestContext(t, testPollTimeout, instantWait)
	tc.evidence.EXPECT().Claims(parent).Return(validClaims(), nil).Once()
	tc.exchange.EXPECT().SendPayload(parent, mustPayload(t)).Return(nil).Once()
	tc.exchange.EXPECT().ReceiveChallenge(parent).Return([]byte(`{
		"version": 2,
		"challenge": {
			"type": "incus-config-nonce",
			"version": 1,
			"data": {"config_key": "user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}
		}
	}`), nil).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, wire.ErrUnsupported)
}

func TestAttestPollsAbsentAndTransientReadsThenSucceeds(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	tc := newTestContext(t, testPollTimeout, instantWait)
	expectHappyClaimsAndChallenge(parent, t, tc)
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return("", false, nil).Once()
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return("", false, timeoutTrueError{}).Once()
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return("", false, temporaryTrueError{}).Once()
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return(testNonceB64, true, nil).Once()
	tc.exchange.EXPECT().SendResponse(parent, mustResponse(t)).Return(nil).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{25 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond}, tc.delays)
}

func TestAttestFailsImmediatelyWhenTimeoutOrTemporaryMethodsAreFalse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{name: "timeout method false", err: timeoutFalseError{}},
		{name: "temporary method false", err: temporaryFalseError{}},
		{name: "permanent error", err: errors.New("guest refused")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parent := context.Background()
			tc := newTestContext(t, testPollTimeout, instantWait)
			expectHappyClaimsAndChallenge(parent, t, tc)
			tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
				Return("3q2-7wARIjNEVWZ3iJmquw", true, tt.err).Once()

			err := tc.service.Attest(parent, tc.exchange)
			require.Error(t, err)
			require.ErrorIs(t, err, tt.err)
			assert.NotContains(t, err.Error(), testNonceB64)
		})
	}
}

func TestAttestContextErrorsTakePrecedenceOverRetryableMethods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		err   error
		cause error
	}{
		{name: "canceled with temporary true", err: canceledTemporaryError{}, cause: context.Canceled},
		{name: "deadline with timeout true", err: deadlineTemporaryError{}, cause: context.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parent := context.Background()
			tc := newTestContext(t, testPollTimeout, instantWait)
			expectHappyClaimsAndChallenge(parent, t, tc)
			tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
				Return("", false, tt.err).Once()

			err := tc.service.Attest(parent, tc.exchange)
			require.Error(t, err)
			require.ErrorIs(t, err, tt.cause)
			assert.Empty(t, tc.delays)
		})
	}
}

func TestAttestCapsBackoffAt250ms(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	tc := newTestContext(t, testPollTimeout, instantWait)
	expectHappyClaimsAndChallenge(parent, t, tc)
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return("", false, nil).Times(6)
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return(testNonceB64, true, nil).Once()
	tc.exchange.EXPECT().SendResponse(parent, mustResponse(t)).Return(nil).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{
		25 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		250 * time.Millisecond,
		250 * time.Millisecond,
	}, tc.delays)
}

func TestAttestPollTimeoutIsScopedAfterChallenge(t *testing.T) {
	t.Parallel()

	parent, cancel := context.WithTimeout(context.Background(), time.Hour)
	t.Cleanup(cancel)
	pollTimeout := 50 * time.Millisecond
	tc := newTestContext(t, pollTimeout, instantWait)

	var claimsDeadline, sendDeadline, recvDeadline, readDeadline time.Time
	var claimsHasDeadline, sendHasDeadline, recvHasDeadline, readHasDeadline bool
	tc.evidence.EXPECT().Claims(mock.Anything).RunAndReturn(func(reqCtx context.Context) (attest.Claims, error) {
		claimsDeadline, claimsHasDeadline = reqCtx.Deadline()
		return validClaims(), nil
	}).Once()
	tc.exchange.EXPECT().
		SendPayload(mock.Anything, mustPayload(t)).
		RunAndReturn(func(reqCtx context.Context, _ []byte) error {
			sendDeadline, sendHasDeadline = reqCtx.Deadline()
			return nil
		}).
		Once()
	tc.exchange.EXPECT().ReceiveChallenge(mock.Anything).RunAndReturn(func(reqCtx context.Context) ([]byte, error) {
		recvDeadline, recvHasDeadline = reqCtx.Deadline()
		return mustChallenge(t), nil
	}).Once()
	tc.evidence.EXPECT().
		ReadConfig(mock.Anything, validKey()).
		RunAndReturn(func(reqCtx context.Context, _ attest.ConfigKey) (string, bool, error) {
			readDeadline, readHasDeadline = reqCtx.Deadline()
			return testNonceB64, true, nil
		}).
		Once()
	tc.exchange.EXPECT().SendResponse(mock.Anything, mustResponse(t)).Return(nil).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.NoError(t, err)

	parentDeadline, parentHasDeadline := parent.Deadline()
	require.True(t, parentHasDeadline)
	require.True(t, claimsHasDeadline, "exchange operations use the caller context")
	require.True(t, sendHasDeadline, "exchange operations use the caller context")
	require.True(t, recvHasDeadline, "exchange operations use the caller context")
	assert.Equal(t, parentDeadline, claimsDeadline)
	assert.Equal(t, parentDeadline, sendDeadline)
	assert.Equal(t, parentDeadline, recvDeadline)
	require.True(t, readHasDeadline, "config reads use the poll timeout context")
	assert.True(t, readDeadline.Before(parentDeadline), "poll deadline must be earlier than the caller deadline")
	remaining := time.Until(readDeadline)
	assert.Greater(t, remaining, time.Duration(0))
	assert.LessOrEqual(t, remaining, pollTimeout)
}

func TestAttestWrapsAdapterReturnedDeadlineExceeded(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	tc := newTestContext(t, testPollTimeout, instantWait)
	expectHappyClaimsAndChallenge(parent, t, tc)
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return("", false, context.DeadlineExceeded).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Empty(t, tc.delays)
}

func TestAttestWrapsParentCancellationDuringPoll(t *testing.T) {
	t.Parallel()

	parent, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	tc := newTestContext(t, testPollTimeout, instantWait)
	expectHappyClaimsAndChallenge(parent, t, tc)
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).RunAndReturn(
		func(ctx context.Context, _ attest.ConfigKey) (string, bool, error) {
			cancel()
			return "", false, ctx.Err()
		},
	).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, tc.delays)
}

func TestAttestFailsImmediatelyOnPermanentReadFailure(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	permanent := errors.New("permission denied")
	tc := newTestContext(t, testPollTimeout, instantWait)
	expectHappyClaimsAndChallenge(parent, t, tc)
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return("", false, permanent).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, permanent)
	assert.Empty(t, tc.delays)
}

func TestAttestMalformedFoundValueFailsOnceWithoutLeakingNonce(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	secret := "secret-nonce-value-do-not-leak"
	tc := newTestContext(t, testPollTimeout, instantWait)
	expectHappyClaimsAndChallenge(parent, t, tc)
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return(secret, true, nil).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, wire.ErrInvalid)
	assert.NotContains(t, err.Error(), secret)
	assert.NotContains(t, err.Error(), testNonceB64)
	assert.Empty(t, tc.delays)
}

func TestAttestWaitExhaustionWrapsPollContextCause(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	tc := newTestContext(t, time.Nanosecond, func(ctx context.Context, _ time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	})
	expectHappyClaimsAndChallenge(parent, t, tc)
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return("", false, nil).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestAttestRequiresExchange(t *testing.T) {
	t.Parallel()

	service, err := New(mocks.NewMockGuestEvidence(t), time.Second)
	require.NoError(t, err)

	err = service.Attest(context.Background(), nil)
	require.Error(t, err)
}

func TestAttestDoesNotIncludeConfigKeyInErrors(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	cause := errors.New("guest refused")
	leaking := leakingReadError{cause: cause}
	tc := newTestContext(t, testPollTimeout, instantWait)
	expectHappyClaimsAndChallenge(parent, t, tc)
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return("", false, leaking).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, cause)
	assert.NotContains(t, err.Error(), testConfigKey)
	assert.NotContains(t, err.Error(), testNonceB64)
}

func TestAttestWrapsInjectedNonContextWaitError(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	waitErr := errors.New("clock stopped")
	tc := newTestContext(t, testPollTimeout, func(ctx context.Context, _ time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return waitErr
	})
	expectHappyClaimsAndChallenge(parent, t, tc)
	tc.evidence.EXPECT().ReadConfig(mock.Anything, validKey()).
		Return("", false, nil).Once()

	err := tc.service.Attest(parent, tc.exchange)
	require.Error(t, err)
	require.ErrorIs(t, err, waitErr)
}

func TestWaitDurationElapsedWait(t *testing.T) {
	t.Parallel()

	err := waitDuration(context.Background(), 0)
	require.NoError(t, err)
}

func TestWaitDurationAlreadyCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitDuration(ctx, time.Hour)
	require.ErrorIs(t, err, context.Canceled)
}

func TestWaitDurationMidWaitCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- waitDuration(ctx, time.Hour)
	}()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}
