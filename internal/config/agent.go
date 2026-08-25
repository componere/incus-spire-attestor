package config

import (
	"fmt"
	"time"

	"github.com/hashicorp/hcl/v2/hclsimple"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// defaultPollTimeout is the agent poll_timeout when the attribute is omitted.
const defaultPollTimeout = 5 * time.Second

// Agent is the decoded agent plugin configuration.
type Agent struct {
	// Project is the optional guest project hint.
	Project attest.ProjectName
	// PollTimeout is how long the agent polls for a challenge key.
	PollTimeout time.Duration
}

// rawAgent is the HCL shape for agent plugin_data.
type rawAgent struct {
	// Project is the optional project attribute.
	Project *string `hcl:"project,optional"`
	// PollTimeout is the optional poll_timeout duration string.
	PollTimeout *string `hcl:"poll_timeout,optional"`
}

// DecodeAgent decodes agent plugin_data HCL into an Agent value.
//
// Missing poll_timeout uses a 5s default. Unknown attributes are rejected.
// DecodeAgent does not validate semantic constraints and does not read the
// filesystem.
func DecodeAgent(src string) (Agent, error) {
	var raw rawAgent
	if err := hclsimple.Decode("agent.hcl", []byte(src), nil, &raw); err != nil {
		return Agent{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	pollTimeout, err := parseOptionalDuration(raw.PollTimeout, defaultPollTimeout, "poll_timeout")
	if err != nil {
		return Agent{}, err
	}
	return Agent{
		Project:     attest.ProjectName(optionalString(raw.Project)),
		PollTimeout: pollTimeout,
	}, nil
}

// optionalString returns the pointed-to string, or "" when raw is omitted.
func optionalString(raw *string) string {
	if raw == nil {
		return ""
	}
	return *raw
}

// parseOptionalDuration parses a raw HCL duration or returns defaultValue when raw is omitted.
func parseOptionalDuration(raw *string, defaultValue time.Duration, attr string) (time.Duration, error) {
	if raw == nil {
		return defaultValue, nil
	}
	value, err := time.ParseDuration(*raw)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid %s %q: %w", ErrInvalid, attr, *raw, err)
	}
	return value, nil
}
