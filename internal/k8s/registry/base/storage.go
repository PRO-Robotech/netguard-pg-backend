package base

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"time"
	"unsafe"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/middleware"
	"netguard-pg-backend/internal/k8s/registry/base/fieldmanager"
	"netguard-pg-backend/internal/k8s/registry/base/patch"
	"netguard-pg-backend/internal/k8s/registry/utils"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	sigyaml "sigs.k8s.io/yaml"

	backendclient "netguard-pg-backend/internal/k8s/client"

	watchpkg "netguard-pg-backend/internal/k8s/registry/watch"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

type BaseStorage[K runtime.Object, D any] struct {
	NewFunc       func() K
	NewListFunc   func() runtime.Object
	backendOps    BackendOperations[D]
	backendClient backendclient.BackendClient
	converter     Converter[K, D]
	validator     Validator[K]
	watcher       *watch.Broadcaster
	resourceName  string
	kindName      string
	isNamespaced  bool
}

func NewBaseStorage[K runtime.Object, D any](
	newFunc func() K,
	newListFunc func() runtime.Object,
	backendOps BackendOperations[D],
	backendClient backendclient.BackendClient,
	converter Converter[K, D],
	validator Validator[K],
	watcher *watch.Broadcaster,
	resourceName string,
	kindName string,
	isNamespaced bool,
) *BaseStorage[K, D] {
	return &BaseStorage[K, D]{
		NewFunc:       newFunc,
		NewListFunc:   newListFunc,
		backendOps:    backendOps,
		backendClient: backendClient,
		converter:     converter,
		validator:     validator,
		watcher:       watcher,
		resourceName:  resourceName,
		kindName:      kindName,
		isNamespaced:  isNamespaced,
	}
}

var _ rest.Storage = &BaseStorage[runtime.Object, any]{}
var _ rest.Scoper = &BaseStorage[runtime.Object, any]{}
var _ rest.Getter = &BaseStorage[runtime.Object, any]{}
var _ rest.Lister = &BaseStorage[runtime.Object, any]{}
var _ rest.Creater = &BaseStorage[runtime.Object, any]{}
var _ rest.Updater = &BaseStorage[runtime.Object, any]{}
var _ rest.Patcher = &BaseStorage[runtime.Object, any]{}
var _ rest.GracefulDeleter = &BaseStorage[runtime.Object, any]{}
var _ rest.Watcher = &BaseStorage[runtime.Object, any]{}

func (s *BaseStorage[K, D]) New() runtime.Object {
	return s.NewFunc()
}
func (s *BaseStorage[K, D]) NewList() runtime.Object {
	return s.NewListFunc()
}
func (s *BaseStorage[K, D]) Destroy() {
}
func (s *BaseStorage[K, D]) NamespaceScoped() bool {
	return s.isNamespaced
}
func (s *BaseStorage[K, D]) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)
	domainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		klog.V(1).InfoS("get: backend error occurred",
			"name", name,
			"error", err.Error())
		if isNotFoundError(err) {
			klog.V(1).InfoS("get: returning Kubernetes NotFound error", "name", name)
			return nil, apierrors.NewNotFound(
				schema.GroupResource{Group: "netguard.sgroups.io", Resource: s.resourceName},
				name,
			)
		}
		klog.V(1).InfoS("get: backend error is not NotFound, returning raw error", "name", name)
		return nil, err
	}
	k8sObj, err := s.converter.FromDomain(ctx, *domainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert domain object to k8s object: %w", err)
	}
	return k8sObj, nil
}
func (s *BaseStorage[K, D]) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	// Extract namespace scope
	scope := utils.ScopeFromContext(ctx)

	// Parse field selectors and label selectors from K8s ListOptions
	var fieldSelectors []*netguardpb.FieldSelector
	var labelSelectors []*netguardpb.LabelSelector

	if options != nil {
		// Parse field selector if present
		if options.FieldSelector != nil {
			fieldSelectorStr := options.FieldSelector.String()
			parsed, err := ParseFieldSelectors(fieldSelectorStr)
			if err != nil {
				return nil, fmt.Errorf("invalid field selector: %w", err)
			}
			fieldSelectors = parsed
		}

		// Parse label selector if present
		if options.LabelSelector != nil {
			labelSelectorStr := options.LabelSelector.String()
			parsed, err := ParseLabelSelectors(labelSelectorStr)
			if err != nil {
				return nil, fmt.Errorf("invalid label selector: %w", err)
			}
			labelSelectors = parsed
		}
	}

	// If we have field or label selectors, convert scope to FieldSelectorScope
	if len(fieldSelectors) > 0 || len(labelSelectors) > 0 {
		klog.V(2).InfoS("List: parsing field/label selectors",
			"resource", s.resourceName,
			"fieldSelectorsCount", len(fieldSelectors),
			"labelSelectorsCount", len(labelSelectors))

		// Convert basic scope to FieldSelectorScope
		var identifiers []models.ResourceIdentifier
		if idScope, ok := scope.(ports.ResourceIdentifierScope); ok {
			identifiers = idScope.Identifiers
		}

		scope = ports.FieldSelectorScope{
			Identifiers:    identifiers,
			FieldSelectors: fieldSelectors,
			LabelSelectors: labelSelectors,
		}

		klog.V(2).InfoS("List: created FieldSelectorScope",
			"resource", s.resourceName,
			"scope", scope.String())
	}

	domainObjs, err := s.listFromBackend(ctx, scope)
	if err != nil {
		return nil, err
	}
	sortBy := utils.ExtractSortByFromContext(ctx)
	err = utils.ApplySorting(domainObjs, sortBy,
		func(obj D) models.ResourceIdentifier {
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
		func(obj D) runtime.Object {
			k8sObj, _ := s.converter.FromDomain(ctx, obj)
			return k8sObj
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sort objects: %w", err)
	}
	listObj, err := s.converter.ToList(ctx, domainObjs)
	if err != nil {
		return nil, fmt.Errorf("failed to convert domain objects to k8s list: %w", err)
	}
	listMetadataBuilder, err := NewListMetadataBuilder(listObj)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize list metadata builder: %w", err)
	}
	listMetadataBuilder.SetResourceVersionFromItems()
	return listObj, nil
}
func (s *BaseStorage[K, D]) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := utils.NewTable(
		metav1.TableColumnDefinition{Name: "Name", Type: "string", Description: "Name of the resource"},
		metav1.TableColumnDefinition{Name: "Age", Type: "string", Description: "Age of the resource"},
	)
	if utils.AppendBookmarkRowIfNeeded(table, object) {
		return table, nil
	}

	if _, isList := object.(metav1.ListInterface); isList {
		items, err := meta.ExtractList(object)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if accessor, err := meta.Accessor(item); err == nil {
				table.Rows = append(table.Rows, metav1.TableRow{
					Object: runtime.RawExtension{Object: item},
					Cells: []interface{}{
						accessor.GetName(),
						utils.TranslateTimestampSince(accessor.GetCreationTimestamp()),
					},
				})
			}
		}
		return table, nil
	}

	if accessor, err := meta.Accessor(object); err == nil {
		table.Rows = append(table.Rows, metav1.TableRow{
			Object: runtime.RawExtension{Object: object},
			Cells: []interface{}{
				accessor.GetName(),
				utils.TranslateTimestampSince(accessor.GetCreationTimestamp()),
			},
		})
	}
	return table, nil
}
func (s *BaseStorage[K, D]) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sObj, ok := obj.(K)
	if !ok {
		return nil, fmt.Errorf("expected %T, got %T", s.NewFunc(), obj)
	}
	if errs := s.validator.ValidateCreate(ctx, k8sObj); len(errs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "netguard.sgroups.io", Kind: s.kindName},
			getObjectName(k8sObj),
			errs,
		)
	}
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}
	if err := s.handleGeneratedName(k8sObj); err != nil {
		return nil, fmt.Errorf("failed to handle generated name: %w", err)
	}
	domainObj, err := s.converter.ToDomain(ctx, k8sObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert k8s object to domain object: %w", err)
	}
	createdDomainObj, err := s.createInBackend(ctx, &domainObj)
	if err != nil {
		return nil, err
	}
	createdK8sObj, err := s.converter.FromDomain(ctx, *createdDomainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert created domain object to k8s object: %w", err)
	}
	s.broadcastWatchEvent(watch.Added, createdK8sObj)
	return createdK8sObj, nil
}
func (s *BaseStorage[K, D]) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	namespace := utils.NamespaceFrom(ctx)
	klog.InfoS("🔄 BaseStorage.Update CALLED",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace,
		"forceAllowCreate", forceAllowCreate)
	currentDomainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) && forceAllowCreate {
			obj, err := objInfo.UpdatedObject(ctx, nil)
			if err != nil {
				return nil, false, err
			}
			created, err := s.Create(ctx, obj, createValidation, &metav1.CreateOptions{})
			return created, true, err
		}
		return nil, false, err
	}
	if s.hasDeletionTimestampSet(currentDomainObj) {
		klog.InfoS("❌ UPDATE REJECTED: Resource is marked for deletion",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace)
		return nil, false, fmt.Errorf(
			"cannot update resource %s/%s: resource is being deleted (has deletionTimestamp set)",
			namespace, name)
	}
	klog.V(3).InfoS("converting current domain object to k8s format",
		"resource", s.resourceName)
	currentK8sObj, err := s.converter.FromDomain(ctx, *currentDomainObj)
	if err != nil {
		klog.V(1).InfoS("conversion failed from domain to k8s",
			"resource", s.resourceName,
			"error", err.Error())
		return nil, false, fmt.Errorf("failed to convert current domain object to k8s object: %w", err)
	}
	klog.V(3).InfoS("conversion success from domain to k8s",
		"resource", s.resourceName)
	klog.V(4).InfoS("retrieving updated object from objInfo",
		"resource", s.resourceName,
		"objInfoType", fmt.Sprintf("%T", objInfo),
		"currentK8sObjType", fmt.Sprintf("%T", currentK8sObj))
	objInfoType := fmt.Sprintf("%T", objInfo)
	if objInfoType == "*rest.defaultUpdatedObjectInfo" {
		klog.V(4).InfoS("detected defaultUpdatedObjectInfo in patch flow",
			"resource", s.resourceName,
			"objInfoType", objInfoType)
		if requestInfo, ok := request.RequestInfoFrom(ctx); ok && requestInfo.Verb == "patch" {
			klog.V(4).InfoS("patch request using GET+UPDATE fallback instead of direct Patch method",
				"verb", requestInfo.Verb,
				"resource", requestInfo.Resource,
				"name", requestInfo.Name,
				"why", "Kubernetes framework chose fallback over direct Patch() method")
		}
	}
	objInfoType = fmt.Sprintf("%T", objInfo)
	var finalK8sObj K
	if objInfoType == "*rest.defaultUpdatedObjectInfo" {
		klog.V(4).InfoS("patch operation detected, attempting to extract patch data from objInfo",
			"resource", s.resourceName,
			"objInfoType", objInfoType,
			"reason", "Kubernetes bypassed Patch() method, using direct Update() with objInfo")
		if requestInfo, ok := request.RequestInfoFrom(ctx); ok && requestInfo.Verb == "patch" {
			klog.V(4).InfoS("confirmed patch request via objInfo",
				"verb", requestInfo.Verb,
				"resource", requestInfo.Resource,
				"name", requestInfo.Name)
			klog.V(4).InfoS("calling extractPatchDataFromObjInfo via reflection",
				"resource", s.resourceName,
				"objInfoType", objInfoType)
			patchData, extractSuccess := extractPatchDataFromObjInfo(objInfo)
			if extractSuccess {
				klog.V(4).InfoS("patch data extracted from objInfo via reflection",
					"resource", s.resourceName,
					"patchType", string(patchData.PatchType),
					"patchSize", len(patchData.Data))
				ctx = WithPatchData(ctx, patchData)
				klog.V(4).InfoS("patch data stored in context for immediate use",
					"resource", s.resourceName,
					"patchType", string(patchData.PatchType))
			} else {
				if storedPatch, ok := PatchDataFrom(ctx); ok && storedPatch != nil {
					patchData = storedPatch
					extractSuccess = true
					klog.V(4).InfoS("using patch data from Patch method context",
						"resource", s.resourceName,
						"patchType", string(patchData.PatchType))
				} else {
					if patchBodyData, ok := middleware.PatchBodyFrom(ctx); ok && patchBodyData != nil {
						patchType := determinePatchType(patchBodyData.ContentType)
						patchData = &PatchData{
							PatchType: patchType,
							Data:      patchBodyData.Body,
							Resource:  s.resourceName,
							Name:      name,
							Namespace: namespace,
						}
						extractSuccess = true
						klog.V(4).InfoS("using patch body from HTTP middleware",
							"resource", s.resourceName,
							"patchType", string(patchType),
							"bodySize", len(patchBodyData.Body),
							"contentType", patchBodyData.ContentType)
					} else {
						klog.ErrorS(nil, "patch body not found in any source",
							"resource", s.resourceName,
							"objInfoType", objInfoType)
						return nil, false, fmt.Errorf(
							"PATCH operation detected but patch data not available. Resource: %s/%s",
							namespace, name)
					}
				}
			}
			if extractSuccess {
				ctx = WithPatchData(ctx, patchData)
			}
		}
	}
	if patchData, hasPatchData := PatchDataFrom(ctx); hasPatchData {
		if patchData == nil {
			klog.ErrorS(nil, "patch data in context is nil",
				"resource", s.resourceName)
			return nil, false, fmt.Errorf("internal error: patch data is nil")
		}
		klog.InfoS("using patched object extracted from objInfo",
			"resource", s.resourceName,
			"method", "Unsafe pointer access to private 'obj' field",
			"reason", "Completely bypassing problematic objInfo.UpdatedObject() call")
		if patchData.ExtractedObject != nil {
			klog.InfoS("✅ USING EXTRACTED OBJECT: Direct object from objInfo reflection",
				"resource", s.resourceName,
				"extractedType", fmt.Sprintf("%T", patchData.ExtractedObject),
				"solution", "Perfect bypass - using pre-patched object from objInfo")
			var ok bool
			finalK8sObj, ok = patchData.ExtractedObject.(K)
			if !ok {
				return nil, false, fmt.Errorf("expected %T, got %T", s.NewFunc(), patchData.ExtractedObject)
			}
		} else {
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
			var ok bool
			finalK8sObj, ok = patchedObj.(K)
			if !ok {
				return nil, false, fmt.Errorf("expected %T, got %T", s.NewFunc(), patchedObj)
			}
		}
	} else {
		klog.InfoS("🔄 NON-PATCH UPDATE: Using original objInfo.UpdatedObject()",
			"resource", s.resourceName,
			"objInfoType", fmt.Sprintf("%T", objInfo))
		updatedObj, err := objInfo.UpdatedObject(ctx, currentK8sObj)
		if err != nil {
			klog.InfoS("❌ FAILED to get updated object from objInfo",
				"resource", s.resourceName,
				"error", err.Error())
			return nil, false, fmt.Errorf("objInfo.UpdatedObject() failed: %w", err)
		}
		klog.InfoS("✅ objInfo.UpdatedObject() SUCCESS",
			"resource", s.resourceName)
		var ok bool
		finalK8sObj, ok = updatedObj.(K)
		if !ok {
			return nil, false, fmt.Errorf("expected %T, got %T", s.NewFunc(), updatedObj)
		}
	}
	accessor, err := meta.Accessor(finalK8sObj)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get object accessor: %w", err)
	}
	deletionTimestamp := accessor.GetDeletionTimestamp()
	if deletionTimestamp != nil && !deletionTimestamp.IsZero() {
		klog.InfoS("🚫 UPDATE BLOCKED: Object is being deleted, rejecting update to prevent ResourceVersion conflicts",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace,
			"deletionTimestamp", deletionTimestamp.String())
		return nil, false, apierrors.NewConflict(
			schema.GroupResource{Group: "netguard.sgroups.io", Resource: s.resourceName},
			name,
			fmt.Errorf("object is being deleted, update rejected to prevent deletion conflicts"),
		)
	}
	if errs := s.validator.ValidateUpdate(ctx, finalK8sObj, currentK8sObj); len(errs) > 0 {
		return nil, false, apierrors.NewInvalid(
			schema.GroupKind{Group: "netguard.sgroups.io", Kind: s.kindName},
			getObjectName(finalK8sObj),
			errs,
		)
	}
	if updateValidation != nil {
		if err := updateValidation(ctx, finalK8sObj, currentK8sObj); err != nil {
			return nil, false, err
		}
	}
	finalDomainObj, err := s.converter.ToDomain(ctx, finalK8sObj)
	if err != nil {
		return nil, false, fmt.Errorf("failed to convert updated k8s object to domain object: %w", err)
	}
	updatedDomainObj, err := s.updateInBackend(ctx, &finalDomainObj)
	if err != nil {
		return nil, false, err
	}
	resultK8sObj, err := s.converter.FromDomain(ctx, *updatedDomainObj)
	if err != nil {
		return nil, false, fmt.Errorf("failed to convert updated domain object to k8s object: %w", err)
	}
	s.broadcastWatchEvent(watch.Modified, resultK8sObj)
	klog.InfoS("✅ BaseStorage.Update SUCCESS",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	return resultK8sObj, false, nil
}
func (s *BaseStorage[K, D]) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	namespace := utils.NamespaceFrom(ctx)
	klog.InfoS("🔥 DELETE METHOD CALLED - STARTING DELETE OPERATION",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
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
		return nil, false, apierrors.NewInvalid(
			schema.GroupKind{Group: "netguard.sgroups.io", Kind: s.kindName},
			getObjectName(k8sObj),
			errs,
		)
	}
	klog.InfoS("✅ DELETE: Validation passed",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
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
	klog.InfoS("🗑️ DELETE: Marking for deletion (soft delete)",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	err = s.markForDeletionInBackend(ctx, namespace, name)
	if err != nil {
		klog.InfoS("❌ DELETE: markForDeletionInBackend failed",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace,
			"error", err.Error())
		return nil, false, err
	}
	klog.InfoS("✅ DELETE: Marked for deletion successfully",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	klog.InfoS("🔄 DELETE: Fetching updated object after marking for deletion",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	updatedDomainObj, err := s.getFromBackend(ctx, namespace, name)
	if err != nil {
		klog.InfoS("❌ DELETE: Failed to fetch updated object",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace,
			"error", err.Error())
		if isNotFoundError(err) {
			klog.InfoS("🧹 DELETE: Resource hidden after soft delete — synthesizing deletion response",
				"resource", s.resourceName,
				"name", name,
				"namespace", namespace)
			if accessor, accessorErr := meta.Accessor(k8sObj); accessorErr == nil {
				now := metav1.Now()
				accessor.SetDeletionTimestamp(&now)
			} else {
				klog.InfoS("⚠️ DELETE: Failed to set synthetic deletionTimestamp",
					"resource", s.resourceName,
					"name", name,
					"namespace", namespace,
					"error", accessorErr.Error())
			}
			s.broadcastWatchEvent(watch.Modified, k8sObj)
			return k8sObj, true, nil
		}
		return nil, false, err
	}
	updatedK8sObj, err := s.converter.FromDomain(ctx, *updatedDomainObj)
	if err != nil {
		klog.InfoS("❌ DELETE: Failed to convert updated domain object",
			"resource", s.resourceName,
			"name", name,
			"namespace", namespace,
			"error", err.Error())
		return nil, false, err
	}
	klog.InfoS("📡 DELETE: Broadcasting watch.Modified event (soft delete)",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	s.broadcastWatchEvent(watch.Modified, updatedK8sObj)
	klog.InfoS("🎉 DELETE: Soft delete completed - resource marked for deletion",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace,
		"message", "Resource will be physically deleted after SGROUP sync by OutboxWorker")
	return updatedK8sObj, true, nil
}
func (s *BaseStorage[K, D]) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)
	klog.InfoS("🗑️🗑️🗑️ DELETE COLLECTION STARTED",
		"resource", s.resourceName,
		"namespace", namespace)
	listObj, err := s.List(ctx, listOptions)
	if err != nil {
		klog.InfoS("❌ DELETE COLLECTION: Failed to list resources",
			"resource", s.resourceName,
			"namespace", namespace,
			"error", err.Error())
		return nil, err
	}
	items, err := meta.ExtractList(listObj)
	if err != nil {
		klog.InfoS("❌ DELETE COLLECTION: Failed to extract items from list",
			"resource", s.resourceName,
			"namespace", namespace,
			"error", err.Error())
		return nil, err
	}
	klog.InfoS("📋 DELETE COLLECTION: Found resources to delete",
		"resource", s.resourceName,
		"namespace", namespace,
		"count", len(items))
	var deletedCount int
	for _, item := range items {
		accessor, err := meta.Accessor(item)
		if err != nil {
			klog.InfoS("⚠️ DELETE COLLECTION: Failed to get accessor for item",
				"resource", s.resourceName,
				"error", err.Error())
			continue
		}
		name := accessor.GetName()
		klog.V(4).InfoS("🗑️ DELETE COLLECTION: Deleting resource",
			"resource", s.resourceName,
			"namespace", namespace,
			"name", name)
		_, _, err = s.Delete(ctx, name, deleteValidation, options)
		if err != nil {
			klog.InfoS("⚠️ DELETE COLLECTION: Failed to delete resource",
				"resource", s.resourceName,
				"namespace", namespace,
				"name", name,
				"error", err.Error())
			continue
		}
		deletedCount++
	}
	klog.InfoS("✅ DELETE COLLECTION: Completed",
		"resource", s.resourceName,
		"namespace", namespace,
		"total", len(items),
		"deleted", deletedCount)
	return &metav1.Status{
		Status: metav1.StatusSuccess,
		Details: &metav1.StatusDetails{
			Kind: s.kindName,
		},
	}, nil
}
func (s *BaseStorage[K, D]) Patch(ctx context.Context, name string, patchType types.PatchType, data []byte, options *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)
	klog.InfoS("🚀🚀🚀 PATCH OPERATION STARTED - DEFINITELY BEING CALLED",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace,
		"patchType", string(patchType),
		"dataSize", len(data))
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
		ssaCtx := NewSSAContext(data, options)
		ctx = WithSSAContext(ctx, ssaCtx)
		currentDomainObj, err := s.getFromBackend(ctx, namespace, name)
		if err != nil {
			return nil, err
		} else {
			currentK8sObj, err = s.converter.FromDomain(ctx, *currentDomainObj)
			if err != nil {
				return nil, fmt.Errorf("failed to convert domain object to k8s object: %w", err)
			}
		}
	} else {
		currentDomainObj, err := s.getFromBackend(ctx, namespace, name)
		if err != nil {
			if isNotFoundError(err) {
				return nil, apierrors.NewNotFound(
					schema.GroupResource{Group: "netguard.sgroups.io", Resource: s.resourceName},
					name,
				)
			}
			return nil, err
		}
		currentK8sObj, err = s.converter.FromDomain(ctx, *currentDomainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert current domain object to k8s object: %w", err)
		}
	}
	var patchedObj runtime.Object
	if patchType == types.ApplyPatchType {
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
	if errs := s.validator.ValidateUpdate(ctx, patchedK8sObj, currentK8sObj); len(errs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "netguard.sgroups.io", Kind: s.kindName},
			getObjectName(patchedK8sObj),
			errs,
		)
	}
	patchedDomainObj, err := s.converter.ToDomain(ctx, patchedK8sObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert patched k8s object to domain object: %w", err)
	}
	var finalDomainObj *D
	var backendErr error
	if isCreateOperation {
		finalDomainObj, backendErr = s.createInBackend(ctx, &patchedDomainObj)
	} else {
		finalDomainObj, backendErr = s.updateInBackend(ctx, &patchedDomainObj)
	}
	if backendErr != nil {
		return nil, backendErr
	}
	finalK8sObj, err := s.converter.FromDomain(ctx, *finalDomainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert final domain object to k8s object: %w", err)
	}
	if patchType == types.ApplyPatchType {
		patchedObjInterface := runtime.Object(patchedK8sObj)
		if patchedObjInterface != nil {
			if err := s.preserveManagedFields(patchedObjInterface, finalK8sObj); err != nil {
				klog.ErrorS(err, "Failed to preserve managedFields after Server-Side Apply")
			}
		}
	}
	s.broadcastWatchEvent(watch.Modified, finalK8sObj)
	return finalK8sObj, nil
}
func (s *BaseStorage[K, D]) Watch(ctx context.Context, options *internalversion.ListOptions) (watch.Interface, error) {
	if s.backendClient == nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("watch not configured for resource %s", s.resourceName))
	}

	if options != nil {
		options.AllowWatchBookmarks = false
	}

	req := s.buildWatchRequest(ctx, options)

	stream, err := s.startWatchStream(ctx, req)
	if err != nil {
		return nil, s.translateWatchError(err)
	}

	watcher, err := watchpkg.NewGRPCWatcher(s.resourceName, stream)
	if err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to initialize gRPC watch: %w", err))
	}

	klog.V(4).InfoS("Created gRPC watch",
		"resource", s.resourceName,
		"namespace", req.Namespace,
		"resourceVersion", req.ResourceVersion,
		"resourceVersionMatch", req.ResourceVersionMatch,
		"timeoutSeconds", req.TimeoutSeconds,
		"labelSelector", req.LabelSelector,
		"fieldSelector", req.FieldSelector)

	return watcher, nil
}
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
		if isNotFoundError(err) {
			if s.isServerSideApplyRequest(ctx) {
				klog.V(1).InfoS("🔄 SERVER-SIDE APPLY CREATE: Detected SSA CREATE in getFromBackend",
					"resource", s.resourceName,
					"name", name,
					"namespace", namespace)
				createdResult, createErr := s.handleSimpleSSACreate(ctx, namespace, name)
				if createErr != nil {
					klog.V(1).InfoS("❌ SSA CREATE failed in getFromBackend",
						"error", createErr.Error())
					return nil, err
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
func (s *BaseStorage[K, D]) markForDeletionInBackend(ctx context.Context, namespace, name string) error {
	klog.InfoS("🔧 markForDeletionInBackend: Marking resource for deletion",
		"resource", s.resourceName,
		"kind", s.kindName,
		"name", name,
		"namespace", namespace)
	err := s.backendOps.MarkForDeletion(ctx, namespace, name, s.kindName)
	if err != nil {
		klog.InfoS("❌ markForDeletionInBackend: MarkForDeletion failed",
			"resource", s.resourceName,
			"kind", s.kindName,
			"name", name,
			"namespace", namespace,
			"error", err.Error())
		return err
	}
	klog.InfoS("✅ markForDeletionInBackend: Successfully marked for deletion",
		"resource", s.resourceName,
		"kind", s.kindName,
		"name", name,
		"namespace", namespace)
	return nil
}
func (s *BaseStorage[K, D]) broadcastWatchEvent(eventType watch.EventType, obj runtime.Object) {
	if s.watcher != nil {
		s.watcher.Action(eventType, obj)
	}
}

func (s *BaseStorage[K, D]) buildWatchRequest(ctx context.Context, options *internalversion.ListOptions) *netguardpb.WatchRequest {
	req := &netguardpb.WatchRequest{
		Namespace: utils.NamespaceFrom(ctx),
	}

	if options == nil {
		req.ResourceVersion = "0"
		return req
	}

	if options.ResourceVersion == "" {
		req.ResourceVersion = "0"
	} else {
		req.ResourceVersion = options.ResourceVersion
	}

	req.ResourceVersionMatch = string(options.ResourceVersionMatch)

	if options.TimeoutSeconds != nil {
		req.TimeoutSeconds = int64(*options.TimeoutSeconds)
	}

	if options.LabelSelector != nil {
		req.LabelSelector = options.LabelSelector.String()
	}

	if options.FieldSelector != nil {
		req.FieldSelector = options.FieldSelector.String()
	}

	return req
}

func (s *BaseStorage[K, D]) startWatchStream(ctx context.Context, req *netguardpb.WatchRequest) (watchpkg.Stream, error) {
	switch s.resourceName {
	case "services":
		return s.backendClient.WatchServices(ctx, req)
	case "addressgroups":
		return s.backendClient.WatchAddressGroups(ctx, req)
	case "addressgroupbindings":
		return s.backendClient.WatchAddressGroupBindings(ctx, req)
	case "addressgroupportmappings":
		return s.backendClient.WatchAddressGroupPortMappings(ctx, req)
	case "addressgroupbindingpolicies":
		return s.backendClient.WatchAddressGroupBindingPolicies(ctx, req)
	case "rules2s":
		return s.backendClient.WatchRuleS2S(ctx, req)
	case "servicealiases":
		return s.backendClient.WatchServiceAliases(ctx, req)
	case "hosts":
		return s.backendClient.WatchHosts(ctx, req)
	case "hostbindings":
		return s.backendClient.WatchHostBindings(ctx, req)
	case "networks":
		return s.backendClient.WatchNetworks(ctx, req)
	case "networkbindings":
		return s.backendClient.WatchNetworkBindings(ctx, req)
	case "ieagagrules":
		return s.backendClient.WatchIEAgAgRules(ctx, req)
	case "svcsvcrules":
		return s.backendClient.WatchSvcSvcRules(ctx, req)
	case "svcfqdnrules":
		return s.backendClient.WatchSvcFqdnRules(ctx, req)
	default:
		return nil, fmt.Errorf("watch not supported for resource %s", s.resourceName)
	}
}

func (s *BaseStorage[K, D]) translateWatchError(err error) error {
	if err == nil {
		return nil
	}

	if stdErrors.Is(err, context.Canceled) || stdErrors.Is(err, context.DeadlineExceeded) {
		return err
	}

	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.InvalidArgument:
			return apierrors.NewBadRequest(st.Message())
		case codes.NotFound:
			return apierrors.NewNotFound(schema.GroupResource{Group: "netguard.sgroups.io", Resource: s.resourceName}, "")
		case codes.PermissionDenied, codes.Unauthenticated:
			return apierrors.NewForbidden(schema.GroupResource{Group: "netguard.sgroups.io", Resource: s.resourceName}, "", err)
		case codes.FailedPrecondition, codes.AlreadyExists:
			return apierrors.NewConflict(schema.GroupResource{Group: "netguard.sgroups.io", Resource: s.resourceName}, "", err)
		}
	}

	return apierrors.NewInternalError(fmt.Errorf("watch stream failed: %w", err))
}
func (s *BaseStorage[K, D]) hasDeletionTimestampSet(domainObj *D) bool {
	k8sObj, err := s.converter.FromDomain(context.Background(), *domainObj)
	if err != nil {
		klog.V(2).InfoS("Failed to convert domain object to check deletionTimestamp",
			"resource", s.resourceName,
			"error", err.Error())
		return false
	}
	accessor, err := meta.Accessor(k8sObj)
	if err != nil {
		klog.V(2).InfoS("Failed to get metadata accessor",
			"resource", s.resourceName,
			"error", err.Error())
		return false
	}
	deletionTime := accessor.GetDeletionTimestamp()
	return deletionTime != nil && !deletionTime.IsZero()
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
func (s *BaseStorage[K, D]) GetConverter() Converter[K, D] {
	return s.converter
}
func (s *BaseStorage[K, D]) GetBackendOps() BackendOperations[D] {
	return s.backendOps
}
func (s *BaseStorage[K, D]) isServerSideApplyRequest(ctx context.Context) bool {
	ssaCtx, ok := GetSSAContext(ctx)
	if ok && ssaCtx != nil {
		klog.V(1).InfoS("🔍 Detected Server-Side Apply request",
			"fieldManager", ssaCtx.FieldManager)
		return true
	}
	return false
}
func (s *BaseStorage[K, D]) handleSimpleSSACreate(ctx context.Context, namespace, name string) (*D, error) {
	klog.V(1).InfoS("🛠️ Creating minimal resource for SSA CREATE",
		"resource", s.resourceName,
		"name", name,
		"namespace", namespace)
	obj := s.NewFunc()
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get object accessor: %w", err)
	}
	accessor.SetName(name)
	if s.isNamespaced {
		accessor.SetNamespace(namespace)
	}
	if err := s.setMinimalRequiredFields(obj); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set minimal fields, proceeding anyway", "error", err.Error())
	}
	accessor.SetManagedFields([]metav1.ManagedFieldsEntry{{
		Manager:    "simple-ssa-manager",
		Operation:  metav1.ManagedFieldsOperationApply,
		APIVersion: "netguard.sgroups.io/v1beta1",
		Time:       &metav1.Time{Time: time.Now()},
	}})
	if errs := s.validator.ValidateCreate(ctx, obj); len(errs) > 0 {
		klog.V(1).InfoS("⚠️ Validation errors for minimal object",
			"errors", errs.ToAggregate().Error())
	}
	domainObj, err := s.converter.ToDomain(ctx, obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to domain object: %w", err)
	}
	err = s.backendOps.Create(ctx, &domainObj)
	if err != nil {
		return nil, fmt.Errorf("failed to create minimal resource in backend: %w", err)
	}
	klog.V(1).InfoS("✅ Minimal SSA CREATE successful in backend")
	return &domainObj, nil
}
func (s *BaseStorage[K, D]) setMinimalRequiredFields(obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	if accessor.GetLabels() == nil {
		accessor.SetLabels(make(map[string]string))
	}
	if accessor.GetAnnotations() == nil {
		accessor.SetAnnotations(make(map[string]string))
	}
	annotations := accessor.GetAnnotations()
	annotations["netguard.sgroups.io/created-via"] = "simple-ssa"
	accessor.SetAnnotations(annotations)
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
		return nil
	default:
		klog.V(1).InfoS("⚠️ Unknown resource type, using minimal fields", "resourceName", s.resourceName)
		return nil
	}
}
func (s *BaseStorage[K, D]) setAddressGroupRequiredFields(obj runtime.Object) error {
	if err := s.setObjectField(obj, "Spec.DefaultAction", "ACCEPT"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set AddressGroup defaultAction", "error", err.Error())
	}
	return nil
}
func (s *BaseStorage[K, D]) setRuleS2SRequiredFields(obj runtime.Object) error {
	if err := s.setObjectField(obj, "Spec.Traffic", "INGRESS"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set RuleS2S traffic", "error", err.Error())
	}
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
	if err := s.setObjectField(obj, "Spec.Action", "ACCEPT"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set RuleS2S action", "error", err.Error())
	}
	return nil
}
func (s *BaseStorage[K, D]) setIEAgAgRuleRequiredFields(obj runtime.Object) error {
	if err := s.setObjectField(obj, "Spec.Traffic", "INGRESS"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set IEAgAgRule traffic", "error", err.Error())
	}
	if err := s.setObjectField(obj, "Spec.Action", "ACCEPT"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set IEAgAgRule action", "error", err.Error())
	}
	return nil
}
func (s *BaseStorage[K, D]) setServiceAliasRequiredFields(obj runtime.Object) error {
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
func (s *BaseStorage[K, D]) setAddressGroupBindingRequiredFields(obj runtime.Object) error {
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
func (s *BaseStorage[K, D]) setAddressGroupPortMappingRequiredFields(obj runtime.Object) error {
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
func (s *BaseStorage[K, D]) setAddressGroupBindingPolicyRequiredFields(obj runtime.Object) error {
	return nil
}
func (s *BaseStorage[K, D]) setNetworkRequiredFields(obj runtime.Object) error {
	if err := s.setObjectField(obj, "Spec.CIDR", "10.0.0.0/24"); err != nil {
		klog.V(1).InfoS("⚠️ Failed to set Network CIDR", "error", err.Error())
	}
	return nil
}
func (s *BaseStorage[K, D]) setNetworkBindingRequiredFields(obj runtime.Object) error {
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
	fieldParts := strings.Split(fieldPath, ".")
	currentValue := objValue
	for i, part := range fieldParts[:len(fieldParts)-1] {
		field := currentValue.FieldByName(part)
		if !field.IsValid() {
			return fmt.Errorf("field %s not found at level %d", part, i)
		}
		if field.Kind() == reflect.Ptr && field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		if field.Kind() == reflect.Ptr {
			currentValue = field.Elem()
		} else {
			currentValue = field
		}
	}
	finalField := currentValue.FieldByName(fieldParts[len(fieldParts)-1])
	if !finalField.IsValid() {
		return fmt.Errorf("final field %s not found", fieldParts[len(fieldParts)-1])
	}
	if !finalField.CanSet() {
		return fmt.Errorf("field %s cannot be set", fieldPath)
	}
	valueReflect := reflect.ValueOf(value)
	if finalField.Type() != valueReflect.Type() {
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
func (s *BaseStorage[K, D]) handleGeneratedName(obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("failed to get object accessor: %w", err)
	}
	if accessor.GetName() != "" {
		return nil
	}
	generateName := accessor.GetGenerateName()
	if generateName != "" {
		timestamp := time.Now().UnixNano() / int64(time.Millisecond)
		random := rand.Intn(999999)
		uniqueName := fmt.Sprintf("%s%x%x", generateName, timestamp, random)
		accessor.SetName(uniqueName)
	}
	return nil
}
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
	sourceManagedFields := sourceAccessor.GetManagedFields()
	if sourceManagedFields == nil {
		klog.V(4).InfoS("No managedFields to preserve",
			"resourceName", destAccessor.GetName(),
			"namespace", destAccessor.GetNamespace())
		return nil
	}
	preservedFields := make([]metav1.ManagedFieldsEntry, len(sourceManagedFields))
	copy(preservedFields, sourceManagedFields)
	destManagedFields := destAccessor.GetManagedFields()
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
		destAccessor.SetManagedFields(preservedFields)
		klog.V(3).InfoS("Preserved managedFields",
			"resourceName", destAccessor.GetName(),
			"namespace", destAccessor.GetNamespace(),
			"fieldCount", len(preservedFields))
	}
	return nil
}
func (s *BaseStorage[K, D]) mergeManagedFields(sourceFields, destFields []metav1.ManagedFieldsEntry) []metav1.ManagedFieldsEntry {
	merged := make([]metav1.ManagedFieldsEntry, 0, len(sourceFields)+len(destFields))
	sourceMap := make(map[string]metav1.ManagedFieldsEntry)
	for _, field := range sourceFields {
		key := s.managedFieldKey(field)
		sourceMap[key] = field
		merged = append(merged, field)
	}
	for _, destField := range destFields {
		key := s.managedFieldKey(destField)
		if _, exists := sourceMap[key]; !exists {
			merged = append(merged, destField)
		}
	}
	return merged
}
func (s *BaseStorage[K, D]) managedFieldKey(field metav1.ManagedFieldsEntry) string {
	return fmt.Sprintf("%s:%s:%s", field.Manager, field.Operation, field.Subresource)
}
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	isEntityNotFound := strings.Contains(errMsg, "entity not found")
	isGeneralNotFound := strings.Contains(errMsg, "not found")
	isK8sNotFound := apierrors.IsNotFound(err)
	klog.V(1).InfoS("🔍 DEBUG: isNotFoundError check",
		"errorMsg", errMsg,
		"entityNotFound", isEntityNotFound,
		"generalNotFound", isGeneralNotFound,
		"k8sNotFound", isK8sNotFound)
	result := isEntityNotFound || isGeneralNotFound || isK8sNotFound
	klog.V(1).InfoS("🔍 DEBUG: isNotFoundError result", "result", result)
	return result
}
func (s *BaseStorage[K, D]) setObjectMeta(obj runtime.Object, name, namespace string) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("failed to get object accessor: %w", err)
	}
	accessor.SetName(name)
	if s.isNamespaced {
		accessor.SetNamespace(namespace)
	}
	gvk := s.getGVK()
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	return nil
}
func (s *BaseStorage[K, D]) getGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "netguard.sgroups.io",
		Version: "v1beta1",
		Kind:    s.kindName,
	}
}
func (s *BaseStorage[K, D]) ApplyServerSide(ctx context.Context, name string, data []byte, fieldManager string, force bool) (runtime.Object, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("apply patch cannot be empty")
	}
	if fieldManager == "" {
		fieldManager = "netguard-apiserver"
	}
	return s.Patch(ctx, name, types.ApplyPatchType, data, &metav1.PatchOptions{FieldManager: fieldManager, Force: &force})
}
func deepMerge(target map[string]interface{}, patch map[string]interface{}) {
	for key, patchValue := range patch {
		if patchValue == nil {
			delete(target, key)
			continue
		}
		targetValue, targetExists := target[key]
		if targetExists {
			if targetMap, targetIsMap := targetValue.(map[string]interface{}); targetIsMap {
				if patchMap, patchIsMap := patchValue.(map[string]interface{}); patchIsMap {
					deepMerge(targetMap, patchMap)
					continue
				}
			}
		}
		target[key] = patchValue
	}
}
func (s *BaseStorage[K, D]) applyPatchManually(ctx context.Context, currentObj K, patchType types.PatchType, patchData []byte) (runtime.Object, error) {
	klog.InfoS("🔧 MANUAL PATCH: Starting manual patch application",
		"resource", s.resourceName,
		"patchType", string(patchType),
		"patchSize", len(patchData))
	currentObjUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(currentObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert current object to unstructured: %w", err)
	}
	currentUnstructured := &unstructured.Unstructured{Object: currentObjUnstructured}
	var patchedUnstructured *unstructured.Unstructured
	switch patchType {
	case types.JSONPatchType:
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
		klog.InfoS("🔧 Applying Merge Patch", "resource", s.resourceName)
		var patchObj map[string]interface{}
		if err := json.Unmarshal(patchData, &patchObj); err != nil {
			return nil, fmt.Errorf("failed to unmarshal merge patch: %w", err)
		}
		patchedUnstructured = currentUnstructured.DeepCopy()
		deepMerge(patchedUnstructured.Object, patchObj)
	case types.StrategicMergePatchType:
		klog.InfoS("🔧 Applying Strategic Merge Patch", "resource", s.resourceName)
		var patchObj map[string]interface{}
		if err := json.Unmarshal(patchData, &patchObj); err != nil {
			return nil, fmt.Errorf("failed to unmarshal strategic merge patch: %w", err)
		}
		patchedUnstructured = currentUnstructured.DeepCopy()
		deepMerge(patchedUnstructured.Object, patchObj)
		klog.InfoS("✅ MANUAL PATCH: Strategic merge patch applied successfully",
			"resource", s.resourceName,
			"patchType", string(patchType))
	default:
		return nil, fmt.Errorf("unsupported patch type: %s", patchType)
	}
	typedObj := s.NewFunc()
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(patchedUnstructured.Object, typedObj); err != nil {
		return nil, fmt.Errorf("failed to convert patched object back to typed object: %w", err)
	}
	klog.InfoS("✅ MANUAL PATCH: Patch applied successfully",
		"resource", s.resourceName,
		"patchType", string(patchType))
	return typedObj, nil
}
func determinePatchType(contentType string) types.PatchType {
	switch {
	case strings.Contains(contentType, "application/json-patch+json"):
		return types.JSONPatchType
	case strings.Contains(contentType, "application/merge-patch+json"):
		return types.MergePatchType
	case strings.Contains(contentType, "application/strategic-merge-patch+json"):
		return types.StrategicMergePatchType
	case strings.Contains(contentType, "application/apply-patch+yaml"):
		return types.ApplyPatchType
	default:
		return types.StrategicMergePatchType
	}
}
func extractPatchDataFromObjInfo(objInfo rest.UpdatedObjectInfo) (*PatchData, bool) {
	klog.InfoS("🔥 REFLECTION FUNCTION CALLED - Starting extraction process",
		"objInfoType", fmt.Sprintf("%T", objInfo))
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
	objInfoType := objInfoValue.Type()
	for i := 0; i < objInfoValue.NumField(); i++ {
		field := objInfoType.Field(i)
		fieldValue := objInfoValue.Field(i)
		klog.InfoS("🔍 FIELD ANALYSIS",
			"fieldName", field.Name,
			"fieldType", field.Type.String(),
			"isExported", field.IsExported(),
			"canInterface", fieldValue.CanInterface())
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
		if !field.IsExported() && fieldValue.CanAddr() {
			klog.InfoS("🔍 PRIVATE FIELD detected (might contain patch data)",
				"fieldName", field.Name,
				"fieldType", field.Type.String())
		}
	}
	var patchedObj runtime.Object
	if objInfoValue.Kind() == reflect.Struct {
		objField := objInfoValue.FieldByName("obj")
		if objField.IsValid() {
			klog.InfoS("✅ FOUND 'obj' field in defaultUpdatedObjectInfo",
				"fieldType", objField.Type().String(),
				"canInterface", objField.CanInterface())
			if !objField.CanInterface() && objField.CanAddr() {
				objFieldValue := reflect.NewAt(objField.Type(), unsafe.Pointer(objField.UnsafeAddr())).Elem()
				klog.InfoS("🔧 UNSAFE ACCESS: Created objFieldValue via reflect.NewAt",
					"canInterface", objFieldValue.CanInterface(),
					"isValid", objFieldValue.IsValid(),
					"kind", objFieldValue.Kind().String())
				if objFieldValue.IsValid() {
					defer func() {
						if r := recover(); r != nil {
							klog.InfoS("⚠️ PANIC during Interface() call",
								"panic", r)
						}
					}()
					objInterface := objFieldValue.Interface()
					if obj, ok := objInterface.(runtime.Object); ok && obj != nil {
						patchedObj = obj
						klog.InfoS("🎉 SUCCESS: Accessed private 'obj' field using unsafe pointers",
							"objType", fmt.Sprintf("%T", patchedObj))
						return &PatchData{
							PatchType:       types.JSONPatchType,
							Data:            []byte("{}"),
							Resource:        "services",
							Name:            "unknown",
							Namespace:       "unknown",
							ExtractedObject: patchedObj,
						}, true
					} else {
						klog.InfoS("❌ Interface conversion failed",
							"objInterface", fmt.Sprintf("%T", objInterface),
							"isNil", obj == nil)
					}
				}
			}
		}
	}
	klog.InfoS("❌ PATCH EXTRACTION: Unable to extract patch data from objInfo",
		"reason", "Could not access private 'obj' field via unsafe pointers",
		"suggestion", "May need different approach or direct objInfo object usage")
	return nil, false
}

var (
	_ rest.Storage           = &BaseStorage[runtime.Object, any]{}
	_ rest.Scoper            = &BaseStorage[runtime.Object, any]{}
	_ rest.StandardStorage   = &BaseStorage[runtime.Object, any]{}
	_ rest.CollectionDeleter = &BaseStorage[runtime.Object, any]{}
	_ rest.Patcher           = &BaseStorage[runtime.Object, any]{}
	_ rest.Getter            = &BaseStorage[runtime.Object, any]{}
	_ rest.Lister            = &BaseStorage[runtime.Object, any]{}
	_ rest.CreaterUpdater    = &BaseStorage[runtime.Object, any]{}
	_ rest.GracefulDeleter   = &BaseStorage[runtime.Object, any]{}
	_ rest.Watcher           = &BaseStorage[runtime.Object, any]{}
)
