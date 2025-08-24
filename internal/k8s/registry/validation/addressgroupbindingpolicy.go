package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// AddressGroupBindingPolicyValidator implements validation for AddressGroupBindingPolicy resources
type AddressGroupBindingPolicyValidator struct {
	helpers *ValidationHelpers
}

// Compile-time interface assertion
var _ base.Validator[*v1beta1.AddressGroupBindingPolicy] = &AddressGroupBindingPolicyValidator{}

// ValidateCreate validates a new AddressGroupBindingPolicy being created
func (v *AddressGroupBindingPolicyValidator) ValidateCreate(ctx context.Context, obj *v1beta1.AddressGroupBindingPolicy) field.ErrorList {
	return v.validate(ctx, obj)
}

// ValidateUpdate validates an AddressGroupBindingPolicy being updated
func (v *AddressGroupBindingPolicyValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.AddressGroupBindingPolicy, old *v1beta1.AddressGroupBindingPolicy) field.ErrorList {
	allErrs := v.validate(ctx, obj)

	// Additional update-specific validation
	if old != nil {
		// Check if immutable fields are changed
		if obj.Spec.AddressGroupRef != old.Spec.AddressGroupRef {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "addressGroupRef"), obj.Spec.AddressGroupRef,
				"addressGroupRef is immutable and cannot be changed"))
		}
		if obj.Spec.ServiceRef != old.Spec.ServiceRef {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "serviceRef"), obj.Spec.ServiceRef,
				"serviceRef is immutable and cannot be changed"))
		}
	}

	return allErrs
}

// ValidateDelete validates an AddressGroupBindingPolicy being deleted
func (v *AddressGroupBindingPolicyValidator) ValidateDelete(ctx context.Context, obj *v1beta1.AddressGroupBindingPolicy) field.ErrorList {
	return field.ErrorList{}
}

// validate performs comprehensive validation of an AddressGroupBindingPolicy object
func (v *AddressGroupBindingPolicyValidator) validate(ctx context.Context, obj *v1beta1.AddressGroupBindingPolicy) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "addressgroupbindingpolicy object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// validateMetadata validates the metadata fields using standard validation
func (v *AddressGroupBindingPolicyValidator) validateMetadata(obj *v1beta1.AddressGroupBindingPolicy) field.ErrorList {
	metaPath := field.NewPath("metadata")
	return ValidateStandardObjectMeta(&obj.ObjectMeta, metaPath)
}

// validateSpec validates the AddressGroupBindingPolicy spec
func (v *AddressGroupBindingPolicyValidator) validateSpec(spec v1beta1.AddressGroupBindingPolicySpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate AddressGroupRef (required)
	allErrs = append(allErrs, v.validateAddressGroupRef(spec.AddressGroupRef, fldPath.Child("addressGroupRef"))...)

	// Validate ServiceRef (required)
	allErrs = append(allErrs, v.validateServiceRef(spec.ServiceRef, fldPath.Child("serviceRef"))...)

	return allErrs
}

// validateAddressGroupRef validates the AddressGroup reference using standard validation
func (v *AddressGroupBindingPolicyValidator) validateAddressGroupRef(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Use standard NamespacedObjectReference validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&ref, fldPath)...)

	// Domain-specific validation: APIVersion must be netguard.sgroups.io/v1beta1
	if ref.APIVersion != "" && ref.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), ref.APIVersion,
			"apiVersion must be 'netguard.sgroups.io/v1beta1'"))
	}

	// Domain-specific validation: Kind must be AddressGroup
	if ref.Kind != "" && ref.Kind != "AddressGroup" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'AddressGroup'"))
	}

	return allErrs
}

// validateServiceRef validates the Service reference using standard validation
func (v *AddressGroupBindingPolicyValidator) validateServiceRef(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Use standard NamespacedObjectReference validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&ref, fldPath)...)

	// Domain-specific validation: APIVersion must be netguard.sgroups.io/v1beta1
	if ref.APIVersion != "" && ref.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), ref.APIVersion,
			"apiVersion must be 'netguard.sgroups.io/v1beta1'"))
	}

	// Domain-specific validation: Kind must be Service
	if ref.Kind != "" && ref.Kind != "Service" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'Service'"))
	}

	return allErrs
}

// NewAddressGroupBindingPolicyValidator creates a new AddressGroupBindingPolicyValidator instance
func NewAddressGroupBindingPolicyValidator() *AddressGroupBindingPolicyValidator {
	return &AddressGroupBindingPolicyValidator{
		helpers: NewValidationHelpers(),
	}
}
