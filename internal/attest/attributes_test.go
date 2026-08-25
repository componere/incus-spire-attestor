package attest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTrustDomain  = "example.org"
	maxSelectors     = 100
	maxSelectorBytes = 32768
)

func attributeInstance() Instance {
	return Instance{
		Project:     ProjectName(testProject),
		Name:        InstanceName(testInstanceName),
		UUID:        InstanceUUID(canonicalUUID),
		Type:        InstanceTypeVirtualMachine,
		Location:    testLocation,
		CloudInitID: testCloudInitID,
		Profiles:    []string{"default"},
		ExpandedConfig: map[string]string{
			"user.role":        "app",
			"user.environment": "prod",
			"volatile.uuid":    canonicalUUID,
		},
	}
}

func selectorValueBytes(selectors []string) int {
	n := 0
	for _, selector := range selectors {
		n += len(selector)
	}
	return n
}

func TestBuildAttributesUsesOnlyAPIInstance(t *testing.T) {
	t.Parallel()

	instance := attributeInstance()
	got, err := BuildAttributes(testTrustDomain, instance, []string{"user.role", "user.environment", "user.missing"})
	require.NoError(t, err)

	assert.Equal(t, "spiffe://example.org/spire/agent/incus/"+canonicalUUID, got.AgentID)
	assert.True(t, got.CanReattest)
	assert.Equal(t, []string{
		"location:" + testLocation,
		"name:" + testInstanceName,
		"profile:default",
		"project:" + testProject,
		"user.environment:prod",
		"user.role:app",
		"uuid:" + canonicalUUID,
	}, got.Selectors)
}

func TestBuildAttributesOmitsMissingSelectedUserKeys(t *testing.T) {
	t.Parallel()

	instance := attributeInstance()
	got, err := BuildAttributes(testTrustDomain, instance, []string{"user.role", "user.absent", "user.also-absent"})
	require.NoError(t, err)

	assert.NotContains(t, got.Selectors, "user.absent:")
	assert.NotContains(t, got.Selectors, "user.also-absent:")
	assert.Contains(t, got.Selectors, "user.role:app")
	for _, selector := range got.Selectors {
		assert.False(t, strings.HasPrefix(selector, "user.absent"))
		assert.False(t, strings.HasPrefix(selector, "user.also-absent"))
	}
}

func TestBuildAttributesSortsAndDeduplicatesSelectors(t *testing.T) {
	t.Parallel()

	instance := attributeInstance()
	instance.Profiles = []string{"web", "default", "web", "core"}
	got, err := BuildAttributes(testTrustDomain, instance, []string{"user.role", "user.environment", "user.role"})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"location:" + testLocation,
		"name:" + testInstanceName,
		"profile:core",
		"profile:default",
		"profile:web",
		"project:" + testProject,
		"user.environment:prod",
		"user.role:app",
		"uuid:" + canonicalUUID,
	}, got.Selectors)
}

func TestBuildAttributesCanonicalizesAPIUUID(t *testing.T) {
	t.Parallel()

	instance := attributeInstance()
	instance.UUID = "550E8400-E29B-41D4-A716-446655440000"
	got, err := BuildAttributes(testTrustDomain, instance, nil)
	require.NoError(t, err)

	assert.Equal(t, "spiffe://example.org/spire/agent/incus/"+canonicalUUID, got.AgentID)
	assert.Contains(t, got.Selectors, "uuid:"+canonicalUUID)
}

func TestBuildAttributesRejectsMalformedUUID(t *testing.T) {
	t.Parallel()

	instance := attributeInstance()
	instance.UUID = "not-a-uuid"
	_, err := BuildAttributes(testTrustDomain, instance, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDenied)
}

func TestBuildAttributesRejectsReservedNonceSelectorKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
	}{
		{name: "exact reserved prefix", key: configKeyPrefix},
		{name: "reserved prefix with hex suffix", key: validConfigKey()},
		{name: "reserved prefix with extra segment", key: configKeyPrefix + "extra"},
		{name: "reserved prefix absent from expanded config", key: configKeyPrefix + "deadbeefdeadbeefdeadbeefdeadbeef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			instance := attributeInstance()
			_, err := BuildAttributes(testTrustDomain, instance, []string{tt.key})
			require.Error(t, err, "reserved nonce namespace must be denied even when absent")
			assert.ErrorIs(t, err, ErrDenied)
		})
	}
}

func TestBuildAttributesEnforcesSelectorCountBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		profileCount int
		wantErr      bool
	}{
		{name: "below 100 selectors", profileCount: 95, wantErr: false},
		{name: "exactly 100 selectors", profileCount: 96, wantErr: false},
		{name: "above 100 selectors", profileCount: 97, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			instance := attributeInstance()
			instance.Profiles = make([]string, tt.profileCount)
			for i := range instance.Profiles {
				instance.Profiles[i] = fmt.Sprintf("p%03d", i)
			}

			got, err := BuildAttributes(testTrustDomain, instance, nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrDenied)
				return
			}

			require.NoError(t, err)
			assert.LessOrEqual(t, len(got.Selectors), maxSelectors)
			assert.Equal(t, 4+tt.profileCount, len(got.Selectors))
		})
	}
}

func TestBuildAttributesEnforcesSelectorByteBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		total   int
		wantErr bool
	}{
		{name: "below 32768 UTF-8 bytes", total: maxSelectorBytes - 1, wantErr: false},
		{name: "exactly 32768 UTF-8 bytes", total: maxSelectorBytes, wantErr: false},
		{name: "above 32768 UTF-8 bytes", total: maxSelectorBytes + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			instance := attributeInstance()
			instance.Profiles = nil
			fixed := []string{
				"location:" + testLocation,
				"name:" + testInstanceName,
				"project:" + testProject,
				"uuid:" + canonicalUUID,
			}
			fixedBytes := selectorValueBytes(fixed)
			require.Greater(t, tt.total, fixedBytes+len("user.pad:"), "test padding must leave room for the user selector")

			padding := strings.Repeat("a", tt.total-fixedBytes-len("user.pad:"))
			instance.ExpandedConfig = map[string]string{"user.pad": padding}

			got, err := BuildAttributes(testTrustDomain, instance, []string{"user.pad"})
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrDenied)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.total, selectorValueBytes(got.Selectors))
		})
	}
}

func TestBuildAttributesRejectsNonVirtualMachineInstance(t *testing.T) {
	t.Parallel()

	instance := attributeInstance()
	instance.Type = "container"
	_, err := BuildAttributes(testTrustDomain, instance, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDenied)
}
