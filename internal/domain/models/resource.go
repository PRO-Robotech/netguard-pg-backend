package models

import "context"

// Resource defines the interface that all domain resources must implement
type Resource interface {
	// GetID returns the unique identifier for the resource
	GetID() string

	// GetName returns the name of the resource
	GetName() string

	// GetNamespace returns the namespace of the resource
	GetNamespace() string

	// GetGeneration returns the generation of the resource
	GetGeneration() int64

	// GetMeta returns the metadata of the resource
	GetMeta() *Meta

	// DeepCopy creates a deep copy of the resource
	DeepCopy() Resource
}

// Repository defines the interface for repository operations
type Repository interface {
	// GetByID retrieves a resource by its identifier
	GetByID(ctx context.Context, id ResourceIdentifier) (Resource, error)

	// Update updates a resource
	Update(ctx context.Context, obj Resource) error

	// Create creates a new resource
	Create(ctx context.Context, obj Resource) error

	// Delete removes a resource by its identifier
	Delete(ctx context.Context, id ResourceIdentifier) error
}
