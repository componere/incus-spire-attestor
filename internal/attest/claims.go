package attest

import (
	"fmt"

	"github.com/google/uuid"
)

// NewInstanceUUID parses raw as a UUID and stores the canonical lowercase form.
func NewInstanceUUID(raw string) (InstanceUUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid instance uuid %q: %w", raw, err)
	}
	return InstanceUUID(id.String()), nil
}

// ValidateClaims reports whether claims contain every required field and name a VM.
func ValidateClaims(claims Claims) error {
	if claims.Name == "" {
		return fmt.Errorf("%w: instance name is required", ErrDenied)
	}
	if claims.Location == "" {
		return fmt.Errorf("%w: location is required", ErrDenied)
	}
	if claims.CloudInitID == "" {
		return fmt.Errorf("%w: cloud-init id is required", ErrDenied)
	}
	if err := requireVirtualMachine(claims.Type, "guest"); err != nil {
		return err
	}
	if _, err := NewInstanceUUID(string(claims.UUID)); err != nil {
		return fmt.Errorf("%w: %v", ErrDenied, err)
	}
	return nil
}

// MatchClaims reports whether claims describe the same allowed VM as instance.
func MatchClaims(claims Claims, instance Instance) error {
	if err := ValidateClaims(claims); err != nil {
		return err
	}
	if err := validateInstance(instance); err != nil {
		return err
	}
	if claims.Project != "" && claims.Project != instance.Project {
		return fmt.Errorf("%w: project mismatch", ErrDenied)
	}
	if claims.Name != instance.Name {
		return fmt.Errorf("%w: instance name mismatch", ErrDenied)
	}
	if claims.Type != instance.Type {
		return fmt.Errorf("%w: instance type mismatch", ErrDenied)
	}
	if claims.Location != instance.Location {
		return fmt.Errorf("%w: location mismatch", ErrDenied)
	}
	if claims.CloudInitID != instance.CloudInitID {
		return fmt.Errorf("%w: cloud-init id mismatch", ErrDenied)
	}

	claimUUID, err := NewInstanceUUID(string(claims.UUID))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDenied, err)
	}
	instanceUUID, err := NewInstanceUUID(string(instance.UUID))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDenied, err)
	}
	if claimUUID != instanceUUID {
		return fmt.Errorf("%w: instance uuid mismatch", ErrDenied)
	}
	return nil
}

// validateInstance reports whether instance contains the required VM identity.
func validateInstance(instance Instance) error {
	if instance.Name == "" {
		return fmt.Errorf("%w: instance name is required", ErrDenied)
	}
	if instance.Location == "" {
		return fmt.Errorf("%w: location is required", ErrDenied)
	}
	if instance.CloudInitID == "" {
		return fmt.Errorf("%w: cloud-init id is required", ErrDenied)
	}
	if err := requireVirtualMachine(instance.Type, "api"); err != nil {
		return err
	}
	if _, err := NewInstanceUUID(string(instance.UUID)); err != nil {
		return fmt.Errorf("%w: %v", ErrDenied, err)
	}
	return nil
}

// requireVirtualMachine denies any instance type other than virtual-machine.
func requireVirtualMachine(instanceType InstanceType, source string) error {
	if instanceType == "" {
		return fmt.Errorf("%w: %s instance type is required", ErrDenied, source)
	}
	if instanceType != InstanceTypeVirtualMachine {
		return fmt.Errorf("%w: %s instance type %q is not %q", ErrDenied, source, instanceType, InstanceTypeVirtualMachine)
	}
	return nil
}
