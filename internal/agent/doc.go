// Package agent implements the guest attestation application flow.
//
// It reads and validates VM claims, sends the first payload, accepts one
// exact challenge, polls the guest config value, and returns a v1 nonce
// response. I/O is confined to the GuestEvidence and Exchange ports.
package agent
