package validation

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
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

func (v *BaseValidator) CheckEntityExists(ctx context.Context, id models.ResourceIdentifier, keyExtractor func(interface{}) string) (bool, interface{}, error) {
	var foundEntity interface{}
	exists := false

	err := v.listFunction(ctx, func(entity interface{}) error {
		if keyExtractor(entity) == id.Key() {
			exists = true
			foundEntity = entity
		}
		return nil
	}, ports.NewResourceIdentifierScope(id))

	if err != nil {
		return false, nil, errors.Wrap(err, fmt.Sprintf("failed to check %s existence", v.entityType))
	}

	return exists, foundEntity, nil
}

func (v *BaseValidator) ValidateEntityDoesNotExistForCreation(ctx context.Context, id models.ResourceIdentifier, keyExtractor func(interface{}) string) error {
	exists, foundEntity, err := v.CheckEntityExists(ctx, id, keyExtractor)
	if err != nil {
		return err
	}

	if exists {
		conflictDetails := fmt.Sprintf("Entity was found in namespace '%s' with name '%s'", id.Namespace, id.Name)
		suggestedAction := fmt.Sprintf("Use a different name or update the existing %s instead", v.entityType)
		return NewEntityAlreadyExistsError(v.entityType, id.Key(), foundEntity, conflictDetails, suggestedAction)
	}

	return nil
}

type MetaProvider interface {
	GetMeta() *models.Meta
}

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

type ObjectReferenceComparison struct {
	OldRef    ObjectReferencer
	NewRef    ObjectReferencer
	FieldName string
}

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
