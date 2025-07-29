package validation

import (
	"context"
	"net"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// NetworkValidator validates Network resources
type NetworkValidator struct{}

// NewNetworkValidator creates a new NetworkValidator
func NewNetworkValidator() *NetworkValidator {
	return &NetworkValidator{}
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

// validateMetadata validates the metadata fields
func (v *NetworkValidator) validateMetadata(obj *v1beta1.Network) field.ErrorList {
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

// validateSpec validates the Network spec
func (v *NetworkValidator) validateSpec(spec v1beta1.NetworkSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate CIDR format
	if spec.CIDR == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("cidr"), "CIDR is required"))
	} else {
		if _, _, err := net.ParseCIDR(spec.CIDR); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("cidr"), spec.CIDR,
				"invalid CIDR format"))
		}
	}

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
