package wire

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	invalid := []byte{0xff, 0xfe, 0xfd}

	tests := []struct {
		name   string
		decode func([]byte) error
	}{
		{name: "payload", decode: func(raw []byte) error { _, err := DecodePayload(raw); return err }},
		{name: "challenge", decode: func(raw []byte) error { _, err := DecodeChallenge(raw); return err }},
		{name: "response", decode: func(raw []byte) error { _, err := DecodeResponse(raw); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertInvalid(t, tt.decode(invalid))
		})
	}
}

func TestDecodeRejectsOversizedMessages(t *testing.T) {
	t.Parallel()

	oversized := bytes.Repeat([]byte("a"), maxMessageBytes+1)
	require.Len(t, oversized, 65537)

	tests := []struct {
		name   string
		decode func([]byte) error
	}{
		{name: "payload", decode: func(raw []byte) error { _, err := DecodePayload(raw); return err }},
		{name: "challenge", decode: func(raw []byte) error { _, err := DecodeChallenge(raw); return err }},
		{name: "response", decode: func(raw []byte) error { _, err := DecodeResponse(raw); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertInvalid(t, tt.decode(oversized))
		})
	}
}

func TestDecodeRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		raw    []byte
		decode func([]byte) error
	}{
		{
			name:   "payload trailing object",
			raw:    append([]byte(validPayloadJSON()), []byte("{}")...),
			decode: func(raw []byte) error { _, err := DecodePayload(raw); return err },
		},
		{
			name:   "payload trailing array",
			raw:    append([]byte(validPayloadJSON()), []byte("[]")...),
			decode: func(raw []byte) error { _, err := DecodePayload(raw); return err },
		},
		{
			name:   "challenge trailing object",
			raw:    append([]byte(validChallengeJSON()), []byte("{}")...),
			decode: func(raw []byte) error { _, err := DecodeChallenge(raw); return err },
		},
		{
			name:   "response trailing object",
			raw:    append([]byte(validResponseJSON()), []byte("{}")...),
			decode: func(raw []byte) error { _, err := DecodeResponse(raw); return err },
		},
		{
			name:   "response trailing true",
			raw:    append([]byte(validResponseJSON()), []byte("true")...),
			decode: func(raw []byte) error { _, err := DecodeResponse(raw); return err },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertInvalid(t, tt.decode(tt.raw))
		})
	}
}

func TestDecodeAcceptsTrailingWhitespace(t *testing.T) {
	t.Parallel()

	t.Run("payload", func(t *testing.T) {
		t.Parallel()
		got, err := DecodePayload(append([]byte(validPayloadJSON()), '\n'))
		require.NoError(t, err)
		assert.Equal(t, validClaims(), got)
	})
	t.Run("challenge", func(t *testing.T) {
		t.Parallel()
		got, err := DecodeChallenge(append([]byte(validChallengeJSON()), '\n'))
		require.NoError(t, err)
		assert.Equal(t, validConfigKey(), got)
	})
	t.Run("response", func(t *testing.T) {
		t.Parallel()
		got, err := DecodeResponse(append([]byte(validResponseJSON()), '\n'))
		require.NoError(t, err)
		assert.Equal(t, validNonce(), got)
	})
}

func TestDecodeRejectsEmptyAndNonJSON(t *testing.T) {
	t.Parallel()

	inputs := [][]byte{nil, {}, []byte("not-json"), []byte("{")}

	tests := []struct {
		name   string
		decode func([]byte) error
	}{
		{name: "payload", decode: func(raw []byte) error { _, err := DecodePayload(raw); return err }},
		{name: "challenge", decode: func(raw []byte) error { _, err := DecodeChallenge(raw); return err }},
		{name: "response", decode: func(raw []byte) error { _, err := DecodeResponse(raw); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, input := range inputs {
				assertInvalid(t, tt.decode(input))
			}
		})
	}
}
