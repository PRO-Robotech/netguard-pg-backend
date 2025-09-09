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

// EntityAlreadyExistsError represents an error when trying to create an entity that already exists
type EntityAlreadyExistsError struct {
	EntityType      string
	EntityID        string
	ExistingEntity  interface{} // Store details of existing entity for debugging
	ConflictDetails string      // Human-readable conflict description
	SuggestedAction string      // Actionable guidance for the user
}

func (e *EntityAlreadyExistsError) Error() string {
	if e.SuggestedAction != "" {
		return fmt.Sprintf("%s with id %s already exists. %s. Suggested action: %s",
			e.EntityType, e.EntityID, e.ConflictDetails, e.SuggestedAction)
	}
	return fmt.Sprintf("%s with id %s already exists. %s",
		e.EntityType, e.EntityID, e.ConflictDetails)
}

// NewEntityAlreadyExistsError creates a new entity already exists error with detailed context
func NewEntityAlreadyExistsError(entityType, entityID string, existingEntity interface{}, conflictDetails, suggestedAction string) *EntityAlreadyExistsError {
	return &EntityAlreadyExistsError{
		EntityType:      entityType,
		EntityID:        entityID,
		ExistingEntity:  existingEntity,
		ConflictDetails: conflictDetails,
		SuggestedAction: suggestedAction,
	}
}

// ValidationConflictError represents a conflict during validation that's not specifically about existence
type ValidationConflictError struct {
	EntityType       string
	EntityID         string
	ConflictType     string // e.g., "port_conflict", "name_collision", "circular_reference"
	ConflictDetails  string
	AffectedEntities []string // IDs of other entities involved in the conflict
}

func (e *ValidationConflictError) Error() string {
	if len(e.AffectedEntities) > 0 {
		return fmt.Sprintf("%s validation conflict for %s with id %s: %s. Affected entities: %v",
			e.ConflictType, e.EntityType, e.EntityID, e.ConflictDetails, e.AffectedEntities)
	}
	return fmt.Sprintf("%s validation conflict for %s with id %s: %s",
		e.ConflictType, e.EntityType, e.EntityID, e.ConflictDetails)
}

// NewValidationConflictError creates a new validation conflict error
func NewValidationConflictError(entityType, entityID, conflictType, conflictDetails string, affectedEntities []string) *ValidationConflictError {
	return &ValidationConflictError{
		EntityType:       entityType,
		EntityID:         entityID,
		ConflictType:     conflictType,
		ConflictDetails:  conflictDetails,
		AffectedEntities: affectedEntities,
	}
}
