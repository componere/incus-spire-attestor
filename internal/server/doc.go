// Package server implements the Incus node-attestor server application flow.
//
// It decodes guest claims, resolves an allowed virtual-machine through a
// narrow Incus port, challenges the guest with a single-use config nonce, and
// emits SPIRE attributes from the retained API snapshot.
package server
