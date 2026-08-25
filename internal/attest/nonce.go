package attest

import (
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
)

// configKeyPrefix is the exact challenge-key namespace.
const configKeyPrefix = "user.spire.attestor.nonce."

// configKeySuffixLen is the number of lowercase hex digits after the prefix.
const configKeySuffixLen = 32

// nonceSize is the required challenge-nonce length in bytes.
const nonceSize = 16

// NewConfigKey accepts only the exact v1 challenge-key grammar.
func NewConfigKey(raw string) (ConfigKey, error) {
	if len(raw) != len(configKeyPrefix)+configKeySuffixLen {
		return "", fmt.Errorf("invalid config key length %d", len(raw))
	}
	if raw[:len(configKeyPrefix)] != configKeyPrefix {
		return "", errors.New("invalid config key prefix")
	}
	for i := len(configKeyPrefix); i < len(raw); i++ {
		if !isLowerHex(raw[i]) {
			return "", errors.New("invalid config key suffix")
		}
	}
	return ConfigKey(raw), nil
}

// NewConfigKeyFromAttemptID formats a 16-byte attempt ID as a challenge key.
func NewConfigKeyFromAttemptID(id [16]byte) ConfigKey {
	return ConfigKey(configKeyPrefix + hex.EncodeToString(id[:]))
}

// NewNonce copies exactly 16 bytes into a Nonce.
func NewNonce(raw []byte) (Nonce, error) {
	var nonce Nonce
	if len(raw) != nonceSize {
		return nonce, fmt.Errorf("nonce length %d, want %d", len(raw), nonceSize)
	}
	copy(nonce[:], raw)
	return nonce, nil
}

// VerifyNonce compares got to expected in constant time when the lengths match.
func VerifyNonce(expected Nonce, got []byte) error {
	if len(got) != nonceSize {
		return fmt.Errorf("%w: nonce length %d, want %d", ErrDenied, len(got), nonceSize)
	}
	if subtle.ConstantTimeCompare(expected[:], got) != 1 {
		return fmt.Errorf("%w: nonce mismatch", ErrDenied)
	}
	return nil
}

// isLowerHex reports whether c is a lowercase hexadecimal digit.
func isLowerHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f'
}
