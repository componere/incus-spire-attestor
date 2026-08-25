package attest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sixteenBytes(fill byte) []byte {
	out := make([]byte, 16)
	for i := range out {
		out[i] = fill
	}
	return out
}

func TestNewNonceAcceptsExactlySixteenBytes(t *testing.T) {
	t.Parallel()

	raw := sixteenBytes(0xab)
	got, err := NewNonce(raw)
	require.NoError(t, err)
	assert.Equal(t, Nonce([16]byte{
		0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab,
		0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab,
	}), got)
}

func TestNewNonceRejectsWrongLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []byte
	}{
		{name: "nil", input: nil},
		{name: "empty", input: []byte{}},
		{name: "fifteen bytes", input: make([]byte, 15)},
		{name: "seventeen bytes", input: make([]byte, 17)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewNonce(tt.input)
			require.Error(t, err, "nonce construction must require exactly 16 bytes")
			require.NotErrorIs(t, err, ErrDenied, "constructor failures must be contextual plain errors")
			assert.Equal(t, Nonce{}, got)
		})
	}
}

func TestVerifyNonceAcceptsExactMatch(t *testing.T) {
	t.Parallel()

	raw := sixteenBytes(0x3c)
	nonce, err := NewNonce(raw)
	require.NoError(t, err)
	require.NoError(t, VerifyNonce(nonce, raw))
}

func TestVerifyNonceRejectsMismatchAndWrongLength(t *testing.T) {
	t.Parallel()

	expectedRaw := sixteenBytes(0x11)
	nonce, err := NewNonce(expectedRaw)
	require.NoError(t, err)

	sameLengthMismatch := sixteenBytes(0x22)
	shortActual := expectedRaw[:15]
	longActual := append(append([]byte{}, expectedRaw...), 0x00)

	tests := []struct {
		name   string
		actual []byte
	}{
		{name: "same-length mismatch", actual: sameLengthMismatch},
		{name: "short actual value", actual: shortActual},
		{name: "long actual value", actual: longActual},
		{name: "nil actual value", actual: nil},
		{name: "empty actual value", actual: []byte{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := VerifyNonce(nonce, tt.actual)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrDenied)
		})
	}
}
