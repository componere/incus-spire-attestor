package wire

import (
	"encoding/json"
	"fmt"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// payloadEnvelope is the compact v1 payload message written on encode.
type payloadEnvelope struct {
	// Version is the outer payload version.
	Version int `json:"version"`
	// Evidence is the single guest-claims evidence item.
	Evidence []payloadEvidence `json:"evidence"`
}

// payloadEvidence is one encoded evidence item.
type payloadEvidence struct {
	// Type is the evidence type name.
	Type string `json:"type"`
	// Version is the evidence body version.
	Version int `json:"version"`
	// Data is the guest-claims body.
	Data payloadData `json:"data"`
}

// payloadData is the encoded incus-guest-claims body.
type payloadData struct {
	// InstanceName is the guest instance name.
	InstanceName string `json:"instance_name"`
	// Project is the optional guest project hint.
	Project string `json:"project,omitempty"`
	// InstanceType is the guest instance type.
	InstanceType string `json:"instance_type"`
	// UUID is the canonical guest instance UUID.
	UUID string `json:"uuid"`
	// Location is the guest cluster member.
	Location string `json:"location"`
	// CloudInitID is the guest cloud-init instance ID.
	CloudInitID string `json:"cloud_init_id"`
}

// decodedPayload is the strict outer payload envelope used on decode.
type decodedPayload struct {
	// Version is the outer payload version.
	Version *int `json:"version"`
	// Evidence is the still-encoded evidence array.
	Evidence json.RawMessage `json:"evidence"`
}

// decodedPayloadData is the strict guest-claims body used on decode.
type decodedPayloadData struct {
	// InstanceName is the guest instance name.
	InstanceName string `json:"instance_name"`
	// Project is the encoded optional guest project hint; nil means omitted.
	Project json.RawMessage `json:"project"`
	// InstanceType is the guest instance type.
	InstanceType string `json:"instance_type"`
	// UUID is the guest instance UUID.
	UUID string `json:"uuid"`
	// Location is the guest cluster member.
	Location string `json:"location"`
	// CloudInitID is the guest cloud-init instance ID.
	CloudInitID string `json:"cloud_init_id"`
}

// EncodePayload encodes claims as a v1 guest-claims payload.
func EncodePayload(claims attest.Claims) ([]byte, error) {
	id, err := payloadUUID(claims.UUID)
	if err != nil {
		return nil, err
	}
	if err := requireNonEmpty(string(claims.Name), "instance_name"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty(string(claims.Type), "instance_type"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty(claims.Location, "location"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty(claims.CloudInitID, "cloud_init_id"); err != nil {
		return nil, err
	}

	return encodeMessage(payloadEnvelope{
		Version: envelopeVersion,
		Evidence: []payloadEvidence{{
			Type:    guestClaimsType,
			Version: guestClaimsVersion,
			Data: payloadData{
				InstanceName: string(claims.Name),
				Project:      string(claims.Project),
				InstanceType: string(claims.Type),
				UUID:         string(id),
				Location:     claims.Location,
				CloudInitID:  claims.CloudInitID,
			},
		}},
	})
}

// DecodePayload decodes a v1 guest-claims payload into claims.
func DecodePayload(raw []byte) (attest.Claims, error) {
	var env decodedPayload
	if err := decodeMessage(raw, &env); err != nil {
		return attest.Claims{}, err
	}
	if err := requireVersion(env.Version, "envelope"); err != nil {
		return attest.Claims{}, err
	}
	if len(env.Evidence) == 0 {
		return attest.Claims{}, fmt.Errorf("%w: evidence is required", ErrInvalid)
	}

	var items []json.RawMessage
	if err := decodeStrict(env.Evidence, &items); err != nil {
		return attest.Claims{}, err
	}
	if len(items) != 1 {
		return attest.Claims{}, fmt.Errorf("%w: evidence count %d, want 1", ErrInvalid, len(items))
	}

	dataRaw, decodeErr := decodeTypedObject(items[0], guestClaimsType, "evidence")
	if decodeErr != nil {
		return attest.Claims{}, decodeErr
	}

	var data decodedPayloadData
	if err := decodeStrict(dataRaw, &data); err != nil {
		return attest.Claims{}, err
	}
	if err := requireNonEmpty(data.InstanceName, "instance_name"); err != nil {
		return attest.Claims{}, err
	}
	if err := requireNonEmpty(data.InstanceType, "instance_type"); err != nil {
		return attest.Claims{}, err
	}
	if err := requireNonEmpty(data.Location, "location"); err != nil {
		return attest.Claims{}, err
	}
	if err := requireNonEmpty(data.CloudInitID, "cloud_init_id"); err != nil {
		return attest.Claims{}, err
	}

	id, err := payloadUUID(attest.InstanceUUID(data.UUID))
	if err != nil {
		return attest.Claims{}, err
	}

	claims := attest.Claims{
		Name:        attest.InstanceName(data.InstanceName),
		UUID:        id,
		Type:        attest.InstanceType(data.InstanceType),
		Location:    data.Location,
		CloudInitID: data.CloudInitID,
	}
	if data.Project != nil {
		var project string
		if err := decodeStrict(data.Project, &project); err != nil {
			return attest.Claims{}, err
		}
		if project == "" {
			return attest.Claims{}, fmt.Errorf("%w: project is empty", ErrInvalid)
		}
		claims.Project = attest.ProjectName(project)
	}
	return claims, nil
}

// payloadUUID canonicalizes a guest UUID without invoking claim validation.
func payloadUUID(raw attest.InstanceUUID) (attest.InstanceUUID, error) {
	if err := requireNonEmpty(string(raw), "uuid"); err != nil {
		return "", err
	}
	id, err := attest.NewInstanceUUID(string(raw))
	if err != nil {
		return "", fmt.Errorf("%w: uuid is not a canonical UUID", ErrInvalid)
	}
	return id, nil
}
