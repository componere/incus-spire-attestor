package wire

import "errors"

// ErrInvalid is the inspectable class for a malformed wire message.
var ErrInvalid = errors.New("invalid wire message")

// ErrUnsupported is the inspectable class for an unknown or later wire contract.
var ErrUnsupported = errors.New("unsupported wire message")
