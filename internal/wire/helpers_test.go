package wire

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/componere/incus-spire-attestor/internal/attest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	canonicalUUID     = "550e8400-e29b-41d4-a716-446655440000"
	testInstanceName  = "vm-01"
	testProject       = "default"
	testLocation      = "member-01"
	testCloudInitID   = "i-0123456789abcdef"
	testConfigKey     = "user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"
	testConfigPrefix  = "user.spire.attestor.nonce."
	validConfigKeyHex = "0123456789abcdef0123456789abcdef"
	testNonceRawURL   = "3q2-7wARIjNEVWZ3iJmquw"
	maxMessageBytes   = 65536
)

func golden(t *testing.T, name string) []byte {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err, "testdata %s must be readable", name)
	return raw
}

func validClaims() attest.Claims {
	return attest.Claims{
		Project:     attest.ProjectName(testProject),
		Name:        attest.InstanceName(testInstanceName),
		UUID:        attest.InstanceUUID(canonicalUUID),
		Type:        attest.InstanceTypeVirtualMachine,
		Location:    testLocation,
		CloudInitID: testCloudInitID,
	}
}

func validConfigKey() attest.ConfigKey {
	return attest.ConfigKey(testConfigKey)
}

func validNonce() attest.Nonce {
	return attest.Nonce{
		0xde, 0xad, 0xbe, 0xef, 0x00, 0x11, 0x22, 0x33,
		0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb,
	}
}

func validPayloadJSON() string {
	return `{"version":1,"evidence":[{"type":"incus-guest-claims","version":1,"data":{` +
		`"instance_name":"vm-01","project":"default","instance_type":"virtual-machine",` +
		`"uuid":"550e8400-e29b-41d4-a716-446655440000","location":"member-01",` +
		`"cloud_init_id":"i-0123456789abcdef"}}]}`
}

func validChallengeJSON() string {
	return `{"version":1,"challenge":{"type":"incus-config-nonce","version":1,` +
		`"data":{"config_key":"user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"}}}`
}

func validResponseJSON() string {
	return `{"version":1,"response":{"type":"incus-config-nonce","version":1,` +
		`"data":{"nonce":"3q2-7wARIjNEVWZ3iJmquw"}}}`
}

func assertInvalid(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err, "expected a wire validation failure")
	assert.ErrorIs(t, err, ErrInvalid, "structural and domain-translated failures must be ErrInvalid")
	assert.NotErrorIs(t, err, ErrUnsupported, "invalid input must not be classified as unsupported")
	assert.NotErrorIs(t, err, attest.ErrDenied, "wire errors must not expose attest.ErrDenied")
}

func assertUnsupported(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err, "expected an unsupported contract")
	assert.ErrorIs(t, err, ErrUnsupported, "unknown versions and types must be ErrUnsupported")
	assert.NotErrorIs(t, err, ErrInvalid, "unsupported contract must not be classified as invalid")
	assert.NotErrorIs(t, err, attest.ErrDenied, "wire errors must not expose attest.ErrDenied")
}

func assertNoSecret(t *testing.T, err error, secrets ...string) {
	t.Helper()

	require.Error(t, err)
	msg := err.Error()
	for _, secret := range secrets {
		assert.NotContains(t, msg, secret, "error must not leak secret material")
	}
}
