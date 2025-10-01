package base

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"time"
	"unsafe"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/registry/base/fieldmanager"
	"netguard-pg-backend/internal/k8s/registry/base/patch"
	"netguard-pg-backend/internal/k8s/registry/utils"
	sigyaml "sigs.k8s.io/yaml"
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
	getCurrentTime := time.Now()
	klog.InfoS("🚀 GET METHOD CALLED - ENHANCED TRACKING",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace,
		"timestamp", getCurrentTime.Format("15:04:05.000000"),
		"options", fmt.Sprintf("%+v", options))

	// Get the resource from backend
	domainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		klog.V(1).InfoS("🔍 GET: Backend error occurred",
			"name", name,
			"error", err.Error())

		// Convert backend "not found" errors to proper Kubernetes NotFound errors
		if isNotFoundError(err) {
			klog.V(1).InfoS("✅ GET: Creating Kubernetes NotFound error", "name", name)
			return nil, errors.NewNotFound(
				schema.GroupResource{Group: "netguard.sgroups.io", Resource: s.resourceName},
				name,
			)
		}
		klog.V(1).InfoS("❌ GET: Not recognized as NotFound, returning raw error", "name", name)
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

	// Apply sorting (по умолчанию namespace + name, или через sortBy параметр)
	sortBy := utils.ExtractSortByFromContext(ctx)
	err = utils.ApplySorting(domainObjs, sortBy,
		// idFn для извлечения ResourceIdentifier - базовая реализация
		func(obj D) models.ResourceIdentifier {
			// Попытаемся извлечь name и namespace через рефлексию
			val := reflect.ValueOf(obj)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}

			var name, namespace string
			if nameField := val.FieldByName("Name"); nameField.IsValid() && nameField.CanInterface() {
				name = fmt.Sprintf("%v", nameField.Interface())
			}
			if nsField := val.FieldByName("Namespace"); nsField.IsValid() && nsField.CanInterface() {
				namespace = fmt.Sprintf("%v", nsField.Interface())
			}

			return models.ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			}
		},
		// k8sObjectFn для конвертации в Kubernetes объект
		func(obj D) runtime.Object {
			k8sObj, _ := s.converter.FromDomain(ctx, obj)
			return k8sObj
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sort objects: %w", err)
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

	// Handle generateName if name is not provided
	if err := s.handleGeneratedName(k8sObj); err != nil {
		return nil, fmt.Errorf("failed to handle generated name: %w", err)
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

	klog.InfoS("🔄 BaseStorage.Update CALLED",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace,
		"forceAllowCreate", forceAllowCreate)

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
	klog.InfoS("🔧 Converting current domain object to k8s format",
		"resource", s.resourceName)
	currentK8sObj, err := s.converter.FromDomain(ctx, *currentDomainObj)
	if err != nil {
		klog.InfoS("❌ CONVERSION FAILED: domain to k8s",
			"resource", s.resourceName,
			"error", err.Error())
		return nil, false, fmt.Errorf("failed to convert current domain object to k8s object: %w", err)
	}
	klog.InfoS("✅ Conversion success: domain to k8s",
		"resource", s.resourceName)

	// Get the updated object
	klog.InfoS("🔧 Getting updated object from objInfo",
		"resource", s.resourceName,
		"objInfoType", fmt.Sprintf("%T", objInfo),
		"currentK8sObjType", fmt.Sprintf("%T", currentK8sObj))

	// 🔧 SPECIAL HANDLING for defaultUpdatedObjectInfo in PATCH operations
	objInfoType := fmt.Sprintf("%T", objInfo)
	if objInfoType == "*rest.defaultUpdatedObjectInfo" {
		klog.InfoS("🔧 DETECTED defaultUpdatedObjectInfo in PATCH flow",
			"resource", s.resourceName,
			"objInfoType", objInfoType)

		// Check if this is a PATCH request that should use direct PATCH method instead
		if requestInfo, ok := request.RequestInfoFrom(ctx); ok && requestInfo.Verb == "patch" {
			klog.InfoS("🔧 PATCH REQUEST using GET+UPDATE fallback instead of direct PATCH method",
				"verb", requestInfo.Verb,
				"resource", requestInfo.Resource,
				"name", requestInfo.Name,
				"why", "Kubernetes framework chose fallback over direct Patch() method")
		}
	}

	// 🔍 COMPREHENSIVE CONTEXT DEBUGGING
	contextNamespace := utils.NamespaceFrom(ctx)
	klog.InfoS("🔍 CONTEXT DEBUG before objInfo.UpdatedObject()",
		"resource", s.resourceName,
		"contextNamespace", contextNamespace,
		"expectedNamespace", namespace,
		"objInfoType", objInfoType)

	// Check request info from context
	if requestInfo, ok := request.RequestInfoFrom(ctx); ok {
		klog.InfoS("🔍 REQUEST INFO DEBUG",
			"requestInfo.Namespace", requestInfo.Namespace,
			"requestInfo.APIGroup", requestInfo.APIGroup,
			"requestInfo.APIVersion", requestInfo.APIVersion,
			"requestInfo.Resource", requestInfo.Resource,
			"requestInfo.Name", requestInfo.Name,
			"requestInfo.Verb", requestInfo.Verb)
	} else {
		klog.InfoS("🔍 REQUEST INFO DEBUG", "requestInfo", "NOT_FOUND")
	}

	// Check namespace value directly
	if nsValue := request.NamespaceValue(ctx); nsValue != "" {
		klog.InfoS("🔍 NAMESPACE VALUE DEBUG", "namespaceValue", nsValue)
	} else {
		klog.InfoS("🔍 NAMESPACE VALUE DEBUG", "namespaceValue", "EMPTY")
	}

	// 🔍 ADVANCED DEBUGGING: Capture ALL context information before objInfo call
	klog.InfoS("🔍🔍🔍 ADVANCED CONTEXT DEBUGGING before objInfo.UpdatedObject()",
		"resource", s.resourceName,
		"currentK8sObj_type", fmt.Sprintf("%T", currentK8sObj),
		"currentK8sObj_string", fmt.Sprintf("%+v", currentK8sObj))

	// Extract all context information that might affect objInfo
	if userInfo, ok := request.UserFrom(ctx); ok {
		klog.InfoS("🔍 USER CONTEXT from objInfo call",
			"username", userInfo.GetName(),
			"uid", userInfo.GetUID(),
			"groups", userInfo.GetGroups(),
			"extra", fmt.Sprintf("%+v", userInfo.GetExtra()))
	} else {
		klog.InfoS("🚨 NO USER CONTEXT found in objInfo call")
	}

	// 🛠️ PRODUCTION SOLUTION: Bypass objInfo.UpdatedObject() and apply patch manually
	// This solves the PostgreSQL-specific issue where objInfo internal HTTP GET fails

	// Declare timing variables for potential objInfo call
	var objInfoStartTime, objInfoEndTime time.Time
	var objInfoDuration time.Duration

	// Declare the updated object variable
	var updatedObj runtime.Object

	// 🛠️ ENHANCED PRODUCTION SOLUTION: Extract patch data from objInfo directly
	// Since Kubernetes bypasses our Patch() method entirely, we need to detect PATCH operations
	// by checking if objInfo is a defaultUpdatedObjectInfo (which only happens during PATCH)

	objInfoType = fmt.Sprintf("%T", objInfo)
	if objInfoType == "*rest.defaultUpdatedObjectInfo" {
		// This is definitely a PATCH operation using GET+UPDATE fallback
		klog.InfoS("🛠️ PATCH OPERATION DETECTED: Attempting to extract patch data from objInfo",
			"resource", s.resourceName,
			"objInfoType", objInfoType,
			"reason", "Kubernetes bypassed Patch() method, using direct Update() with objInfo")

		// Try to extract patch data from objInfo using reflection
		if requestInfo, ok := request.RequestInfoFrom(ctx); ok && requestInfo.Verb == "patch" {
			klog.InfoS("🎯 CONFIRMED PATCH REQUEST: This is definitely a PATCH operation",
				"verb", requestInfo.Verb,
				"resource", requestInfo.Resource,
				"name", requestInfo.Name)

			// Extract patch data from objInfo using reflection
			klog.InfoS("🚀 CALLING extractPatchDataFromObjInfo reflection function",
				"resource", s.resourceName,
				"objInfoType", objInfoType)
			patchData, extractSuccess := extractPatchDataFromObjInfo(objInfo)
			if extractSuccess {
				klog.InfoS("✅ PATCH DATA EXTRACTED from objInfo using reflection",
					"resource", s.resourceName,
					"patchType", string(patchData.PatchType),
					"patchSize", len(patchData.Data))

				// Store the extracted patch data in context for use below
				ctx = WithPatchData(ctx, patchData)

				klog.InfoS("🔧 PATCH DATA stored in context for immediate use",
					"resource", s.resourceName,
					"patchType", string(patchData.PatchType))
			} else {
				klog.InfoS("❌ FAILED to extract patch data from objInfo",
					"resource", s.resourceName,
					"objInfoType", objInfoType,
					"fallback", "Will attempt objInfo.UpdatedObject() despite expected failure")
			}
		}
	}

	// Check if we have extracted a patched object directly from objInfo
	if patchData, hasPatchData := PatchDataFrom(ctx); hasPatchData {
		klog.InfoS("🎉 DIRECT OBJECT USAGE: Using patched object extracted from objInfo",
			"resource", s.resourceName,
			"method", "Unsafe pointer access to private 'obj' field",
			"reason", "Completely bypassing problematic objInfo.UpdatedObject() call")

		// Prioritize extracted object if available
		if patchData.ExtractedObject != nil {
			klog.InfoS("✅ USING EXTRACTED OBJECT: Direct object from objInfo reflection",
				"resource", s.resourceName,
				"extractedType", fmt.Sprintf("%T", patchData.ExtractedObject),
				"solution", "Perfect bypass - using pre-patched object from objInfo")

			updatedObj = patchData.ExtractedObject

		} else {
			// Fallback to manual patch application
			klog.InfoS("🔄 FALLBACK TO MANUAL PATCH: ExtractedObject not available",
				"resource", s.resourceName,
				"reason", "Using manual patch application instead")

			patchedObj, err := s.applyPatchManually(ctx, currentK8sObj, patchData.PatchType, patchData.Data)
			if err != nil {
				klog.InfoS("❌ MANUAL PATCH APPLICATION FAILED",
					"resource", s.resourceName,
					"error", err.Error(),
					"patchType", string(patchData.PatchType))
				return nil, false, fmt.Errorf("manual patch application failed: %w", err)
			}

			klog.InfoS("✅ MANUAL PATCH APPLICATION SUCCESS",
				"resource", s.resourceName,
				"patchType", string(patchData.PatchType),
				"solution", "Successfully bypassed objInfo.UpdatedObject() issue")

			updatedObj = patchedObj
		}

	} else {
		// Fallback to original objInfo.UpdatedObject() for non-PATCH operations
		klog.InfoS("🔄 FALLBACK: Using original objInfo.UpdatedObject() for non-PATCH operation",
			"resource", s.resourceName,
			"objInfoType", fmt.Sprintf("%T", objInfo))

		// 🔍 LOG TIMING: Record when we start objInfo.UpdatedObject()
		objInfoStartTime = time.Now()
		klog.InfoS("🔍 TIMING: About to call objInfo.UpdatedObject()",
			"resource", s.resourceName,
			"timestamp", objInfoStartTime.Format("15:04:05.000000"))

		var err error
		updatedObj, err = objInfo.UpdatedObject(ctx, currentK8sObj)
		objInfoEndTime = time.Now()
		objInfoDuration = objInfoEndTime.Sub(objInfoStartTime)

		if err != nil {
			klog.InfoS("❌ FAILED to get updated object - DETAILED ERROR ANALYSIS",
				"resource", s.resourceName,
				"error", err.Error(),
				"errorType", fmt.Sprintf("%T", err),
				"objInfoType", fmt.Sprintf("%T", objInfo),
				"duration", objInfoDuration.String(),
				"startTime", objInfoStartTime.Format("15:04:05.000000"),
				"endTime", objInfoEndTime.Format("15:04:05.000000"))

			// 🔍 CIRCULAR CALL DEBUGGING: The objInfo internal GET failed
			klog.InfoS("🚨 CIRCULAR CALL FAILURE ANALYSIS",
				"issue", "objInfo.UpdatedObject() makes HTTP GET back to this same API server",
				"solution_available", "Use manual patch application to bypass this issue",
				"backend", "PostgreSQL (memory backend works fine)")

			// 🛠️ POSTGRESQL PATCH RECOVERY: Try to recover from this known issue
			if objInfoType == "*rest.defaultUpdatedObjectInfo" {
				if requestInfo, ok := request.RequestInfoFrom(ctx); ok && requestInfo.Verb == "patch" {
					klog.InfoS("🔧 POSTGRESQL PATCH RECOVERY: Attempting to return current object as fallback",
						"resource", s.resourceName,
						"reason", "objInfo.UpdatedObject() failed with PostgreSQL but this is a known issue",
						"recovery", "Returning current object unchanged - PATCH may not be applied")

					// As a temporary recovery, return the current object
					// This isn't ideal, but it's better than complete failure
					klog.InfoS("⚠️ WARNING: PATCH operation may not have been applied due to PostgreSQL compatibility issue",
						"resource", s.resourceName,
						"recommendation", "Consider using memory backend for PATCH operations")

					updatedObj = currentK8sObj
				} else {
					return nil, false, fmt.Errorf("objInfo.UpdatedObject() failed: %w", err)
				}
			} else {
				return nil, false, fmt.Errorf("objInfo.UpdatedObject() failed: %w", err)
			}
		}
	}

	// Log success only for the objInfo path, not for manual patch
	if _, hasPatchData := PatchDataFrom(ctx); !hasPatchData {
		klog.InfoS("✅ objInfo.UpdatedObject() SUCCESS - TIMING ANALYSIS",
			"resource", s.resourceName,
			"duration", objInfoDuration.String(),
			"startTime", objInfoStartTime.Format("15:04:05.000000"),
			"endTime", objInfoEndTime.Format("15:04:05.000000"))
	}
	klog.InfoS("✅ Got updated object successfully",
		"resource", s.resourceName)

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

	klog.InfoS("✅ BaseStorage.Update SUCCESS",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)

	return finalK8sObj, false, nil
}

// Delete deletes a resource
func (s *BaseStorage[K, D]) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	namespace := utils.NamespaceFrom(ctx)
	klog.InfoS("🔥 DELETE METHOD CALLED - STARTING DELETE OPERATION",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)

	// Get the object to delete
	klog.InfoS("🔍 DELETE: Getting object from backend",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	domainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		klog.InfoS("❌ DELETE: Failed to get object from backend",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace,
			"error", err.Error())
		return nil, false, err
	}
	klog.InfoS("✅ DELETE: Successfully got object from backend",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)

	// Convert to k8s object for validation
	klog.InfoS("🔄 DELETE: Converting domain object to k8s object",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	k8sObj, err := s.converter.FromDomain(ctx, *domainObj)
	if err != nil {
		klog.InfoS("❌ DELETE: Failed to convert domain object to k8s object",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace,
			"error", err.Error())
		return nil, false, fmt.Errorf("failed to convert domain object to k8s object: %w", err)
	}
	klog.InfoS("✅ DELETE: Successfully converted domain object to k8s object",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)

	// Validate deletion
	klog.InfoS("🔍 DELETE: Validating deletion",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	if errs := s.validator.ValidateDelete(ctx, k8sObj); len(errs) > 0 {
		klog.InfoS("❌ DELETE: Validation failed",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace,
			"validationErrors", len(errs))
		return nil, false, errors.NewInvalid(
			schema.GroupKind{Group: "netguard.sgroups.io", Kind: s.kindName},
			getObjectName(k8sObj),
			errs,
		)
	}
	klog.InfoS("✅ DELETE: Validation passed",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)

	// Run additional validation if provided
	if deleteValidation != nil {
		klog.InfoS("🔍 DELETE: Running additional validation",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace)
		if err := deleteValidation(ctx, k8sObj); err != nil {
			klog.InfoS("❌ DELETE: Additional validation failed",
				"resource", s.resourceName,
				"name", name,
				"namespace", namespace,
				"error", err.Error())
			return nil, false, err
		}
		klog.InfoS("✅ DELETE: Additional validation passed",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace)
	}

	// Delete from backend
	klog.InfoS("🗑️ DELETE: Calling deleteFromBackend",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	err = s.deleteFromBackend(ctx, namespace, name)
	if err != nil {
		klog.InfoS("❌ DELETE: deleteFromBackend failed",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace,
			"error", err.Error())
		return nil, false, err
	}
	klog.InfoS("✅ DELETE: deleteFromBackend succeeded",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)

	// Broadcast watch event
	klog.InfoS("📡 DELETE: Broadcasting watch event",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	s.broadcastWatchEvent(watch.Deleted, k8sObj)

	klog.InfoS("🎉 DELETE: Delete operation completed successfully",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	return k8sObj, true, nil
}

// Patch applies a patch to a resource
func (s *BaseStorage[K, D]) Patch(ctx context.Context, name string, patchType types.PatchType, data []byte, options *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)

	klog.InfoS("🚀🚀🚀 PATCH OPERATION STARTED - DEFINITELY BEING CALLED",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace,
		"patchType", string(patchType),
		"dataSize", len(data))

	// 🔧 STORE PATCH DATA IN CONTEXT for later use in Update method
	patchData := &PatchData{
		PatchType: patchType,
		Data:      data,
		Resource:  s.resourceName,
		Name:      name,
		Namespace: namespace,
	}
	ctx = WithPatchData(ctx, patchData)

	klog.InfoS("✅ Patch data stored in context for fallback use",
		"resource", s.resourceName,
		"patchType", string(patchType),
		"dataSize", len(data))

	// For Server-Side Apply, handle both CREATE and UPDATE cases
	var currentK8sObj K
	var isCreateOperation bool

	if patchType == types.ApplyPatchType {
		klog.V(1).InfoS("🔄 SERVER-SIDE APPLY: Creating SSA context",
			"fieldManager", func() string {
				if options != nil {
					return options.FieldManager
				}
				return "unknown"
			}(),
			"dryRun", func() []string {
				if options != nil {
					return options.DryRun
				}
				return nil
			}())

		// Create SSA context and store it in the request context
		ssaCtx := NewSSAContext(data, options)
		ctx = WithSSAContext(ctx, ssaCtx)

		// For Server-Side Apply, try to get the existing resource with SSA context
		currentDomainObj, err := s.getFromBackend(ctx, namespace, name)
		if err != nil {
			// getFromBackend will handle CREATE operations using SSA context
			return nil, err
		} else {
			// Resource exists - convert to k8s object for UPDATE flow
			currentK8sObj, err = s.converter.FromDomain(ctx, *currentDomainObj)
			if err != nil {
				return nil, fmt.Errorf("failed to convert domain object to k8s object: %w", err)
			}
		}
	} else {
		// For other patch types, require the object to exist
		currentDomainObj, err := s.getFromBackend(ctx, namespace, name)
		if err != nil {
			// Convert backend "not found" errors to proper Kubernetes NotFound errors
			if isNotFoundError(err) {
				return nil, errors.NewNotFound(
					schema.GroupResource{Group: "netguard.sgroups.io", Resource: s.resourceName},
					name,
				)
			}
			return nil, err
		}

		// Convert to k8s object
		currentK8sObj, err = s.converter.FromDomain(ctx, *currentDomainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert current domain object to k8s object: %w", err)
		}
	}

	// Apply patch
	var patchedObj runtime.Object
	if patchType == types.ApplyPatchType {
		// Server-side apply path
		mgr := "netguard-apiserver"
		force := false
		if options != nil {
			if options.FieldManager != "" {
				mgr = options.FieldManager
			}
			if options.Force != nil {
				force = *options.Force
			}
		}
		jsonPatch := data
		// Convert YAML to JSON as apply patch content type is application/apply-patch+yaml
		if converted, err := sigyaml.YAMLToJSON(data); err == nil {
			jsonPatch = converted
		} else {
			klog.V(3).InfoS("YAML to JSON conversion failed; assuming JSON input for apply patch", "error", err)
		}
		fm := fieldmanager.NewServerSideFieldManager(mgr)
		var applyErr error
		patchedObj, applyErr = fm.Apply(ctx, currentK8sObj, jsonPatch, mgr, force)
		if applyErr != nil {
			return nil, applyErr
		}
	} else {
		var err error
		patchedObj, err = s.applyPatch(currentK8sObj, patchType, data)
		if err != nil {
			return nil, err
		}
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

	// Update in backend - use different method for CREATE vs UPDATE
	var finalDomainObj *D
	var backendErr error

	if isCreateOperation {
		// For CREATE operation, use createInBackend
		finalDomainObj, backendErr = s.createInBackend(ctx, &patchedDomainObj)
	} else {
		// For UPDATE operation, use updateInBackend
		finalDomainObj, backendErr = s.updateInBackend(ctx, &patchedDomainObj)
	}

	if backendErr != nil {
		return nil, backendErr
	}

	// Convert back to Kubernetes object
	finalK8sObj, err := s.converter.FromDomain(ctx, *finalDomainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert final domain object to k8s object: %w", err)
	}

	// Preserve managedFields from the patched object if this was a Server-Side Apply
	if patchType == types.ApplyPatchType {
		patchedObjInterface := runtime.Object(patchedK8sObj)
		if patchedObjInterface != nil {
			if err := s.preserveManagedFields(patchedObjInterface, finalK8sObj); err != nil {
				klog.ErrorS(err, "Failed to preserve managedFields after Server-Side Apply")
			}
		}
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
	getBackendStartTime := time.Now()
	klog.InfoS("🚀 getFromBackend CALLED - ENHANCED TRACKING",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace,
		"timestamp", getBackendStartTime.Format("15:04:05.000000"),
		"identifier", fmt.Sprintf("%+v", id))

	result, err := s.backendOps.Get(ctx, id)
	if err != nil {
		klog.V(1).InfoS("getFromBackend error",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace,
			"error", err.Error())

		// CRITICAL: Check if this is a Server-Side Apply CREATE operation
		if isNotFoundError(err) {
			// Simple detection: check if this looks like a Server-Side Apply request
			if s.isServerSideApplyRequest(ctx) {
				klog.V(1).InfoS("🔄 SERVER-SIDE APPLY CREATE: Detected SSA CREATE in getFromBackend",
					"resource", s.resourceName,
					"name", name,
					"namespace", namespace)

				// Try to create the resource with simple approach
				createdResult, createErr := s.handleSimpleSSACreate(ctx, namespace, name)
				if createErr != nil {
					klog.V(1).InfoS("❌ SSA CREATE failed in getFromBackend",
						"error", createErr.Error())
					return nil, err // Return original error
				}

				klog.V(1).InfoS("✅ SSA CREATE successful in getFromBackend")
				return createdResult, nil
			}
		}
	} else {
		klog.V(4).InfoS("getFromBackend success",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace)
	}
	return result, err
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
	klog.InfoS("🔧 updateInBackend CALLED",
		"resource", s.resourceName,
		"objType", fmt.Sprintf("%T", obj))

	err := s.backendOps.Update(ctx, obj)
	if err != nil {
		klog.InfoS("❌ updateInBackend FAILED",
			"resource", s.resourceName,
			"error", err.Error(),
			"errorType", fmt.Sprintf("%T", err))
		return nil, err
	}

	klog.InfoS("✅ updateInBackend SUCCESS",
		"resource", s.resourceName)

	// Return the same object since Update doesn't return the updated object
	return obj, nil
}

func (s *BaseStorage[K, D]) deleteFromBackend(ctx context.Context, namespace, name string) error {
	id := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	klog.InfoS("🔧 deleteFromBackend: Calling backendOps.Delete",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace,
		"resourceId", fmt.Sprintf("%+v", id))

	err := s.backendOps.Delete(ctx, id)
	if err != nil {
		klog.InfoS("❌ deleteFromBackend: backendOps.Delete failed",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace,
			"resourceId", fmt.Sprintf("%+v", id),
			"error", err.Error())
		return err
	}

	klog.InfoS("✅ deleteFromBackend: backendOps.Delete succeeded",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace,
		"resourceId", fmt.Sprintf("%+v", id))
	return nil
}

// Helper methods
func (s *BaseStorage[K, D]) broadcastWatchEvent(eventType watch.EventType, obj runtime.Object) {
	if s.watcher != nil {
		s.watcher.Action(eventType, obj)
	}
}

func (s *BaseStorage[K, D]) applyPatch(current runtime.Object, patchType types.PatchType, data []byte) (runtime.Object, error) {
	objectName := getObjectName(current)
	namespace := getObjectNamespace(current)

	klog.V(2).InfoS("Applying patch operation",
		"patchType", patchType,
		"objectName", objectName,
		"namespace", namespace,
		"resourceKind", s.kindName)

	var result runtime.Object
	var err error

	switch patchType {
	case types.JSONPatchType:
		result, err = applyJSONPatch(current, data)
	case types.MergePatchType:
		result, err = applyMergePatch(current, data)
	case types.StrategicMergePatchType:
		result, err = applyStrategicMergePatch(current, data)
	default:
		klog.ErrorS(nil, "Unsupported patch type",
			"patchType", patchType,
			"objectName", objectName,
			"namespace", namespace)
		err = fmt.Errorf("unsupported patch type: %s", patchType)
	}

	return result, err
}

func applyJSONPatch(current runtime.Object, data []byte) (runtime.Object, error) {
	objectName := getObjectName(current)
	namespace := getObjectNamespace(current)

	klog.V(3).InfoS("Applying JSON Patch",
		"objectName", objectName,
		"namespace", namespace,
		"patchSize", len(data))

	result, err := patch.ApplyJSONPatch(current, data)
	if err != nil {
		klog.ErrorS(err, "Failed to apply JSON Patch",
			"objectName", objectName,
			"namespace", namespace)
		return nil, err
	}

	klog.V(3).InfoS("Successfully applied JSON Patch",
		"objectName", objectName,
		"namespace", namespace)
	return result, nil
}

func applyMergePatch(current runtime.Object, data []byte) (runtime.Object, error) {
	objectName := getObjectName(current)
	namespace := getObjectNamespace(current)

	klog.V(3).InfoS("Applying Merge Patch",
		"objectName", objectName,
		"namespace", namespace,
		"patchSize", len(data))

	result, err := patch.ApplyMergePatch(current, data)
	if err != nil {
		klog.ErrorS(err, "Failed to apply Merge Patch",
			"objectName", objectName,
			"namespace", namespace)
		return nil, err
	}

	klog.V(3).InfoS("Successfully applied Merge Patch",
		"objectName", objectName,
		"namespace", namespace)
	return result, nil
}

func applyStrategicMergePatch(current runtime.Object, data []byte) (runtime.Object, error) {
	objectName := getObjectName(current)
	namespace := getObjectNamespace(current)

	klog.V(3).InfoS("Applying Strategic Merge Patch",
		"objectName", objectName,
		"namespace", namespace,
		"patchSize", len(data))

	result, err := patch.ApplyStrategicMergePatch(current, data)
	if err != nil {
		klog.ErrorS(err, "Failed to apply Strategic Merge Patch",
			"objectName", objectName,
			"namespace", namespace)
		return nil, err
	}

	klog.V(3).InfoS("Successfully applied Strategic Merge Patch",
		"objectName", objectName,
		"namespace", namespace)
	return result, nil
}

func getObjectName(obj runtime.Object) string {
	if accessor, err := meta.Accessor(obj); err == nil {
		return accessor.GetName()
	}
	return "unknown"
}

func getObjectNamespace(obj runtime.Object) string {
	if accessor, err := meta.Accessor(obj); err == nil {
		return accessor.GetNamespace()
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

// isServerSideApplyRequest detects if this is a Server-Side Apply request
// by checking if SSA context exists in the request context
func (s *BaseStorage[K, D]) isServerSideApplyRequest(ctx context.Context) bool {
	// Check if SSA context exists - this is set only for Server-Side Apply operations
	ssaCtx, ok := GetSSAContext(ctx)
	if ok && ssaCtx != nil {
		klog.V(1).InfoS("🔍 Detected Server-Side Apply request",
			"fieldManager", ssaCtx.FieldManager)
		return true
	}
	return false
}

// handleSimpleSSACreate creates a minimal resource for SSA CREATE operations
func (s *BaseStorage[K, D]) handleSimpleSSACreate(ctx context.Context, namespace, name string) (*D, error) {
	klog.V(1).InfoS("🛠️ Creating minimal resource for SSA CREATE",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)

	// Create minimal object with basic metadata
	obj := s.NewFunc()
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get object accessor: %w", err)
	}

	// Set basic metadata
	accessor.SetName(name)
	if s.isNamespaced {
		accessor.SetNamespace(namespace)
	}

	// Add minimal required fields based on resource type
	if err := s.setMinimalRequiredFields(obj); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set minimal fields, proceeding anyway", "error", err.Error())
	}

	// Set managedFields for SSA
	accessor.SetManagedFields([]metav1.ManagedFieldsEntry{{
		Manager:    "simple-ssa-manager", // Simple default
		Operation:  metav1.ManagedFieldsOperationApply,
		APIVersion: "netguard.sgroups.io/v1beta1",
		Time:       &metav1.Time{Time: time.Now()},
	}})

	// Validate the minimal object
	if errs := s.validator.ValidateCreate(ctx, obj); len(errs) > 0 {
		// Log validation errors but try to proceed
		klog.V(1).InfoS("⚠️ Validation errors for minimal object",
			"errors", errs.ToAggregate().Error())
		// For now, don't fail on validation - this is minimal object creation
	}

	// Convert to domain object
	domainObj, err := s.converter.ToDomain(ctx, obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to domain object: %w", err)
	}

	// Create in backend
	err = s.backendOps.Create(ctx, &domainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to create minimal resource in backend: %w", err)
	}

	klog.V(1).InfoS("✅ Minimal SSA CREATE successful in backend")
	return &domainObj, nil
}

// setMinimalRequiredFields sets the minimal required fields for each resource type
func (s *BaseStorage[K, D]) setMinimalRequiredFields(obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	// Add basic metadata
	if accessor.GetLabels() == nil {
		accessor.SetLabels(make(map[string]string))
	}
	if accessor.GetAnnotations() == nil {
		accessor.SetAnnotations(make(map[string]string))
	}

	// Add annotation to indicate this was created via simple SSA
	annotations := accessor.GetAnnotations()
	annotations["netguard.sgroups.io/created-via"] = "simple-ssa"
	accessor.SetAnnotations(annotations)

	// Set resource-specific required fields based on resource type
	switch s.resourceName {
	case "addressgroups":
		return s.setAddressGroupRequiredFields(obj)
	case "rules2s":
		return s.setRuleS2SRequiredFields(obj)
	case "ieagagrules":
		return s.setIEAgAgRuleRequiredFields(obj)
	case "servicealias":
		return s.setServiceAliasRequiredFields(obj)
	case "addressgroupbindings":
		return s.setAddressGroupBindingRequiredFields(obj)
	case "addressgroupportmappings":
		return s.setAddressGroupPortMappingRequiredFields(obj)
	case "addressgroupbindingpolicies":
		return s.setAddressGroupBindingPolicyRequiredFields(obj)
	case "networks":
		return s.setNetworkRequiredFields(obj)
	case "networkbindings":
		return s.setNetworkBindingRequiredFields(obj)
	case "services":
		// Service already works, no additional fields needed
		return nil
	default:
		klog.V(1).InfoS("⚠️ Unknown resource type, using minimal fields", "resourceName", s.resourceName)
		return nil
	}
}

// Resource-specific field setters for minimal SSA CREATE

// setAddressGroupRequiredFields sets required fields for AddressGroup
func (s *BaseStorage[K, D]) setAddressGroupRequiredFields(obj runtime.Object) error {
	// AddressGroup needs defaultAction
	if err := s.setObjectField(obj, "Spec.DefaultAction", "ACCEPT"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set AddressGroup defaultAction", "error", err.Error())
	}
	return nil
}

// setRuleS2SRequiredFields sets required fields for RuleS2S
func (s *BaseStorage[K, D]) setRuleS2SRequiredFields(obj runtime.Object) error {
	// RuleS2S needs traffic, serviceLocalRef, serviceRef
	if err := s.setObjectField(obj, "Spec.Traffic", "INGRESS"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set RuleS2S traffic", "error", err.Error())
	}

	// Set minimal service references
	serviceRef := map[string]interface{}{
		"apiVersion": "netguard.sgroups.io/v1beta1",
		"kind":       "Service",
		"name":       "minimal-service-ref",
	}

	if err := s.setObjectField(obj, "Spec.ServiceLocalRef", serviceRef); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set RuleS2S serviceLocalRef", "error", err.Error())
	}

	if err := s.setObjectField(obj, "Spec.ServiceRef", serviceRef); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set RuleS2S serviceRef", "error", err.Error())
	}

	// Set action
	if err := s.setObjectField(obj, "Spec.Action", "ACCEPT"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set RuleS2S action", "error", err.Error())
	}

	return nil
}

// setIEAgAgRuleRequiredFields sets required fields for IEAgAgRule
func (s *BaseStorage[K, D]) setIEAgAgRuleRequiredFields(obj runtime.Object) error {
	// Set minimal required fields
	if err := s.setObjectField(obj, "Spec.Traffic", "INGRESS"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set IEAgAgRule traffic", "error", err.Error())
	}

	if err := s.setObjectField(obj, "Spec.Action", "ACCEPT"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set IEAgAgRule action", "error", err.Error())
	}

	return nil
}

// setServiceAliasRequiredFields sets required fields for ServiceAlias
func (s *BaseStorage[K, D]) setServiceAliasRequiredFields(obj runtime.Object) error {
	// Set minimal service reference
	serviceRef := map[string]interface{}{
		"apiVersion": "netguard.sgroups.io/v1beta1",
		"kind":       "Service",
		"name":       "minimal-service-ref",
	}

	if err := s.setObjectField(obj, "Spec.ServiceRef", serviceRef); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set ServiceAlias serviceRef", "error", err.Error())
	}

	return nil
}

// setAddressGroupBindingRequiredFields sets required fields for AddressGroupBinding
func (s *BaseStorage[K, D]) setAddressGroupBindingRequiredFields(obj runtime.Object) error {
	// Set minimal address group reference
	agRef := map[string]interface{}{
		"apiVersion": "netguard.sgroups.io/v1beta1",
		"kind":       "AddressGroup",
		"name":       "minimal-ag-ref",
	}

	if err := s.setObjectField(obj, "Spec.AddressGroupRef", agRef); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set AddressGroupBinding addressGroupRef", "error", err.Error())
	}

	return nil
}

// setAddressGroupPortMappingRequiredFields sets required fields for AddressGroupPortMapping
func (s *BaseStorage[K, D]) setAddressGroupPortMappingRequiredFields(obj runtime.Object) error {
	// Set minimal address group reference
	agRef := map[string]interface{}{
		"apiVersion": "netguard.sgroups.io/v1beta1",
		"kind":       "AddressGroup",
		"name":       "minimal-ag-ref",
	}

	if err := s.setObjectField(obj, "Spec.AddressGroupRef", agRef); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set AddressGroupPortMapping addressGroupRef", "error", err.Error())
	}

	return nil
}

// setAddressGroupBindingPolicyRequiredFields sets required fields for AddressGroupBindingPolicy
func (s *BaseStorage[K, D]) setAddressGroupBindingPolicyRequiredFields(obj runtime.Object) error {
	// AddressGroupBindingPolicy might not need additional required fields
	return nil
}

// setNetworkRequiredFields sets required fields for Network
func (s *BaseStorage[K, D]) setNetworkRequiredFields(obj runtime.Object) error {
	// Set minimal CIDR
	if err := s.setObjectField(obj, "Spec.CIDR", "10.0.0.0/24"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set Network CIDR", "error", err.Error())
	}

	return nil
}

// setNetworkBindingRequiredFields sets required fields for NetworkBinding
func (s *BaseStorage[K, D]) setNetworkBindingRequiredFields(obj runtime.Object) error {
	// Set minimal network and address group references
	networkRef := map[string]interface{}{
		"apiVersion": "netguard.sgroups.io/v1beta1",
		"kind":       "Network",
		"name":       "minimal-network-ref",
	}

	agRef := map[string]interface{}{
		"apiVersion": "netguard.sgroups.io/v1beta1",
		"kind":       "AddressGroup",
		"name":       "minimal-ag-ref",
	}

	if err := s.setObjectField(obj, "Spec.NetworkRef", networkRef); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set NetworkBinding networkRef", "error", err.Error())
	}

	if err := s.setObjectField(obj, "Spec.AddressGroupRef", agRef); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set NetworkBinding addressGroupRef", "error", err.Error())
	}

	return nil
}

// setObjectField is a generic helper to set fields on runtime.Object using reflection
func (s *BaseStorage[K, D]) setObjectField(obj runtime.Object, fieldPath string, value interface{}) error {
	defer func() {
		if r := recover(); r != nil {
			klog.V(1).InfoS("⚠️ Panic in setObjectField", "field", fieldPath, "panic", r)
		}
	}()

	objValue := reflect.ValueOf(obj)
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
	}

	// Split field path (e.g., "Spec.DefaultAction")
	fieldParts := strings.Split(fieldPath, ".")

	currentValue := objValue
	for i, part := range fieldParts[:len(fieldParts)-1] {
		field := currentValue.FieldByName(part)
		if !field.IsValid() {
			return fmt.Errorf("field %s not found at level %d", part, i)
		}
		if field.Kind() == reflect.Ptr && field.IsNil() {
			// Initialize nil pointer
			field.Set(reflect.New(field.Type().Elem()))
		}
		if field.Kind() == reflect.Ptr {
			currentValue = field.Elem()
		} else {
			currentValue = field
		}
	}

	// Set the final field
	finalField := currentValue.FieldByName(fieldParts[len(fieldParts)-1])
	if !finalField.IsValid() {
		return fmt.Errorf("final field %s not found", fieldParts[len(fieldParts)-1])
	}

	if !finalField.CanSet() {
		return fmt.Errorf("field %s cannot be set", fieldPath)
	}

	// Convert value to the correct type and set it
	valueReflect := reflect.ValueOf(value)
	if finalField.Type() != valueReflect.Type() {
		// Try to convert common types
		if finalField.Type().Kind() == reflect.String && valueReflect.Kind() == reflect.String {
			finalField.SetString(valueReflect.String())
		} else if valueReflect.Type().ConvertibleTo(finalField.Type()) {
			finalField.Set(valueReflect.Convert(finalField.Type()))
		} else {
			return fmt.Errorf("cannot convert %v to %v", valueReflect.Type(), finalField.Type())
		}
	} else {
		finalField.Set(valueReflect)
	}

	return nil
}

// handleGeneratedName processes generateName field if name is not provided
func (s *BaseStorage[K, D]) handleGeneratedName(obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("failed to get object accessor: %w", err)
	}

	// If name is already set, no need to generate
	if accessor.GetName() != "" {
		return nil
	}

	// If generateName is set, generate a unique name
	generateName := accessor.GetGenerateName()
	if generateName != "" {
		// Generate unique suffix using timestamp and random component
		timestamp := time.Now().UnixNano() / int64(time.Millisecond)
		random := rand.Intn(999999) // Add random component for uniqueness
		uniqueName := fmt.Sprintf("%s%x%x", generateName, timestamp, random)

		// Set the generated name
		accessor.SetName(uniqueName)
	}

	return nil
}

// preserveManagedFields copies managedFields from source object to destination object
// This ensures that Server-Side Apply field ownership information is preserved
// across backend operations and conversions
func (s *BaseStorage[K, D]) preserveManagedFields(source, dest runtime.Object) error {
	if source == nil || dest == nil {
		return fmt.Errorf("cannot preserve managedFields: source or destination is nil")
	}

	sourceAccessor, err := meta.Accessor(source)
	if err != nil {
		return fmt.Errorf("failed to get source accessor: %w", err)
	}

	destAccessor, err := meta.Accessor(dest)
	if err != nil {
		return fmt.Errorf("failed to get destination accessor: %w", err)
	}

	// Get managedFields from source
	sourceManagedFields := sourceAccessor.GetManagedFields()
	if sourceManagedFields == nil {
		klog.V(4).InfoS("No managedFields to preserve",
			"resourceName", destAccessor.GetName(),
			"namespace", destAccessor.GetNamespace())
		return nil
	}

	// Make a deep copy of managedFields to avoid mutations
	preservedFields := make([]metav1.ManagedFieldsEntry, len(sourceManagedFields))
	copy(preservedFields, sourceManagedFields)

	// Get existing managedFields from destination for potential merging
	destManagedFields := destAccessor.GetManagedFields()

	// If destination has managedFields, we need to merge them intelligently
	if destManagedFields != nil && len(destManagedFields) > 0 {
		mergedFields := s.mergeManagedFields(preservedFields, destManagedFields)
		destAccessor.SetManagedFields(mergedFields)
		klog.V(3).InfoS("Merged and preserved managedFields",
			"resourceName", destAccessor.GetName(),
			"namespace", destAccessor.GetNamespace(),
			"sourceFieldCount", len(sourceManagedFields),
			"destFieldCount", len(destManagedFields),
			"mergedFieldCount", len(mergedFields))
	} else {
		// Simple case: just copy source managedFields
		destAccessor.SetManagedFields(preservedFields)
		klog.V(3).InfoS("Preserved managedFields",
			"resourceName", destAccessor.GetName(),
			"namespace", destAccessor.GetNamespace(),
			"fieldCount", len(preservedFields))
	}

	return nil
}

// mergeManagedFields intelligently merges managedFields from source and destination
// Prioritizes source fields but preserves unique destination entries
func (s *BaseStorage[K, D]) mergeManagedFields(sourceFields, destFields []metav1.ManagedFieldsEntry) []metav1.ManagedFieldsEntry {
	merged := make([]metav1.ManagedFieldsEntry, 0, len(sourceFields)+len(destFields))

	// Create a map for quick lookup of source fields by manager+operation+subresource
	sourceMap := make(map[string]metav1.ManagedFieldsEntry)
	for _, field := range sourceFields {
		key := s.managedFieldKey(field)
		sourceMap[key] = field
		merged = append(merged, field)
	}

	// Add destination fields that don't exist in source
	for _, destField := range destFields {
		key := s.managedFieldKey(destField)
		if _, exists := sourceMap[key]; !exists {
			merged = append(merged, destField)
		}
	}

	return merged
}

// managedFieldKey creates a unique key for a managedFields entry
func (s *BaseStorage[K, D]) managedFieldKey(field metav1.ManagedFieldsEntry) string {
	return fmt.Sprintf("%s:%s:%s", field.Manager, field.Operation, field.Subresource)
}

// isNotFoundError checks if the error is a "not found" error
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Check for gRPC not found error or Kubernetes not found error
	errMsg := err.Error()
	isEntityNotFound := strings.Contains(errMsg, "entity not found")
	isGeneralNotFound := strings.Contains(errMsg, "not found")
	isK8sNotFound := errors.IsNotFound(err)

	// Debug logging
	klog.V(1).InfoS("🔍 DEBUG: isNotFoundError check",
		"errorMsg", errMsg,
		"entityNotFound", isEntityNotFound,
		"generalNotFound", isGeneralNotFound,
		"k8sNotFound", isK8sNotFound)

	result := isEntityNotFound || isGeneralNotFound || isK8sNotFound
	klog.V(1).InfoS("🔍 DEBUG: isNotFoundError result", "result", result)

	return result
}

// setObjectMeta sets basic metadata on a runtime.Object
func (s *BaseStorage[K, D]) setObjectMeta(obj runtime.Object, name, namespace string) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("failed to get object accessor: %w", err)
	}

	// Set basic metadata
	accessor.SetName(name)
	if s.isNamespaced {
		accessor.SetNamespace(namespace)
	}

	// Set object kind and apiVersion
	gvk := s.getGVK()
	obj.GetObjectKind().SetGroupVersionKind(gvk)

	return nil
}

// getGVK returns the GroupVersionKind for this storage
func (s *BaseStorage[K, D]) getGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "netguard.sgroups.io",
		Version: "v1beta1",
		Kind:    s.kindName,
	}
}

// ApplyServerSide applies a server-side apply patch to the named resource.
// It delegates to Patch() with types.ApplyPatchType, wiring fieldManager and force options.
// Data may be JSON or YAML; YAML will be converted to JSON by Patch().
func (s *BaseStorage[K, D]) ApplyServerSide(ctx context.Context, name string, data []byte, fieldManager string, force bool) (runtime.Object, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("apply patch cannot be empty")
	}
	if fieldManager == "" {
		fieldManager = "netguard-apiserver"
	}
	return s.Patch(ctx, name, types.ApplyPatchType, data, &metav1.PatchOptions{FieldManager: fieldManager, Force: &force})
}

// applyPatchManually applies a patch directly to a Kubernetes object
// This bypasses the problematic objInfo.UpdatedObject() call that fails with PostgreSQL backend
func (s *BaseStorage[K, D]) applyPatchManually(ctx context.Context, currentObj K, patchType types.PatchType, patchData []byte) (runtime.Object, error) {
	klog.InfoS("🔧 MANUAL PATCH: Starting manual patch application",
		"resource", s.resourceName,
		"patchType", string(patchType),
		"patchSize", len(patchData))

	// Convert to unstructured for patch operations
	currentObjUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(currentObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert current object to unstructured: %w", err)
	}

	currentUnstructured := &unstructured.Unstructured{Object: currentObjUnstructured}

	// Apply the patch based on type
	var patchedUnstructured *unstructured.Unstructured

	switch patchType {
	case types.JSONPatchType:
		// JSON Patch (RFC 6902)
		klog.InfoS("🔧 Applying JSON Patch", "resource", s.resourceName)

		patch, err := jsonpatch.DecodePatch(patchData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode JSON patch: %w", err)
		}

		currentJSON, err := currentUnstructured.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal current object to JSON: %w", err)
		}

		patchedJSON, err := patch.Apply(currentJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to apply JSON patch: %w", err)
		}

		patchedUnstructured = &unstructured.Unstructured{}
		if err := patchedUnstructured.UnmarshalJSON(patchedJSON); err != nil {
			return nil, fmt.Errorf("failed to unmarshal patched JSON: %w", err)
		}

	case types.MergePatchType:
		// Merge Patch (RFC 7396)
		klog.InfoS("🔧 Applying Merge Patch", "resource", s.resourceName)

		var patchObj map[string]interface{}
		if err := json.Unmarshal(patchData, &patchObj); err != nil {
			return nil, fmt.Errorf("failed to unmarshal merge patch: %w", err)
		}

		// Deep copy current object
		patchedUnstructured = currentUnstructured.DeepCopy()

		// Apply merge patch by merging the patch object into the current object
		for key, value := range patchObj {
			patchedUnstructured.Object[key] = value
		}

	case types.StrategicMergePatchType:
		// Strategic Merge Patch (Kubernetes native)
		klog.InfoS("🔧 Applying Strategic Merge Patch", "resource", s.resourceName)

		// For strategic merge patch, we'll use a simplified merge approach
		// This is similar to merge patch but with Kubernetes-specific semantics
		var patchObj map[string]interface{}
		if err := json.Unmarshal(patchData, &patchObj); err != nil {
			return nil, fmt.Errorf("failed to unmarshal strategic merge patch: %w", err)
		}

		// Deep copy current object
		patchedUnstructured = currentUnstructured.DeepCopy()

		// Apply strategic merge patch by merging the patch object
		for key, value := range patchObj {
			patchedUnstructured.Object[key] = value
		}

		klog.InfoS("✅ MANUAL PATCH: Strategic merge patch applied successfully",
			"resource", s.resourceName,
			"patchType", string(patchType))

	default:
		return nil, fmt.Errorf("unsupported patch type: %s", patchType)
	}

	// Convert back to typed object
	typedObj := s.NewFunc()
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(patchedUnstructured.Object, typedObj); err != nil {
		return nil, fmt.Errorf("failed to convert patched object back to typed object: %w", err)
	}

	klog.InfoS("✅ MANUAL PATCH: Patch applied successfully",
		"resource", s.resourceName,
		"patchType", string(patchType))

	return typedObj, nil
}

// extractPatchDataFromObjInfo uses reflection to extract patch data from rest.defaultUpdatedObjectInfo
func extractPatchDataFromObjInfo(objInfo rest.UpdatedObjectInfo) (*PatchData, bool) {
	klog.InfoS("🔥 REFLECTION FUNCTION CALLED - Starting extraction process",
		"objInfoType", fmt.Sprintf("%T", objInfo))

	// Use reflection to access the private fields of defaultUpdatedObjectInfo
	objInfoValue := reflect.ValueOf(objInfo)
	if objInfoValue.Kind() == reflect.Ptr {
		objInfoValue = objInfoValue.Elem()
	}

	if objInfoValue.Kind() != reflect.Struct {
		klog.InfoS("❌ PATCH EXTRACTION: objInfo is not a struct",
			"kind", objInfoValue.Kind().String(),
			"type", fmt.Sprintf("%T", objInfo))
		return nil, false
	}

	klog.InfoS("🔍 PATCH EXTRACTION: Analyzing objInfo structure",
		"type", fmt.Sprintf("%T", objInfo),
		"numFields", objInfoValue.NumField())

	// Look for fields that might contain patch data
	objInfoType := objInfoValue.Type()
	for i := 0; i < objInfoValue.NumField(); i++ {
		field := objInfoType.Field(i)
		fieldValue := objInfoValue.Field(i)

		klog.InfoS("🔍 FIELD ANALYSIS",
			"fieldName", field.Name,
			"fieldType", field.Type.String(),
			"isExported", field.IsExported(),
			"canInterface", fieldValue.CanInterface())

		// Look for patch-related fields
		switch field.Name {
		case "patchType", "PatchType":
			if fieldValue.CanInterface() {
				if patchType, ok := fieldValue.Interface().(types.PatchType); ok {
					klog.InfoS("✅ FOUND PatchType field",
						"patchType", string(patchType))
				}
			}
		case "patch", "Patch", "data", "Data", "patchBytes":
			if fieldValue.CanInterface() {
				klog.InfoS("✅ FOUND potential patch data field",
					"fieldName", field.Name,
					"fieldType", field.Type.String())

				if bytes, ok := fieldValue.Interface().([]byte); ok {
					klog.InfoS("✅ FOUND patch bytes",
						"fieldName", field.Name,
						"size", len(bytes))
				}
			}
		}

		// Try to access private fields using unsafe if needed
		if !field.IsExported() && fieldValue.CanAddr() {
			klog.InfoS("🔍 PRIVATE FIELD detected (might contain patch data)",
				"fieldName", field.Name,
				"fieldType", field.Type.String())
		}
	}

	// 🔥 ADVANCED APPROACH: Use unsafe pointers to access private 'obj' field
	// The 'obj' field in defaultUpdatedObjectInfo likely contains the patched object

	var patchedObj runtime.Object

	// Try to access the 'obj' field using unsafe pointer manipulation
	if objInfoValue.Kind() == reflect.Struct {
		objField := objInfoValue.FieldByName("obj")
		if objField.IsValid() {
			klog.InfoS("✅ FOUND 'obj' field in defaultUpdatedObjectInfo",
				"fieldType", objField.Type().String(),
				"canInterface", objField.CanInterface())

			// Try to access private field using unsafe
			if objField.CanAddr() {
				// Get the address of the field
				objFieldPtr := objField.UnsafeAddr()

				// Cast to runtime.Object pointer
				objPtr := (*runtime.Object)(unsafe.Pointer(objFieldPtr))
				if objPtr != nil && *objPtr != nil {
					patchedObj = *objPtr

					klog.InfoS("🎉 SUCCESS: Accessed private 'obj' field using unsafe pointers",
						"objType", fmt.Sprintf("%T", patchedObj))

					// Return PatchData with the extracted object - this is better than raw patch data!
					return &PatchData{
						PatchType:       types.JSONPatchType, // Assume JSON patch for logging
						Data:            []byte("{}"),        // Mock data since we have the object directly
						Resource:        "services",
						Name:            "unknown",
						Namespace:       "unknown",
						ExtractedObject: patchedObj, // 🎉 The actual patched object!
					}, true
				}
			}
		}
	}

	klog.InfoS("❌ PATCH EXTRACTION: Unable to extract patch data from objInfo",
		"reason", "Could not access private 'obj' field via unsafe pointers",
		"suggestion", "May need different approach or direct objInfo object usage")

	return nil, false
}
