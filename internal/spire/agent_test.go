package spire

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spiffe/spire-plugin-sdk/pluginsdk"
	"github.com/spiffe/spire-plugin-sdk/plugintest"
	nodeattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/agent/nodeattestor/v1"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/componere/incus-spire-attestor/internal/agent"
	"github.com/componere/incus-spire-attestor/internal/agent/mocks"
	"github.com/componere/incus-spire-attestor/internal/attest"
	"github.com/componere/incus-spire-attestor/internal/config"
	"github.com/componere/incus-spire-attestor/internal/wire"
)

const (
	// testTrustDomain is the SPIRE core trust domain used by agent tests.
	testTrustDomain = "example.org"
	// testPollTimeout is the decoded agent poll timeout used by fixtures.
	testPollTimeout = 5 * time.Second
	// testNonceB64 is a valid unpadded base64url nonce fixture.
	testNonceB64 = "3q2-7wARIjNEVWZ3iJmquw"
	// testConfigKey is a valid v1 challenge key fixture.
	testConfigKey = "user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"
	// testValidHCL is valid agent plugin_data.
	testValidHCL = `
project = "default"
poll_timeout = "5s"
`
	// testReplacementHCL is a second valid agent configuration.
	testReplacementHCL = `
project = "other"
poll_timeout = "2s"
`
	// testInvalidHCL is syntactically invalid agent plugin_data.
	testInvalidHCL = `project = `
	// distinctiveChallenge is malformed challenge bytes that must not leak.
	distinctiveChallenge = `{"leak":"challenge-secret-xyz"}`
)

// agentHarness is a plugintest-served AgentPlugin and its SDK clients.
type agentHarness struct {
	// plugin is the served agent plugin.
	plugin *AgentPlugin
	// attestor is the NodeAttestor plugin client.
	attestor *nodeattestorv1.NodeAttestorPluginClient
	// cfg is the Config service client.
	cfg *configv1.ConfigServiceClient
}

// serveAgent serves plugin with build through plugintest.
func serveAgent(t *testing.T, build agentRuntimeBuilder) *agentHarness {
	t.Helper()
	plugin := &AgentPlugin{build: build}
	attestor := new(nodeattestorv1.NodeAttestorPluginClient)
	cfg := new(configv1.ConfigServiceClient)
	plugintest.ServeInBackground(t, plugintest.Config{
		PluginServer: nodeattestorv1.NodeAttestorPluginServer(plugin),
		PluginClient: attestor,
		ServiceServers: []pluginsdk.ServiceServer{
			configv1.ConfigServiceServer(plugin),
		},
		ServiceClients: []pluginsdk.ServiceClient{
			cfg,
		},
	})
	return &agentHarness{plugin: plugin, attestor: attestor, cfg: cfg}
}

// validClaims returns the default guest-claims fixture.
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

// otherClaims returns a distinguishable guest-claims fixture.
func otherClaims() attest.Claims {
	claims := validClaims()
	claims.Name = "vm-02"
	claims.Project = "other"
	return claims
}

// validKey returns the fixture challenge key.
func validKey() attest.ConfigKey {
	key, err := attest.NewConfigKey(testConfigKey)
	if err != nil {
		panic(err)
	}
	return key
}

// mustPayload encodes claims as a v1 payload.
func mustPayload(t *testing.T, claims attest.Claims) []byte {
	t.Helper()
	raw, err := wire.EncodePayload(claims)
	require.NoError(t, err)
	return raw
}

// mustChallenge encodes the fixture challenge key.
func mustChallenge(t *testing.T) []byte {
	t.Helper()
	raw, err := wire.EncodeChallenge(validKey())
	require.NoError(t, err)
	return raw
}

// mustResponse encodes the fixture nonce response.
func mustResponse(t *testing.T) []byte {
	t.Helper()
	nonce, err := wire.ParseNonce(testNonceB64)
	require.NoError(t, err)
	raw, err := wire.EncodeResponse(nonce)
	require.NoError(t, err)
	return raw
}

// mustService constructs an agent Service over evidence.
func mustService(t *testing.T, evidence agent.GuestEvidence, pollTimeout time.Duration) *agent.Service {
	t.Helper()
	svc, err := agent.New(evidence, pollTimeout)
	require.NoError(t, err)
	return svc
}

// happyEvidence stubs a successful guest attestation.
func happyEvidence(t *testing.T, claims attest.Claims) *mocks.MockGuestEvidence {
	t.Helper()
	evidence := mocks.NewMockGuestEvidence(t)
	evidence.EXPECT().Claims(mock.Anything).Return(claims, nil).Once()
	evidence.EXPECT().ReadConfig(mock.Anything, validKey()).Return(testNonceB64, true, nil).Once()
	return evidence
}

// staticRuntime returns a builder that always publishes rt.
func staticRuntime(rt *agentRuntime) agentRuntimeBuilder {
	return func(context.Context, config.Agent, string) (*agentRuntime, error) {
		return rt, nil
	}
}

// configureAgent issues Configure with hcl and the test trust domain.
func configureAgent(t *testing.T, h *agentHarness, hcl string) {
	t.Helper()
	_, err := h.cfg.Configure(t.Context(), &configv1.ConfigureRequest{
		CoreConfiguration: &configv1.CoreConfiguration{TrustDomain: testTrustDomain},
		HclConfiguration:  hcl,
	})
	require.NoError(t, err)
}

// attestOnce runs one AidAttestation exchange and returns the stream messages.
func attestOnce(t *testing.T, h *agentHarness, challenge []byte) (*nodeattestorv1.PayloadOrChallengeResponse, *nodeattestorv1.PayloadOrChallengeResponse, error) {
	t.Helper()
	stream, err := h.attestor.AidAttestation(t.Context())
	require.NoError(t, err)
	payload, err := stream.Recv()
	if err != nil {
		return nil, nil, err
	}
	if err := stream.Send(&nodeattestorv1.Challenge{Challenge: challenge}); err != nil {
		return payload, nil, err
	}
	response, err := stream.Recv()
	return payload, response, err
}

// requireStatus asserts err is a gRPC status with code.
func requireStatus(t *testing.T, err error, code codes.Code) *status.Status {
	t.Helper()
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "expected gRPC status, got %v", err)
	require.Equal(t, code, st.Code(), st.Message())
	return st
}

// TestValidateDoesNotBuildRuntime proves Validate is decode and pure validation only.
func TestValidateDoesNotBuildRuntime(t *testing.T) {
	t.Parallel()

	var built atomic.Bool
	h := serveAgent(t, func(context.Context, config.Agent, string) (*agentRuntime, error) {
		built.Store(true)
		return nil, errors.New("builder must not run during Validate")
	})

	valid, err := h.cfg.Validate(t.Context(), &configv1.ValidateRequest{
		CoreConfiguration: &configv1.CoreConfiguration{TrustDomain: testTrustDomain},
		HclConfiguration:  testValidHCL,
	})
	require.NoError(t, err)
	require.True(t, valid.GetValid())
	assert.Empty(t, valid.GetNotes())

	invalid, err := h.cfg.Validate(t.Context(), &configv1.ValidateRequest{
		CoreConfiguration: &configv1.CoreConfiguration{TrustDomain: testTrustDomain},
		HclConfiguration:  testInvalidHCL,
	})
	require.NoError(t, err)
	require.False(t, invalid.GetValid())
	require.Len(t, invalid.GetNotes(), 1)
	assert.NotEmpty(t, invalid.GetNotes()[0])

	missingCore, err := h.cfg.Validate(t.Context(), &configv1.ValidateRequest{
		HclConfiguration: testValidHCL,
	})
	require.NoError(t, err)
	require.False(t, missingCore.GetValid())
	require.Len(t, missingCore.GetNotes(), 1)

	assert.False(t, built.Load(), "Validate must not invoke the runtime builder")
	assert.Nil(t, h.plugin.runtime.Load(), "Validate must not publish a runtime")
}

// TestAidAttestationFailsClosedWhenUnconfigured proves a nil runtime is rejected.
func TestAidAttestationFailsClosedWhenUnconfigured(t *testing.T) {
	t.Parallel()

	h := serveAgent(t, func(context.Context, config.Agent, string) (*agentRuntime, error) {
		t.Fatal("unconfigured attestation must not build a runtime")
		return nil, nil
	})

	stream, err := h.attestor.AidAttestation(t.Context())
	require.NoError(t, err)
	_, err = stream.Recv()
	st := requireStatus(t, err, codes.FailedPrecondition)
	assert.Equal(t, "plugin is not configured", st.Message())
}

// TestAidAttestationTranslatesPayloadChallengeAndResponse proves the stream mapping.
func TestAidAttestationTranslatesPayloadChallengeAndResponse(t *testing.T) {
	t.Parallel()

	evidence := happyEvidence(t, validClaims())
	h := serveAgent(t, staticRuntime(&agentRuntime{
		config:  config.Agent{Project: "default", PollTimeout: testPollTimeout},
		service: mustService(t, evidence, testPollTimeout),
	}))
	configureAgent(t, h, testValidHCL)

	payload, response, err := attestOnce(t, h, mustChallenge(t))
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.NotNil(t, response)
	assert.Equal(t, mustPayload(t, validClaims()), payload.GetPayload())
	assert.Empty(t, payload.GetChallengeResponse())
	assert.Equal(t, mustResponse(t), response.GetChallengeResponse())
	assert.Empty(t, response.GetPayload())
}

// TestAidAttestationRejectsNilAndMalformedChallenge proves challenge translation failures.
func TestAidAttestationRejectsNilAndMalformedChallenge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		challenge []byte
	}{
		{name: "nil challenge field"},
		{name: "malformed challenge bytes", challenge: []byte(distinctiveChallenge)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			evidence := mocks.NewMockGuestEvidence(t)
			evidence.EXPECT().Claims(mock.Anything).Return(validClaims(), nil).Once()
			h := serveAgent(t, staticRuntime(&agentRuntime{
				config:  config.Agent{PollTimeout: testPollTimeout},
				service: mustService(t, evidence, testPollTimeout),
			}))
			configureAgent(t, h, testValidHCL)

			_, _, err := attestOnce(t, h, tt.challenge)
			st := requireStatus(t, err, codes.InvalidArgument)
			assert.NotContains(t, st.Message(), distinctiveChallenge)
			assert.NotContains(t, st.Message(), testConfigKey)
			assert.NotContains(t, st.Message(), testNonceB64)
			evidence.AssertNotCalled(t, "ReadConfig", mock.Anything, mock.Anything)
		})
	}
}

// TestConfigureFailedRebuildKeepsOldRuntime proves a failed swap is a no-op.
func TestConfigureFailedRebuildKeepsOldRuntime(t *testing.T) {
	t.Parallel()

	evidence := happyEvidence(t, validClaims())
	var calls atomic.Int32
	h := serveAgent(t, func(context.Context, config.Agent, string) (*agentRuntime, error) {
		if calls.Add(1) == 1 {
			return &agentRuntime{
				config:  config.Agent{Project: "default", PollTimeout: testPollTimeout},
				service: mustService(t, evidence, testPollTimeout),
			}, nil
		}
		return nil, errors.New("rebuild failed")
	})
	configureAgent(t, h, testValidHCL)

	_, err := h.cfg.Configure(t.Context(), &configv1.ConfigureRequest{
		CoreConfiguration: &configv1.CoreConfiguration{TrustDomain: testTrustDomain},
		HclConfiguration:  testReplacementHCL,
	})
	st := requireStatus(t, err, codes.Unknown)
	assert.Contains(t, st.Message(), "rebuild failed")

	payload, response, err := attestOnce(t, h, mustChallenge(t))
	require.NoError(t, err)
	assert.Equal(t, mustPayload(t, validClaims()), payload.GetPayload())
	assert.Equal(t, mustResponse(t), response.GetChallengeResponse())
}

// TestConfigureSuccessfulSwapUsesNewRuntime proves later calls use the new snapshot.
func TestConfigureSuccessfulSwapUsesNewRuntime(t *testing.T) {
	t.Parallel()

	first := happyEvidence(t, validClaims())
	second := happyEvidence(t, otherClaims())
	var calls atomic.Int32
	h := serveAgent(t, func(_ context.Context, cfg config.Agent, _ string) (*agentRuntime, error) {
		n := calls.Add(1)
		if n == 1 {
			return &agentRuntime{
				config:  cfg,
				service: mustService(t, first, cfg.PollTimeout),
			}, nil
		}
		return &agentRuntime{
			config:  cfg,
			service: mustService(t, second, cfg.PollTimeout),
		}, nil
	})
	configureAgent(t, h, testValidHCL)
	payload, _, err := attestOnce(t, h, mustChallenge(t))
	require.NoError(t, err)
	assert.Equal(t, mustPayload(t, validClaims()), payload.GetPayload())

	configureAgent(t, h, testReplacementHCL)
	payload, response, err := attestOnce(t, h, mustChallenge(t))
	require.NoError(t, err)
	assert.Equal(t, mustPayload(t, otherClaims()), payload.GetPayload())
	assert.Equal(t, mustResponse(t), response.GetChallengeResponse())
}

// TestAidAttestationRetainsInFlightSnapshotAcrossSwap proves one load per RPC.
func TestAidAttestationRetainsInFlightSnapshotAcrossSwap(t *testing.T) {
	t.Parallel()

	oldStarted := make(chan struct{})
	oldRelease := make(chan struct{})
	oldEvidence := mocks.NewMockGuestEvidence(t)
	oldEvidence.EXPECT().Claims(mock.Anything).RunAndReturn(func(context.Context) (attest.Claims, error) {
		close(oldStarted)
		<-oldRelease
		return validClaims(), nil
	}).Once()
	oldEvidence.EXPECT().ReadConfig(mock.Anything, validKey()).Return(testNonceB64, true, nil).Once()

	newEvidence := happyEvidence(t, otherClaims())
	var calls atomic.Int32
	h := serveAgent(t, func(_ context.Context, cfg config.Agent, _ string) (*agentRuntime, error) {
		if calls.Add(1) == 1 {
			return &agentRuntime{
				config:  cfg,
				service: mustService(t, oldEvidence, cfg.PollTimeout),
			}, nil
		}
		return &agentRuntime{
			config:  cfg,
			service: mustService(t, newEvidence, cfg.PollTimeout),
		}, nil
	})
	configureAgent(t, h, testValidHCL)

	type attestResult struct {
		payload *nodeattestorv1.PayloadOrChallengeResponse
		err     error
	}
	oldDone := make(chan attestResult, 1)
	go func() {
		payload, _, err := attestOnce(t, h, mustChallenge(t))
		oldDone <- attestResult{payload: payload, err: err}
	}()

	select {
	case <-oldStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for in-flight attestation to load the old runtime")
	}

	configureAgent(t, h, testReplacementHCL)
	newPayload, _, err := attestOnce(t, h, mustChallenge(t))
	require.NoError(t, err)
	assert.Equal(t, mustPayload(t, otherClaims()), newPayload.GetPayload())

	close(oldRelease)
	old := <-oldDone
	require.NoError(t, old.err)
	assert.Equal(t, mustPayload(t, validClaims()), old.payload.GetPayload())
}

// TestConfigureSerializesConcurrentCalls proves Configure holds the mutex for the full build.
func TestConfigureSerializesConcurrentCalls(t *testing.T) {
	t.Parallel()

	var inFlight atomic.Int32
	var overlapped atomic.Bool
	started := make(chan struct{})
	var once sync.Once
	h := serveAgent(t, func(context.Context, config.Agent, string) (*agentRuntime, error) {
		if inFlight.Add(1) > 1 {
			overlapped.Store(true)
		}
		once.Do(func() { close(started) })
		time.Sleep(100 * time.Millisecond)
		inFlight.Add(-1)
		return &agentRuntime{
			config:  config.Agent{PollTimeout: testPollTimeout},
			service: mustService(t, mocks.NewMockGuestEvidence(t), testPollTimeout),
		}, nil
	})

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	start := func(hcl string) {
		wg.Go(func() {
			_, err := h.cfg.Configure(t.Context(), &configv1.ConfigureRequest{
				CoreConfiguration: &configv1.CoreConfiguration{TrustDomain: testTrustDomain},
				HclConfiguration:  hcl,
			})
			errs <- err
		})
	}
	start(testValidHCL)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the first Configure to enter the builder")
	}
	start(testReplacementHCL)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.False(t, overlapped.Load(), "concurrent Configure calls must not overlap in the builder")
}

// TestConfigureRejectsInvalidInputWithoutPublishing proves decode failures stay fail-closed.
func TestConfigureRejectsInvalidInputWithoutPublishing(t *testing.T) {
	t.Parallel()

	var built atomic.Bool
	h := serveAgent(t, func(context.Context, config.Agent, string) (*agentRuntime, error) {
		built.Store(true)
		return nil, errors.New("builder must not run for invalid Configure")
	})

	_, err := h.cfg.Configure(t.Context(), &configv1.ConfigureRequest{
		CoreConfiguration: &configv1.CoreConfiguration{TrustDomain: testTrustDomain},
		HclConfiguration:  testInvalidHCL,
	})
	requireStatus(t, err, codes.InvalidArgument)
	assert.False(t, built.Load())
	assert.Nil(t, h.plugin.runtime.Load())
}
