// Package config decodes and validates Incus attestor plugin HCL.
//
// Decode functions parse plugin_data without I/O. Validation is pure and
// separate: it does not read TLS files or contact Incus. The only
// inspectable error class is ErrInvalid.
package config
