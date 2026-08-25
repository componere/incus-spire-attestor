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
	_, err := validateClaims(claims)
	return err
}

// validateClaims returns the canonical guest UUID when claims are valid.
func validateClaims(claims Claims) (InstanceUUID, error) {
	if claims.Name == "" {
		return "", fmt.Errorf("%w: instance name is required", ErrDenied)
	}
	if claims.Location == "" {
		return "", fmt.Errorf("%w: location is required", ErrDenied)
	}
	if claims.CloudInitID == "" {
		return "", fmt.Errorf("%w: cloud-init id is required", ErrDenied)
	}
	if err := requireVirtualMachine(claims.Type, "guest"); err != nil {
		return "", err
	}
	id, err := NewInstanceUUID(string(claims.UUID))
	if err != nil {
		return "", fmt.Errorf("%w: invalid instance uuid", ErrDenied)
	}
	return id, nil
}

// MatchClaims reports whether claims describe the same allowed VM as instance.
func MatchClaims(claims Claims, instance Instance) error {
	claimUUID, err := validateClaims(claims)
	if err != nil {
		return err
	}
	instanceUUID, err := validateInstance(instance)
	if err != nil {
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
	if claimUUID != instanceUUID {
		return fmt.Errorf("%w: instance uuid mismatch", ErrDenied)
	}
	return nil
}

// validateInstance returns the canonical API UUID when the identity snapshot is valid.
func validateInstance(instance Instance) (InstanceUUID, error) {
	if instance.Project == "" {
		return "", fmt.Errorf("%w: project is required", ErrDenied)
	}
	if instance.Name == "" {
		return "", fmt.Errorf("%w: instance name is required", ErrDenied)
	}
	if instance.Location == "" {
		return "", fmt.Errorf("%w: location is required", ErrDenied)
	}
	if instance.CloudInitID == "" {
		return "", fmt.Errorf("%w: cloud-init id is required", ErrDenied)
	}
	if err := requireVirtualMachine(instance.Type, "api"); err != nil {
		return "", err
	}
	id, err := NewInstanceUUID(string(instance.UUID))
	if err != nil {
		return "", fmt.Errorf("%w: invalid instance uuid", ErrDenied)
	}
	return id, nil
}

// requireVirtualMachine denies any instance type other than virtual-machine.
func requireVirtualMachine(instanceType InstanceType, source string) error {
	if instanceType == "" {
		return fmt.Errorf("%w: %s instance type is required", ErrDenied, source)
	}
	if instanceType != InstanceTypeVirtualMachine {
		return fmt.Errorf(
			"%w: %s instance type %q is not %q",
			ErrDenied,
			source,
			instanceType,
			InstanceTypeVirtualMachine,
		)
	}
	return nil
}
