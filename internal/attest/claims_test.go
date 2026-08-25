package attest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testInstanceName = "web-01"
	testLocation     = "node-a"
	testCloudInitID  = "i-abc123"
	testProject      = "default"
)

func validClaims() Claims {
	return Claims{
		Name:        InstanceName(testInstanceName),
		UUID:        InstanceUUID(canonicalUUID),
		Type:        InstanceTypeVirtualMachine,
		Location:    testLocation,
		CloudInitID: testCloudInitID,
	}
}

func validInstance() Instance {
	return Instance{
		Project:        ProjectName(testProject),
		Name:           InstanceName(testInstanceName),
		UUID:           InstanceUUID(canonicalUUID),
		Type:           InstanceTypeVirtualMachine,
		Location:       testLocation,
		CloudInitID:    testCloudInitID,
		Profiles:       []string{"default"},
		ExpandedConfig: map[string]string{"user.role": "app"},
	}
}

func TestValidateClaimsAcceptsRequiredFieldsWithOptionalProject(t *testing.T) {
	t.Parallel()

	t.Run("absent project", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, ValidateClaims(validClaims()))
	})

	t.Run("asserted project", func(t *testing.T) {
		t.Parallel()
		claims := validClaims()
		claims.Project = ProjectName(testProject)
		require.NoError(t, ValidateClaims(claims))
	})
}

func TestValidateClaimsRejectsMissingRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(Claims) Claims
	}{
		{
			name: "missing name",
			mutate: func(c Claims) Claims {
				c.Name = ""
				return c
			},
		},
		{
			name: "missing UUID",
			mutate: func(c Claims) Claims {
				c.UUID = ""
				return c
			},
		},
		{
			name: "missing type",
			mutate: func(c Claims) Claims {
				c.Type = ""
				return c
			},
		},
		{
			name: "missing location",
			mutate: func(c Claims) Claims {
				c.Location = ""
				return c
			},
		},
		{
			name: "missing cloud-init ID",
			mutate: func(c Claims) Claims {
				c.CloudInitID = ""
				return c
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateClaims(tt.mutate(validClaims()))
			require.Error(t, err, "missing required claim must be rejected")
			assert.ErrorIs(t, err, ErrDenied)
		})
	}
}

func TestValidateClaimsRejectsNonVirtualMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		guestType InstanceType
	}{
		{name: "container", guestType: "container"},
		{name: "unknown type", guestType: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			claims := validClaims()
			claims.Type = tt.guestType
			err := ValidateClaims(claims)
			require.Error(t, err, "non-VM guest type must be denied")
			assert.ErrorIs(t, err, ErrDenied)
		})
	}
}

func TestMatchClaimsAcceptsMatchingVirtualMachine(t *testing.T) {
	t.Parallel()

	t.Run("absent project is not compared", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, MatchClaims(validClaims(), validInstance()))
	})

	t.Run("asserted project matches", func(t *testing.T) {
		t.Parallel()
		claims := validClaims()
		claims.Project = ProjectName(testProject)
		require.NoError(t, MatchClaims(claims, validInstance()))
	})
}

func TestMatchClaimsCanonicalizesUUIDOnBothSides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		claimsUUID   InstanceUUID
		instanceUUID InstanceUUID
	}{
		{
			name:         "uppercase claims UUID matches canonical instance UUID",
			claimsUUID:   "550E8400-E29B-41D4-A716-446655440000",
			instanceUUID: InstanceUUID(canonicalUUID),
		},
		{
			name:         "canonical claims UUID matches uppercase instance UUID",
			claimsUUID:   InstanceUUID(canonicalUUID),
			instanceUUID: "550E8400-E29B-41D4-A716-446655440000",
		},
		{
			name:         "URN claims UUID matches hyphenless instance UUID",
			claimsUUID:   InstanceUUID("urn:uuid:" + canonicalUUID),
			instanceUUID: "550e8400e29b41d4a716446655440000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			claims := validClaims()
			claims.UUID = tt.claimsUUID
			instance := validInstance()
			instance.UUID = tt.instanceUUID
			require.NoError(t, MatchClaims(claims, instance))
		})
	}
}

func TestMatchClaimsRejectsMalformedUUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(Claims, Instance) (Claims, Instance)
	}{
		{
			name: "malformed claims UUID",
			mutate: func(c Claims, inst Instance) (Claims, Instance) {
				c.UUID = "not-a-uuid"
				return c, inst
			},
		},
		{
			name: "malformed instance UUID",
			mutate: func(c Claims, inst Instance) (Claims, Instance) {
				inst.UUID = "also-not-a-uuid"
				return c, inst
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			claims, instance := tt.mutate(validClaims(), validInstance())
			err := MatchClaims(claims, instance)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrDenied)
		})
	}
}

func TestMatchClaimsRejectsGuestAndAPIContainers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(Claims, Instance) (Claims, Instance)
	}{
		{
			name: "guest container",
			mutate: func(c Claims, inst Instance) (Claims, Instance) {
				c.Type = "container"
				return c, inst
			},
		},
		{
			name: "API container",
			mutate: func(c Claims, inst Instance) (Claims, Instance) {
				inst.Type = "container"
				return c, inst
			},
		},
		{
			name: "both sides container",
			mutate: func(c Claims, inst Instance) (Claims, Instance) {
				c.Type = "container"
				inst.Type = "container"
				return c, inst
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			claims, instance := tt.mutate(validClaims(), validInstance())
			err := MatchClaims(claims, instance)
			require.Error(t, err, "containers must be denied")
			assert.ErrorIs(t, err, ErrDenied)
		})
	}
}

func TestMatchClaimsRejectsEachFieldMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(Claims, Instance) (Claims, Instance)
	}{
		{
			name: "name",
			mutate: func(c Claims, inst Instance) (Claims, Instance) {
				c.Name = "other"
				return c, inst
			},
		},
		{
			name: "UUID",
			mutate: func(c Claims, inst Instance) (Claims, Instance) {
				c.UUID = "11111111-1111-1111-1111-111111111111"
				return c, inst
			},
		},
		{
			name: "location",
			mutate: func(c Claims, inst Instance) (Claims, Instance) {
				c.Location = "node-b"
				return c, inst
			},
		},
		{
			name: "cloud-init ID",
			mutate: func(c Claims, inst Instance) (Claims, Instance) {
				c.CloudInitID = "i-other"
				return c, inst
			},
		},
		{
			name: "non-VM API type",
			mutate: func(c Claims, inst Instance) (Claims, Instance) {
				inst.Type = InstanceType("virtual-machine-legacy")
				return c, inst
			},
		},
		{
			name: "asserted project",
			mutate: func(c Claims, inst Instance) (Claims, Instance) {
				c.Project = "other-project"
				return c, inst
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			claims, instance := tt.mutate(validClaims(), validInstance())
			err := MatchClaims(claims, instance)
			require.Error(t, err, "field mismatch must deny attestation")
			assert.ErrorIs(t, err, ErrDenied)
		})
	}
}

func TestMatchClaimsRejectsMissingRequiredInstanceFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(Instance) Instance
	}{
		{
			name: "missing instance project",
			mutate: func(inst Instance) Instance {
				inst.Project = ""
				return inst
			},
		},
		{
			name: "missing instance name",
			mutate: func(inst Instance) Instance {
				inst.Name = ""
				return inst
			},
		},
		{
			name: "missing instance UUID",
			mutate: func(inst Instance) Instance {
				inst.UUID = ""
				return inst
			},
		},
		{
			name: "missing instance type",
			mutate: func(inst Instance) Instance {
				inst.Type = ""
				return inst
			},
		},
		{
			name: "missing instance location",
			mutate: func(inst Instance) Instance {
				inst.Location = ""
				return inst
			},
		},
		{
			name: "missing instance cloud-init ID",
			mutate: func(inst Instance) Instance {
				inst.CloudInitID = ""
				return inst
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := MatchClaims(validClaims(), tt.mutate(validInstance()))
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrDenied)
		})
	}
}
