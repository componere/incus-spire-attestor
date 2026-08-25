package wire

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// responseEnvelope is the compact v1 response message written on encode.
type responseEnvelope struct {
	// Version is the outer response version.
	Version int `json:"version"`
	// Response is the config-nonce response body.
	Response responseBody `json:"response"`
}

// responseBody is the encoded config-nonce response wrapper.
type responseBody struct {
	// Type is the response type name.
	Type string `json:"type"`
	// Version is the response body version.
	Version int `json:"version"`
	// Data is the nonce body.
	Data responseData `json:"data"`
}

// responseData is the encoded nonce body.
type responseData struct {
	// Nonce is the unpadded base64url nonce.
	Nonce string `json:"nonce"`
}

// decodedResponse is the strict outer response envelope used on decode.
type decodedResponse struct {
	// Version is the outer response version.
	Version *int `json:"version"`
	// Response is the still-encoded response object.
	Response json.RawMessage `json:"response"`
}

// decodedResponseData is the strict nonce body used on decode.
type decodedResponseData struct {
	// Nonce is the unpadded base64url nonce.
	Nonce string `json:"nonce"`
}

// EncodeResponse encodes nonce as a v1 config-nonce response.
func EncodeResponse(nonce attest.Nonce) ([]byte, error) {
	return encodeMessage(responseEnvelope{
		Version: envelopeVersion,
		Response: responseBody{
			Type:    configNonceType,
			Version: configNonceVersion,
			Data: responseData{
				Nonce: base64.RawURLEncoding.EncodeToString(nonce[:]),
			},
		},
	})
}

// DecodeResponse decodes a v1 config-nonce response into a nonce.
func DecodeResponse(raw []byte) (attest.Nonce, error) {
	var env decodedResponse
	if err := decodeMessage(raw, &env); err != nil {
		return attest.Nonce{}, err
	}
	if err := requireVersion(env.Version, "envelope"); err != nil {
		return attest.Nonce{}, err
	}

	dataRaw, decodeErr := decodeTypedObject(env.Response, configNonceType, "response")
	if decodeErr != nil {
		return attest.Nonce{}, decodeErr
	}

	var data decodedResponseData
	if err := decodeStrict(dataRaw, &data); err != nil {
		return attest.Nonce{}, err
	}
	if err := requireNonEmpty(data.Nonce, "nonce"); err != nil {
		return attest.Nonce{}, err
	}
	return ParseNonce(data.Nonce)
}

// ParseNonce decodes an unpadded base64url nonce and requires exactly 16 bytes.
func ParseNonce(raw string) (attest.Nonce, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return attest.Nonce{}, fmt.Errorf("%w: invalid nonce encoding", ErrInvalid)
	}
	nonce, err := attest.NewNonce(decoded)
	if err != nil {
		return attest.Nonce{}, fmt.Errorf("%w: invalid nonce length", ErrInvalid)
	}
	return nonce, nil
}
