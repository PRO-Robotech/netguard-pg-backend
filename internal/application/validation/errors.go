package validation

import (
	"fmt"
)

// ValidationError represents a generic validation error
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// NewValidationError creates a new validation error
func NewValidationError(message string) *ValidationError {
	return &ValidationError{
		Message: message,
	}
}

// EntityNotFoundError represents an error when an entity is not found
type EntityNotFoundError struct {
	EntityType string
	EntityID   string
}

func (e *EntityNotFoundError) Error() string {
	return fmt.Sprintf("%s with id %s not found", e.EntityType, e.EntityID)
}

// NewEntityNotFoundError creates a new entity not found error
func NewEntityNotFoundError(entityType, entityID string) *EntityNotFoundError {
	return &EntityNotFoundError{
		EntityType: entityType,
		EntityID:   entityID,
	}
}

// DependencyExistsError represents an error when a dependency exists and prevents an operation
type DependencyExistsError struct {
	EntityType     string
	EntityID       string
	DependencyType string
}

func (e *DependencyExistsError) Error() string {
	return fmt.Sprintf("cannot delete %s with id %s: it is referenced by %s", e.EntityType, e.EntityID, e.DependencyType)
}

// NewDependencyExistsError creates a new dependency exists error
func NewDependencyExistsError(entityType, entityID, dependencyType string) *DependencyExistsError {
	return &DependencyExistsError{
		EntityType:     entityType,
		EntityID:       entityID,
		DependencyType: dependencyType,
	}
}