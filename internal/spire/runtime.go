package spire

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/componere/incus-spire-attestor/internal/agent"
	"github.com/componere/incus-spire-attestor/internal/attest"
	"github.com/componere/incus-spire-attestor/internal/config"
	"github.com/componere/incus-spire-attestor/internal/incus/guest"
	"github.com/componere/incus-spire-attestor/internal/incus/host"
	"github.com/componere/incus-spire-attestor/internal/server"
	"github.com/componere/incus-spire-attestor/internal/wire"
)

// maxCredentialBytes is the maximum accepted TLS credential file size.
const maxCredentialBytes = 65536

// errNotConfigured is returned when an attestation RPC loads a nil runtime.
var errNotConfigured = status.Error(codes.FailedPrecondition, "plugin is not configured")

// agentRuntime is the immutable guest attestation snapshot.
type agentRuntime struct {
	// config is the validated agent plugin configuration.
	config config.Agent
	// service is the guest attestation application.
	service *agent.Service
}

// serverRuntime is the immutable server attestation snapshot.
type serverRuntime struct {
	// config is the validated server plugin configuration.
	config config.Server
	// trustDomain is the SPIRE core trust domain.
	trustDomain string
	// service is the server attestation application.
	service *server.Service
	// closeIdle closes idle connections on the host client.
	closeIdle func()
}

// agentRuntimeBuilder constructs an agent runtime from validated configuration.
type agentRuntimeBuilder func(context.Context, config.Agent, string) (*agentRuntime, error)

// serverRuntimeBuilder constructs a server runtime from validated configuration.
type serverRuntimeBuilder func(context.Context, config.Server, string) (*serverRuntime, error)

// buildAgentRuntime constructs the production guest runtime.
//
// It validates cfg and trustDomain without I/O, then wires the guest
// adapter to the agent service. The published runtime is never mutated.
func buildAgentRuntime(_ context.Context, cfg config.Agent, trustDomain string) (*agentRuntime, error) {
	if err := config.ValidateAgent(cfg, trustDomain); err != nil {
		return nil, err
	}
	service, err := agent.New(guest.New(cfg.Project), cfg.PollTimeout)
	if err != nil {
		return nil, fmt.Errorf("construct agent service: %w", err)
	}
	return &agentRuntime{
		config:  cfg,
		service: service,
	}, nil
}

// buildServerRuntime constructs the production host runtime.
//
// It validates cfg and trustDomain without I/O, then reads the three TLS
// files, connects to Incus, and constructs the server service. Idle
// connections on a new client are closed if construction fails after
// connecting. The published runtime is never mutated.
func buildServerRuntime(ctx context.Context, cfg config.Server, trustDomain string) (*serverRuntime, error) {
	if err := config.ValidateServer(cfg, trustDomain); err != nil {
		return nil, err
	}

	ca, err := readCredential(cfg.TLSCAPath, "tls_ca_path")
	if err != nil {
		return nil, err
	}
	cert, err := readCredential(cfg.TLSCertPath, "tls_cert_path")
	if err != nil {
		return nil, err
	}
	key, err := readCredential(cfg.TLSKeyPath, "tls_key_path")
	if err != nil {
		return nil, err
	}

	cfg.Projects = append([]attest.ProjectName(nil), cfg.Projects...)
	cfg.UserSelectors = append([]string(nil), cfg.UserSelectors...)

	client, err := host.New(ctx, cfg.IncusEndpoint, ca, cert, key)
	if err != nil {
		return nil, err
	}

	service, err := server.New(client, cfg, trustDomain, rand.Reader)
	if err != nil {
		client.CloseIdleConnections()
		return nil, fmt.Errorf("construct server service: %w", err)
	}
	return &serverRuntime{
		config:      cfg,
		trustDomain: trustDomain,
		service:     service,
		closeIdle:   client.CloseIdleConnections,
	}, nil
}

// readCredential reads a bounded TLS credential and names only its path role.
func readCredential(path, role string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", role, err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxCredentialBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", role, err)
	}
	if len(data) > maxCredentialBytes {
		return nil, fmt.Errorf("read %s: exceeds %d bytes", role, maxCredentialBytes)
	}
	return data, nil
}

// mapRPCError maps an application error to a gRPC status.
//
// Context cancellation and deadlines keep their standard codes. Inspectable
// configuration and wire failures become InvalidArgument. Attestation denial
// becomes PermissionDenied. Other operational errors remain Unknown and
// retain their wrapped messages. The mapper never adds payload, challenge,
// response, nonce, or configuration values.
func mapRPCError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	case errors.Is(err, config.ErrInvalid),
		errors.Is(err, wire.ErrInvalid),
		errors.Is(err, wire.ErrUnsupported):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, attest.ErrDenied):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Unknown, err.Error())
	}
}
