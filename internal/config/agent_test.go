package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

const (
	testTrustDomain    = "example.org"
	testProject        attest.ProjectName = "default"
	defaultPollTimeout                    = 5 * time.Second
)

func TestDecodeAgentAppliesDefaultsAndExplicitValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want Agent
	}{
		{
			name: "empty HCL uses default poll timeout and no project",
			src:  "",
			want: Agent{PollTimeout: defaultPollTimeout},
		},
		{
			name: "explicit project and poll timeout",
			src: `
project      = "default"
poll_timeout = "7s"
`,
			want: Agent{
				Project:     testProject,
				PollTimeout: 7 * time.Second,
			},
		},
		{
			name: "project only keeps the default poll timeout",
			src:  `project = "default"`,
			want: Agent{
				Project:     testProject,
				PollTimeout: defaultPollTimeout,
			},
		},
		{
			name: "poll timeout only leaves the project empty",
			src:  `poll_timeout = "250ms"`,
			want: Agent{PollTimeout: 250 * time.Millisecond},
		},
		{
			name: "explicit zero poll timeout decodes as zero",
			src:  `poll_timeout = "0s"`,
			want: Agent{PollTimeout: 0},
		},
		{
			name: "explicit negative poll timeout decodes for later validation",
			src:  `poll_timeout = "-1s"`,
			want: Agent{PollTimeout: -1 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeAgent(tt.src)
			require.NoError(t, err, "valid agent HCL must decode")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDecodeAgentRejectsInvalidHCL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{name: "malformed poll timeout", src: `poll_timeout = "not-a-duration"`},
		{name: "empty poll timeout", src: `poll_timeout = ""`},
		{name: "poll timeout missing unit", src: `poll_timeout = "5"`},
		{name: "numeric poll timeout", src: `poll_timeout = 5`},
		{
			name: "unknown attribute",
			src: `
project    = "default"
unexpected = true
`,
		},
		{name: "malformed assignment", src: `project =`},
		{name: "unclosed quoted project", src: `project = "default`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeAgent(tt.src)
			require.Error(t, err, "invalid agent HCL must be rejected")
			assert.ErrorIs(t, err, ErrInvalid, "decode syntax, type, unknown-field, and duration failures must wrap ErrInvalid")
			assert.Zero(t, got, "rejected agent config must be the zero value")
		})
	}
}

func TestValidateAgentAcceptsOptionalProject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Agent
	}{
		{
			name: "absent project with default poll timeout",
			cfg:  Agent{PollTimeout: defaultPollTimeout},
		},
		{
			name: "explicit project with default poll timeout",
			cfg: Agent{
				Project:     testProject,
				PollTimeout: defaultPollTimeout,
			},
		},
		{
			name: "sub-second positive poll timeout",
			cfg:  Agent{PollTimeout: time.Millisecond},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateAgent(tt.cfg, testTrustDomain)
			require.NoError(t, err, "valid agent configuration must be accepted")
		})
	}
}

func TestValidateAgentRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         Agent
		trustDomain string
	}{
		{
			name:        "zero poll timeout",
			cfg:         Agent{PollTimeout: 0},
			trustDomain: testTrustDomain,
		},
		{
			name:        "negative poll timeout",
			cfg:         Agent{PollTimeout: -time.Second},
			trustDomain: testTrustDomain,
		},
		{
			name:        "missing core trust domain",
			cfg:         Agent{PollTimeout: defaultPollTimeout},
			trustDomain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateAgent(tt.cfg, tt.trustDomain)
			require.Error(t, err, "invalid agent configuration must be rejected")
			assert.ErrorIs(t, err, ErrInvalid)
		})
	}
}

func TestValidateAgentRejectsExplicitZeroPollTimeoutAfterDecode(t *testing.T) {
	t.Parallel()

	cfg, err := DecodeAgent(`poll_timeout = "0s"`)
	require.NoError(t, err, "explicit zero duration must decode")
	assert.Equal(t, time.Duration(0), cfg.PollTimeout)

	err = ValidateAgent(cfg, testTrustDomain)
	require.Error(t, err, "decoded zero poll timeout must fail validation")
	assert.ErrorIs(t, err, ErrInvalid)
}
