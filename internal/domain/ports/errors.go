package ports

import (
	"errors"
	"fmt"
)

// Standard repository errors
var (
	// ErrNotFound is returned when the requested entity is not found
	ErrNotFound = errors.New("entity not found")
)

// CIDROverlapError is returned when a CIDR range overlaps with existing networks
type CIDROverlapError struct {
	CIDR            string
	OverlappingCIDR string
	NetworkName     string
	Err             error
}

func (e *CIDROverlapError) Error() string {
	if e.NetworkName != "" && e.OverlappingCIDR != "" {
		return fmt.Sprintf("CIDR '%s' overlaps with existing network %s (CIDR: %s)",
			e.CIDR, e.NetworkName, e.OverlappingCIDR)
	}
	return fmt.Sprintf("CIDR '%s' overlaps with existing network", e.CIDR)
}

func (e *CIDROverlapError) Unwrap() error {
	return e.Err
}
