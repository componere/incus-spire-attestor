package spire

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	nodeattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/server/nodeattestor/v1"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"

	"github.com/componere/incus-spire-attestor/internal/config"
)

// ServerPlugin is the SPIRE server NodeAttestor and Config v1 adapter.
type ServerPlugin struct {
	// UnimplementedNodeAttestorServer embeds the forward-compatible NodeAttestor stub.
	nodeattestorv1.UnimplementedNodeAttestorServer
	// UnimplementedConfigServer embeds the forward-compatible Config stub.
	configv1.UnimplementedConfigServer

	// configureMu serializes Configure from entry through runtime publication.
	configureMu sync.Mutex
	// runtime is the published immutable server snapshot.
	runtime atomic.Pointer[serverRuntime]
	// build constructs a complete server runtime from validated configuration.
	build serverRuntimeBuilder
}

// serverConfigSource is the shared Config RPC request surface.
type serverConfigSource interface {
	// GetCoreConfiguration returns the SPIRE core configuration.
	GetCoreConfiguration() *configv1.CoreConfiguration
	// GetHclConfiguration returns the plugin HCL configuration.
	GetHclConfiguration() string
}

// NewServerPlugin constructs a ServerPlugin with the production runtime builder.
func NewServerPlugin() *ServerPlugin {
	return &ServerPlugin{build: buildServerRuntime}
}

// Validate reports whether the server HCL and core configuration are usable.
//
// Validate only decodes HCL and runs pure validation. It does not build a
// runtime, read TLS files, or contact Incus. Invalid input returns a non-error
// response with Valid=false and one diagnostic note.
func (p *ServerPlugin) Validate(_ context.Context, req *configv1.ValidateRequest) (*configv1.ValidateResponse, error) {
	if _, _, err := decodeValidServer(req); err != nil {
		return &configv1.ValidateResponse{
			Valid: false,
			Notes: []string{err.Error()},
		}, nil
	}
	return &configv1.ValidateResponse{Valid: true}, nil
}

// Configure decodes, validates, and publishes a complete server runtime.
//
// Configure acquires the plugin mutex at entry and holds it through decode,
// validation, complete runtime construction, the atomic swap, and old-runtime
// retirement. A failed build leaves the previously published runtime
// unchanged. A successful swap closes idle connections on the superseded
// runtime only.
func (p *ServerPlugin) Configure(ctx context.Context, req *configv1.ConfigureRequest) (*configv1.ConfigureResponse, error) {
	p.configureMu.Lock()
	defer p.configureMu.Unlock()

	cfg, trustDomain, err := decodeValidServer(req)
	if err != nil {
		return nil, mapRPCError(err)
	}
	rt, err := p.build(ctx, cfg, trustDomain)
	if err != nil {
		return nil, mapRPCError(err)
	}
	old := p.runtime.Swap(rt)
	if old != nil && old.closeIdle != nil {
		old.closeIdle()
	}
	return &configv1.ConfigureResponse{}, nil
}

// Attest loads one runtime snapshot and runs host attestation.
func (p *ServerPlugin) Attest(stream nodeattestorv1.NodeAttestor_AttestServer) error {
	rt := p.runtime.Load()
	if rt == nil {
		return errNotConfigured
	}
	return mapRPCError(rt.service.Attest(stream.Context(), &serverExchange{stream: stream}))
}

// decodeValidServer decodes and purely validates server plugin configuration.
func decodeValidServer(req serverConfigSource) (config.Server, string, error) {
	if req == nil {
		return config.Server{}, "", fmt.Errorf("%w: request is required", config.ErrInvalid)
	}
	core := req.GetCoreConfiguration()
	if core == nil {
		return config.Server{}, "", fmt.Errorf("%w: core configuration is required", config.ErrInvalid)
	}
	cfg, err := config.DecodeServer(req.GetHclConfiguration())
	if err != nil {
		return config.Server{}, "", err
	}
	if err := config.ValidateServer(cfg, core.GetTrustDomain()); err != nil {
		return config.Server{}, "", err
	}
	return cfg, core.GetTrustDomain(), nil
}
