package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// AddressGroupPortMappingValidator implements validation for AddressGroupPortMapping resources
type AddressGroupPortMappingValidator struct {
	helpers *ValidationHelpers
}

// Compile-time interface assertion
var _ base.Validator[*v1beta1.AddressGroupPortMapping] = &AddressGroupPortMappingValidator{}

// ValidateCreate validates a new AddressGroupPortMapping being created
func (v *AddressGroupPortMappingValidator) ValidateCreate(ctx context.Context, obj *v1beta1.AddressGroupPortMapping) field.ErrorList {
	return v.validate(ctx, obj)
}

// ValidateUpdate validates an AddressGroupPortMapping being updated
func (v *AddressGroupPortMappingValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.AddressGroupPortMapping, old *v1beta1.AddressGroupPortMapping) field.ErrorList {
	return v.validate(ctx, obj)
}

// ValidateDelete validates an AddressGroupPortMapping being deleted
func (v *AddressGroupPortMappingValidator) ValidateDelete(ctx context.Context, obj *v1beta1.AddressGroupPortMapping) field.ErrorList {
	return field.ErrorList{}
}

// validate performs comprehensive validation of an AddressGroupPortMapping object
func (v *AddressGroupPortMappingValidator) validate(ctx context.Context, obj *v1beta1.AddressGroupPortMapping) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "addressgroupportmapping object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec (empty for now)
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	// Validate AccessPorts
	allErrs = append(allErrs, v.validateAccessPorts(obj.AccessPorts, field.NewPath("accessPorts"))...)

	return allErrs
}

// validateMetadata validates the metadata fields using standard validation
func (v *AddressGroupPortMappingValidator) validateMetadata(obj *v1beta1.AddressGroupPortMapping) field.ErrorList {
	metaPath := field.NewPath("metadata")
	return ValidateStandardObjectMeta(&obj.ObjectMeta, metaPath)
}

// validateSpec validates the AddressGroupPortMapping spec (empty spec)
func (v *AddressGroupPortMappingValidator) validateSpec(spec v1beta1.AddressGroupPortMappingSpec, fldPath *field.Path) field.ErrorList {
	// Spec is empty for now, nothing to validate
	return field.ErrorList{}
}

// validateAccessPorts validates the AccessPorts specification
func (v *AddressGroupPortMappingValidator) validateAccessPorts(accessPorts v1beta1.AccessPortsSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate each service ports reference
	for i, servicePortsRef := range accessPorts.Items {
		itemPath := fldPath.Child("items").Index(i)
		allErrs = append(allErrs, v.validateServicePortsRef(servicePortsRef, itemPath)...)
	}

	return allErrs
}

// validateServicePortsRef validates a ServicePortsRef using standard validation
func (v *AddressGroupPortMappingValidator) validateServicePortsRef(ref v1beta1.ServicePortsRef, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate NamespacedObjectReference using standard validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&ref.NamespacedObjectReference, fldPath)...)
	// Domain-specific validation for Service references
	allErrs = append(allErrs, v.validateServiceRefDomain(ref.NamespacedObjectReference, fldPath)...)

	// Validate Ports using custom validation (complex structure)
	allErrs = append(allErrs, v.validateProtocolPorts(ref.Ports, fldPath.Child("ports"))...)

	return allErrs
}

// validateServiceRefDomain validates domain-specific rules for Service references
func (v *AddressGroupPortMappingValidator) validateServiceRefDomain(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Domain-specific validation: APIVersion must be netguard.sgroups.io/v1beta1
	if ref.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), ref.APIVersion,
			"apiVersion must be 'netguard.sgroups.io/v1beta1'"))
	}

	// Domain-specific validation: Kind must be Service
	if ref.Kind != "Service" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'Service'"))
	}

	return allErrs
}

// validateProtocolPorts validates protocol ports configuration
func (v *AddressGroupPortMappingValidator) validateProtocolPorts(ports v1beta1.ProtocolPorts, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate TCP ports
	for i, portConfig := range ports.TCP {
		portPath := fldPath.Child("TCP").Index(i)
		allErrs = append(allErrs, v.validatePortConfig(portConfig, portPath)...)
	}

	// Validate UDP ports
	for i, portConfig := range ports.UDP {
		portPath := fldPath.Child("UDP").Index(i)
		allErrs = append(allErrs, v.validatePortConfig(portConfig, portPath)...)
	}

	return allErrs
}

// validatePortConfig validates a port configuration using standard validation
func (v *AddressGroupPortMappingValidator) validatePortConfig(config v1beta1.PortConfig, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate port string is required
	allErrs = append(allErrs, v.helpers.ValidateRequiredString(config.Port, fldPath.Child("port"), "port")...)

	// Validate port string format (basic validation - detailed parsing handled elsewhere)
	if config.Port != "" && len(config.Port) > 32 {
		allErrs = append(allErrs, field.TooLong(fldPath.Child("port"), config.Port, 32))
	}

	// Validate description length using standard validation
	allErrs = append(allErrs, ValidateDescription(config.Description, fldPath.Child("description"), 256)...)

	return allErrs
}

// Note: Port string validation is now handled by standard validation functions and simplified port config validation

// Note: Single port validation is now handled by standard validation functions

// Note: Port range validation is now handled by standard validation functions

// NewAddressGroupPortMappingValidator creates a new AddressGroupPortMappingValidator instance
func NewAddressGroupPortMappingValidator() *AddressGroupPortMappingValidator {
	return &AddressGroupPortMappingValidator{
		helpers: NewValidationHelpers(),
	}
}
