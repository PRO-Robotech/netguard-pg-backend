package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// IEAgAgRuleValidator implements validation for IEAgAgRule resources
type IEAgAgRuleValidator struct {
	helpers *ValidationHelpers
}

// Compile-time interface assertion
var _ base.Validator[*v1beta1.IEAgAgRule] = &IEAgAgRuleValidator{}

// ValidateCreate validates a new IEAgAgRule being created
func (v *IEAgAgRuleValidator) ValidateCreate(ctx context.Context, obj *v1beta1.IEAgAgRule) field.ErrorList {
	return v.validate(ctx, obj)
}

// ValidateUpdate validates an IEAgAgRule being updated
func (v *IEAgAgRuleValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.IEAgAgRule, old *v1beta1.IEAgAgRule) field.ErrorList {
	allErrs := v.validate(ctx, obj)

	// Additional update-specific validation
	if old != nil {
		// Check if immutable fields are changed
		if obj.Spec.Transport != old.Spec.Transport {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "transport"), obj.Spec.Transport,
				"transport is immutable and cannot be changed"))
		}
		if obj.Spec.Traffic != old.Spec.Traffic {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "traffic"), obj.Spec.Traffic,
				"traffic is immutable and cannot be changed"))
		}
		if obj.Spec.AddressGroupLocal != old.Spec.AddressGroupLocal {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "addressGroupLocal"), obj.Spec.AddressGroupLocal,
				"addressGroupLocal is immutable and cannot be changed"))
		}
		if obj.Spec.AddressGroup != old.Spec.AddressGroup {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "addressGroup"), obj.Spec.AddressGroup,
				"addressGroup is immutable and cannot be changed"))
		}
	}

	return allErrs
}

// ValidateDelete validates an IEAgAgRule being deleted
func (v *IEAgAgRuleValidator) ValidateDelete(ctx context.Context, obj *v1beta1.IEAgAgRule) field.ErrorList {
	return field.ErrorList{}
}

// validate performs comprehensive validation of an IEAgAgRule object
func (v *IEAgAgRuleValidator) validate(ctx context.Context, obj *v1beta1.IEAgAgRule) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "ieagagrule object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// validateMetadata validates the metadata fields using standard validation
func (v *IEAgAgRuleValidator) validateMetadata(obj *v1beta1.IEAgAgRule) field.ErrorList {
	metaPath := field.NewPath("metadata")
	return ValidateStandardObjectMeta(&obj.ObjectMeta, metaPath)
}

// validateSpec validates the IEAgAgRule spec
func (v *IEAgAgRuleValidator) validateSpec(spec v1beta1.IEAgAgRuleSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate description length
	if len(spec.Description) > 512 {
		allErrs = append(allErrs, field.TooLong(fldPath.Child("description"), spec.Description, 512))
	}

	// Validate Transport (required)
	allErrs = append(allErrs, v.validateTransport(spec.Transport, fldPath.Child("transport"))...)

	// Validate Traffic (required) using standard validation
	allErrs = append(allErrs, ValidateRequiredEnum(string(spec.Traffic), "traffic", []string{"INGRESS", "EGRESS"}, fldPath.Child("traffic"))...)

	// Validate AddressGroupLocal (required)
	allErrs = append(allErrs, v.validateAddressGroupLocal(spec.AddressGroupLocal, fldPath.Child("addressGroupLocal"))...)

	// Validate AddressGroup (required)
	allErrs = append(allErrs, v.validateAddressGroup(spec.AddressGroup, fldPath.Child("addressGroup"))...)

	// Validate Ports (optional)
	if len(spec.Ports) > 0 {
		allErrs = append(allErrs, v.validatePorts(spec.Ports, fldPath.Child("ports"))...)
	}

	// Validate Action (optional but if present should be valid)
	if spec.Action != "" {
		allErrs = append(allErrs, v.validateAction(string(spec.Action), fldPath.Child("action"))...)
	}

	// Validate Priority (optional but if present should be non-negative)
	if spec.Priority < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("priority"), spec.Priority,
			"priority must be non-negative"))
	}

	return allErrs
}

// validateTransport validates the transport protocol enum
func (v *IEAgAgRuleValidator) validateTransport(transport v1beta1.TransportProtocol, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if transport == "" {
		allErrs = append(allErrs, field.Required(fldPath, "transport is required"))
		return allErrs
	}

	// Validate enum values
	switch transport {
	case v1beta1.ProtocolTCP, v1beta1.ProtocolUDP:
		// Valid values
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath, string(transport),
			[]string{"TCP", "UDP"}))
	}

	return allErrs
}

// validateAction validates the action enum
func (v *IEAgAgRuleValidator) validateAction(action string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate enum values
	switch action {
	case "ACCEPT", "DROP":
		// Valid values
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath, action,
			[]string{"ACCEPT", "DROP"}))
	}

	return allErrs
}

// validateAddressGroupLocal validates the local address group reference using standard validation
func (v *IEAgAgRuleValidator) validateAddressGroupLocal(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Use standard ObjectReference validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&ref, fldPath)...)

	// Domain-specific validation: APIVersion must be netguard.sgroups.io/v1beta1
	if ref.APIVersion != "" && ref.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), ref.APIVersion,
			"apiVersion must be 'netguard.sgroups.io/v1beta1'"))
	}

	// Domain-specific validation: Kind must be AddressGroup
	if ref.Kind != "" && ref.Kind != "AddressGroup" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'AddressGroup'"))
	}

	return allErrs
}

// validateAddressGroup validates the remote address group reference using standard validation
func (v *IEAgAgRuleValidator) validateAddressGroup(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Use standard ObjectReference validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&ref, fldPath)...)

	// Domain-specific validation: APIVersion must be netguard.sgroups.io/v1beta1
	if ref.APIVersion != "" && ref.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), ref.APIVersion,
			"apiVersion must be 'netguard.sgroups.io/v1beta1'"))
	}

	// Domain-specific validation: Kind must be AddressGroup
	if ref.Kind != "" && ref.Kind != "AddressGroup" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'AddressGroup'"))
	}

	return allErrs
}

// validatePorts validates the list of port specifications
func (v *IEAgAgRuleValidator) validatePorts(ports []v1beta1.PortSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for i, portSpec := range ports {
		portPath := fldPath.Index(i)
		allErrs = append(allErrs, v.validatePortSpec(portSpec, portPath)...)
	}

	return allErrs
}

// validatePortSpec validates a single port specification
func (v *IEAgAgRuleValidator) validatePortSpec(spec v1beta1.PortSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Must specify either Port or PortRange, but not both
	hasPort := spec.Port != 0
	hasPortRange := spec.PortRange != nil

	if !hasPort && !hasPortRange {
		allErrs = append(allErrs, field.Required(fldPath, "either port or portRange must be specified"))
		return allErrs
	}

	if hasPort && hasPortRange {
		allErrs = append(allErrs, field.Invalid(fldPath, spec,
			"cannot specify both port and portRange"))
		return allErrs
	}

	// Validate single port
	if hasPort {
		if spec.Port < 1 || spec.Port > 65535 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("port"), spec.Port,
				"port must be between 1 and 65535"))
		}
	}

	// Validate port range
	if hasPortRange {
		allErrs = append(allErrs, v.validatePortRange(*spec.PortRange, fldPath.Child("portRange"))...)
	}

	return allErrs
}

// validatePortRange validates a port range specification
func (v *IEAgAgRuleValidator) validatePortRange(portRange v1beta1.PortRange, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate From port
	if portRange.From < 1 || portRange.From > 65535 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("from"), portRange.From,
			"from port must be between 1 and 65535"))
	}

	// Validate To port
	if portRange.To < 1 || portRange.To > 65535 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("to"), portRange.To,
			"to port must be between 1 and 65535"))
	}

	// Validate range order
	if portRange.From > portRange.To {
		allErrs = append(allErrs, field.Invalid(fldPath, portRange,
			"from port must be less than or equal to to port"))
	}

	return allErrs
}

// NewIEAgAgRuleValidator creates a new IEAgAgRuleValidator instance
func NewIEAgAgRuleValidator() *IEAgAgRuleValidator {
	return &IEAgAgRuleValidator{
		helpers: NewValidationHelpers(),
	}
}
