package validation

import (
	"context"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// AddressGroupPortMappingValidator implements validation for AddressGroupPortMapping resources
type AddressGroupPortMappingValidator struct{}

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

// validateMetadata validates the metadata fields
func (v *AddressGroupPortMappingValidator) validateMetadata(obj *v1beta1.AddressGroupPortMapping) field.ErrorList {
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

// validateServicePortsRef validates a ServicePortsRef
func (v *AddressGroupPortMappingValidator) validateServicePortsRef(ref v1beta1.ServicePortsRef, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate NamespacedObjectReference
	allErrs = append(allErrs, v.validateNamespacedObjectReference(ref.NamespacedObjectReference, fldPath, "Service")...)

	// Validate Ports
	allErrs = append(allErrs, v.validateProtocolPorts(ref.Ports, fldPath.Child("ports"))...)

	return allErrs
}

// validateNamespacedObjectReference validates a namespaced object reference
func (v *AddressGroupPortMappingValidator) validateNamespacedObjectReference(ref v1beta1.NamespacedObjectReference, fldPath *field.Path, expectedKind string) field.ErrorList {
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
	} else if ref.Kind != expectedKind {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be '"+expectedKind+"'"))
	}

	// Validate Name
	if ref.Name == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name is required"))
	} else if !isDNS1123Subdomain(ref.Name) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("name"), ref.Name,
			"name must be a valid DNS-1123 subdomain"))
	}

	// Validate Namespace (optional but if present should be valid)
	if ref.Namespace != "" && !isDNS1123Subdomain(ref.Namespace) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("namespace"), ref.Namespace,
			"namespace must be a valid DNS-1123 subdomain"))
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

// validatePortConfig validates a port configuration
func (v *AddressGroupPortMappingValidator) validatePortConfig(config v1beta1.PortConfig, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate port string
	if config.Port == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("port"), "port is required"))
		return allErrs
	}

	allErrs = append(allErrs, v.validatePortString(config.Port, fldPath.Child("port"))...)

	// Validate description length
	if len(config.Description) > 256 {
		allErrs = append(allErrs, field.TooLong(fldPath.Child("description"), config.Description, 256))
	}

	return allErrs
}

// validatePortString validates port string format and range (reused from service validator)
func (v *AddressGroupPortMappingValidator) validatePortString(portStr string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if portStr == "" {
		allErrs = append(allErrs, field.Required(fldPath, "port is required"))
		return allErrs
	}

	if strings.Contains(portStr, "-") {
		// Port range format: "8080-9090"
		allErrs = append(allErrs, v.validatePortRange(portStr, fldPath)...)
	} else {
		// Single port format: "80"
		allErrs = append(allErrs, v.validateSinglePort(portStr, fldPath)...)
	}

	return allErrs
}

// validateSinglePort validates a single port number
func (v *AddressGroupPortMappingValidator) validateSinglePort(portStr string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, portStr, "port must be a valid integer"))
		return allErrs
	}

	if port < 1 || port > 65535 {
		allErrs = append(allErrs, field.Invalid(fldPath, portStr, "port must be between 1 and 65535"))
	}

	return allErrs
}

// validatePortRange validates a port range
func (v *AddressGroupPortMappingValidator) validatePortRange(portStr string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	parts := strings.Split(portStr, "-")
	if len(parts) != 2 {
		allErrs = append(allErrs, field.Invalid(fldPath, portStr, "port range must be in format 'start-end'"))
		return allErrs
	}

	startStr := strings.TrimSpace(parts[0])
	endStr := strings.TrimSpace(parts[1])

	start, err := strconv.Atoi(startStr)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, portStr, "start port must be a valid integer"))
		return allErrs
	}

	end, err := strconv.Atoi(endStr)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, portStr, "end port must be a valid integer"))
		return allErrs
	}

	// Validate port ranges
	if start < 1 || start > 65535 {
		allErrs = append(allErrs, field.Invalid(fldPath, portStr, "start port must be between 1 and 65535"))
	}

	if end < 1 || end > 65535 {
		allErrs = append(allErrs, field.Invalid(fldPath, portStr, "end port must be between 1 and 65535"))
	}

	if start > end {
		allErrs = append(allErrs, field.Invalid(fldPath, portStr, "start port must be less than or equal to end port"))
	}

	return allErrs
}

// NewAddressGroupPortMappingValidator creates a new AddressGroupPortMappingValidator instance
func NewAddressGroupPortMappingValidator() *AddressGroupPortMappingValidator {
	return &AddressGroupPortMappingValidator{}
}
