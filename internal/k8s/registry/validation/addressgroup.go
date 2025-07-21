package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// AddressGroupValidator implements validation for AddressGroup resources
type AddressGroupValidator struct{}

// Compile-time interface assertion
var _ base.Validator[*v1beta1.AddressGroup] = &AddressGroupValidator{}

// ValidateCreate validates a new AddressGroup being created
func (v *AddressGroupValidator) ValidateCreate(ctx context.Context, obj *v1beta1.AddressGroup) field.ErrorList {
	return v.validate(ctx, obj)
}

// ValidateUpdate validates an AddressGroup being updated
func (v *AddressGroupValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.AddressGroup, old *v1beta1.AddressGroup) field.ErrorList {
	allErrs := v.validate(ctx, obj)

	// Additional update-specific validation
	if old != nil {
		// All AddressGroup fields are mutable for now
	}

	return allErrs
}

// ValidateDelete validates an AddressGroup being deleted
func (v *AddressGroupValidator) ValidateDelete(ctx context.Context, obj *v1beta1.AddressGroup) field.ErrorList {
	// For delete operations, we might want to check:
	// - If AddressGroup is referenced by AddressGroupBinding
	// - If AddressGroup is referenced by IEAgAgRule
	// For now, deletion is always allowed
	return field.ErrorList{}
}

// validate performs comprehensive validation of an AddressGroup object (internal method)
func (v *AddressGroupValidator) validate(ctx context.Context, obj *v1beta1.AddressGroup) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "addressgroup object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// validateMetadata validates the metadata fields
func (v *AddressGroupValidator) validateMetadata(obj *v1beta1.AddressGroup) field.ErrorList {
	allErrs := field.ErrorList{}
	metaPath := field.NewPath("metadata")

	// Validate name is not empty
	if obj.Name == "" {
		allErrs = append(allErrs, field.Required(metaPath.Child("name"), "name is required"))
	}

	// Validate name format (Kubernetes DNS-1123 subdomain)
	if obj.Name != "" && !isDNS1123Subdomain(obj.Name) {
		allErrs = append(allErrs, field.Invalid(metaPath.Child("name"), obj.Name,
			"name must be a valid DNS-1123 subdomain"))
	}

	// Validate namespace format if present
	if obj.Namespace != "" && !isDNS1123Subdomain(obj.Namespace) {
		allErrs = append(allErrs, field.Invalid(metaPath.Child("namespace"), obj.Namespace,
			"namespace must be a valid DNS-1123 subdomain"))
	}

	return allErrs
}

// validateSpec validates the AddressGroup spec
func (v *AddressGroupValidator) validateSpec(spec v1beta1.AddressGroupSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate DefaultAction (required)
	allErrs = append(allErrs, v.validateDefaultAction(spec.DefaultAction, fldPath.Child("defaultAction"))...)

	// Logs and Trace are bool fields, no additional validation needed

	return allErrs
}

// validateDefaultAction validates the DefaultAction field
func (v *AddressGroupValidator) validateDefaultAction(action v1beta1.RuleAction, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Check if DefaultAction is empty (required field)
	if action == "" {
		allErrs = append(allErrs, field.Required(fldPath, "defaultAction is required"))
		return allErrs
	}

	// Validate enum values
	switch action {
	case v1beta1.ActionAccept, v1beta1.ActionDrop:
		// Valid values
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath, action,
			[]string{string(v1beta1.ActionAccept), string(v1beta1.ActionDrop)}))
	}

	return allErrs
}

// Note: Address validation functions removed as AddressGroup no longer contains addresses

// Note: isDNS1123Subdomain function is defined in service.go and shared across validators

// NewAddressGroupValidator creates a new AddressGroupValidator instance
func NewAddressGroupValidator() *AddressGroupValidator {
	return &AddressGroupValidator{}
}
