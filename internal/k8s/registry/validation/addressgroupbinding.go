package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// AddressGroupBindingValidator implements validation for AddressGroupBinding resources
type AddressGroupBindingValidator struct {
	helpers *ValidationHelpers
}

// Compile-time interface assertion
var _ base.Validator[*v1beta1.AddressGroupBinding] = &AddressGroupBindingValidator{}

// ValidateCreate validates a new AddressGroupBinding being created
func (v *AddressGroupBindingValidator) ValidateCreate(ctx context.Context, obj *v1beta1.AddressGroupBinding) field.ErrorList {
	return v.validate(ctx, obj)
}

// ValidateUpdate validates an AddressGroupBinding being updated
func (v *AddressGroupBindingValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.AddressGroupBinding, old *v1beta1.AddressGroupBinding) field.ErrorList {
	allErrs := v.validate(ctx, obj)

	// Additional update-specific validation
	if old != nil {
		// Check if immutable fields are changed
		if obj.Spec.ServiceRef != old.Spec.ServiceRef {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "serviceRef"), obj.Spec.ServiceRef,
				"serviceRef is immutable and cannot be changed"))
		}
		if obj.Spec.AddressGroupRef != old.Spec.AddressGroupRef {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "addressGroupRef"), obj.Spec.AddressGroupRef,
				"addressGroupRef is immutable and cannot be changed"))
		}
	}

	return allErrs
}

// ValidateDelete validates an AddressGroupBinding being deleted
func (v *AddressGroupBindingValidator) ValidateDelete(ctx context.Context, obj *v1beta1.AddressGroupBinding) field.ErrorList {
	// For delete operations, we might want to check:
	// - If binding is referenced by policies
	// - If there are dependent resources
	return field.ErrorList{}
}

// validate performs comprehensive validation of an AddressGroupBinding object (internal method)
func (v *AddressGroupBindingValidator) validate(ctx context.Context, obj *v1beta1.AddressGroupBinding) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "addressgroupbinding object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// validateMetadata validates the metadata fields using standard validation
func (v *AddressGroupBindingValidator) validateMetadata(obj *v1beta1.AddressGroupBinding) field.ErrorList {
	metaPath := field.NewPath("metadata")
	return ValidateStandardObjectMeta(&obj.ObjectMeta, metaPath)
}

// validateSpec validates the AddressGroupBinding spec using standard validation
func (v *AddressGroupBindingValidator) validateSpec(spec v1beta1.AddressGroupBindingSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate ServiceRef using standard NamespacedObjectReference validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&spec.ServiceRef, fldPath.Child("serviceRef"))...)
	// Additional domain-specific validation for Service references
	allErrs = append(allErrs, v.validateServiceRefDomain(spec.ServiceRef, fldPath.Child("serviceRef"))...)

	// Validate AddressGroupRef using standard NamespacedObjectReference validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&spec.AddressGroupRef, fldPath.Child("addressGroupRef"))...)
	// Additional domain-specific validation for AddressGroup references
	allErrs = append(allErrs, v.validateAddressGroupRefDomain(spec.AddressGroupRef, fldPath.Child("addressGroupRef"))...)

	return allErrs
}

// validateServiceRefDomain validates domain-specific rules for Service references
func (v *AddressGroupBindingValidator) validateServiceRefDomain(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Domain-specific validation: APIVersion must be netguard.sgroups.io/v1beta1
	if ref.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), ref.APIVersion,
			"apiVersion must be 'netguard.sgroups.io/v1beta1'"))
	}

	// Domain-specific validation: Kind must be Service
	if ref.Kind != "Service" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'Service'"))
	}

	return allErrs
}

// validateAddressGroupRefDomain validates domain-specific rules for AddressGroup references
func (v *AddressGroupBindingValidator) validateAddressGroupRefDomain(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Domain-specific validation: APIVersion must be netguard.sgroups.io/v1beta1
	if ref.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), ref.APIVersion,
			"apiVersion must be 'netguard.sgroups.io/v1beta1'"))
	}

	// Domain-specific validation: Kind must be AddressGroup
	if ref.Kind != "AddressGroup" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'AddressGroup'"))
	}

	return allErrs
}

// Note: Generic object reference validation is now handled by standard ValidateObjectReference and ValidateNamespacedObjectReference

// NewAddressGroupBindingValidator creates a new AddressGroupBindingValidator instance
func NewAddressGroupBindingValidator() *AddressGroupBindingValidator {
	return &AddressGroupBindingValidator{
		helpers: NewValidationHelpers(),
	}
}
