package base

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/k8s/registry/utils"
)

// StatusREST implements the status subresource for a BaseStorage
type StatusREST[K runtime.Object, D any] struct {
	// parent is the parent storage that contains the main resource
	parent *BaseStorage[K, D]

	// converter handles conversion between Kubernetes objects and domain models
	converter Converter[K, D]

	// validator handles validation of Kubernetes objects
	validator Validator[K]

	// resourceName is the name of the resource (e.g., "services", "addressgroups")
	resourceName string

	// kindName is the kind name of the resource (e.g., "Service", "AddressGroup")
	kindName string
}

// NewStatusREST creates a new StatusREST instance
func NewStatusREST[K runtime.Object, D any](parent *BaseStorage[K, D]) *StatusREST[K, D] {
	return &StatusREST[K, D]{
		parent:       parent,
		converter:    parent.converter,
		validator:    parent.validator,
		resourceName: parent.resourceName,
		kindName:     parent.kindName,
	}
}

// Compile-time interface assertions
var _ rest.Storage = &StatusREST[runtime.Object, any]{}
var _ rest.Getter = &StatusREST[runtime.Object, any]{}
var _ rest.Updater = &StatusREST[runtime.Object, any]{}
var _ rest.Patcher = &StatusREST[runtime.Object, any]{}

// New returns a new instance of the resource
func (s *StatusREST[K, D]) New() runtime.Object {
	return s.parent.NewFunc()
}

// Destroy cleans up resources when the storage is being shut down
func (s *StatusREST[K, D]) Destroy() {
	// Clean up any resources here if needed
}

// Get retrieves the status of a resource by name
func (s *StatusREST[K, D]) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)

	// Get the resource from backend
	domainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	// Convert to Kubernetes object
	k8sObj, err := s.converter.FromDomain(ctx, domainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert domain object to k8s object: %w", err)
	}

	return k8sObj, nil
}

// Update updates the status of an existing resource
func (s *StatusREST[K, D]) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	namespace := utils.NamespaceFrom(ctx)

	// Get the current object
	currentDomainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		return nil, false, err
	}

	// Convert current object to k8s format
	currentK8sObj, err := s.converter.FromDomain(ctx, currentDomainObj)
	if err != nil {
		return nil, false, fmt.Errorf("failed to convert current domain object to k8s object: %w", err)
	}

	// Get the updated object
	updatedObj, err := objInfo.UpdatedObject(ctx, currentK8sObj)
	if err != nil {
		return nil, false, err
	}

	updatedK8sObj, ok := updatedObj.(K)
	if !ok {
		return nil, false, fmt.Errorf("expected %T, got %T", s.parent.NewFunc(), updatedObj)
	}

	// Validate the updated object (status update validation)
	if errs := s.validator.ValidateUpdate(ctx, updatedK8sObj, currentK8sObj); len(errs) > 0 {
		return nil, false, errors.NewInvalid(
			schema.GroupKind{Group: "netguard.sgroups.io", Kind: s.kindName},
			getObjectName(updatedK8sObj),
			errs,
		)
	}

	// Run additional validation if provided
	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, currentK8sObj); err != nil {
			return nil, false, err
		}
	}

	// Convert to domain object
	updatedDomainObj, err := s.converter.ToDomain(ctx, updatedK8sObj)
	if err != nil {
		return nil, false, fmt.Errorf("failed to convert updated k8s object to domain object: %w", err)
	}

	// Update status in backend
	finalDomainObj, err := s.updateStatusInBackend(ctx, updatedDomainObj)
	if err != nil {
		return nil, false, err
	}

	// Convert back to Kubernetes object
	finalK8sObj, err := s.converter.FromDomain(ctx, finalDomainObj)
	if err != nil {
		return nil, false, fmt.Errorf("failed to convert updated domain object to k8s object: %w", err)
	}

	// Broadcast watch event
	s.broadcastWatchEvent(watch.Modified, finalK8sObj)

	return finalK8sObj, false, nil
}

// Patch applies a patch to the status of a resource
func (s *StatusREST[K, D]) Patch(ctx context.Context, name string, patchType types.PatchType, data []byte, options *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)

	// Get the current object
	currentDomainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	// Convert to k8s object
	currentK8sObj, err := s.converter.FromDomain(ctx, currentDomainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert current domain object to k8s object: %w", err)
	}

	// Apply patch
	patchedObj, err := s.applyPatch(currentK8sObj, patchType, data)
	if err != nil {
		return nil, err
	}

	patchedK8sObj, ok := patchedObj.(K)
	if !ok {
		return nil, fmt.Errorf("expected %T, got %T", s.parent.NewFunc(), patchedObj)
	}

	// Validate the patched object
	if errs := s.validator.ValidateUpdate(ctx, patchedK8sObj, currentK8sObj); len(errs) > 0 {
		return nil, errors.NewInvalid(
			schema.GroupKind{Group: "netguard.sgroups.io", Kind: s.kindName},
			getObjectName(patchedK8sObj),
			errs,
		)
	}

	// Convert to domain object
	patchedDomainObj, err := s.converter.ToDomain(ctx, patchedK8sObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert patched k8s object to domain object: %w", err)
	}

	// Update status in backend
	finalDomainObj, err := s.updateStatusInBackend(ctx, patchedDomainObj)
	if err != nil {
		return nil, err
	}

	// Convert back to Kubernetes object
	finalK8sObj, err := s.converter.FromDomain(ctx, finalDomainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert final domain object to k8s object: %w", err)
	}

	// Broadcast watch event
	s.broadcastWatchEvent(watch.Modified, finalK8sObj)

	return finalK8sObj, nil
}

// Helper methods for backend operations
func (s *StatusREST[K, D]) getFromBackend(ctx context.Context, namespace, name string) (D, error) {
	// Delegate to parent storage
	domainObjPtr, err := s.parent.getFromBackend(ctx, namespace, name)
	if err != nil {
		var zero D
		return zero, err
	}
	return *domainObjPtr, nil
}

func (s *StatusREST[K, D]) updateStatusInBackend(ctx context.Context, obj D) (D, error) {
	// This is a generic method that needs to be implemented by specific backend methods
	// For now, we'll return an error indicating the method needs to be implemented
	var zero D
	return zero, fmt.Errorf("updateStatusInBackend not implemented for resource %s", s.resourceName)
}

// Helper methods
func (s *StatusREST[K, D]) broadcastWatchEvent(eventType watch.EventType, obj runtime.Object) {
	if s.parent.watcher != nil {
		s.parent.watcher.Action(eventType, obj)
	}
}

func (s *StatusREST[K, D]) applyPatch(current runtime.Object, patchType types.PatchType, data []byte) (runtime.Object, error) {
	// Delegate to parent storage
	return s.parent.applyPatch(current, patchType, data)
}
