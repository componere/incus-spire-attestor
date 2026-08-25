package wire

import (
	"encoding/json"
	"fmt"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// challengeEnvelope is the compact v1 challenge message written on encode.
type challengeEnvelope struct {
	// Version is the outer challenge version.
	Version int `json:"version"`
	// Challenge is the config-nonce challenge body.
	Challenge challengeBody `json:"challenge"`
}

// challengeBody is the encoded config-nonce challenge wrapper.
type challengeBody struct {
	// Type is the challenge type name.
	Type string `json:"type"`
	// Version is the challenge body version.
	Version int `json:"version"`
	// Data is the challenge key body.
	Data challengeData `json:"data"`
}

// challengeData is the encoded challenge key body.
type challengeData struct {
	// ConfigKey is the guest-visible nonce key.
	ConfigKey string `json:"config_key"`
}

// decodedChallenge is the strict outer challenge envelope used on decode.
type decodedChallenge struct {
	// Version is the outer challenge version.
	Version *int `json:"version"`
	// Challenge is the still-encoded challenge object.
	Challenge json.RawMessage `json:"challenge"`
}

// decodedChallengeData is the strict challenge key body used on decode.
type decodedChallengeData struct {
	// ConfigKey is the guest-visible nonce key.
	ConfigKey string `json:"config_key"`
}

// EncodeChallenge encodes key as a v1 config-nonce challenge.
func EncodeChallenge(key attest.ConfigKey) ([]byte, error) {
	parsed, err := attest.NewConfigKey(string(key))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid config_key", ErrInvalid)
	}

	return encodeMessage(challengeEnvelope{
		Version: envelopeVersion,
		Challenge: challengeBody{
			Type:    configNonceType,
			Version: configNonceVersion,
			Data:    challengeData{ConfigKey: string(parsed)},
		},
	})
}

// DecodeChallenge decodes a v1 config-nonce challenge into a config key.
func DecodeChallenge(raw []byte) (attest.ConfigKey, error) {
	var env decodedChallenge
	if err := decodeMessage(raw, &env); err != nil {
		return "", err
	}
	if err := requireVersion(env.Version, "envelope"); err != nil {
		return "", err
	}

	dataRaw, decodeErr := decodeTypedObject(env.Challenge, configNonceType, "challenge")
	if decodeErr != nil {
		return "", decodeErr
	}

	var data decodedChallengeData
	if err := decodeStrict(dataRaw, &data); err != nil {
		return "", err
	}
	if err := requireNonEmpty(data.ConfigKey, "config_key"); err != nil {
		return "", err
	}

	key, err := attest.NewConfigKey(data.ConfigKey)
	if err != nil {
		return "", fmt.Errorf("%w: invalid config_key", ErrInvalid)
	}
	return key, nil
}
