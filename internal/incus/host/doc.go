// Package host implements the official Incus API adapter for the server port.
//
// It looks up instances and performs ETag-protected one-key nonce mutations
// through github.com/lxc/incus/v7/client. Mapping and exact-key updates stay
// in this package; TLS and REST mechanics stay in the pinned client.
// On a standalone server, Incus reports instance location as "none"; the
// adapter substitutes Environment.ServerName from one /1.0 read at construction.
package host
