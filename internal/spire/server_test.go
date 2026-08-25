package spire

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spiffe/spire-plugin-sdk/pluginsdk"
	"github.com/spiffe/spire-plugin-sdk/plugintest"
	nodeattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/server/nodeattestor/v1"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/componere/incus-spire-attestor/internal/attest"
	"github.com/componere/incus-spire-attestor/internal/config"
	"github.com/componere/incus-spire-attestor/internal/server"
	"github.com/componere/incus-spire-attestor/internal/server/mocks"
	"github.com/componere/incus-spire-attestor/internal/wire"
)

const (
	testTrustDomain  = "example.org"
	testInstanceName = "web-01"
	testLocation     = "node-a"
	testCloudInitID  = "i-abc123"
	testProject      = "default"
	canonicalUUID    = "550e8400-e29b-41d4-a716-446655440000"
	attemptFill      = byte(0x11)
	nonceFill        = byte(0x22)
)

// serverPluginEnv hosts a ServerPlugin behind plugintest clients.
type serverPluginEnv struct {
	t        *testing.T
	plugin   *ServerPlugin
	attestor *nodeattestorv1.NodeAttestorPluginClient
	config   *configv1.ConfigServiceClient
}

func validServerHCL() string {
	return `
incus_endpoint = "https://incus.example.invalid:8443"
tls_ca_path    = "/no/such/incus/ca.pem"
tls_cert_path  = "/no/such/incus/client.pem"
tls_key_path   = "/no/such/incus/client-key.pem"
projects       = ["default"]
user_selectors = ["user.role"]
`
}

func validServerConfig() config.Server {
	return config.Server{
		IncusEndpoint:            "https://incus.example.invalid:8443",
		TLSCAPath:                "/no/such/incus/ca.pem",
		TLSCertPath:              "/no/such/incus/client.pem",
		TLSKeyPath:               "/no/such/incus/client-key.pem",
		Projects:                 []attest.ProjectName{testProject},
		UserSelectors:            []string{"user.role"},
		IncusTimeout:             5 * time.Second,
		ChallengeResponseTimeout: 10 * time.Second,
		CleanupTimeout:           5 * time.Second,
	}
}

func validClaims() attest.Claims {
	return attest.Claims{
		Project:     testProject,
		Name:        attest.InstanceName(testInstanceName),
		UUID:        attest.InstanceUUID(canonicalUUID),
		Type:        attest.InstanceTypeVirtualMachine,
		Location:    testLocation,
		CloudInitID: testCloudInitID,
	}
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

func unusedBuilder(t *testing.T) serverRuntimeBuilder {
	t.Helper()
	return func(context.Context, config.Server, string) (*serverRuntime, error) {
		t.Fatal("runtime builder must not run")
		return nil, errors.New("runtime builder must not run")
	}
}

func newServerRuntime(
	t *testing.T,
	client server.Incus,
	cfg config.Server,
	trustDomain string,
	closeIdle func(),
	random io.Reader,
) *serverRuntime {
	t.Helper()
	if random == nil {
		random = pairReader()
	}
	if closeIdle == nil {
		closeIdle = func() {}
	}
	svc, err := server.New(client, cfg, trustDomain, random)
	require.NoError(t, err)
	return &serverRuntime{
		config:      cfg,
		trustDomain: trustDomain,
		service:     svc,
		closeIdle:   closeIdle,
	}
}

func expectHintedSuccess(t *testing.T, incus *mocks.MockIncus, instance attest.Instance) {
	t.Helper()
	key, stored, _ := pairValues()
	incus.EXPECT().Lookup(mock.Anything, instance.Project, instance.Name).Return(instance, true, nil).Once()
	incus.EXPECT().SetNonce(mock.Anything, instance, key, stored).Return(nil).Once()
	incus.EXPECT().UnsetNonce(mock.Anything, instance, key).Return(nil).Once()
}

func serveServerPlugin(t *testing.T, plugin *ServerPlugin) *serverPluginEnv {
	t.Helper()
	attestor := new(nodeattestorv1.NodeAttestorPluginClient)
	configClient := new(configv1.ConfigServiceClient)
	plugintest.ServeInBackground(t, plugintest.Config{
		PluginServer: nodeattestorv1.NodeAttestorPluginServer(plugin),
		PluginClient: attestor,
		ServiceServers: []pluginsdk.ServiceServer{
			configv1.ConfigServiceServer(plugin),
		},
		ServiceClients: []pluginsdk.ServiceClient{
			configClient,
		},
	})
	require.True(t, attestor.IsInitialized())
	require.True(t, configClient.IsInitialized())
	return &serverPluginEnv{t: t, plugin: plugin, attestor: attestor, config: configClient}
}

func (e *serverPluginEnv) configure(trustDomain, hcl string) error {
	e.t.Helper()
	_, err := e.config.Configure(e.t.Context(), &configv1.ConfigureRequest{
		CoreConfiguration: &configv1.CoreConfiguration{TrustDomain: trustDomain},
		HclConfiguration:  hcl,
	})
	return err
}

func (e *serverPluginEnv) validate(trustDomain, hcl string) (*configv1.ValidateResponse, error) {
	e.t.Helper()
	return e.config.Validate(e.t.Context(), &configv1.ValidateRequest{
		CoreConfiguration: &configv1.CoreConfiguration{TrustDomain: trustDomain},
		HclConfiguration:  hcl,
	})
}

func (e *serverPluginEnv) attest(
	ctx context.Context,
	payload []byte,
	response []byte,
) (*nodeattestorv1.AgentAttributes, error) {
	e.t.Helper()
	stream, err := e.attestor.Attest(ctx)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&nodeattestorv1.AttestRequest{
		Request: &nodeattestorv1.AttestRequest_Payload{Payload: payload},
	}); err != nil {
		return nil, err
	}
	challenge, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	if challenge.GetChallenge() == nil {
		return nil, errors.New("expected challenge")
	}
	if err := stream.Send(&nodeattestorv1.AttestRequest{
		Request: &nodeattestorv1.AttestRequest_ChallengeResponse{ChallengeResponse: response},
	}); err != nil {
		return nil, err
	}
	result, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	attrs := result.GetAgentAttributes()
	if attrs == nil {
		return nil, errors.New("expected agent attributes")
	}
	return attrs, nil
}

func TestValidateDoesNotBuildRuntime(t *testing.T) {
	t.Parallel()

	var builds atomic.Int32
	plugin := &ServerPlugin{build: func(context.Context, config.Server, string) (*serverRuntime, error) {
		builds.Add(1)
		return nil, errors.New("builder must not run during Validate")
	}}
	env := serveServerPlugin(t, plugin)

	resp, err := env.validate(testTrustDomain, validServerHCL())
	require.NoError(t, err)
	assert.True(t, resp.GetValid())
	assert.Empty(t, resp.GetNotes())
	assert.Zero(t, builds.Load())

	resp, err = env.validate(testTrustDomain, "")
	require.NoError(t, err)
	assert.False(t, resp.GetValid())
	require.Len(t, resp.GetNotes(), 1)
	assert.NotEmpty(t, resp.GetNotes()[0])
	assert.Zero(t, builds.Load())
}

func TestValidateGuardsNilRequestAndCore(t *testing.T) {
	t.Parallel()

	plugin := &ServerPlugin{build: unusedBuilder(t)}

	resp, err := plugin.Validate(t.Context(), nil)
	require.NoError(t, err)
	assert.False(t, resp.GetValid())
	require.Len(t, resp.GetNotes(), 1)

	resp, err = plugin.Validate(t.Context(), &configv1.ValidateRequest{
		HclConfiguration: validServerHCL(),
	})
	require.NoError(t, err)
	assert.False(t, resp.GetValid())
	require.Len(t, resp.GetNotes(), 1)
}

func TestAttestFailsClosedWhenUnconfigured(t *testing.T) {
	t.Parallel()

	env := serveServerPlugin(t, &ServerPlugin{build: unusedBuilder(t)})
	stream, err := env.attestor.Attest(t.Context())
	require.NoError(t, err)
	_, err = stream.Recv()
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "plugin is not configured")
}

func TestAttestTranslatesValidFlowAndAttributes(t *testing.T) {
	t.Parallel()

	incus := mocks.NewMockIncus(t)
	instance := validInstance()
	expectHintedSuccess(t, incus, instance)
	_, _, nonce := pairValues()

	plugin := &ServerPlugin{build: func(_ context.Context, cfg config.Server, trustDomain string) (*serverRuntime, error) {
		return newServerRuntime(t, incus, cfg, trustDomain, nil, pairReader()), nil
	}}
	env := serveServerPlugin(t, plugin)
	require.NoError(t, env.configure(testTrustDomain, validServerHCL()))

	got, err := env.attest(t.Context(), mustPayload(t, validClaims()), mustResponse(t, nonce))
	require.NoError(t, err)

	want, err := attest.BuildAttributes(testTrustDomain, instance, validServerConfig().UserSelectors)
	require.NoError(t, err)
	assert.Equal(t, want.AgentID, got.GetSpiffeId())
	assert.Equal(t, want.CanReattest, got.GetCanReattest())
	assert.Equal(t, want.Selectors, got.GetSelectorValues())
}

func TestAttestRejectsMalformedStreamOrdering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(*testing.T, *serverPluginEnv)
	}{
		{
			name: "challenge response as first message",
			run: func(t *testing.T, env *serverPluginEnv) {
				stream, err := env.attestor.Attest(t.Context())
				require.NoError(t, err)
				require.NoError(t, stream.Send(&nodeattestorv1.AttestRequest{
					Request: &nodeattestorv1.AttestRequest_ChallengeResponse{
						ChallengeResponse: []byte(`{"version":1}`),
					},
				}))
				_, err = stream.Recv()
				require.Equal(t, codes.InvalidArgument, status.Code(err))
				assert.NotContains(t, status.Convert(err).Message(), `{"version":1}`)
			},
		},
		{
			name: "payload as second message",
			run: func(t *testing.T, env *serverPluginEnv) {
				stream, err := env.attestor.Attest(t.Context())
				require.NoError(t, err)
				require.NoError(t, stream.Send(&nodeattestorv1.AttestRequest{
					Request: &nodeattestorv1.AttestRequest_Payload{Payload: mustPayload(t, validClaims())},
				}))
				challenge, err := stream.Recv()
				require.NoError(t, err)
				require.NotNil(t, challenge.GetChallenge())
				require.NoError(t, stream.Send(&nodeattestorv1.AttestRequest{
					Request: &nodeattestorv1.AttestRequest_Payload{Payload: mustPayload(t, validClaims())},
				}))
				_, err = stream.Recv()
				require.Equal(t, codes.InvalidArgument, status.Code(err))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			incus := mocks.NewMockIncus(t)
			if tt.name == "payload as second message" {
				expectHintedSuccess(t, incus, validInstance())
			}
			plugin := &ServerPlugin{build: func(_ context.Context, cfg config.Server, trustDomain string) (*serverRuntime, error) {
				return newServerRuntime(t, incus, cfg, trustDomain, nil, pairReader()), nil
			}}
			env := serveServerPlugin(t, plugin)
			require.NoError(t, env.configure(testTrustDomain, validServerHCL()))
			tt.run(t, env)
		})
	}
}

func TestFailedReconfigureRetainsOldRuntime(t *testing.T) {
	t.Parallel()

	incus := mocks.NewMockIncus(t)
	instance := validInstance()
	expectHintedSuccess(t, incus, instance)
	_, _, nonce := pairValues()

	var builds atomic.Int32
	plugin := &ServerPlugin{build: func(_ context.Context, cfg config.Server, trustDomain string) (*serverRuntime, error) {
		if builds.Add(1) == 1 {
			return newServerRuntime(t, incus, cfg, trustDomain, nil, pairReader()), nil
		}
		return nil, errors.New("incus connect failed")
	}}
	env := serveServerPlugin(t, plugin)
	require.NoError(t, env.configure(testTrustDomain, validServerHCL()))

	err := env.configure(testTrustDomain, validServerHCL())
	require.Equal(t, codes.Unknown, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "incus connect failed")

	got, err := env.attest(t.Context(), mustPayload(t, validClaims()), mustResponse(t, nonce))
	require.NoError(t, err)
	want, err := attest.BuildAttributes(testTrustDomain, instance, validServerConfig().UserSelectors)
	require.NoError(t, err)
	assert.Equal(t, want.AgentID, got.GetSpiffeId())
}

func TestSuccessfulReplacementClosesSupersededIdle(t *testing.T) {
	t.Parallel()

	first := mocks.NewMockIncus(t)
	second := mocks.NewMockIncus(t)
	instance := validInstance()
	expectHintedSuccess(t, second, instance)
	_, _, nonce := pairValues()

	var firstIdle atomic.Int32
	var secondIdle atomic.Int32
	var builds atomic.Int32
	plugin := &ServerPlugin{build: func(_ context.Context, cfg config.Server, trustDomain string) (*serverRuntime, error) {
		if builds.Add(1) == 1 {
			return newServerRuntime(t, first, cfg, trustDomain, func() { firstIdle.Add(1) }, pairReader()), nil
		}
		return newServerRuntime(t, second, cfg, trustDomain, func() { secondIdle.Add(1) }, pairReader()), nil
	}}
	env := serveServerPlugin(t, plugin)
	require.NoError(t, env.configure(testTrustDomain, validServerHCL()))
	assert.Zero(t, firstIdle.Load())
	assert.Zero(t, secondIdle.Load())

	require.NoError(t, env.configure(testTrustDomain, validServerHCL()))
	assert.Equal(t, int32(1), firstIdle.Load())
	assert.Zero(t, secondIdle.Load())

	got, err := env.attest(t.Context(), mustPayload(t, validClaims()), mustResponse(t, nonce))
	require.NoError(t, err)
	want, err := attest.BuildAttributes(testTrustDomain, instance, validServerConfig().UserSelectors)
	require.NoError(t, err)
	assert.Equal(t, want.AgentID, got.GetSpiffeId())
	assert.Equal(t, int32(1), firstIdle.Load())
	assert.Zero(t, secondIdle.Load())
}

func TestInFlightAttestUsesOldSnapshotWhileNewUsesReplacement(t *testing.T) {
	t.Parallel()

	oldIncus := mocks.NewMockIncus(t)
	newIncus := mocks.NewMockIncus(t)
	instance := validInstance()
	_, _, nonce := pairValues()

	lookupStarted := make(chan struct{})
	releaseLookup := make(chan struct{})
	oldIncus.EXPECT().
		Lookup(mock.Anything, instance.Project, instance.Name).
		RunAndReturn(func(context.Context, attest.ProjectName, attest.InstanceName) (attest.Instance, bool, error) {
			close(lookupStarted)
			<-releaseLookup
			return instance, true, nil
		}).
		Once()
	key, stored, _ := pairValues()
	oldIncus.EXPECT().SetNonce(mock.Anything, instance, key, stored).Return(nil).Once()
	oldIncus.EXPECT().UnsetNonce(mock.Anything, instance, key).Return(nil).Once()
	expectHintedSuccess(t, newIncus, instance)

	var builds atomic.Int32
	plugin := &ServerPlugin{build: func(_ context.Context, cfg config.Server, trustDomain string) (*serverRuntime, error) {
		n := builds.Add(1)
		client := oldIncus
		domain := "old.example"
		if n > 1 {
			client = newIncus
			domain = "new.example"
		}
		return newServerRuntime(t, client, cfg, domain, nil, pairReader()), nil
	}}
	env := serveServerPlugin(t, plugin)
	require.NoError(t, env.configure(testTrustDomain, validServerHCL()))

	oldDone := make(chan attestResult, 1)
	go func() {
		attrs, err := env.attest(t.Context(), mustPayload(t, validClaims()), mustResponse(t, nonce))
		oldDone <- attestResult{attrs: attrs, err: err}
	}()

	select {
	case <-lookupStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for old snapshot lookup")
	}

	require.NoError(t, env.configure(testTrustDomain, validServerHCL()))
	newAttrs, err := env.attest(t.Context(), mustPayload(t, validClaims()), mustResponse(t, nonce))
	require.NoError(t, err)
	wantNew, err := attest.BuildAttributes("new.example", instance, validServerConfig().UserSelectors)
	require.NoError(t, err)
	assert.Equal(t, wantNew.AgentID, newAttrs.GetSpiffeId())

	close(releaseLookup)
	select {
	case got := <-oldDone:
		require.NoError(t, got.err)
		wantOld, err := attest.BuildAttributes("old.example", instance, validServerConfig().UserSelectors)
		require.NoError(t, err)
		assert.Equal(t, wantOld.AgentID, got.attrs.GetSpiffeId())
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for in-flight old snapshot")
	}
}

func TestConfigureSerializesBuilders(t *testing.T) {
	t.Parallel()

	var inFlight atomic.Int32
	var overlapped atomic.Bool
	var builds atomic.Int32
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})

	plugin := &ServerPlugin{build: func(_ context.Context, cfg config.Server, trustDomain string) (*serverRuntime, error) {
		n := builds.Add(1)
		if inFlight.Add(1) > 1 {
			overlapped.Store(true)
		}
		defer inFlight.Add(-1)
		if n == 1 {
			close(firstEntered)
			<-releaseFirst
		}
		incus := mocks.NewMockIncus(t)
		return newServerRuntime(t, incus, cfg, trustDomain, nil, pairReader()), nil
	}}
	env := serveServerPlugin(t, plugin)

	firstErr := make(chan error, 1)
	go func() { firstErr <- env.configure(testTrustDomain, validServerHCL()) }()

	select {
	case <-firstEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first Configure builder")
	}

	secondErr := make(chan error, 1)
	go func() { secondErr <- env.configure(testTrustDomain, validServerHCL()) }()

	select {
	case err := <-secondErr:
		t.Fatalf("second Configure returned before the first finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)
	require.NoError(t, <-firstErr)
	require.NoError(t, <-secondErr)
	assert.False(t, overlapped.Load())
	assert.Equal(t, int32(2), builds.Load())
}

func TestBlockedSecondRecvReturnsOnDeadlineAndCancellation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code codes.Code
		ctx  func() (context.Context, context.CancelFunc)
	}{
		{
			name: "deadline",
			code: codes.DeadlineExceeded,
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 200*time.Millisecond)
			},
		},
		{
			name: "cancellation",
			code: codes.Canceled,
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			incus := mocks.NewMockIncus(t)
			instance := validInstance()
			key, stored, _ := pairValues()
			incus.EXPECT().Lookup(mock.Anything, instance.Project, instance.Name).Return(instance, true, nil).Once()
			incus.EXPECT().SetNonce(mock.Anything, instance, key, stored).Return(nil).Once()
			incus.EXPECT().UnsetNonce(mock.Anything, instance, key).Return(nil).Once()

			plugin := &ServerPlugin{build: func(_ context.Context, cfg config.Server, trustDomain string) (*serverRuntime, error) {
				return newServerRuntime(t, incus, cfg, trustDomain, nil, pairReader()), nil
			}}
			env := serveServerPlugin(t, plugin)
			require.NoError(t, env.configure(testTrustDomain, validServerHCL()))

			ctx, cancel := tt.ctx()
			defer cancel()
			stream, err := env.attestor.Attest(ctx)
			require.NoError(t, err)
			require.NoError(t, stream.Send(&nodeattestorv1.AttestRequest{
				Request: &nodeattestorv1.AttestRequest_Payload{Payload: mustPayload(t, validClaims())},
			}))
			challenge, err := stream.Recv()
			require.NoError(t, err)
			require.NotNil(t, challenge.GetChallenge())
			if tt.code == codes.Canceled {
				cancel()
			}
			_, err = stream.Recv()
			require.Equal(t, tt.code, status.Code(err))
		})
	}
}

func TestConfigureGuardsNilRequestAndCore(t *testing.T) {
	t.Parallel()

	plugin := &ServerPlugin{build: unusedBuilder(t)}

	_, err := plugin.Configure(t.Context(), nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = plugin.Configure(t.Context(), &configv1.ConfigureRequest{
		HclConfiguration: validServerHCL(),
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestFailedReconfigureDoesNotCloseCurrentIdle(t *testing.T) {
	t.Parallel()

	incus := mocks.NewMockIncus(t)
	var idle atomic.Int32
	var builds atomic.Int32
	plugin := &ServerPlugin{build: func(_ context.Context, cfg config.Server, trustDomain string) (*serverRuntime, error) {
		if builds.Add(1) == 1 {
			return newServerRuntime(t, incus, cfg, trustDomain, func() { idle.Add(1) }, pairReader()), nil
		}
		return nil, errors.New("rebuild failed")
	}}
	env := serveServerPlugin(t, plugin)
	require.NoError(t, env.configure(testTrustDomain, validServerHCL()))
	require.Equal(t, codes.Unknown, status.Code(env.configure(testTrustDomain, validServerHCL())))
	assert.Zero(t, idle.Load())
}

// attestResult is one attestation outcome collected from a background RPC.
type attestResult struct {
	// attrs are the terminal agent attributes.
	attrs *nodeattestorv1.AgentAttributes
	// err is the attestation failure, if any.
	err error
}
