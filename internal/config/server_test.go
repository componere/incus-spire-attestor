package config

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

const (
	testEndpoint                        = "https://incus.example.invalid:8443"
	testTLSCAPath                       = "/no/such/incus/ca.pem"
	testTLSCertPath                     = "/no/such/incus/client.pem"
	testTLSKeyPath                      = "/no/such/incus/client-key.pem"
	reservedNonceExact                  = "user.spire.attestor.nonce"
	testReservedNoncePrefix             = "user.spire.attestor.nonce."
	testDefaultIncusTimeout             = 5 * time.Second
	testDefaultChallengeResponseTimeout = 10 * time.Second
	testDefaultCleanupTimeout           = 5 * time.Second
	testMaxProjects                     = 32
	testMaxUserSelectors                = 32
)

func validServer() Server {
	return Server{
		IncusEndpoint:            testEndpoint,
		TLSCAPath:                testTLSCAPath,
		TLSCertPath:              testTLSCertPath,
		TLSKeyPath:               testTLSKeyPath,
		Projects:                 []attest.ProjectName{testProject},
		IncusTimeout:             testDefaultIncusTimeout,
		ChallengeResponseTimeout: testDefaultChallengeResponseTimeout,
		CleanupTimeout:           testDefaultCleanupTimeout,
	}
}

func numberedProjects(n int) []attest.ProjectName {
	projects := make([]attest.ProjectName, n)
	for i := range projects {
		projects[i] = attest.ProjectName(fmt.Sprintf("project-%02d", i+1))
	}
	return projects
}

func numberedUserSelectors(n int) []string {
	selectors := make([]string, n)
	for i := range selectors {
		selectors[i] = fmt.Sprintf("user.sel-%02d", i)
	}
	return selectors
}

func TestDecodeServerAppliesDefaultsAndExplicitValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want Server
	}{
		{
			name: "empty HCL uses duration defaults and leaves required fields empty",
			src:  "",
			want: Server{
				IncusTimeout:             testDefaultIncusTimeout,
				ChallengeResponseTimeout: testDefaultChallengeResponseTimeout,
				CleanupTimeout:           testDefaultCleanupTimeout,
			},
		},
		{
			name: "explicit values including selectors and non-default durations",
			src: `
incus_endpoint = "https://incus.example.invalid:8443"
tls_ca_path    = "/run/secrets/incus/ca.pem"
tls_cert_path  = "/run/secrets/incus/client.pem"
tls_key_path   = "/run/secrets/incus/client-key.pem"
projects       = ["default", "ops"]
user_selectors = ["user.environment", "user.role"]
incus_timeout              = "6s"
challenge_response_timeout = "12s"
cleanup_timeout            = "4s"
`,
			want: Server{
				IncusEndpoint:            testEndpoint,
				TLSCAPath:                "/run/secrets/incus/ca.pem",
				TLSCertPath:              "/run/secrets/incus/client.pem",
				TLSKeyPath:               "/run/secrets/incus/client-key.pem",
				Projects:                 []attest.ProjectName{testProject, "ops"},
				UserSelectors:            []string{"user.environment", "user.role"},
				IncusTimeout:             6 * time.Second,
				ChallengeResponseTimeout: 12 * time.Second,
				CleanupTimeout:           4 * time.Second,
			},
		},
		{
			name: "omitted selectors stay empty while required fields decode",
			src: `
incus_endpoint = "https://incus.example.invalid:8443"
tls_ca_path    = "/no/such/incus/ca.pem"
tls_cert_path  = "/no/such/incus/client.pem"
tls_key_path   = "/no/such/incus/client-key.pem"
projects       = ["default"]
`,
			want: Server{
				IncusEndpoint:            testEndpoint,
				TLSCAPath:                testTLSCAPath,
				TLSCertPath:              testTLSCertPath,
				TLSKeyPath:               testTLSKeyPath,
				Projects:                 []attest.ProjectName{testProject},
				IncusTimeout:             testDefaultIncusTimeout,
				ChallengeResponseTimeout: testDefaultChallengeResponseTimeout,
				CleanupTimeout:           testDefaultCleanupTimeout,
			},
		},
		{
			name: "explicit zero durations decode as zero",
			src: `
incus_timeout              = "0s"
challenge_response_timeout = "0s"
cleanup_timeout            = "0s"
`,
			want: Server{
				IncusTimeout:             0,
				ChallengeResponseTimeout: 0,
				CleanupTimeout:           0,
			},
		},
		{
			name: "explicit negative durations decode for later validation",
			src: `
incus_timeout              = "-1s"
challenge_response_timeout = "-2s"
cleanup_timeout            = "-3s"
`,
			want: Server{
				IncusTimeout:             -1 * time.Second,
				ChallengeResponseTimeout: -2 * time.Second,
				CleanupTimeout:           -3 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeServer(tt.src)
			require.NoError(t, err, "valid server HCL must decode")
			assert.Equal(t, tt.want.IncusEndpoint, got.IncusEndpoint)
			assert.Equal(t, tt.want.TLSCAPath, got.TLSCAPath)
			assert.Equal(t, tt.want.TLSCertPath, got.TLSCertPath)
			assert.Equal(t, tt.want.TLSKeyPath, got.TLSKeyPath)
			assert.Equal(t, tt.want.Projects, got.Projects)
			assert.Equal(t, tt.want.UserSelectors, got.UserSelectors)
			assert.Equal(t, tt.want.IncusTimeout, got.IncusTimeout)
			assert.Equal(t, tt.want.ChallengeResponseTimeout, got.ChallengeResponseTimeout)
			assert.Equal(t, tt.want.CleanupTimeout, got.CleanupTimeout)
		})
	}
}

func TestDecodeServerRejectsInvalidHCL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{name: "malformed incus_timeout", src: `incus_timeout = "not-a-duration"`},
		{name: "malformed challenge_response_timeout", src: `challenge_response_timeout = "not-a-duration"`},
		{name: "malformed cleanup_timeout", src: `cleanup_timeout = "not-a-duration"`},
		{name: "empty incus_timeout", src: `incus_timeout = ""`},
		{name: "empty challenge_response_timeout", src: `challenge_response_timeout = ""`},
		{name: "empty cleanup_timeout", src: `cleanup_timeout = ""`},
		{name: "incus_timeout missing unit", src: `incus_timeout = "5"`},
		{name: "numeric incus_timeout", src: `incus_timeout = 5`},
		{
			name: "unknown attribute",
			src: `
incus_endpoint = "https://incus.example.invalid:8443"
unexpected     = true
`,
		},
		{name: "malformed assignment", src: `incus_endpoint =`},
		{name: "unclosed quoted endpoint", src: `incus_endpoint = "https://incus.example.invalid:8443`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeServer(tt.src)
			require.Error(t, err, "invalid server HCL must be rejected")
			require.ErrorIs(
				t,
				err,
				ErrInvalid,
				"decode syntax, type, unknown-field, and duration failures must wrap ErrInvalid",
			)
			assert.Zero(t, got, "rejected server config must be the zero value")
		})
	}
}

func TestValidateServerAcceptsValidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(Server) Server
	}{
		{
			name: "required fields with duration defaults and no selectors",
			mutate: func(cfg Server) Server {
				return cfg
			},
		},
		{
			name: "explicit durations and selectors",
			mutate: func(cfg Server) Server {
				cfg.UserSelectors = []string{"user.environment", "user.role"}
				cfg.IncusTimeout = 6 * time.Second
				cfg.ChallengeResponseTimeout = 12 * time.Second
				cfg.CleanupTimeout = 4 * time.Second
				return cfg
			},
		},
		{
			name: "one project",
			mutate: func(cfg Server) Server {
				cfg.Projects = numberedProjects(1)
				return cfg
			},
		},
		{
			name: "32 distinct projects",
			mutate: func(cfg Server) Server {
				cfg.Projects = numberedProjects(testMaxProjects)
				return cfg
			},
		},
		{
			name: "32 distinct user selectors",
			mutate: func(cfg Server) Server {
				cfg.UserSelectors = numberedUserSelectors(testMaxUserSelectors)
				return cfg
			},
		},
		{
			name: "selector adjacent to the reserved nonce namespace",
			mutate: func(cfg Server) Server {
				cfg.UserSelectors = []string{"user.spire.attestor.other"}
				return cfg
			},
		},
		{
			name: "uppercase reserved prefix is not reserved",
			mutate: func(cfg Server) Server {
				cfg.UserSelectors = []string{"user.SPIRE.attestor.nonce.deadbeef"}
				return cfg
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateServer(tt.mutate(validServer()), testTrustDomain)
			require.NoError(t, err, "valid server configuration must be accepted")
		})
	}
}

func TestValidateServerAcceptsSyntacticallyValidNonexistentTLSPaths(t *testing.T) {
	t.Parallel()

	cfg := validServer()
	cfg.TLSCAPath = "/definitely/missing/ca.pem"
	cfg.TLSCertPath = "/definitely/missing/client.pem"
	cfg.TLSKeyPath = "/definitely/missing/client-key.pem"

	err := ValidateServer(cfg, testTrustDomain)
	require.NoError(t, err, "pure validation must not read TLS files")
}

func TestValidateServerRejectsInvalidDurations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Server)
	}{
		{
			name:   "zero incus timeout",
			mutate: func(cfg *Server) { cfg.IncusTimeout = 0 },
		},
		{
			name:   "negative incus timeout",
			mutate: func(cfg *Server) { cfg.IncusTimeout = -time.Second },
		},
		{
			name:   "zero challenge-response timeout",
			mutate: func(cfg *Server) { cfg.ChallengeResponseTimeout = 0 },
		},
		{
			name:   "negative challenge-response timeout",
			mutate: func(cfg *Server) { cfg.ChallengeResponseTimeout = -time.Second },
		},
		{
			name:   "zero cleanup timeout",
			mutate: func(cfg *Server) { cfg.CleanupTimeout = 0 },
		},
		{
			name:   "negative cleanup timeout",
			mutate: func(cfg *Server) { cfg.CleanupTimeout = -time.Second },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validServer()
			tt.mutate(&cfg)
			err := ValidateServer(cfg, testTrustDomain)
			require.Error(t, err, "non-positive duration must be rejected")
			assert.ErrorIs(t, err, ErrInvalid)
		})
	}
}

func TestValidateServerRejectsExplicitZeroDurationsAfterDecode(t *testing.T) {
	t.Parallel()

	cfg, err := DecodeServer(`
incus_timeout              = "0s"
challenge_response_timeout = "0s"
cleanup_timeout            = "0s"
`)
	require.NoError(t, err, "explicit zero durations must decode")
	assert.Zero(t, cfg.IncusTimeout)
	assert.Zero(t, cfg.ChallengeResponseTimeout)
	assert.Zero(t, cfg.CleanupTimeout)

	err = ValidateServer(cfg, testTrustDomain)
	require.Error(t, err, "decoded zero durations must fail validation")
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestValidateServerRejectsAbsentRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(*Server)
		trustDomain string
	}{
		{
			name:        "absent endpoint",
			mutate:      func(cfg *Server) { cfg.IncusEndpoint = "" },
			trustDomain: testTrustDomain,
		},
		{
			name:        "absent TLS CA path",
			mutate:      func(cfg *Server) { cfg.TLSCAPath = "" },
			trustDomain: testTrustDomain,
		},
		{
			name:        "absent TLS cert path",
			mutate:      func(cfg *Server) { cfg.TLSCertPath = "" },
			trustDomain: testTrustDomain,
		},
		{
			name:        "absent TLS key path",
			mutate:      func(cfg *Server) { cfg.TLSKeyPath = "" },
			trustDomain: testTrustDomain,
		},
		{
			name:        "nil projects",
			mutate:      func(cfg *Server) { cfg.Projects = nil },
			trustDomain: testTrustDomain,
		},
		{
			name:        "empty projects",
			mutate:      func(cfg *Server) { cfg.Projects = []attest.ProjectName{} },
			trustDomain: testTrustDomain,
		},
		{
			name:        "missing core trust domain",
			mutate:      func(*Server) {},
			trustDomain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validServer()
			tt.mutate(&cfg)
			err := ValidateServer(cfg, tt.trustDomain)
			require.Error(t, err, "missing required server configuration must be rejected")
			assert.ErrorIs(t, err, ErrInvalid)
		})
	}
}

func TestValidateServerEnforcesProjectCountAndUniqueness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		projects []attest.ProjectName
		wantErr  bool
	}{
		{name: "0 projects", projects: nil, wantErr: true},
		{name: "1 project", projects: numberedProjects(1), wantErr: false},
		{name: "32 distinct projects", projects: numberedProjects(testMaxProjects), wantErr: false},
		{name: "33 distinct projects", projects: numberedProjects(testMaxProjects + 1), wantErr: true},
		{name: "empty project name", projects: []attest.ProjectName{testProject, ""}, wantErr: true},
		{name: "duplicate projects", projects: []attest.ProjectName{testProject, testProject}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validServer()
			cfg.Projects = tt.projects
			err := ValidateServer(cfg, testTrustDomain)
			if tt.wantErr {
				require.Error(t, err, "invalid project list must be rejected")
				assert.ErrorIs(t, err, ErrInvalid)
				return
			}
			require.NoError(t, err, "valid project list must be accepted")
		})
	}
}

func TestValidateServerEnforcesSelectorRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		selectors []string
		wantErr   bool
	}{
		{name: "0 selectors", selectors: nil, wantErr: false},
		{name: "32 distinct user selectors", selectors: numberedUserSelectors(testMaxUserSelectors), wantErr: false},
		{name: "33 distinct user selectors", selectors: numberedUserSelectors(testMaxUserSelectors + 1), wantErr: true},
		{name: "duplicate selectors", selectors: []string{"user.role", "user.role"}, wantErr: true},
		{name: "empty selector", selectors: []string{"user.role", ""}, wantErr: true},
		{name: "non-user selector", selectors: []string{"volatile.uuid"}, wantErr: true},
		{name: "bare user selector", selectors: []string{"user"}, wantErr: true},
		{name: "bare user prefix", selectors: []string{"user."}, wantErr: true},
		{name: "exact reserved nonce key", selectors: []string{reservedNonceExact}, wantErr: true},
		{name: "reserved nonce prefix only", selectors: []string{testReservedNoncePrefix}, wantErr: true},
		{
			name:      "reserved nonce prefix with suffix",
			selectors: []string{testReservedNoncePrefix + "0123456789abcdef0123456789abcdef"},
			wantErr:   true,
		},
		{
			name:      "uppercase reserved prefix is not reserved",
			selectors: []string{"user.SPIRE.attestor.nonce"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validServer()
			cfg.UserSelectors = tt.selectors
			err := ValidateServer(cfg, testTrustDomain)
			if tt.wantErr {
				require.Error(t, err, "invalid user selector list must be rejected")
				assert.ErrorIs(t, err, ErrInvalid)
				return
			}
			require.NoError(t, err, "valid user selector list must be accepted")
		})
	}
}

func TestValidateServerJoinsMultipleViolations(t *testing.T) {
	t.Parallel()

	cfg := Server{
		IncusEndpoint:            "",
		TLSCAPath:                "",
		TLSCertPath:              "",
		TLSKeyPath:               "",
		Projects:                 nil,
		UserSelectors:            []string{reservedNonceExact, testReservedNoncePrefix + "aa", "volatile.uuid"},
		IncusTimeout:             0,
		ChallengeResponseTimeout: 0,
		CleanupTimeout:           0,
	}

	err := ValidateServer(cfg, "")
	require.Error(t, err, "multi-violation server configuration must be rejected")
	require.ErrorIs(t, err, ErrInvalid)

	message := err.Error()
	for _, detail := range []string{
		"incus_endpoint",
		"tls_ca_path",
		"tls_cert_path",
		"tls_key_path",
		"projects",
		"user_selectors",
		"incus_timeout",
		"challenge_response_timeout",
		"cleanup_timeout",
		"trust_domain",
	} {
		assert.Containsf(t, message, detail, "joined error must retain %q; got %q", detail, message)
	}
}
