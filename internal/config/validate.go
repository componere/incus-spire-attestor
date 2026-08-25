package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// maxProjects is the maximum number of distinct server projects.
const maxProjects = 32

// maxUserSelectors is the maximum number of distinct user selector keys.
const maxUserSelectors = 32

// userKeyPrefix is the required prefix for user selector keys.
const userKeyPrefix = "user."

// reservedNonceKey is the reserved nonce-key name that selectors may not use.
const reservedNonceKey = "user.spire.attestor.nonce"

// reservedNoncePrefix is the nonce-key namespace that selectors may not use.
const reservedNoncePrefix = reservedNonceKey + "."

// ValidateAgent reports whether cfg and trustDomain form a valid agent configuration.
//
// Project may be empty. PollTimeout must be positive. trustDomain must be
// nonempty. All violations are collected. ValidateAgent does not read the
// filesystem.
func ValidateAgent(cfg Agent, trustDomain string) error {
	return errors.Join(
		requireNonempty(trustDomain, "trust_domain"),
		requirePositive(cfg.PollTimeout, "poll_timeout"),
	)
}

// ValidateServer reports whether cfg and trustDomain form a valid server configuration.
//
// The endpoint, three TLS paths, and one to 32 distinct nonempty projects are
// required. User selectors must be at most 32 distinct nonempty user.* keys
// outside the reserved nonce namespace. All durations must be positive.
// trustDomain must be nonempty. All violations are collected. ValidateServer
// does not read TLS files or contact Incus.
func ValidateServer(cfg Server, trustDomain string) error {
	return errors.Join(
		requireNonempty(trustDomain, "trust_domain"),
		requireNonempty(cfg.IncusEndpoint, "incus_endpoint"),
		requireNonempty(cfg.TLSCAPath, "tls_ca_path"),
		requireNonempty(cfg.TLSCertPath, "tls_cert_path"),
		requireNonempty(cfg.TLSKeyPath, "tls_key_path"),
		requirePositive(cfg.IncusTimeout, "incus_timeout"),
		requirePositive(cfg.ChallengeResponseTimeout, "challenge_response_timeout"),
		requirePositive(cfg.CleanupTimeout, "cleanup_timeout"),
		validateProjects(cfg.Projects),
		validateUserSelectors(cfg.UserSelectors),
	)
}

// requireNonempty reports whether value is a nonempty string.
func requireNonempty(value, attr string) error {
	if value == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalid, attr)
	}
	return nil
}

// requirePositive reports whether value is a positive duration.
func requirePositive(value time.Duration, attr string) error {
	if value <= 0 {
		return fmt.Errorf("%w: %s must be positive", ErrInvalid, attr)
	}
	return nil
}

// validateProjects reports whether projects is a valid server allowlist.
func validateProjects(projects []attest.ProjectName) error {
	var errs []error
	if len(projects) == 0 {
		errs = append(errs, fmt.Errorf("%w: at least one project is required", ErrInvalid))
	}
	if len(projects) > maxProjects {
		errs = append(errs, fmt.Errorf("%w: projects count %d exceeds %d", ErrInvalid, len(projects), maxProjects))
	}
	seen := make(map[attest.ProjectName]struct{}, len(projects))
	for _, project := range projects {
		if project == "" {
			errs = append(errs, fmt.Errorf("%w: project must be nonempty", ErrInvalid))
			continue
		}
		if _, ok := seen[project]; ok {
			errs = append(errs, fmt.Errorf("%w: duplicate project %q", ErrInvalid, project))
			continue
		}
		seen[project] = struct{}{}
	}
	return errors.Join(errs...)
}

// validateUserSelectors reports whether keys are valid user selector keys.
func validateUserSelectors(keys []string) error {
	var errs []error
	if len(keys) > maxUserSelectors {
		errs = append(errs, fmt.Errorf("%w: user_selectors count %d exceeds %d", ErrInvalid, len(keys), maxUserSelectors))
	}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if err := validateUserSelector(key); err != nil {
			errs = append(errs, err)
			continue
		}
		if _, ok := seen[key]; ok {
			errs = append(errs, fmt.Errorf("%w: duplicate user selector %q", ErrInvalid, key))
			continue
		}
		seen[key] = struct{}{}
	}
	return errors.Join(errs...)
}

// validateUserSelector reports whether key is an allowed user selector.
func validateUserSelector(key string) error {
	if key == "" {
		return fmt.Errorf("%w: user selector must be nonempty", ErrInvalid)
	}
	if !strings.HasPrefix(key, userKeyPrefix) || key == userKeyPrefix {
		return fmt.Errorf("%w: user selector %q must be a user.* key", ErrInvalid, key)
	}
	if key == reservedNonceKey || strings.HasPrefix(key, reservedNoncePrefix) {
		return fmt.Errorf("%w: user selector %q uses the reserved nonce namespace", ErrInvalid, key)
	}
	return nil
}
