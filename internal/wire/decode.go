package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// maxMessageSize is the maximum opaque message length in bytes.
const maxMessageSize = 65536

// envelopeVersion is the only accepted outer message version.
const envelopeVersion = 1

// guestClaimsType is the v1 payload evidence type.
const guestClaimsType = "incus-guest-claims"

// guestClaimsVersion is the only accepted guest-claims body version.
const guestClaimsVersion = 1

// configNonceType is the v1 challenge and response body type.
const configNonceType = "incus-config-nonce"

// configNonceVersion is the only accepted config-nonce body version.
const configNonceVersion = 1

// typedObject is the shared type/version envelope around a typed JSON body.
type typedObject struct {
	// Type is the nested body type name.
	Type *string `json:"type"`
	// Version is the nested body version.
	Version *int `json:"version"`
	// Data is the still-encoded typed body.
	Data json.RawMessage `json:"data"`
}

// encodeMessage marshals v as compact JSON and enforces the message size bound.
func encodeMessage(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("%w: encode failed", ErrInvalid)
	}
	if len(raw) > maxMessageSize {
		return nil, fmt.Errorf("%w: message exceeds %d bytes", ErrInvalid, maxMessageSize)
	}
	return raw, nil
}

// decodeMessage reads one bounded JSON value into dest with unknown-field rejection.
func decodeMessage(raw []byte, dest any) error {
	msg, err := decodeRaw(raw)
	if err != nil {
		return err
	}
	return decodeStrict(msg, dest)
}

// decodeRaw enforces size, UTF-8, one-value, and duplicate-name rules.
func decodeRaw(raw []byte) (json.RawMessage, error) {
	if len(raw) > maxMessageSize {
		return nil, fmt.Errorf("%w: message exceeds %d bytes", ErrInvalid, maxMessageSize)
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("%w: message is not valid UTF-8", ErrInvalid)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	var msg json.RawMessage
	if err := dec.Decode(&msg); err != nil {
		return nil, wrapJSONError(err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing JSON data", ErrInvalid)
	}
	if err := rejectDuplicateNames(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// decodeStrict unmarshals raw into dest and rejects unknown fields.
func decodeStrict(raw []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return wrapJSONError(err)
	}
	if dec.More() {
		return fmt.Errorf("%w: trailing JSON data", ErrInvalid)
	}
	return nil
}

// decodeTypedObject strictly decodes a type/version wrapper and returns its data.
func decodeTypedObject(raw json.RawMessage, wantType, what string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: %s is required", ErrInvalid, what)
	}

	var obj typedObject
	if err := decodeStrict(raw, &obj); err != nil {
		return nil, err
	}
	if err := requireType(obj.Type, wantType, what); err != nil {
		return nil, err
	}
	if err := requireVersion(obj.Version, 1, what); err != nil {
		return nil, err
	}
	if len(obj.Data) == 0 {
		return nil, fmt.Errorf("%w: %s data is required", ErrInvalid, what)
	}
	return obj.Data, nil
}

// requireVersion reports a missing version as invalid and any other value as unsupported.
func requireVersion(got *int, want int, what string) error {
	if got == nil {
		return fmt.Errorf("%w: %s version is required", ErrInvalid, what)
	}
	if *got != want {
		return fmt.Errorf("%w: %s version %d", ErrUnsupported, what, *got)
	}
	return nil
}

// requireType reports a missing type as invalid and any other value as unsupported.
func requireType(got *string, want, what string) error {
	if got == nil || *got == "" {
		return fmt.Errorf("%w: %s type is required", ErrInvalid, what)
	}
	if *got != want {
		return fmt.Errorf("%w: %s type %q", ErrUnsupported, what, *got)
	}
	return nil
}

// requireNonEmpty reports an empty required string field.
func requireNonEmpty(value, name string) error {
	if value == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalid, name)
	}
	return nil
}

// rejectDuplicateNames rejects duplicate object member names at every nesting.
func rejectDuplicateNames(raw json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := rejectDuplicateValue(dec); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("%w: trailing JSON data", ErrInvalid)
	}
	return nil
}

// rejectDuplicateValue walks one JSON value and rejects duplicate object names.
func rejectDuplicateValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return wrapJSONError(err)
	}

	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return wrapJSONError(err)
			}
			key, ok := keyTok.(string)
			if !ok {
				return fmt.Errorf("%w: object key is not a string", ErrInvalid)
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("%w: duplicate field %q", ErrInvalid, key)
			}
			seen[key] = struct{}{}
			if err := rejectDuplicateValue(dec); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil {
			return wrapJSONError(err)
		}
		return nil
	case '[':
		for dec.More() {
			if err := rejectDuplicateValue(dec); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil {
			return wrapJSONError(err)
		}
		return nil
	default:
		return fmt.Errorf("%w: invalid JSON delimiter", ErrInvalid)
	}
}

// wrapInvalid translates a domain error into ErrInvalid without wrapping its class.
func wrapInvalid(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
}

// wrapJSONError maps encoding/json failures to ErrInvalid without raw input.
func wrapJSONError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: empty JSON message", ErrInvalid)
	}

	var syn *json.SyntaxError
	if errors.As(err, &syn) {
		return fmt.Errorf("%w: invalid JSON at offset %d", ErrInvalid, syn.Offset)
	}

	var typ *json.UnmarshalTypeError
	if errors.As(err, &typ) {
		if typ.Field != "" {
			return fmt.Errorf("%w: invalid type for field %s", ErrInvalid, typ.Field)
		}
		return fmt.Errorf("%w: invalid JSON type", ErrInvalid)
	}

	const unknownPrefix = "json: unknown field "
	msg := err.Error()
	if strings.HasPrefix(msg, unknownPrefix) {
		return fmt.Errorf("%w: unknown field %s", ErrInvalid, strings.TrimPrefix(msg, unknownPrefix))
	}
	return fmt.Errorf("%w: invalid JSON", ErrInvalid)
}
