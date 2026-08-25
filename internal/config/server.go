package config

import (
	"fmt"
	"time"

	"github.com/hashicorp/hcl/v2/hclsimple"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// defaultIncusTimeout is the server incus_timeout when the attribute is omitted.
const defaultIncusTimeout = 5 * time.Second

// defaultChallengeResponseTimeout is the server challenge_response_timeout when omitted.
const defaultChallengeResponseTimeout = 10 * time.Second

// defaultCleanupTimeout is the server cleanup_timeout when the attribute is omitted.
const defaultCleanupTimeout = 5 * time.Second

// Server is the decoded server plugin configuration.
type Server struct {
	// IncusEndpoint is the Incus API URL.
	IncusEndpoint string
	// TLSCAPath is the path to the Incus CA certificate.
	TLSCAPath string
	// TLSCertPath is the path to the Incus client certificate.
	TLSCertPath string
	// TLSKeyPath is the path to the Incus client key.
	TLSKeyPath string
	// Projects is the allowlist of Incus projects the server may attest.
	Projects []attest.ProjectName
	// UserSelectors are expanded_config keys emitted as selectors.
	UserSelectors []string
	// IncusTimeout bounds Incus API lookup and mutation work.
	IncusTimeout time.Duration
	// ChallengeResponseTimeout bounds waiting for the agent nonce response.
	ChallengeResponseTimeout time.Duration
	// CleanupTimeout bounds post-attempt nonce-key removal.
	CleanupTimeout time.Duration
}

// rawServer is the HCL shape for server plugin_data.
type rawServer struct {
	// IncusEndpoint is the optional incus_endpoint attribute.
	IncusEndpoint *string `hcl:"incus_endpoint,optional"`
	// TLSCAPath is the optional tls_ca_path attribute.
	TLSCAPath *string `hcl:"tls_ca_path,optional"`
	// TLSCertPath is the optional tls_cert_path attribute.
	TLSCertPath *string `hcl:"tls_cert_path,optional"`
	// TLSKeyPath is the optional tls_key_path attribute.
	TLSKeyPath *string `hcl:"tls_key_path,optional"`
	// Projects is the optional projects list.
	Projects []string `hcl:"projects,optional"`
	// UserSelectors is the optional user_selectors list.
	UserSelectors []string `hcl:"user_selectors,optional"`
	// IncusTimeout is the optional incus_timeout duration string.
	IncusTimeout *string `hcl:"incus_timeout,optional"`
	// ChallengeResponseTimeout is the optional challenge_response_timeout duration string.
	ChallengeResponseTimeout *string `hcl:"challenge_response_timeout,optional"`
	// CleanupTimeout is the optional cleanup_timeout duration string.
	CleanupTimeout *string `hcl:"cleanup_timeout,optional"`
}

// DecodeServer decodes server plugin_data HCL into a Server value.
//
// Missing deadlines use 5s, 10s, and 5s defaults. Unknown attributes are
// rejected. DecodeServer does not validate semantic constraints, read TLS
// files, or contact Incus.
func DecodeServer(src string) (Server, error) {
	var raw rawServer
	if err := hclsimple.Decode("server.hcl", []byte(src), nil, &raw); err != nil {
		return Server{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	incusTimeout, err := parseOptionalDuration(raw.IncusTimeout, defaultIncusTimeout, "incus_timeout")
	if err != nil {
		return Server{}, err
	}
	challengeTimeout, err := parseOptionalDuration(raw.ChallengeResponseTimeout, defaultChallengeResponseTimeout, "challenge_response_timeout")
	if err != nil {
		return Server{}, err
	}
	cleanupTimeout, err := parseOptionalDuration(raw.CleanupTimeout, defaultCleanupTimeout, "cleanup_timeout")
	if err != nil {
		return Server{}, err
	}
	projects := make([]attest.ProjectName, len(raw.Projects))
	for i, name := range raw.Projects {
		projects[i] = attest.ProjectName(name)
	}
	return Server{
		IncusEndpoint:            optionalString(raw.IncusEndpoint),
		TLSCAPath:                optionalString(raw.TLSCAPath),
		TLSCertPath:              optionalString(raw.TLSCertPath),
		TLSKeyPath:               optionalString(raw.TLSKeyPath),
		Projects:                 projects,
		UserSelectors:            raw.UserSelectors,
		IncusTimeout:             incusTimeout,
		ChallengeResponseTimeout: challengeTimeout,
		CleanupTimeout:           cleanupTimeout,
	}, nil
}
