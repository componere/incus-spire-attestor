package wire

import (
	"encoding/json"
	"testing"

	"github.com/componere/incus-spire-attestor/internal/attest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	paddedNonceRawURL = "3q2-7wARIjNEVWZ3iJmquw=="
	stdNonceEncoding  = "3q2+7wARIjNEVWZ3iJmquw=="
	nonce15RawURL     = "3q2-7wARIjNEVWZ3iJmq"
	nonce17RawURL     = "3q2-7wARIjNEVWZ3iJmqu8w"
	malformedNonce    = "!!!not-base64url!!!"
)

func TestDecodeResponseGoldenRoundTrip(t *testing.T) {
	t.Parallel()

	got, err := DecodeResponse(golden(t, "response-v1.json"))
	require.NoError(t, err, "architecture response example must decode")
	assert.Equal(t, validNonce(), got)

	encoded, err := EncodeResponse(got)
	require.NoError(t, err)

	roundTripped, err := DecodeResponse(encoded)
	require.NoError(t, err)
	assert.Equal(t, validNonce(), roundTripped)
}

func TestEncodeResponseSemanticRoundTrip(t *testing.T) {
	t.Parallel()

	encoded, err := EncodeResponse(validNonce())
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), maxMessageBytes)
	require.True(t, json.Valid(encoded), "encoded response must be JSON")
	assert.NotContains(t, string(encoded), "=", "response nonce must use unpadded base64url")
	assert.Contains(t, string(encoded), testNonceRawURL)

	got, err := DecodeResponse(encoded)
	require.NoError(t, err)
	assert.Equal(t, validNonce(), got)
}

func TestParseNonceAcceptsValidRawURL(t *testing.T) {
	t.Parallel()

	got, err := ParseNonce(testNonceRawURL)
	require.NoError(t, err)
	assert.Equal(t, validNonce(), got)
}

func TestParseNonceRejectsInvalidEncodings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "padded rawurl", input: paddedNonceRawURL},
		{name: "standard base64", input: stdNonceEncoding},
		{name: "malformed", input: malformedNonce},
		{name: "empty", input: ""},
		{name: "fifteen decoded bytes", input: nonce15RawURL},
		{name: "seventeen decoded bytes", input: nonce17RawURL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseNonce(tt.input)
			assertInvalid(t, err)
			assertNoSecret(t, err, tt.input, testNonceRawURL, paddedNonceRawURL, stdNonceEncoding, nonce15RawURL, nonce17RawURL)
			assert.Equal(t, attest.Nonce{}, got)
		})
	}
}

func TestDecodeResponseRejectsNonceEncodings(t *testing.T) {
	t.Parallel()

	wrap := func(nonce string) string {
		return `{"version":1,"response":{"type":"incus-config-nonce","version":1,"data":{"nonce":` +
			jsonString(nonce) + `}}}`
	}

	tests := []struct {
		name  string
		nonce string
	}{
		{name: "padded rawurl", nonce: paddedNonceRawURL},
		{name: "standard base64", nonce: stdNonceEncoding},
		{name: "malformed", nonce: malformedNonce},
		{name: "empty", nonce: ""},
		{name: "fifteen decoded bytes", nonce: nonce15RawURL},
		{name: "seventeen decoded bytes", nonce: nonce17RawURL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeResponse([]byte(wrap(tt.nonce)))
			assertInvalid(t, err)
			assertNoSecret(t, err, tt.nonce, testNonceRawURL, paddedNonceRawURL, stdNonceEncoding)
			assert.Equal(t, attest.Nonce{}, got)
		})
	}
}

func TestDecodeResponseRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "outer extra field",
			raw:  `{"version":1,"response":{"type":"incus-config-nonce","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}},"extra":true}`,
		},
		{
			name: "body extra field",
			raw:  `{"version":1,"response":{"type":"incus-config-nonce","version":1,"extra":true,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`,
		},
		{
			name: "data extra field",
			raw:  `{"version":1,"response":{"type":"incus-config-nonce","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw","config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeResponse([]byte(tt.raw))
			assertInvalid(t, err)
			assertNoSecret(t, err, testNonceRawURL)
			assert.Equal(t, attest.Nonce{}, got)
		})
	}
}

func TestDecodeResponseRejectsDuplicateMembers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "outer version duplicated",
			raw:  `{"version":1,"version":1,"response":{"type":"incus-config-nonce","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`,
		},
		{
			name: "outer response duplicated",
			raw:  `{"version":1,"response":{"type":"incus-config-nonce","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}},"response":{"type":"incus-config-nonce","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`,
		},
		{
			name: "body type duplicated",
			raw:  `{"version":1,"response":{"type":"incus-config-nonce","type":"incus-config-nonce","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`,
		},
		{
			name: "body version duplicated",
			raw:  `{"version":1,"response":{"type":"incus-config-nonce","version":1,"version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`,
		},
		{
			name: "body data duplicated",
			raw:  `{"version":1,"response":{"type":"incus-config-nonce","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"},"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`,
		},
		{
			name: "data nonce duplicated",
			raw:  `{"version":1,"response":{"type":"incus-config-nonce","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw","nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeResponse([]byte(tt.raw))
			assertInvalid(t, err)
			assertNoSecret(t, err, testNonceRawURL)
			assert.Equal(t, attest.Nonce{}, got)
		})
	}
}

func TestDecodeResponseRejectsMissingAndEmptyFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{name: "missing version", raw: `{"response":{"type":"incus-config-nonce","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`},
		{name: "missing response", raw: `{"version":1}`},
		{name: "null response", raw: `{"version":1,"response":null}`},
		{name: "missing type", raw: `{"version":1,"response":{"version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`},
		{name: "empty type", raw: `{"version":1,"response":{"type":"","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`},
		{name: "missing body version", raw: `{"version":1,"response":{"type":"incus-config-nonce","data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`},
		{name: "missing data", raw: `{"version":1,"response":{"type":"incus-config-nonce","version":1}}`},
		{name: "null data", raw: `{"version":1,"response":{"type":"incus-config-nonce","version":1,"data":null}}`},
		{name: "missing nonce", raw: `{"version":1,"response":{"type":"incus-config-nonce","version":1,"data":{}}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeResponse([]byte(tt.raw))
			assertInvalid(t, err)
			assertNoSecret(t, err, testNonceRawURL)
			assert.Equal(t, attest.Nonce{}, got)
		})
	}
}

func TestDecodeResponseRejectsWrongTypeAndVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{name: "wrong response type", raw: `{"version":1,"response":{"type":"incus-guest-claims","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`},
		{name: "unknown response type", raw: `{"version":1,"response":{"type":"tpm-ek-certificate","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`},
		{name: "outer version 2", raw: `{"version":2,"response":{"type":"incus-config-nonce","version":1,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`},
		{name: "body version 2", raw: `{"version":1,"response":{"type":"incus-config-nonce","version":2,"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeResponse([]byte(tt.raw))
			assertUnsupported(t, err)
			assertNoSecret(t, err, testNonceRawURL)
			assert.Equal(t, attest.Nonce{}, got)
		})
	}
}
