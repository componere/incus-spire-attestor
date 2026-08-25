// Package wire implements bounded v1 JSON codecs for Incus node-attestor
// messages.
//
// Codecs translate between internal/attest values and the opaque SPIRE payload,
// challenge, and response bytes. Every message is at most 64 KiB, must be valid
// UTF-8, and must contain exactly one JSON value. Unknown fields and duplicate
// member names are rejected at every object nesting. v1 uses exact type and
// version pairs with no negotiation or implicit downgrade.
package wire
