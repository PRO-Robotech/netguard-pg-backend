package base

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/registry/utils"
)

// BaseStorage provides a generic implementation of REST storage for Kubernetes resources
type BaseStorage[K runtime.Object, D any] struct {
	// NewFunc creates a new instance of the Kubernetes object
	NewFunc func() K

	// NewListFunc creates a new instance of the Kubernetes list object
	NewListFunc func() runtime.Object

	// backendOps handles backend operations for this resource type
	backendOps BackendOperations[D]

	// converter handles conversion between Kubernetes objects and domain models
	converter Converter[K, D]

	// validator handles validation of Kubernetes objects
	validator Validator[K]

	// watcher broadcasts watch events to clients
	watcher *watch.Broadcaster

	// resourceName is the name of the resource (e.g., "services", "addressgroups")
	resourceName string

	// kindName is the kind name of the resource (e.g., "Service", "AddressGroup")
	kindName string

	// isNamespaced indicates if the resource is namespaced
	isNamespaced bool
}

// NewBaseStorage creates a new BaseStorage instance
func NewBaseStorage[K runtime.Object, D any](
	newFunc func() K,
	newListFunc func() runtime.Object,
	backendOps BackendOperations[D],
	converter Converter[K, D],
	validator Validator[K],
	watcher *watch.Broadcaster,
	resourceName string,
	kindName string,
	isNamespaced bool,
) *BaseStorage[K, D] {
	return &BaseStorage[K, D]{
		NewFunc:      newFunc,
		NewListFunc:  newListFunc,
		backendOps:   backendOps,
		converter:    converter,
		validator:    validator,
		watcher:      watcher,
		resourceName: resourceName,
		kindName:     kindName,
		isNamespaced: isNamespaced,
	}
}

// Compile-time interface assertions
var _ rest.Storage = &BaseStorage[runtime.Object, any]{}
var _ rest.Scoper = &BaseStorage[runtime.Object, any]{}
var _ rest.Getter = &BaseStorage[runtime.Object, any]{}
var _ rest.Lister = &BaseStorage[runtime.Object, any]{}
var _ rest.Creater = &BaseStorage[runtime.Object, any]{}
var _ rest.Updater = &BaseStorage[runtime.Object, any]{}
var _ rest.Patcher = &BaseStorage[runtime.Object, any]{}
var _ rest.GracefulDeleter = &BaseStorage[runtime.Object, any]{}
var _ rest.Watcher = &BaseStorage[runtime.Object, any]{}

// New returns a new instance of the resource
func (s *BaseStorage[K, D]) New() runtime.Object {
	return s.NewFunc()
}

// NewList returns a new instance of the resource list
func (s *BaseStorage[K, D]) NewList() runtime.Object {
	return s.NewListFunc()
}

// Destroy cleans up resources when the storage is being shut down
func (s *BaseStorage[K, D]) Destroy() {
	// Clean up any resources here if needed
}

// NamespaceScoped returns whether the resource is namespace scoped
func (s *BaseStorage[K, D]) NamespaceScoped() bool {
	return s.isNamespaced
}

// Get retrieves a resource by name
func (s *BaseStorage[K, D]) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)

	// Get the resource from backend
	domainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	// Convert to Kubernetes object
	k8sObj, err := s.converter.FromDomain(ctx, *domainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert domain object to k8s object: %w", err)
	}

	return k8sObj, nil
}

// List retrieves a list of resources
func (s *BaseStorage[K, D]) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	scope := utils.ScopeFromContext(ctx)

	// Get resources from backend
	domainObjs, err := s.listFromBackend(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Convert to Kubernetes list
	listObj, err := s.converter.ToList(ctx, domainObjs)
	if err != nil {
		return nil, fmt.Errorf("failed to convert domain objects to k8s list: %w", err)
	}

	return listObj, nil
}

// ConvertToTable converts the list to table format (required by rest.Lister)
// This is a basic implementation that should be overridden by specific storage types
func (s *BaseStorage[K, D]) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	// Basic implementation - should be overridden by specific storage implementations
	table := &metav1.Table{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "meta.k8s.io/v1",
			Kind:       "Table",
		},
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Description: "Name of the resource"},
			{Name: "Age", Type: "string", Description: "Age of the resource"},
		},
	}

	// Add rows based on the object type
	if _, isList := object.(metav1.ListInterface); isList {
		items, err := meta.ExtractList(object)
		if err != nil {
			return nil, err
		}

		for _, item := range items {
			if accessor, err := meta.Accessor(item); err == nil {
				row := metav1.TableRow{
					Cells: []interface{}{
						accessor.GetName(),
						"<unknown>", // Age calculation would go here
					},
					Object: runtime.RawExtension{Object: item},
				}
				table.Rows = append(table.Rows, row)
			}
		}
	} else {
		// Single object
		if accessor, err := meta.Accessor(object); err == nil {
			row := metav1.TableRow{
				Cells: []interface{}{
					accessor.GetName(),
					"<unknown>",
				},
				Object: runtime.RawExtension{Object: object},
			}
			table.Rows = append(table.Rows, row)
		}
	}

	return table, nil
}

// Create creates a new resource
func (s *BaseStorage[K, D]) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sObj, ok := obj.(K)
	if !ok {
		return nil, fmt.Errorf("expected %T, got %T", s.NewFunc(), obj)
	}

	// Validate the object
	if errs := s.validator.ValidateCreate(ctx, k8sObj); len(errs) > 0 {
		return nil, errors.NewInvalid(
			schema.GroupKind{Group: "netguard.sgroups.io", Kind: s.kindName},
			getObjectName(k8sObj),
			errs,
		)
	}

	// Run additional validation if provided
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	// Convert to domain object
	domainObj, err := s.converter.ToDomain(ctx, k8sObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert k8s object to domain object: %w", err)
	}

	// Create in backend
	createdDomainObj, err := s.createInBackend(ctx, &domainObj)
	if err != nil {
		return nil, err
	}

	// Convert back to Kubernetes object
	createdK8sObj, err := s.converter.FromDomain(ctx, *createdDomainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert created domain object to k8s object: %w", err)
	}

	// Broadcast watch event
	s.broadcastWatchEvent(watch.Added, createdK8sObj)

	return createdK8sObj, nil
}

// Update updates an existing resource
func (s *BaseStorage[K, D]) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	namespace := utils.NamespaceFrom(ctx)

	// Get the current object
	currentDomainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) && forceAllowCreate {
			// Create new object
			obj, err := objInfo.UpdatedObject(ctx, nil)
			if err != nil {
				return nil, false, err
			}
			created, err := s.Create(ctx, obj, createValidation, &metav1.CreateOptions{})
			return created, true, err
		}
		return nil, false, err
	}

	// Convert current object to k8s format
	currentK8sObj, err := s.converter.FromDomain(ctx, *currentDomainObj)
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
		return nil, false, fmt.Errorf("expected %T, got %T", s.NewFunc(), updatedObj)
	}

	// Validate the updated object
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

	// Update in backend
	finalDomainObj, err := s.updateInBackend(ctx, &updatedDomainObj)
	if err != nil {
		return nil, false, err
	}

	// Convert back to Kubernetes object
	finalK8sObj, err := s.converter.FromDomain(ctx, *finalDomainObj)
	if err != nil {
		return nil, false, fmt.Errorf("failed to convert updated domain object to k8s object: %w", err)
	}

	// Broadcast watch event
	s.broadcastWatchEvent(watch.Modified, finalK8sObj)

	return finalK8sObj, false, nil
}

// Delete deletes a resource
func (s *BaseStorage[K, D]) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	namespace := utils.NamespaceFrom(ctx)

	// Get the object to delete
	domainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		return nil, false, err
	}

	// Convert to k8s object for validation
	k8sObj, err := s.converter.FromDomain(ctx, *domainObj)
	if err != nil {
		return nil, false, fmt.Errorf("failed to convert domain object to k8s object: %w", err)
	}

	// Validate deletion
	if errs := s.validator.ValidateDelete(ctx, k8sObj); len(errs) > 0 {
		return nil, false, errors.NewInvalid(
			schema.GroupKind{Group: "netguard.sgroups.io", Kind: s.kindName},
			getObjectName(k8sObj),
			errs,
		)
	}

	// Run additional validation if provided
	if deleteValidation != nil {
		if err := deleteValidation(ctx, k8sObj); err != nil {
			return nil, false, err
		}
	}

	// Delete from backend
	err = s.deleteFromBackend(ctx, namespace, name)
	if err != nil {
		return nil, false, err
	}

	// Broadcast watch event
	s.broadcastWatchEvent(watch.Deleted, k8sObj)

	return k8sObj, true, nil
}

// Patch applies a patch to a resource
func (s *BaseStorage[K, D]) Patch(ctx context.Context, name string, patchType types.PatchType, data []byte, options *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)

	// Get the current object
	currentDomainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	// Convert to k8s object
	currentK8sObj, err := s.converter.FromDomain(ctx, *currentDomainObj)
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
		return nil, fmt.Errorf("expected %T, got %T", s.NewFunc(), patchedObj)
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

	// Update in backend
	finalDomainObj, err := s.updateInBackend(ctx, &patchedDomainObj)
	if err != nil {
		return nil, err
	}

	// Convert back to Kubernetes object
	finalK8sObj, err := s.converter.FromDomain(ctx, *finalDomainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert final domain object to k8s object: %w", err)
	}

	// Broadcast watch event
	s.broadcastWatchEvent(watch.Modified, finalK8sObj)

	return finalK8sObj, nil
}

// Watch returns a watch.Interface for the resource
func (s *BaseStorage[K, D]) Watch(ctx context.Context, options *internalversion.ListOptions) (watch.Interface, error) {
	watchInterface, err := s.watcher.Watch()
	if err != nil {
		return nil, err
	}
	return watchInterface, nil
}

// Helper methods for backend operations - NO MORE STUBS!
func (s *BaseStorage[K, D]) getFromBackend(ctx context.Context, namespace, name string) (*D, error) {
	id := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	return s.backendOps.Get(ctx, id)
}

func (s *BaseStorage[K, D]) listFromBackend(ctx context.Context, scope interface{}) ([]D, error) {
	if scope == nil {
		return s.backendOps.List(ctx, nil)
	}
	if portScope, ok := scope.(ports.Scope); ok {
		return s.backendOps.List(ctx, portScope)
	}
	return s.backendOps.List(ctx, nil)
}

func (s *BaseStorage[K, D]) createInBackend(ctx context.Context, obj *D) (*D, error) {
	err := s.backendOps.Create(ctx, obj)
	if err != nil {
		return nil, err
	}
	// Return the same object since Create doesn't return the created object
	return obj, nil
}

func (s *BaseStorage[K, D]) updateInBackend(ctx context.Context, obj *D) (*D, error) {
	err := s.backendOps.Update(ctx, obj)
	if err != nil {
		return nil, err
	}
	// Return the same object since Update doesn't return the updated object
	return obj, nil
}

func (s *BaseStorage[K, D]) deleteFromBackend(ctx context.Context, namespace, name string) error {
	id := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	return s.backendOps.Delete(ctx, id)
}

// Helper methods
func (s *BaseStorage[K, D]) broadcastWatchEvent(eventType watch.EventType, obj runtime.Object) {
	if s.watcher != nil {
		s.watcher.Action(eventType, obj)
	}
}

func (s *BaseStorage[K, D]) applyPatch(current runtime.Object, patchType types.PatchType, data []byte) (runtime.Object, error) {
	switch patchType {
	case types.JSONPatchType:
		return applyJSONPatch(current, data)
	case types.MergePatchType:
		return applyMergePatch(current, data)
	case types.StrategicMergePatchType:
		return applyStrategicMergePatch(current, data)
	default:
		return nil, fmt.Errorf("unsupported patch type: %s", patchType)
	}
}

func applyJSONPatch(current runtime.Object, data []byte) (runtime.Object, error) {
	// JSON patch implementation would go here
	return nil, fmt.Errorf("JSON patch not implemented yet")
}

func applyMergePatch(current runtime.Object, data []byte) (runtime.Object, error) {
	// Merge patch implementation would go here
	return nil, fmt.Errorf("merge patch not implemented yet")
}

func applyStrategicMergePatch(current runtime.Object, data []byte) (runtime.Object, error) {
	// Strategic merge patch implementation would go here
	return nil, fmt.Errorf("strategic merge patch not implemented yet")
}

func getObjectName(obj runtime.Object) string {
	if accessor, err := meta.Accessor(obj); err == nil {
		return accessor.GetName()
	}
	return "unknown"
}

// GetConverter returns the converter for the storage
func (s *BaseStorage[K, D]) GetConverter() Converter[K, D] {
	return s.converter
}

// GetBackendOps returns the backend operations for the storage
func (s *BaseStorage[K, D]) GetBackendOps() BackendOperations[D] {
	return s.backendOps
}
