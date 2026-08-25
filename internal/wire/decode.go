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

	if err := rejectDuplicateNames(raw); err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// decodeStrict unmarshals raw into dest and rejects unknown fields.
func decodeStrict(raw []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return wrapJSONError(err)
	}
	if err := requireJSONEOF(dec); err != nil {
		return err
	}
	return nil
}

// requireJSONEOF rejects any second top-level JSON value or trailing garbage.
func requireJSONEOF(dec *json.Decoder) error {
	var extra json.RawMessage
	err := dec.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return fmt.Errorf("%w: trailing JSON data", ErrInvalid)
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
	if err := requireVersion(obj.Version, what); err != nil {
		return nil, err
	}
	if len(obj.Data) == 0 {
		return nil, fmt.Errorf("%w: %s data is required", ErrInvalid, what)
	}
	return obj.Data, nil
}

// requireVersion reports a missing version as invalid and any non-v1 value as unsupported.
func requireVersion(got *int, what string) error {
	if got == nil {
		return fmt.Errorf("%w: %s version is required", ErrInvalid, what)
	}
	if *got != 1 {
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
		return fmt.Errorf("%w: unsupported %s type", ErrUnsupported, what)
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
	if err := requireJSONEOF(dec); err != nil {
		return err
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
		return rejectDuplicateObject(dec)
	case '[':
		return rejectDuplicateArray(dec)
	default:
		return fmt.Errorf("%w: invalid JSON delimiter", ErrInvalid)
	}
}

// rejectDuplicateObject walks an open JSON object and rejects repeated member names.
func rejectDuplicateObject(dec *json.Decoder) error {
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
			return fmt.Errorf("%w: duplicate field", ErrInvalid)
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
}

// rejectDuplicateArray walks an open JSON array and checks every element.
func rejectDuplicateArray(dec *json.Decoder) error {
	for dec.More() {
		if err := rejectDuplicateValue(dec); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return wrapJSONError(err)
	}
	return nil
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
	if strings.HasPrefix(err.Error(), unknownPrefix) {
		return fmt.Errorf("%w: unknown field", ErrInvalid)
	}
	return fmt.Errorf("%w: invalid JSON", ErrInvalid)
}
