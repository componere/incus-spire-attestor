// Package guest implements the Incus guest-socket evidence adapter.
//
// Client reads instance locators from the guest Unix socket and the DMI
// product UUID, then reads one challenged config value for the agent poll
// loop. The guest API returns raw JSON and plain text rather than the main
// Incus sync envelope.
package guest
