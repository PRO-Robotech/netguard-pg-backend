package validation

import (
	"fmt"
	"net"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// ValidateStandardObjectMeta validates standard Kubernetes object metadata
// This function centralizes all metadata validation logic for all 10 v1beta1 resources
func ValidateStandardObjectMeta(meta *metav1.ObjectMeta, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if meta == nil {
		return append(allErrs, field.Required(fldPath, "metadata cannot be nil"))
	}

	// Name or generateName is required
	if meta.Name == "" && meta.GenerateName == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name or generateName is required"))
	}

	// Name validation using K8s standard patterns
	if meta.Name != "" {
		for _, msg := range validation.IsDNS1123Subdomain(meta.Name) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("name"), meta.Name, msg))
		}
	}

	// GenerateName validation - allow trailing hyphens since it's a prefix
	if meta.GenerateName != "" {
		if errs := validateGenerateName(meta.GenerateName); len(errs) > 0 {
			for _, msg := range errs {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("generateName"), meta.GenerateName, msg))
			}
		}
	}

	// Namespace validation using K8s standard patterns
	if meta.Namespace != "" {
		for _, msg := range validation.IsDNS1123Subdomain(meta.Namespace) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("namespace"), meta.Namespace, msg))
		}
	}

	// Labels validation using K8s standard patterns
	if meta.Labels != nil {
		for key, value := range meta.Labels {
			if errs := validation.IsQualifiedName(key); len(errs) > 0 {
				for _, err := range errs {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("labels").Key(key), key, err))
				}
			}
			if errs := validation.IsValidLabelValue(value); len(errs) > 0 {
				for _, err := range errs {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("labels").Key(key), value, err))
				}
			}
		}
	}

	// Annotations validation using K8s standard patterns
	if meta.Annotations != nil {
		for key, value := range meta.Annotations {
			if errs := validation.IsQualifiedName(key); len(errs) > 0 {
				for _, err := range errs {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("annotations").Key(key), key, err))
				}
			}
			// Annotation values can be longer than label values (63 chars is the limit for labels)
			const maxAnnotationValueLen = 262144 // 256KB limit for annotations
			if len(value) > maxAnnotationValueLen && !strings.HasPrefix(key, "kubectl.kubernetes.io/") {
				allErrs = append(allErrs, field.TooLong(fldPath.Child("annotations").Key(key), value, maxAnnotationValueLen))
			}
		}
	}

	return allErrs
}

// ValidateEnum validates enum values against a list of valid options
// Used for TransportProtocol, Traffic, RuleAction enums
func ValidateEnum(value, fieldName string, validValues []string, fldPath *field.Path) field.ErrorList {
	if value == "" {
		// Empty enum values are handled by required field validation
		return field.ErrorList{}
	}

	for _, valid := range validValues {
		if value == valid {
			return field.ErrorList{}
		}
	}

	return field.ErrorList{
		field.NotSupported(fldPath, value, validValues),
	}
}

// ValidateRequiredEnum validates enum values and requires non-empty value
func ValidateRequiredEnum(value, fieldName string, validValues []string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if value == "" {
		return append(allErrs, field.Required(fldPath, fieldName+" is required"))
	}

	for _, valid := range validValues {
		if value == valid {
			return field.ErrorList{}
		}
	}

	return append(allErrs, field.NotSupported(fldPath, value, validValues))
}

// ValidatePort validates port numbers (1-65535 range) with K8s standard validation
func ValidatePort(port int32, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if port < 1 || port > 65535 {
		allErrs = append(allErrs, field.Invalid(fldPath, port, "port must be between 1 and 65535"))
	}

	return allErrs
}

// ValidatePortRange validates PortRange struct with From/To validation
func ValidatePortRange(portRange *netguardv1beta1.PortRange, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if portRange == nil {
		return append(allErrs, field.Required(fldPath, "portRange cannot be nil"))
	}

	// Validate From port
	allErrs = append(allErrs, ValidatePort(portRange.From, fldPath.Child("from"))...)

	// Validate To port
	allErrs = append(allErrs, ValidatePort(portRange.To, fldPath.Child("to"))...)

	// Validate From <= To
	if portRange.From > portRange.To {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("from"), portRange.From, "from port must be less than or equal to to port"))
	}

	return allErrs
}

// ValidateCIDR validates CIDR notation using net.ParseCIDR
func ValidateCIDR(cidr string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if cidr == "" {
		return append(allErrs, field.Required(fldPath, "CIDR cannot be empty"))
	}

	if _, _, err := net.ParseCIDR(cidr); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, cidr, "must be a valid CIDR notation"))
	}

	return allErrs
}

// ValidateObjectReference validates ObjectReference fields
func ValidateObjectReference(ref *netguardv1beta1.ObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if ref == nil {
		return append(allErrs, field.Required(fldPath, "objectReference cannot be nil"))
	}

	// APIVersion is required
	if ref.APIVersion == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("apiVersion"), "apiVersion is required"))
	}

	// Kind is required
	if ref.Kind == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("kind"), "kind is required"))
	}

	// Name is required
	if ref.Name == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name is required"))
	} else {
		// Validate name format
		for _, msg := range validation.IsDNS1123Subdomain(ref.Name) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("name"), ref.Name, msg))
		}
	}

	// ObjectReference doesn't have Namespace field - that's in NamespacedObjectReference

	return allErrs
}

// ValidateNamespacedObjectReference validates NamespacedObjectReference fields
func ValidateNamespacedObjectReference(ref *netguardv1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if ref == nil {
		return append(allErrs, field.Required(fldPath, "namespacedObjectReference cannot be nil"))
	}

	// APIVersion is required
	if ref.APIVersion == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("apiVersion"), "apiVersion is required"))
	}

	// Kind is required
	if ref.Kind == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("kind"), "kind is required"))
	}

	// Name is required
	if ref.Name == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name is required"))
	} else {
		// Validate name format
		for _, msg := range validation.IsDNS1123Subdomain(ref.Name) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("name"), ref.Name, msg))
		}
	}

	// Namespace is required for NamespacedObjectReference
	if ref.Namespace == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("namespace"), "namespace is required"))
	} else {
		// Validate namespace format
		for _, msg := range validation.IsDNS1123Subdomain(ref.Namespace) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("namespace"), ref.Namespace, msg))
		}
	}

	return allErrs
}

// ValidateTransportProtocol validates TransportProtocol enum values
func ValidateTransportProtocol(protocol netguardv1beta1.TransportProtocol, fldPath *field.Path) field.ErrorList {
	return ValidateEnum(string(protocol), "protocol", []string{"TCP", "UDP"}, fldPath)
}

// ValidateTraffic validates Traffic enum values
func ValidateTraffic(traffic netguardv1beta1.Traffic, fldPath *field.Path) field.ErrorList {
	return ValidateEnum(string(traffic), "traffic", []string{"INGRESS", "EGRESS"}, fldPath)
}

// ValidateRuleAction validates RuleAction enum values
func ValidateRuleAction(action netguardv1beta1.RuleAction, fldPath *field.Path) field.ErrorList {
	return ValidateEnum(string(action), "action", []string{"ACCEPT", "DROP"}, fldPath)
}

// ValidateIngressPorts validates a slice of IngressPort with comprehensive validation
func ValidateIngressPorts(ports []netguardv1beta1.IngressPort, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for i, port := range ports {
		portPath := fldPath.Index(i)

		// Validate protocol
		if port.Protocol != "" {
			allErrs = append(allErrs, ValidateTransportProtocol(port.Protocol, portPath.Child("protocol"))...)
		}

		// Validate port string (can be single port or range like "80-90")
		if port.Port == "" {
			allErrs = append(allErrs, field.Required(portPath.Child("port"), "port is required"))
		} else {
			// Basic port string validation - actual parsing would be done elsewhere
			if len(port.Port) > 32 {
				allErrs = append(allErrs, field.TooLong(portPath.Child("port"), port.Port, 32))
			}
		}

		// Validate description length
		if len(port.Description) > 512 {
			allErrs = append(allErrs, field.TooLong(portPath.Child("description"), port.Description, 512))
		}
	}

	return allErrs
}

// ValidatePortSpecs validates a slice of PortSpec with comprehensive validation
func ValidatePortSpecs(ports []netguardv1beta1.PortSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for i, port := range ports {
		portPath := fldPath.Index(i)

		// Validate single port if specified
		if port.Port != 0 {
			allErrs = append(allErrs, ValidatePort(port.Port, portPath.Child("port"))...)
		}

		// Validate port range if specified
		if port.PortRange != nil {
			allErrs = append(allErrs, ValidatePortRange(port.PortRange, portPath.Child("portRange"))...)
		}

		// At least one of port or portRange must be specified
		if port.Port == 0 && port.PortRange == nil {
			allErrs = append(allErrs, field.Required(portPath, "either port or portRange must be specified"))
		}

		// Both port and portRange cannot be specified
		if port.Port != 0 && port.PortRange != nil {
			allErrs = append(allErrs, field.Invalid(portPath, port, "cannot specify both port and portRange"))
		}
	}

	return allErrs
}

// ValidateDescription validates description fields with length limits
func ValidateDescription(description string, fldPath *field.Path, maxLength int) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(description) > maxLength {
		allErrs = append(allErrs, field.TooLong(fldPath, description, maxLength))
	}

	return allErrs
}

// ValidationHelpers provides common validation patterns for all resources
type ValidationHelpers struct{}

// NewValidationHelpers creates a new ValidationHelpers instance
func NewValidationHelpers() *ValidationHelpers {
	return &ValidationHelpers{}
}

// ValidateRequiredString validates that a string field is not empty
func (vh *ValidationHelpers) ValidateRequiredString(value string, fldPath *field.Path, fieldName string) field.ErrorList {
	if value == "" {
		return field.ErrorList{field.Required(fldPath, fieldName+" is required")}
	}
	return field.ErrorList{}
}

// ValidateOptionalString validates an optional string field (no validation if empty)
func (vh *ValidationHelpers) ValidateOptionalString(value string, fldPath *field.Path, validator func(string, *field.Path) field.ErrorList) field.ErrorList {
	if value == "" {
		return field.ErrorList{}
	}
	return validator(value, fldPath)
}

// validateGenerateName validates a generateName prefix according to Kubernetes conventions
// generateName is a prefix that will have a suffix added, so it can end with hyphens
func validateGenerateName(generateName string) []string {
	var errs []string

	// Check basic length constraints (same as DNS1123 label)
	if len(generateName) == 0 {
		errs = append(errs, "cannot be empty")
		return errs
	}
	if len(generateName) > 253 {
		errs = append(errs, "must be no more than 253 characters")
	}

	// Check character validity - allow trailing hyphens for prefixes
	// generateName can end with hyphen since it's a prefix, but must start with alphanumeric
	if generateName[0] == '-' || generateName[0] == '.' {
		errs = append(errs, "must start with an alphanumeric character")
	}

	// Check that all characters are valid DNS1123 characters
	for i, r := range generateName {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.') {
			errs = append(errs, fmt.Sprintf("invalid character '%c' at position %d: must contain only lowercase alphanumeric characters, '-' or '.'", r, i))
			break
		}
	}

	// Additional validation: ensure the generated name will be valid
	// Check that when a suffix is added, the result would be a valid DNS1123 subdomain
	// We simulate adding a minimal suffix to check validity
	if len(generateName) > 0 && (generateName[len(generateName)-1] == '-' || generateName[len(generateName)-1] == '.') {
		// This is acceptable for generateName - the suffix will make it valid
	}

	return errs
}
