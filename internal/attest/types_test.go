package attest

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	canonicalUUID     = "550e8400-e29b-41d4-a716-446655440000"
	configKeyPrefix   = "user.spire.attestor.nonce."
	validConfigKeyHex = "0123456789abcdef0123456789abcdef"
)

func validConfigKey() string {
	return configKeyPrefix + validConfigKeyHex
}

func TestNewInstanceUUIDCanonicalizesValidRepresentations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "already canonical lowercase", input: canonicalUUID},
		{name: "uppercase hexadecimal", input: "550E8400-E29B-41D4-A716-446655440000"},
		{name: "mixed case hexadecimal", input: "550e8400-E29B-41d4-A716-446655440000"},
		{name: "URN form", input: "urn:uuid:" + canonicalUUID},
		{name: "braced form", input: "{" + canonicalUUID + "}"},
		{name: "32-character hexadecimal without hyphens", input: "550e8400e29b41d4a716446655440000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewInstanceUUID(tt.input)
			require.NoError(t, err, "valid UUID representation must be accepted")
			assert.Equal(t, InstanceUUID(canonicalUUID), got, "stored UUID must be canonical lowercase")
			assert.Equal(t, canonicalUUID, string(got), "String form must be google/uuid canonical lowercase")
		})
	}
}

func TestNewInstanceUUIDRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "not a UUID", input: "not-a-uuid"},
		{name: "truncated", input: "550e8400-e29b-41d4-a716"},
		{name: "too long", input: canonicalUUID + "00"},
		{name: "invalid hexadecimal", input: "550e8400-e29b-41d4-a716-44665544000g"},
		{name: "whitespace only", input: "   "},
		{name: "missing hyphens and wrong length", input: "550e8400e29b41d4a7164466554400"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewInstanceUUID(tt.input)
			require.Error(t, err, "malformed UUID must be rejected")
			assert.False(t, errors.Is(err, ErrDenied), "constructor failures must be contextual plain errors")
			assert.Zero(t, got, "rejected UUID must be the zero value")
		})
	}
}

func TestNewConfigKeyAcceptsExactGrammar(t *testing.T) {
	t.Parallel()

	got, err := NewConfigKey(validConfigKey())
	require.NoError(t, err, "exact prefix plus 32 lowercase hex must be accepted")
	assert.Equal(t, ConfigKey(validConfigKey()), got)
}

func TestNewConfigKeyRejectsInvalidGrammar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "prefix only", input: configKeyPrefix},
		{name: "uppercase hexadecimal", input: configKeyPrefix + "0123456789ABCDEF0123456789ABCDEF"},
		{name: "mixed-case hexadecimal", input: configKeyPrefix + "0123456789abcdef0123456789ABCDEf"},
		{name: "31 hex characters", input: configKeyPrefix + "0123456789abcdef0123456789abcde"},
		{name: "33 hex characters", input: configKeyPrefix + validConfigKeyHex + "0"},
		{name: "extra trailing segment", input: validConfigKey() + ".extra"},
		{name: "slash in key", input: validConfigKey() + "/extra"},
		{name: "slash inside hex", input: configKeyPrefix + "0123456789abcdef/123456789abcdef"},
		{name: "query string", input: validConfigKey() + "?x=1"},
		{name: "fragment", input: validConfigKey() + "#frag"},
		{name: "percent-encoded dot", input: configKeyPrefix + "0123456789abcdef%2e23456789abcdef"},
		{name: "percent-encoded hex character", input: configKeyPrefix + "0123456789abcdef%610123456789abcd"},
		{name: "prefix collision without hex", input: "user.spire.attestor.nonce"},
		{name: "prefix collision with extra label", input: "user.spire.attestor.nonceX." + validConfigKeyHex},
		{name: "adjacent namespace collision", input: "user.spire.attestor.nonces." + validConfigKeyHex},
		{name: "reserved prefix as a longer user key", input: "user.spire.attestor.nonce." + validConfigKeyHex + ".more"},
		{name: "leading whitespace", input: " " + validConfigKey()},
		{name: "trailing whitespace", input: validConfigKey() + " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewConfigKey(tt.input)
			require.Error(t, err, "invalid config key grammar must be rejected")
			assert.False(t, errors.Is(err, ErrDenied), "constructor failures must be contextual plain errors")
			assert.Zero(t, got, "rejected config key must be the zero value")
		})
	}
}

func TestNewConfigKeyFromAttemptIDEmitsExactPrefixAndLowercaseHex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   [16]byte
	}{
		{name: "all zeros", id: [16]byte{}},
		{
			name: "distinct bytes",
			id:   [16]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef},
		},
		{
			name: "high bytes that would be uppercase if encoded that way",
			id:   [16]byte{0xff, 0xee, 0xdd, 0xcc, 0xbb, 0xaa, 0x99, 0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11, 0x00},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NewConfigKeyFromAttemptID(tt.id)
			want := ConfigKey(configKeyPrefix + hex.EncodeToString(tt.id[:]))
			assert.Equal(t, want, got, "attempt ID must encode as prefix plus 32 lowercase hex characters")
			assert.True(t, strings.HasPrefix(string(got), configKeyPrefix))
			assert.Equal(t, 32, len(strings.TrimPrefix(string(got), configKeyPrefix)))
			assert.Equal(t, strings.ToLower(string(got)), string(got), "hex suffix must be lowercase")
		})
	}
}
