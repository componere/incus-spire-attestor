package attest

// ProjectName is an Incus project name.
type ProjectName string

// InstanceName is an Incus instance name.
type InstanceName string

// InstanceUUID is a canonical lowercase Incus instance UUID.
type InstanceUUID string

// InstanceType is an Incus instance type.
type InstanceType string

// InstanceTypeVirtualMachine is the only instance type v1 attests.
const InstanceTypeVirtualMachine InstanceType = "virtual-machine"

// ConfigKey is a guest-visible nonce challenge key.
type ConfigKey string

// Nonce is the 16-byte single-use challenge secret.
type Nonce [16]byte

// Claims are guest-supplied instance locators.
//
// Project may be empty. Every other field is required.
type Claims struct {
	// Project is the optional guest project hint.
	Project ProjectName
	// Name is the guest instance name.
	Name InstanceName
	// UUID is the guest instance UUID.
	UUID InstanceUUID
	// Type is the guest instance type.
	Type InstanceType
	// Location is the guest cluster member.
	Location string
	// CloudInitID is the guest cloud-init instance ID.
	CloudInitID string
}

// Instance is the authoritative Incus API snapshot used for identity.
type Instance struct {
	// Project is the API project that contains the instance.
	Project ProjectName
	// Name is the API instance name.
	Name InstanceName
	// UUID is the API volatile.uuid value.
	UUID InstanceUUID
	// Type is the API instance type.
	Type InstanceType
	// Location is the API cluster member.
	Location string
	// CloudInitID is the API volatile.cloud-init.instance-id value.
	CloudInitID string
	// Profiles are the API profiles applied to the instance.
	Profiles []string
	// ExpandedConfig is the API expanded instance configuration.
	ExpandedConfig map[string]string
}

// Attributes are the SPIRE agent identity and selectors for one attestation.
type Attributes struct {
	// AgentID is the SPIFFE ID derived from the API UUID.
	AgentID string
	// CanReattest reports that the agent may re-attest.
	CanReattest bool
	// Selectors are the sorted, deduplicated incus selector values.
	Selectors []string
}
