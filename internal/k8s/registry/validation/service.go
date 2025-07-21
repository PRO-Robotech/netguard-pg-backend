package validation

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// ServiceValidator implements validation for Service resources
type ServiceValidator struct{}

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

// validateMetadata validates the metadata fields
func (v *ServiceValidator) validateMetadata(obj *v1beta1.Service) field.ErrorList {
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

// validateSpec validates the Service spec
func (v *ServiceValidator) validateSpec(spec v1beta1.ServiceSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate description length (optional but if present should be reasonable)
	if len(spec.Description) > 512 {
		allErrs = append(allErrs, field.TooLong(fldPath.Child("description"), spec.Description, 512))
	}

	// Validate ingress ports
	allErrs = append(allErrs, v.validateIngressPorts(spec.IngressPorts, fldPath.Child("ingressPorts"))...)

	return allErrs
}

// validateIngressPorts validates the list of ingress ports
func (v *ServiceValidator) validateIngressPorts(ports []v1beta1.IngressPort, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(ports) == 0 {
		// IngressPorts is optional, but if present should have at least one port
		// This is actually valid - empty list means no ingress ports
		return allErrs
	}

	// Track unique port+protocol combinations to prevent duplicates
	seen := make(map[string]bool)

	for i, port := range ports {
		portPath := fldPath.Index(i)
		allErrs = append(allErrs, v.validateIngressPort(port, portPath)...)

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

// validateIngressPort validates a single ingress port configuration
func (v *ServiceValidator) validateIngressPort(port v1beta1.IngressPort, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate protocol
	allErrs = append(allErrs, v.validateTransportProtocol(string(port.Protocol), fldPath.Child("protocol"))...)

	// Validate port
	allErrs = append(allErrs, v.validatePortString(port.Port, fldPath.Child("port"))...)

	// Validate description length
	if len(port.Description) > 256 {
		allErrs = append(allErrs, field.TooLong(fldPath.Child("description"), port.Description, 256))
	}

	return allErrs
}

// validateTransportProtocol validates transport protocol enum
func (v *ServiceValidator) validateTransportProtocol(protocol string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	switch v1beta1.TransportProtocol(protocol) {
	case v1beta1.ProtocolTCP, v1beta1.ProtocolUDP:
		// Valid protocols
	case "":
		allErrs = append(allErrs, field.Required(fldPath, "protocol is required"))
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath, protocol,
			[]string{string(v1beta1.ProtocolTCP), string(v1beta1.ProtocolUDP)}))
	}

	return allErrs
}

// validatePortString validates port string format and range
func (v *ServiceValidator) validatePortString(portStr string, fldPath *field.Path) field.ErrorList {
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
func (v *ServiceValidator) validateSinglePort(portStr string, fldPath *field.Path) field.ErrorList {
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
func (v *ServiceValidator) validatePortRange(portStr string, fldPath *field.Path) field.ErrorList {
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

// Note: isDNS1123Subdomain function is defined in utils.go and shared across validators

// NewServiceValidator creates a new ServiceValidator instance
func NewServiceValidator() *ServiceValidator {
	return &ServiceValidator{}
}
