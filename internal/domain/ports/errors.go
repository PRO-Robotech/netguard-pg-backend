package ports

import "errors"

// Standard repository errors
var (
	// ErrNotFound is returned when the requested entity is not found
	ErrNotFound = errors.New("entity not found")
)
