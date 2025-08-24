package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// RuleS2SValidator implements validation for RuleS2S resources
type RuleS2SValidator struct {
	helpers *ValidationHelpers
}

// Compile-time interface assertion
var _ base.Validator[*v1beta1.RuleS2S] = &RuleS2SValidator{}

// ValidateCreate validates a new RuleS2S being created
func (v *RuleS2SValidator) ValidateCreate(ctx context.Context, obj *v1beta1.RuleS2S) field.ErrorList {
	return v.validate(ctx, obj)
}

// ValidateUpdate validates a RuleS2S being updated
func (v *RuleS2SValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.RuleS2S, old *v1beta1.RuleS2S) field.ErrorList {
	allErrs := v.validate(ctx, obj)

	// Additional update-specific validation
	if old != nil {
		// Check if immutable fields are changed
		if obj.Spec.Traffic != old.Spec.Traffic {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "traffic"), obj.Spec.Traffic,
				"traffic is immutable and cannot be changed"))
		}
		if obj.Spec.ServiceLocalRef != old.Spec.ServiceLocalRef {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "serviceLocalRef"), obj.Spec.ServiceLocalRef,
				"serviceLocalRef is immutable and cannot be changed"))
		}
		if obj.Spec.ServiceRef != old.Spec.ServiceRef {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "serviceRef"), obj.Spec.ServiceRef,
				"serviceRef is immutable and cannot be changed"))
		}
	}

	return allErrs
}

// ValidateDelete validates a RuleS2S being deleted
func (v *RuleS2SValidator) ValidateDelete(ctx context.Context, obj *v1beta1.RuleS2S) field.ErrorList {
	return field.ErrorList{}
}

// validate performs comprehensive validation of a RuleS2S object
func (v *RuleS2SValidator) validate(ctx context.Context, obj *v1beta1.RuleS2S) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "rules2s object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// validateMetadata validates the metadata fields using standard validation
func (v *RuleS2SValidator) validateMetadata(obj *v1beta1.RuleS2S) field.ErrorList {
	metaPath := field.NewPath("metadata")
	return ValidateStandardObjectMeta(&obj.ObjectMeta, metaPath)
}

// validateSpec validates the RuleS2S spec using standard validation
func (v *RuleS2SValidator) validateSpec(spec v1beta1.RuleS2SSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate Traffic using standard validation
	allErrs = append(allErrs, v.validateTrafficRequired(spec.Traffic, fldPath.Child("traffic"))...)

	// Validate ServiceLocalRef using standard validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&spec.ServiceLocalRef, fldPath.Child("serviceLocalRef"))...)
	allErrs = append(allErrs, v.validateServiceLocalRefDomain(spec.ServiceLocalRef, fldPath.Child("serviceLocalRef"))...)

	// Validate ServiceRef using standard validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&spec.ServiceRef, fldPath.Child("serviceRef"))...)
	allErrs = append(allErrs, v.validateServiceRefDomain(spec.ServiceRef, fldPath.Child("serviceRef"))...)

	return allErrs
}

// validateTrafficRequired validates the traffic direction enum with required check
func (v *RuleS2SValidator) validateTrafficRequired(traffic v1beta1.Traffic, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if traffic == "" {
		allErrs = append(allErrs, field.Required(fldPath, "traffic is required"))
		return allErrs
	}

	// Use standard Traffic validation
	allErrs = append(allErrs, ValidateTraffic(traffic, fldPath)...)
	return allErrs
}

// validateServiceLocalRefDomain validates domain-specific rules for ServiceLocalRef
func (v *RuleS2SValidator) validateServiceLocalRefDomain(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Domain-specific validation: Kind must be ServiceAlias
	if ref.Kind != "ServiceAlias" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'ServiceAlias'"))
	}

	return allErrs
}

// validateServiceRefDomain validates domain-specific rules for ServiceRef
func (v *RuleS2SValidator) validateServiceRefDomain(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Domain-specific validation: Kind must be ServiceAlias
	if ref.Kind != "ServiceAlias" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'ServiceAlias'"))
	}

	return allErrs
}

// Note: NamespacedObjectReference validation is now handled by standard ValidateNamespacedObjectReference

// NewRuleS2SValidator creates a new RuleS2SValidator instance
func NewRuleS2SValidator() *RuleS2SValidator {
	return &RuleS2SValidator{
		helpers: NewValidationHelpers(),
	}
}
