package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// AddressGroupValidator implements validation for AddressGroup resources
type AddressGroupValidator struct {
	helpers *ValidationHelpers
}

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

// validateMetadata validates the metadata fields using standard validation
func (v *AddressGroupValidator) validateMetadata(obj *v1beta1.AddressGroup) field.ErrorList {
	metaPath := field.NewPath("metadata")
	return ValidateStandardObjectMeta(&obj.ObjectMeta, metaPath)
}

// validateSpec validates the AddressGroup spec using standard validation
func (v *AddressGroupValidator) validateSpec(spec v1beta1.AddressGroupSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate DefaultAction using standard validation
	allErrs = append(allErrs, v.validateDefaultAction(spec.DefaultAction, fldPath.Child("defaultAction"))...)

	// Logs and Trace are bool fields, no additional validation needed

	return allErrs
}

// validateDefaultAction validates the DefaultAction field using standard validation
func (v *AddressGroupValidator) validateDefaultAction(action v1beta1.RuleAction, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Check if DefaultAction is empty (required field)
	if action == "" {
		allErrs = append(allErrs, field.Required(fldPath, "defaultAction is required"))
		return allErrs
	}

	// Use standard RuleAction validation
	allErrs = append(allErrs, ValidateRuleAction(action, fldPath)...)

	return allErrs
}

// Note: Address validation functions removed as AddressGroup no longer contains addresses

// Note: isDNS1123Subdomain function is defined in service.go and shared across validators

// NewAddressGroupValidator creates a new AddressGroupValidator instance
func NewAddressGroupValidator() *AddressGroupValidator {
	return &AddressGroupValidator{
		helpers: NewValidationHelpers(),
	}
}
