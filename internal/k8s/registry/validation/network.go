package validation

import (
	"context"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// NetworkValidator validates Network resources
type NetworkValidator struct {
	helpers *ValidationHelpers
}

// NewNetworkValidator creates a new NetworkValidator
func NewNetworkValidator() *NetworkValidator {
	return &NetworkValidator{
		helpers: NewValidationHelpers(),
	}
}

// ValidateCreate validates a new Network being created
func (v *NetworkValidator) ValidateCreate(ctx context.Context, obj *v1beta1.Network) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "network object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// ValidateUpdate validates a Network being updated
func (v *NetworkValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.Network, oldObj *v1beta1.Network) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "network object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	// Check if Spec fields are being changed when Ready condition is true
	if isReadyConditionTrue(oldObj) {
		if obj.Spec.CIDR != oldObj.Spec.CIDR {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec.cidr"),
				"cannot change CIDR when Ready condition is true"))
		}
	}

	return allErrs
}

// ValidateDelete validates a Network being deleted
func (v *NetworkValidator) ValidateDelete(ctx context.Context, obj *v1beta1.Network) field.ErrorList {
	// No specific validation for deletion
	return field.ErrorList{}
}

// validateMetadata validates the metadata fields using standard validation
func (v *NetworkValidator) validateMetadata(obj *v1beta1.Network) field.ErrorList {
	metaPath := field.NewPath("metadata")
	return ValidateStandardObjectMeta(&obj.ObjectMeta, metaPath)
}

// validateSpec validates the Network spec
func (v *NetworkValidator) validateSpec(spec v1beta1.NetworkSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate CIDR format using standard validation
	allErrs = append(allErrs, ValidateCIDR(spec.CIDR, fldPath.Child("cidr"))...)

	return allErrs
}

// isReadyConditionTrue checks if the Ready condition is true
func isReadyConditionTrue(obj *v1beta1.Network) bool {
	for _, condition := range obj.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			return true
		}
	}
	return false
}
