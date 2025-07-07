package service

import (
	"context"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// serviceStrategy implements verification logic for Service
type serviceStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

// NewStrategy creates a new strategy for Service objects
func NewStrategy(scheme *runtime.Scheme) *serviceStrategy {
	return &serviceStrategy{scheme, names.SimpleNameGenerator}
}

// NamespaceScoped returns true because all Services are namespaced.
func (serviceStrategy) NamespaceScoped() bool {
	return true
}

// PrepareForCreate clears fields that are not allowed to be set by end users on creation.
func (serviceStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	service := obj.(*netguardv1beta1.Service)
	// Clear status on create
	service.Status = netguardv1beta1.ServiceStatus{}
}

// PrepareForUpdate clears fields that are not allowed to be set by end users on update.
func (serviceStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	newService := obj.(*netguardv1beta1.Service)
	oldService := old.(*netguardv1beta1.Service)
	// Preserve status
	newService.Status = oldService.Status
}

// Validate validates a new Service.
func (serviceStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

// WarningsOnCreate returns warnings for the creation of the given object.
func (serviceStrategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	return nil
}

// Canonicalize normalizes the object after validation.
func (serviceStrategy) Canonicalize(obj runtime.Object) {
}

// ValidateUpdate is the default update validation for an end user.
func (serviceStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

// WarningsOnUpdate returns warnings for the given update.
func (serviceStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return nil
}

// AllowCreateOnUpdate is false for Services; this means POST is needed to create one.
func (serviceStrategy) AllowCreateOnUpdate() bool {
	return false
}

// AllowUnconditionalUpdate is the default update policy for Service objects.
func (serviceStrategy) AllowUnconditionalUpdate() bool {
	return false
}

// GetAttrs returns labels and fields of a given object for filtering purposes.
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	service := obj.(*netguardv1beta1.Service)
	return labels.Set(service.ObjectMeta.Labels), ToSelectableFields(service), nil
}

// MatchService returns a generic matcher for a given label and field selector.
func MatchService(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: GetAttrs,
	}
}

// ToSelectableFields returns a field set that represents the object
func ToSelectableFields(service *netguardv1beta1.Service) fields.Set {
	return generic.ObjectMetaFieldsSet(&service.ObjectMeta, true)
}
