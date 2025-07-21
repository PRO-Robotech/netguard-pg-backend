package base

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// BackendOperations defines the interface for backend operations for any resource type.
// This interface unifies all CRUD operations across different resource types,
// eliminating the need for resource-specific method stubs in BaseStorage.
type BackendOperations[D any] interface {
	// Get retrieves a single resource by its identifier
	Get(ctx context.Context, id models.ResourceIdentifier) (*D, error)

	// List retrieves multiple resources based on the provided scope
	List(ctx context.Context, scope ports.Scope) ([]D, error)

	// Create creates a new resource in the backend
	Create(ctx context.Context, obj *D) error

	// Update updates an existing resource in the backend
	Update(ctx context.Context, obj *D) error

	// Delete removes a resource from the backend by its identifier
	Delete(ctx context.Context, id models.ResourceIdentifier) error
}
