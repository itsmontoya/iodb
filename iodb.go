package iodb

import "errors"

var (
	// ErrEmptyKey is returned when a key argument is an empty string.
	ErrEmptyKey = errors.New("invalid key, cannot be empty")
	// ErrInvalidKeyFormat is returned when a key contains disallowed characters or separators.
	ErrInvalidKeyFormat = errors.New("invalid key format: only letters, digits, '.', '_', '-' are allowed; key must not contain path separators")
)
