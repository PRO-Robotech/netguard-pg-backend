package validation

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// BaseValidator provides common validation functionality for all entity validators
type BaseValidator struct {
	reader       ports.Reader
	entityType   string
	listFunction func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error
}

// NewBaseValidator creates a new base validator
func NewBaseValidator(reader ports.Reader, entityType string, listFunction func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error) *BaseValidator {
	return &BaseValidator{
		reader:       reader,
		entityType:   entityType,
		listFunction: listFunction,
	}
}

// ValidateExists checks if an entity exists
func (v *BaseValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier, keyExtractor func(interface{}) string) error {
	exists := false
	err := v.listFunction(ctx, func(entity interface{}) error {
		if keyExtractor(entity) == id.Key() {
			exists = true
		}
		return nil
	}, ports.NewResourceIdentifierScope(id))

	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to check %s existence", v.entityType))
	}

	if !exists {
		return NewEntityNotFoundError(v.entityType, id.Key())
	}

	return nil
}
