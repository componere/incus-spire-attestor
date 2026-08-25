// Package attest implements the pure Incus node-attestation domain rules.
//
// It normalizes identifiers, compares guest claims to an Incus API snapshot,
// verifies challenge nonces, and builds SPIRE agent attributes without I/O.
package attest
