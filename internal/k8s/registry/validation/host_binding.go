package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// HostBindingValidator validates HostBinding resources
type HostBindingValidator struct {
	helpers *ValidationHelpers
}

// NewHostBindingValidator creates a new HostBindingValidator
func NewHostBindingValidator() *HostBindingValidator {
	return &HostBindingValidator{
		helpers: NewValidationHelpers(),
	}
}

// ValidateCreate validates a new HostBinding being created
func (v *HostBindingValidator) ValidateCreate(ctx context.Context, obj *v1beta1.HostBinding) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "hostBinding object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// ValidateUpdate validates a HostBinding being updated
func (v *HostBindingValidator) ValidateUpdate(ctx context.Context, obj, oldObj *v1beta1.HostBinding) field.ErrorList {
	allErrs := v.ValidateCreate(ctx, obj)

	if obj == nil || oldObj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "hostBinding object cannot be nil for update"))
	}

	// Validate that immutable fields haven't changed
	allErrs = append(allErrs, v.validateImmutableFields(obj, oldObj)...)

	return allErrs
}

// ValidateStatusUpdate validates a HostBinding status being updated
func (v *HostBindingValidator) ValidateStatusUpdate(ctx context.Context, obj, oldObj *v1beta1.HostBinding) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "hostBinding object cannot be nil"))
	}

	// Validate status fields
	allErrs = append(allErrs, v.validateStatus(obj.Status, field.NewPath("status"))...)

	return allErrs
}

// validateMetadata validates the HostBinding metadata
func (v *HostBindingValidator) validateMetadata(obj *v1beta1.HostBinding) field.ErrorList {
	allErrs := field.ErrorList{}
	path := field.NewPath("metadata")

	// Standard metadata validation
	allErrs = append(allErrs, ValidateStandardObjectMeta(&obj.ObjectMeta, path)...)

	return allErrs
}

// validateSpec validates the HostBinding spec
func (v *HostBindingValidator) validateSpec(spec v1beta1.HostBindingSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate HostRef - required
	allErrs = append(allErrs, v.validateObjectReference(spec.HostRef, fldPath.Child("hostRef"), "Host")...)

	// Validate AddressGroupRef - required
	allErrs = append(allErrs, v.validateObjectReference(spec.AddressGroupRef, fldPath.Child("addressGroupRef"), "AddressGroup")...)

	// Validate that HostRef and AddressGroupRef are different (they're in same namespace by definition since ObjectReference doesn't have namespace)
	if spec.HostRef.Name == spec.AddressGroupRef.Name {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("addressGroupRef"), spec.AddressGroupRef, "addressGroupRef cannot reference the same resource as hostRef"))
	}

	return allErrs
}

// validateObjectReference validates a NamespacedObjectReference
func (v *HostBindingValidator) validateObjectReference(objRef v1beta1.NamespacedObjectReference, fldPath *field.Path, expectedKind string) field.ErrorList {
	allErrs := field.ErrorList{}

	// Name is required
	if objRef.Name == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name is required"))
	}

	// Namespace is required
	if objRef.Namespace == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("namespace"), "namespace is required"))
	}

	// Validate Kind if specified
	if objRef.Kind != "" && objRef.Kind != expectedKind {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), objRef.Kind, fmt.Sprintf("kind should be %s", expectedKind)))
	}

	// Validate APIVersion if specified
	if objRef.APIVersion != "" && objRef.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), objRef.APIVersion, "apiVersion should be netguard.sgroups.io/v1beta1"))
	}

	// Validate name format (basic DNS subdomain validation)
	if objRef.Name != "" {
		if len(objRef.Name) > 253 {
			allErrs = append(allErrs, field.TooLong(fldPath.Child("name"), objRef.Name, 253))
		}
		// Additional name format validation could be added here
	}

	return allErrs
}

// validateStatus validates the HostBinding status
func (v *HostBindingValidator) validateStatus(status v1beta1.HostBindingStatus, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate conditions using standard validation
	// Conditions are typically managed by the system and don't need validation during create/update operations

	// Validate ObservedGeneration (should be non-negative)
	if status.ObservedGeneration < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("observedGeneration"), status.ObservedGeneration, "observedGeneration cannot be negative"))
	}

	return allErrs
}

// validateImmutableFields validates that immutable fields haven't changed
func (v *HostBindingValidator) validateImmutableFields(obj, oldObj *v1beta1.HostBinding) field.ErrorList {
	allErrs := field.ErrorList{}
	fldPath := field.NewPath("spec")

	// HostRef is immutable
	if !v.objectReferencesEqual(obj.Spec.HostRef, oldObj.Spec.HostRef) {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("hostRef"), "hostRef is immutable"))
	}

	// AddressGroupRef is immutable
	if !v.objectReferencesEqual(obj.Spec.AddressGroupRef, oldObj.Spec.AddressGroupRef) {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("addressGroupRef"), "addressGroupRef is immutable"))
	}

	return allErrs
}

// objectReferencesEqual compares two NamespacedObjectReferences for equality
func (v *HostBindingValidator) objectReferencesEqual(ref1, ref2 v1beta1.NamespacedObjectReference) bool {
	return ref1.Name == ref2.Name &&
		ref1.Namespace == ref2.Namespace &&
		ref1.Kind == ref2.Kind &&
		ref1.APIVersion == ref2.APIVersion
}

// ValidateDelete validates a HostBinding being deleted
func (v *HostBindingValidator) ValidateDelete(ctx context.Context, obj *v1beta1.HostBinding) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "hostBinding object cannot be nil"))
	}

	return allErrs
}
