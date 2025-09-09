package validation

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// ServiceValidator implements validation for Service resources
type ServiceValidator struct {
	helpers *ValidationHelpers
}

// Compile-time interface assertion
var _ base.Validator[*v1beta1.Service] = &ServiceValidator{}

// ValidateCreate validates a new Service being created
func (v *ServiceValidator) ValidateCreate(ctx context.Context, obj *v1beta1.Service) field.ErrorList {
	return v.validate(ctx, obj)
}

// ValidateUpdate validates a Service being updated
func (v *ServiceValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.Service, old *v1beta1.Service) field.ErrorList {
	allErrs := v.validate(ctx, obj)

	// Additional update-specific validation
	// For example, check if immutable fields are changed
	if old != nil {
		// In the future, if there are immutable fields, validate them here
		// For now, all Service fields are mutable
	}

	return allErrs
}

// ValidateDelete validates a Service being deleted
func (v *ServiceValidator) ValidateDelete(ctx context.Context, obj *v1beta1.Service) field.ErrorList {
	// For delete operations, we can add checks like:
	// - Ensure no dependencies exist
	// - Check for finalizers
	// For now, deletion is always allowed
	return field.ErrorList{}
}

// validate performs comprehensive validation of a Service object (internal method)
func (v *ServiceValidator) validate(ctx context.Context, obj *v1beta1.Service) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "service object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// validateMetadata validates the metadata fields using standard validation
func (v *ServiceValidator) validateMetadata(obj *v1beta1.Service) field.ErrorList {
	metaPath := field.NewPath("metadata")
	return ValidateStandardObjectMeta(&obj.ObjectMeta, metaPath)
}

// validateSpec validates the Service spec using standard validation
func (v *ServiceValidator) validateSpec(spec v1beta1.ServiceSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate description length using standard helper
	allErrs = append(allErrs, ValidateDescription(spec.Description, fldPath.Child("description"), 512)...)

	// Validate ingress ports using standard validation
	allErrs = append(allErrs, ValidateIngressPorts(spec.IngressPorts, fldPath.Child("ingressPorts"))...)

	// Add duplicate checking (domain-specific business logic)
	allErrs = append(allErrs, v.validateIngressPortsDuplicates(spec.IngressPorts, fldPath.Child("ingressPorts"))...)

	return allErrs
}

// validateIngressPortsDuplicates validates for duplicate port configurations
func (v *ServiceValidator) validateIngressPortsDuplicates(ports []v1beta1.IngressPort, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(ports) == 0 {
		return allErrs
	}

	// Track unique port+protocol combinations to prevent duplicates
	seen := make(map[string]bool)

	for i, port := range ports {
		portPath := fldPath.Index(i)

		// Check for duplicates
		key := fmt.Sprintf("%s-%s", port.Protocol, port.Port)
		if seen[key] {
			allErrs = append(allErrs, field.Duplicate(portPath,
				fmt.Sprintf("duplicate port configuration: protocol=%s port=%s", port.Protocol, port.Port)))
		}
		seen[key] = true
	}

	return allErrs
}

// NewServiceValidator creates a new ServiceValidator instance
func NewServiceValidator() *ServiceValidator {
	return &ServiceValidator{
		helpers: NewValidationHelpers(),
	}
}
