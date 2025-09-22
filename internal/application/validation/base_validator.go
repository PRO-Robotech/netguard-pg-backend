package validation

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
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

	var err error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = v.listFunction(ctx, func(entity interface{}) error {
			if keyExtractor(entity) == id.Key() {
				exists = true
			}
			return nil
		}, ports.NewResourceIdentifierScope(id))

		if err == nil {
			break
		}

		if strings.Contains(err.Error(), "conn busy") && attempt < maxRetries-1 {
			time.Sleep(time.Duration(10*(1<<attempt)) * time.Millisecond)
			continue
		}

		return errors.Wrap(err, fmt.Sprintf("failed to check %s existence", v.entityType))
	}

	if !exists {
		return NewEntityNotFoundError(v.entityType, id.Key())
	}

	return nil
}

// CheckEntityExists verifies if an entity with the given ID already exists
// This is specifically designed for creation validation to prevent duplicates
func (v *BaseValidator) CheckEntityExists(ctx context.Context, id models.ResourceIdentifier, keyExtractor func(interface{}) string) (bool, interface{}, error) {
	startTime := time.Now()

	// Log validation start with detailed context
	klog.V(2).Infof("🔍 VALIDATION: Checking existence for %s entity: %s", v.entityType, id.Key())

	var foundEntity interface{}
	exists := false
	queryCount := 0

	err := v.listFunction(ctx, func(entity interface{}) error {
		queryCount++
		if keyExtractor(entity) == id.Key() {
			exists = true
			foundEntity = entity
			klog.V(3).Infof("🚨 CONFLICT: Found existing %s entity: %s", v.entityType, id.Key())
		}
		return nil
	}, ports.NewResourceIdentifierScope(id))

	duration := time.Since(startTime)

	// Log performance metrics and results
	if err != nil {
		klog.Errorf("❌ VALIDATION ERROR: Failed existence check for %s %s after %v (queries: %d): %v",
			v.entityType, id.Key(), duration, queryCount, err)
		return false, nil, errors.Wrap(err, fmt.Sprintf("failed to check %s existence", v.entityType))
	}

	if exists {
		klog.V(2).Infof("⚠️  VALIDATION CONFLICT: %s entity %s already exists (check took %v, queries: %d)",
			v.entityType, id.Key(), duration, queryCount)
	} else {
		klog.V(2).Infof("✅ VALIDATION PASS: %s entity %s does not exist (check took %v, queries: %d)",
			v.entityType, id.Key(), duration, queryCount)
	}

	return exists, foundEntity, nil
}

// ValidateEntityDoesNotExistForCreation performs comprehensive existence validation for entity creation
// This is the main method that should be called during creation validation
func (v *BaseValidator) ValidateEntityDoesNotExistForCreation(ctx context.Context, id models.ResourceIdentifier, keyExtractor func(interface{}) string) error {
	startTime := time.Now()

	// Log validation entry with full context
	klog.V(1).Infof("🚀 CREATION VALIDATION: Starting existence validation for %s: %s", v.entityType, id.Key())

	exists, foundEntity, err := v.CheckEntityExists(ctx, id, keyExtractor)
	if err != nil {
		klog.Errorf("❌ CREATION VALIDATION FAILED: Database error during existence check for %s %s: %v",
			v.entityType, id.Key(), err)
		return err
	}

	if exists {
		// Generate detailed conflict information
		conflictDetails := fmt.Sprintf("Entity was found in namespace '%s' with name '%s'", id.Namespace, id.Name)
		suggestedAction := fmt.Sprintf("Use a different name or update the existing %s instead", v.entityType)

		// Log detailed conflict information for troubleshooting
		klog.Warningf("🚨 CREATION VALIDATION REJECTED: %s %s already exists - rejecting duplicate creation (validation took %v)",
			v.entityType, id.Key(), time.Since(startTime))

		if klog.V(3).Enabled() {
			// In verbose mode, log details about the existing entity for debugging
			klog.V(3).Infof("🔍 EXISTING ENTITY DETAILS: %+v", foundEntity)
		}

		return NewEntityAlreadyExistsError(v.entityType, id.Key(), foundEntity, conflictDetails, suggestedAction)
	}

	// Log successful validation
	klog.V(1).Infof("✅ CREATION VALIDATION PASSED: %s %s is available for creation (validation took %v)",
		v.entityType, id.Key(), time.Since(startTime))

	return nil
}

// 🚀 VALIDATION PHASE 1: Ready Condition Framework
// Based on k8s-controller webhook_utils.go:102-122 and validation.go:95-111

// MetaProvider interface for accessing Meta field from any resource
type MetaProvider interface {
	GetMeta() *models.Meta
}

// IsReadyConditionTrue checks if the Ready condition is true for the given object
// Ported from k8s-controller webhook_utils.go:102-122
func (v *BaseValidator) IsReadyConditionTrue(obj interface{}) bool {
	// Use type assertion to get Meta from the object
	switch o := obj.(type) {
	case *models.Service:
		return o.Meta.IsReady()
	case models.Service:
		return o.Meta.IsReady()
	case *models.AddressGroup:
		return o.Meta.IsReady()
	case models.AddressGroup:
		return o.Meta.IsReady()
	case *models.AddressGroupBinding:
		return o.Meta.IsReady()
	case models.AddressGroupBinding:
		return o.Meta.IsReady()
	case *models.ServiceAlias:
		return o.Meta.IsReady()
	case models.ServiceAlias:
		return o.Meta.IsReady()
	case *models.RuleS2S:
		return o.Meta.IsReady()
	case models.RuleS2S:
		return o.Meta.IsReady()
	case *models.AddressGroupPortMapping:
		return o.Meta.IsReady()
	case models.AddressGroupPortMapping:
		return o.Meta.IsReady()
	case *models.AddressGroupBindingPolicy:
		return o.Meta.IsReady()
	case models.AddressGroupBindingPolicy:
		return o.Meta.IsReady()
	case *models.IEAgAgRule:
		return o.Meta.IsReady()
	case models.IEAgAgRule:
		return o.Meta.IsReady()
	case *models.Network:
		return o.Meta.IsReady()
	case models.Network:
		return o.Meta.IsReady()
	case *models.NetworkBinding:
		return o.Meta.IsReady()
	case models.NetworkBinding:
		return o.Meta.IsReady()
	case MetaProvider:
		return o.GetMeta().IsReady()
	default:
		// If we don't know how to check the condition, assume it's not ready
		return false
	}
}

// ValidateSpecNotChangedWhenReady validates that the Spec hasn't changed during an update
// if the Ready condition is true
// Ported from k8s-controller webhook_utils.go:147-158
func (v *BaseValidator) ValidateSpecNotChangedWhenReady(oldObj, newObj interface{}, oldSpec, newSpec interface{}) error {
	// Check if specs are different
	if !reflect.DeepEqual(oldSpec, newSpec) {
		// Check if the Ready condition is true in the old object
		if v.IsReadyConditionTrue(oldObj) {
			return fmt.Errorf("spec cannot be changed when Ready condition is true")
		}
	}
	return nil
}

// ValidateFieldNotChangedWhenReady validates that a field hasn't changed during an update
// if the Ready condition is true
// Ported from k8s-controller webhook_utils.go:135-145
func (v *BaseValidator) ValidateFieldNotChangedWhenReady(fieldName string, oldObj, newObj interface{}, oldValue, newValue interface{}) error {
	if !reflect.DeepEqual(oldValue, newValue) {
		// Check if the Ready condition is true in the old object
		if v.IsReadyConditionTrue(oldObj) {
			return fmt.Errorf("cannot change %s when Ready condition is true", fieldName)
		}
	}
	return nil
}

// ObjectReferencer interface for objects that have reference fields
type ObjectReferencer interface {
	GetName() string
	GetKind() string
	GetAPIVersion() string
	GetNamespace() string
}

// 🚀 VALIDATION PHASE 2: Object Reference Immutability
// Adapters for v1beta1.NamespacedObjectReference to implement ObjectReferencer interface

// NamespacedObjectReferenceAdapter adapts v1beta1.NamespacedObjectReference to ObjectReferencer
type NamespacedObjectReferenceAdapter struct {
	Ref netguardv1beta1.NamespacedObjectReference
}

func (a *NamespacedObjectReferenceAdapter) GetName() string {
	return a.Ref.Name
}

func (a *NamespacedObjectReferenceAdapter) GetKind() string {
	return a.Ref.Kind
}

func (a *NamespacedObjectReferenceAdapter) GetAPIVersion() string {
	return a.Ref.APIVersion
}

func (a *NamespacedObjectReferenceAdapter) GetNamespace() string {
	return a.Ref.Namespace
}

// ValidateObjectReferencesNotChangedWhenReady validates multiple object references haven't changed
// during an update if the Ready condition is true
// Ported from k8s-controller validation patterns
func (v *BaseValidator) ValidateObjectReferencesNotChangedWhenReady(oldObj, newObj interface{}, referenceComparisons []ObjectReferenceComparison) error {
	// Only validate if the old object is Ready
	if !v.IsReadyConditionTrue(oldObj) {
		return nil // Allow changes when not Ready
	}

	for _, comparison := range referenceComparisons {
		if err := v.ValidateObjectReferenceNotChangedWhenReady(oldObj, newObj, comparison.OldRef, comparison.NewRef, comparison.FieldName); err != nil {
			return err
		}
	}
	return nil
}

// ObjectReferenceComparison holds a single reference comparison
type ObjectReferenceComparison struct {
	OldRef    ObjectReferencer
	NewRef    ObjectReferencer
	FieldName string
}

// ValidateObjectReferenceNotChangedWhenReady validates that a reference hasn't changed during an update
// if the Ready condition is true
// Ported from k8s-controller validation.go:95-111
func (v *BaseValidator) ValidateObjectReferenceNotChangedWhenReady(oldObj, newObj interface{}, oldRef, newRef ObjectReferencer, fieldName string) error {
	// Check if any reference fields have changed
	if oldRef.GetName() != newRef.GetName() ||
		oldRef.GetKind() != newRef.GetKind() ||
		oldRef.GetAPIVersion() != newRef.GetAPIVersion() ||
		oldRef.GetNamespace() != newRef.GetNamespace() {

		// Check if the Ready condition is true in the old object
		if v.IsReadyConditionTrue(oldObj) {
			return fmt.Errorf("cannot change %s when Ready condition is true", fieldName)
		}
	}
	return nil
}
