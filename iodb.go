package iodb

import "errors"

var (
	ErrEmptyKey         = errors.New("invalid key, cannot be empty")
	ErrInvalidKeyFormat = errors.New("invalid key format: only letters, digits, '.', '_', '-' are allowed; key must not contain path separators")
)
