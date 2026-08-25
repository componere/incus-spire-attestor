package attest

import "errors"

// ErrDenied is the inspectable class for attestation denial.
var ErrDenied = errors.New("attestation denied")
