package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// ServiceAliasValidator implements validation for ServiceAlias resources
type ServiceAliasValidator struct{}

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

// validateMetadata validates the metadata fields
func (v *ServiceAliasValidator) validateMetadata(obj *v1beta1.ServiceAlias) field.ErrorList {
	allErrs := field.ErrorList{}
	metaPath := field.NewPath("metadata")

	if obj.Name == "" {
		allErrs = append(allErrs, field.Required(metaPath.Child("name"), "name is required"))
	}

	if obj.Name != "" && !isDNS1123Subdomain(obj.Name) {
		allErrs = append(allErrs, field.Invalid(metaPath.Child("name"), obj.Name,
			"name must be a valid DNS-1123 subdomain"))
	}

	if obj.Namespace != "" && !isDNS1123Subdomain(obj.Namespace) {
		allErrs = append(allErrs, field.Invalid(metaPath.Child("namespace"), obj.Namespace,
			"namespace must be a valid DNS-1123 subdomain"))
	}

	return allErrs
}

// validateSpec validates the ServiceAlias spec
func (v *ServiceAliasValidator) validateSpec(spec v1beta1.ServiceAliasSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate ServiceRef (required)
	allErrs = append(allErrs, v.validateServiceRef(spec.ServiceRef, fldPath.Child("serviceRef"))...)

	return allErrs
}

// validateServiceRef validates the Service object reference
func (v *ServiceAliasValidator) validateServiceRef(ref v1beta1.ObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate APIVersion
	if ref.APIVersion == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("apiVersion"), "apiVersion is required"))
	} else if ref.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), ref.APIVersion,
			"apiVersion must be 'netguard.sgroups.io/v1beta1'"))
	}

	// Validate Kind
	if ref.Kind == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("kind"), "kind is required"))
	} else if ref.Kind != "Service" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'Service'"))
	}

	// Validate Name
	if ref.Name == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name is required"))
	} else if !isDNS1123Subdomain(ref.Name) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("name"), ref.Name,
			"name must be a valid DNS-1123 subdomain"))
	}

	return allErrs
}

// NewServiceAliasValidator creates a new ServiceAliasValidator instance
func NewServiceAliasValidator() *ServiceAliasValidator {
	return &ServiceAliasValidator{}
}
