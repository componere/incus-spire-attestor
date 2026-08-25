package spire

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	nodeattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/agent/nodeattestor/v1"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"

	"github.com/componere/incus-spire-attestor/internal/config"
)

// AgentPlugin is the SPIRE agent NodeAttestor and Config v1 adapter.
type AgentPlugin struct {
	// UnimplementedNodeAttestorServer embeds the forward-compatible NodeAttestor stub.
	nodeattestorv1.UnimplementedNodeAttestorServer
	// UnimplementedConfigServer embeds the forward-compatible Config stub.
	configv1.UnimplementedConfigServer

	// configureMu serializes Configure from entry through runtime publication.
	configureMu sync.Mutex
	// runtime is the published immutable agent snapshot.
	runtime atomic.Pointer[agentRuntime]
	// build constructs a complete agent runtime from validated configuration.
	build agentRuntimeBuilder
}

// configSource is the shared Config RPC request surface.
type configSource interface {
	// GetCoreConfiguration returns the SPIRE core configuration.
	GetCoreConfiguration() *configv1.CoreConfiguration
	// GetHclConfiguration returns the plugin HCL configuration.
	GetHclConfiguration() string
}

// NewAgentPlugin constructs an AgentPlugin with the production runtime builder.
func NewAgentPlugin() *AgentPlugin {
	return &AgentPlugin{build: buildAgentRuntime}
}

// Validate reports whether the agent HCL and core configuration are usable.
//
// Validate only decodes HCL and runs pure validation. It does not build a
// runtime, read files, or contact Incus. Invalid input returns a non-error
// response with Valid=false and one diagnostic note.
func (p *AgentPlugin) Validate(_ context.Context, req *configv1.ValidateRequest) (*configv1.ValidateResponse, error) {
	resp := &configv1.ValidateResponse{Valid: true}
	if req == nil {
		resp.Valid = false
		resp.Notes = []string{fmt.Errorf("%w: request is required", config.ErrInvalid).Error()}
		return resp, nil
	}
	if _, _, err := decodeValidAgent(req); err != nil {
		resp.Valid = false
		resp.Notes = []string{err.Error()}
	}
	return resp, nil
}

// Configure decodes, validates, and publishes a complete agent runtime.
//
// Configure acquires the plugin mutex at entry and holds it through decode,
// validation, runtime construction, and the atomic swap. A failed build
// leaves the previously published runtime unchanged.
func (p *AgentPlugin) Configure(
	ctx context.Context,
	req *configv1.ConfigureRequest,
) (*configv1.ConfigureResponse, error) {
	p.configureMu.Lock()
	defer p.configureMu.Unlock()
	if req == nil {
		return nil, mapRPCError(fmt.Errorf("%w: request is required", config.ErrInvalid))
	}
	cfg, trustDomain, err := decodeValidAgent(req)
	if err != nil {
		return nil, mapRPCError(err)
	}
	rt, err := p.build(ctx, cfg, trustDomain)
	if err != nil {
		return nil, mapRPCError(err)
	}
	p.runtime.Store(rt)
	return &configv1.ConfigureResponse{}, nil
}

// AidAttestation loads one runtime snapshot and runs guest attestation.
func (p *AgentPlugin) AidAttestation(stream nodeattestorv1.NodeAttestor_AidAttestationServer) error {
	rt := p.runtime.Load()
	if rt == nil {
		return errNotConfigured
	}
	return mapRPCError(rt.service.Attest(stream.Context(), &agentExchange{stream: stream}))
}

// decodeValidAgent decodes and purely validates agent plugin configuration.
func decodeValidAgent(req configSource) (config.Agent, string, error) {
	core := req.GetCoreConfiguration()
	if core == nil {
		return config.Agent{}, "", fmt.Errorf("%w: core configuration is required", config.ErrInvalid)
	}
	cfg, err := config.DecodeAgent(req.GetHclConfiguration())
	if err != nil {
		return config.Agent{}, "", err
	}
	if err := config.ValidateAgent(cfg, core.GetTrustDomain()); err != nil {
		return config.Agent{}, "", err
	}
	return cfg, core.GetTrustDomain(), nil
}
