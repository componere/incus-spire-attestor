package wire

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

func TestDecodeChallengeGoldenRoundTrip(t *testing.T) {
	t.Parallel()

	got, err := DecodeChallenge(golden(t, "challenge-v1.json"))
	require.NoError(t, err, "architecture challenge example must decode")
	assert.Equal(t, validConfigKey(), got)

	encoded, err := EncodeChallenge(got)
	require.NoError(t, err)

	roundTripped, err := DecodeChallenge(encoded)
	require.NoError(t, err)
	assert.Equal(t, validConfigKey(), roundTripped)
}

func TestEncodeChallengeSemanticRoundTrip(t *testing.T) {
	t.Parallel()

	encoded, err := EncodeChallenge(validConfigKey())
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), maxMessageBytes)
	require.True(t, json.Valid(encoded), "encoded challenge must be JSON")

	got, err := DecodeChallenge(encoded)
	require.NoError(t, err)
	assert.Equal(t, validConfigKey(), got)
}

func TestEncodeChallengeRejectsInvalidGrammar(t *testing.T) {
	t.Parallel()

	got, err := EncodeChallenge(attest.ConfigKey("user.spire.attestor.nonce.NOTHEX"))
	assertInvalid(t, err)
	assert.Empty(t, got)
}

func TestDecodeChallengeTranslatesConfigKeyGrammar(t *testing.T) {
	t.Parallel()

	wrap := func(key string) string {
		return `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,"data":{"config_key":` +
			jsonString(key) + `}}}`
	}

	tests := []struct {
		name string
		key  string
	}{
		{name: "empty", key: ""},
		{name: "prefix only", key: testConfigPrefix},
		{name: "uppercase hexadecimal", key: testConfigPrefix + "0123456789ABCDEF0123456789ABCDEF"},
		{name: "mixed-case hexadecimal", key: testConfigPrefix + "0123456789abcdef0123456789ABCDEf"},
		{name: "31 hex characters", key: testConfigPrefix + "0123456789abcdef0123456789abcde"},
		{name: "33 hex characters", key: testConfigPrefix + validConfigKeyHex + "0"},
		{name: "extra trailing segment", key: testConfigKey + ".extra"},
		{name: "slash in key", key: testConfigKey + "/extra"},
		{name: "slash inside hex", key: testConfigPrefix + "0123456789abcdef/123456789abcdef"},
		{name: "query string", key: testConfigKey + "?x=1"},
		{name: "fragment", key: testConfigKey + "#frag"},
		{name: "percent-encoded dot", key: testConfigPrefix + "0123456789abcdef%2e23456789abcdef"},
		{name: "percent-encoded hex character", key: testConfigPrefix + "0123456789abcdef%610123456789abcd"},
		{name: "prefix collision without hex", key: "user.spire.attestor.nonce"},
		{name: "prefix collision with extra label", key: "user.spire.attestor.nonceX." + validConfigKeyHex},
		{name: "adjacent namespace collision", key: "user.spire.attestor.nonces." + validConfigKeyHex},
		{name: "reserved prefix as a longer user key", key: testConfigKey + ".more"},
		{name: "leading whitespace", key: " " + testConfigKey},
		{name: "trailing whitespace", key: testConfigKey + " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeChallenge([]byte(wrap(tt.key)))
			assertInvalid(t, err)
			assert.Zero(t, got)
		})
	}
}

func TestDecodeChallengeRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "outer extra field",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}},"extra":true}`,
		},
		{
			name: "body extra field",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,"extra":true,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
		{
			name: "data extra field",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef","nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeChallenge([]byte(tt.raw))
			assertInvalid(t, err)
			assert.Zero(t, got)
		})
	}
}

func TestDecodeChallengeRejectsDuplicateMembers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "outer version duplicated",
			raw:  `{"version":1,"version":1,"challenge":{"type":"incus-config-nonce","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
		{
			name: "outer challenge duplicated",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}},"challenge":{"type":"incus-config-nonce","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
		{
			name: "body type duplicated",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","type":"incus-config-nonce","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
		{
			name: "body version duplicated",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,"version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
		{
			name: "body data duplicated",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"},"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
		{
			name: "data config_key duplicated",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef","config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeChallenge([]byte(tt.raw))
			assertInvalid(t, err)
			assert.Zero(t, got)
		})
	}
}

func TestDecodeChallengeRejectsMissingAndEmptyFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "missing version",
			raw:  `{"challenge":{"type":"incus-config-nonce","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
		{name: "missing challenge", raw: `{"version":1}`},
		{name: "null challenge", raw: `{"version":1,"challenge":null}`},
		{
			name: "missing type",
			raw:  `{"version":1,"challenge":{"version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
		{
			name: "empty type",
			raw:  `{"version":1,"challenge":{"type":"","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
		{
			name: "missing body version",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
		{name: "missing data", raw: `{"version":1,"challenge":{"type":"incus-config-nonce","version":1}}`},
		{name: "null data", raw: `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,"data":null}}`},
		{
			name: "missing config_key",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,"data":{}}}`,
		},
		{
			name: "empty config_key",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,"data":{"config_key":""}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeChallenge([]byte(tt.raw))
			assertInvalid(t, err)
			assert.Zero(t, got)
		})
	}
}

func TestDecodeChallengeRejectsWrongTypeAndVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want error
	}{
		{
			name: "wrong challenge type",
			raw:  `{"version":1,"challenge":{"type":"incus-guest-claims","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
			want: ErrUnsupported,
		},
		{
			name: "unknown challenge type",
			raw:  `{"version":1,"challenge":{"type":"tpm-signed-document","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
			want: ErrUnsupported,
		},
		{
			name: "outer version 2",
			raw:  `{"version":2,"challenge":{"type":"incus-config-nonce","version":1,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
			want: ErrUnsupported,
		},
		{
			name: "body version 2",
			raw:  `{"version":1,"challenge":{"type":"incus-config-nonce","version":2,"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
			want: ErrUnsupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeChallenge([]byte(tt.raw))
			require.Error(t, err)
			require.ErrorIs(t, err, tt.want)
			require.NotErrorIs(t, err, attest.ErrDenied)
			assert.Zero(t, got)
		})
	}
}

func jsonString(v string) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(raw)
}
