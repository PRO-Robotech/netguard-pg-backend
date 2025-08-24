package base

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Converter defines the interface for converting between Kubernetes objects and domain models
type Converter[K runtime.Object, D any] interface {
	// ToDomain converts a Kubernetes object to a domain model
	ToDomain(ctx context.Context, k8sObj K) (D, error)

	// FromDomain converts a domain model to a Kubernetes object
	FromDomain(ctx context.Context, domainObj D) (K, error)

	// ToList converts a slice of domain models to a Kubernetes list object
	ToList(ctx context.Context, domainObjs []D) (runtime.Object, error)
}

// Validator defines the interface for validating Kubernetes objects
type Validator[K runtime.Object] interface {
	// ValidateCreate validates a new object being created
	ValidateCreate(ctx context.Context, obj K) field.ErrorList

	// ValidateUpdate validates an object being updated
	ValidateUpdate(ctx context.Context, obj K, old K) field.ErrorList

	// ValidateDelete validates an object being deleted
	ValidateDelete(ctx context.Context, obj K) field.ErrorList
}

// ResourceIdentifier defines the interface for extracting resource identifiers
type ResourceIdentifier interface {
	// GetNamespace returns the namespace of the resource
	GetNamespace() string

	// GetName returns the name of the resource
	GetName() string

	// GetUID returns the UID of the resource
	GetUID() string
}

// DomainObject defines the interface that domain objects must implement
type DomainObject interface {
	// GetID returns the unique identifier for the domain object
	GetID() string

	// GetNamespace returns the namespace for the domain object
	GetNamespace() string

	// GetName returns the name for the domain object
	GetName() string
}
