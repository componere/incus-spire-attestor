package wire

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

func TestDecodePayloadGoldenRoundTrip(t *testing.T) {
	t.Parallel()

	got, err := DecodePayload(golden(t, "payload-v1.json"))
	require.NoError(t, err, "architecture payload example must decode")
	assert.Equal(t, validClaims(), got)

	encoded, err := EncodePayload(got)
	require.NoError(t, err, "decoded golden claims must encode")

	roundTripped, err := DecodePayload(encoded)
	require.NoError(t, err)
	assert.Equal(t, validClaims(), roundTripped)
}

func TestEncodePayloadSemanticRoundTrip(t *testing.T) {
	t.Parallel()

	encoded, err := EncodePayload(validClaims())
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), maxMessageBytes)
	require.True(t, json.Valid(encoded), "encoded payload must be JSON")

	got, err := DecodePayload(encoded)
	require.NoError(t, err)
	assert.Equal(t, validClaims(), got)
}

func TestEncodePayloadOmitsAbsentProject(t *testing.T) {
	t.Parallel()

	claims := validClaims()
	claims.Project = ""

	encoded, err := EncodePayload(claims)
	require.NoError(t, err)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(encoded, &envelope))
	evidence, ok := envelope["evidence"].([]any)
	require.True(t, ok)
	require.Len(t, evidence, 1)
	item, ok := evidence[0].(map[string]any)
	require.True(t, ok)
	data, ok := item["data"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, data, "project", "empty project must be omitted from the wire object")

	got, err := DecodePayload(encoded)
	require.NoError(t, err)
	assert.Equal(t, claims, got)
}

func TestDecodePayloadAllowsOmittedProject(t *testing.T) {
	t.Parallel()

	raw := `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{` +
		`"instance_name":"vm-01","instance_type":"virtual-machine",` +
		`"uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01",` +
		`"cloud_init_id":"i-0123456789abcdef"}}]}`

	got, err := DecodePayload([]byte(raw))
	require.NoError(t, err, "omitted project is the optional guest hint")
	want := validClaims()
	want.Project = ""
	assert.Equal(t, want, got)
}

func TestDecodePayloadAcceptsContainerType(t *testing.T) {
	t.Parallel()

	raw := `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{` +
		`"instance_name":"vm-01","project":"default","instance_type":"container",` +
		`"uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01",` +
		`"cloud_init_id":"i-0123456789abcdef"}}]}`

	got, err := DecodePayload([]byte(raw))
	require.NoError(t, err, "wire must accept container type so domain can deny later")
	assert.Equal(t, attest.InstanceType("container"), got.Type)
	assert.NotErrorIs(t, err, attest.ErrDenied)
}

func TestEncodePayloadAcceptsContainerType(t *testing.T) {
	t.Parallel()

	claims := validClaims()
	claims.Type = "container"

	raw, err := EncodePayload(claims)
	require.NoError(t, err, "wire must preserve container type so domain can deny later")
	got, err := DecodePayload(raw)
	require.NoError(t, err)
	assert.Equal(t, attest.InstanceType("container"), got.Type)
}

func TestEncodePayloadRejectsStructuralValidationFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*attest.Claims)
	}{
		{name: "missing instance name", mutate: func(c *attest.Claims) { c.Name = "" }},
		{name: "missing location", mutate: func(c *attest.Claims) { c.Location = "" }},
		{name: "missing cloud-init id", mutate: func(c *attest.Claims) { c.CloudInitID = "" }},
		{name: "missing instance type", mutate: func(c *attest.Claims) { c.Type = "" }},
		{name: "invalid uuid", mutate: func(c *attest.Claims) { c.UUID = "not-a-uuid" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			claims := validClaims()
			tt.mutate(&claims)

			got, err := EncodePayload(claims)
			assertInvalid(t, err)
			assert.Empty(t, got)
		})
	}
}

func TestEncodePayloadRejectsOversizedMessage(t *testing.T) {
	t.Parallel()

	claims := validClaims()
	claims.Location = strings.Repeat("m", maxMessageBytes+1)

	got, err := EncodePayload(claims)
	assertInvalid(t, err)
	assert.Empty(t, got)
}

func TestDecodePayloadRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "outer extra field",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}],"extra":true}`,
		},
		{
			name: "item extra field",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"extra":true,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "data extra field",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef","hwaddr":"00:16:3e:aa:bb:cc"}}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodePayload([]byte(tt.raw))
			assertInvalid(t, err)
			assert.Zero(t, got)
		})
	}
}

func TestDecodePayloadRejectsDuplicateMembers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "outer version duplicated",
			raw:  `{"version":1,"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "outer evidence duplicated",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}],"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "item type duplicated",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "item version duplicated",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "item data duplicated",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"},"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "data instance_name duplicated",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","instance_name":"other","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "data uuid duplicated",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodePayload([]byte(tt.raw))
			assertInvalid(t, err)
			assert.Zero(t, got)
		})
	}
}

func TestDecodePayloadRejectsMissingAndEmptyFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "missing version",
			raw:  `{"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{name: "missing evidence", raw: `{"version":1}`},
		{name: "null evidence", raw: `{"version":1,"evidence":null}`},
		{
			name: "missing item type",
			raw:  `{"version":1,"evidence":[{"version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "empty item type",
			raw:  `{"version":1,"evidence":[{"type":"","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "missing item version",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{name: "missing item data", raw: `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1}]}`},
		{
			name: "null item data",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":null}]}`,
		},
		{
			name: "missing instance_name",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "empty instance_name",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "missing instance_type",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "empty instance_type",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "missing uuid",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "empty uuid",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "invalid uuid",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"not-a-uuid","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "missing location",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "empty location",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "missing cloud_init_id",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01"}}]}`,
		},
		{
			name: "empty cloud_init_id",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":""}}]}`,
		},
		{
			name: "present empty project",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodePayload([]byte(tt.raw))
			assertInvalid(t, err)
			assert.Zero(t, got)
		})
	}
}

func TestDecodePayloadRejectsEvidenceCountAndType(t *testing.T) {
	t.Parallel()

	item := `{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}`

	tests := []struct {
		name string
		raw  string
		want error
	}{
		{name: "zero evidence items", raw: `{"version":1,"evidence":[]}`, want: ErrInvalid},
		{name: "two evidence items", raw: `{"version":1,"evidence":[` + item + `,` + item + `]}`, want: ErrInvalid},
		{
			name: "wrong evidence type",
			raw:  `{"version":1,"evidence":[{"type":"tpm-signed-document","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
			want: ErrUnsupported,
		},
		{
			name: "challenge type as evidence",
			raw:  `{"version":1,"evidence":[{"type":"incus-config-nonce","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
			want: ErrUnsupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodePayload([]byte(tt.raw))
			require.Error(t, err)
			require.ErrorIs(t, err, tt.want)
			require.NotErrorIs(t, err, attest.ErrDenied)
			assert.Zero(t, got)
		})
	}
}

func TestDecodePayloadRejectsWrongVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "outer version 2",
			raw:  `{"version":2,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "outer version 0",
			raw:  `{"version":0,"evidence":[{"type":"incus-guest-claims","version":1,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "item version 2",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":2,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
		{
			name: "item version 0",
			raw:  `{"version":1,"evidence":[{"type":"incus-guest-claims","version":0,"data":{"instance_name":"vm-01","project":"default","instance_type":"virtual-machine","uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01","cloud_init_id":"i-0123456789abcdef"}}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodePayload([]byte(tt.raw))
			assertUnsupported(t, err)
			assert.Zero(t, got)
		})
	}
}
