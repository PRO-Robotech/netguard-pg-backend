package ports

import (
	"fmt"
	"strings"

	"netguard-pg-backend/internal/domain/models"
)

// EmptyScope represents an empty scope
type EmptyScope struct{}

// IsEmpty returns true for EmptyScope
func (EmptyScope) IsEmpty() bool {
	return true
}

// String returns a string representation of EmptyScope
func (EmptyScope) String() string {
	return "empty"
}

// ResourceIdentifierScope represents a scope with resource identifiers
type ResourceIdentifierScope struct {
	Identifiers []models.ResourceIdentifier
}

// IsEmpty returns true if ResourceIdentifierScope is empty
func (s ResourceIdentifierScope) IsEmpty() bool {
	return len(s.Identifiers) == 0
}

// String returns a string representation of ResourceIdentifierScope
func (s ResourceIdentifierScope) String() string {
	if s.IsEmpty() {
		return "empty"
	}

	identifiers := make([]string, 0, len(s.Identifiers))
	for _, id := range s.Identifiers {
		identifiers = append(identifiers, id.Key())
	}

	return fmt.Sprintf("identifiers(%s)", strings.Join(identifiers, ","))
}

// NewResourceIdentifierScope creates a new ResourceIdentifierScope
func NewResourceIdentifierScope(identifiers ...models.ResourceIdentifier) ResourceIdentifierScope {
	return ResourceIdentifierScope{
		Identifiers: identifiers,
	}
}
