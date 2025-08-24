package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// ServiceAliasValidator implements validation for ServiceAlias resources
type ServiceAliasValidator struct {
	helpers *ValidationHelpers
}

// Compile-time interface assertion
var _ base.Validator[*v1beta1.ServiceAlias] = &ServiceAliasValidator{}

// ValidateCreate validates a new ServiceAlias being created
func (v *ServiceAliasValidator) ValidateCreate(ctx context.Context, obj *v1beta1.ServiceAlias) field.ErrorList {
	return v.validate(ctx, obj)
}

// ValidateUpdate validates a ServiceAlias being updated
func (v *ServiceAliasValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.ServiceAlias, old *v1beta1.ServiceAlias) field.ErrorList {
	allErrs := v.validate(ctx, obj)

	// Additional update-specific validation
	if old != nil {
		// Check if immutable fields are changed
		if obj.Spec.ServiceRef != old.Spec.ServiceRef {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "serviceRef"), obj.Spec.ServiceRef,
				"serviceRef is immutable and cannot be changed"))
		}
	}

	return allErrs
}

// ValidateDelete validates a ServiceAlias being deleted
func (v *ServiceAliasValidator) ValidateDelete(ctx context.Context, obj *v1beta1.ServiceAlias) field.ErrorList {
	// For delete operations, we might want to check:
	// - If ServiceAlias is referenced by RuleS2S
	// For now, deletion is always allowed
	return field.ErrorList{}
}

// validate performs comprehensive validation of a ServiceAlias object
func (v *ServiceAliasValidator) validate(ctx context.Context, obj *v1beta1.ServiceAlias) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "servicealias object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// validateMetadata validates the metadata fields using standard validation
func (v *ServiceAliasValidator) validateMetadata(obj *v1beta1.ServiceAlias) field.ErrorList {
	metaPath := field.NewPath("metadata")
	return ValidateStandardObjectMeta(&obj.ObjectMeta, metaPath)
}

// validateSpec validates the ServiceAlias spec
func (v *ServiceAliasValidator) validateSpec(spec v1beta1.ServiceAliasSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate ServiceRef (required)
	allErrs = append(allErrs, v.validateServiceRef(spec.ServiceRef, fldPath.Child("serviceRef"))...)

	return allErrs
}

// validateServiceRef validates the Service object reference using standard validation
func (v *ServiceAliasValidator) validateServiceRef(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
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

// NewServiceAliasValidator creates a new ServiceAliasValidator instance
func NewServiceAliasValidator() *ServiceAliasValidator {
	return &ServiceAliasValidator{
		helpers: NewValidationHelpers(),
	}
}
