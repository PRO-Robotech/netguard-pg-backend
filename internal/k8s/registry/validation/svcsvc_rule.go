package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// SvcSvcRuleValidator implements validation for SvcSvcRule resources
type SvcSvcRuleValidator struct {
	helpers *ValidationHelpers
}

// Compile-time interface assertion
var _ base.Validator[*v1beta1.SvcSvcRule] = &SvcSvcRuleValidator{}

// ValidateCreate validates a new SvcSvcRule being created
func (v *SvcSvcRuleValidator) ValidateCreate(ctx context.Context, obj *v1beta1.SvcSvcRule) field.ErrorList {
	return v.validate(ctx, obj)
}

// ValidateUpdate validates a SvcSvcRule being updated
func (v *SvcSvcRuleValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.SvcSvcRule, old *v1beta1.SvcSvcRule) field.ErrorList {
	allErrs := v.validate(ctx, obj)

	// Additional update-specific validation
	if old != nil {
		// Check if immutable fields are changed
		if obj.Spec.ServiceFrom != old.Spec.ServiceFrom {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "serviceFrom"), obj.Spec.ServiceFrom,
				"serviceFrom is immutable and cannot be changed"))
		}
		if obj.Spec.ServiceTo != old.Spec.ServiceTo {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "serviceTo"), obj.Spec.ServiceTo,
				"serviceTo is immutable and cannot be changed"))
		}
	}

	return allErrs
}

// ValidateDelete validates a SvcSvcRule being deleted
func (v *SvcSvcRuleValidator) ValidateDelete(ctx context.Context, obj *v1beta1.SvcSvcRule) field.ErrorList {
	return field.ErrorList{}
}

// validate performs comprehensive validation of a SvcSvcRule object
func (v *SvcSvcRuleValidator) validate(ctx context.Context, obj *v1beta1.SvcSvcRule) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "svcsvc_rule object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// validateMetadata validates the metadata fields using standard validation
func (v *SvcSvcRuleValidator) validateMetadata(obj *v1beta1.SvcSvcRule) field.ErrorList {
	metaPath := field.NewPath("metadata")
	return ValidateStandardObjectMeta(&obj.ObjectMeta, metaPath)
}

// validateSpec validates the SvcSvcRule spec using standard validation
func (v *SvcSvcRuleValidator) validateSpec(spec v1beta1.SvcSvcRuleSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate ServiceFrom using standard validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&spec.ServiceFrom, fldPath.Child("serviceFrom"))...)
	allErrs = append(allErrs, v.validateServiceFromDomain(spec.ServiceFrom, fldPath.Child("serviceFrom"))...)

	// Validate ServiceTo using standard validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&spec.ServiceTo, fldPath.Child("serviceTo"))...)
	allErrs = append(allErrs, v.validateServiceToDomain(spec.ServiceTo, fldPath.Child("serviceTo"))...)

	// Validate Action using standard validation
	allErrs = append(allErrs, v.validateActionRequired(spec.Action, fldPath.Child("action"))...)

	// Validate Priority
	allErrs = append(allErrs, v.validatePriority(spec.Priority, fldPath.Child("priority"))...)

	return allErrs
}

// validateServiceFromDomain validates domain-specific rules for ServiceFrom
func (v *SvcSvcRuleValidator) validateServiceFromDomain(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if ref.Kind != "Service" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'Service'"))
	}

	return allErrs
}

// validateServiceToDomain validates domain-specific rules for ServiceTo
func (v *SvcSvcRuleValidator) validateServiceToDomain(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if ref.Kind != "Service" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'Service'"))
	}

	return allErrs
}

// validateActionRequired validates the action enum with required check
func (v *SvcSvcRuleValidator) validateActionRequired(action v1beta1.RuleAction, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if action == "" {
		allErrs = append(allErrs, field.Required(fldPath, "action is required"))
		return allErrs
	}

	// Use standard RuleAction validation
	allErrs = append(allErrs, ValidateRuleAction(action, fldPath)...)
	return allErrs
}

// validatePriority validates the priority field (0-1000 range)
func (v *SvcSvcRuleValidator) validatePriority(priority int32, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if priority < 0 || priority > 1000 {
		allErrs = append(allErrs, field.Invalid(fldPath, priority,
			"priority must be between 0 and 1000"))
	}

	return allErrs
}

// NewSvcSvcRuleValidator creates a new SvcSvcRuleValidator instance
func NewSvcSvcRuleValidator() *SvcSvcRuleValidator {
	return &SvcSvcRuleValidator{
		helpers: NewValidationHelpers(),
	}
}
