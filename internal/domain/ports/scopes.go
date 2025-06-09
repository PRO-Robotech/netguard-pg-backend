package ports

import (
	"fmt"
	"strings"
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

// NameScope represents a scope with a name
type NameScope struct {
	Names []string
}

// IsEmpty returns true if NameScope is empty
func (s NameScope) IsEmpty() bool {
	return len(s.Names) == 0
}

// String returns a string representation of NameScope
func (s NameScope) String() string {
	if s.IsEmpty() {
		return "empty"
	}
	return fmt.Sprintf("names(%s)", strings.Join(s.Names, ","))
}

// NewNameScope creates a new NameScope
func NewNameScope(names ...string) NameScope {
	return NameScope{Names: names}
}
