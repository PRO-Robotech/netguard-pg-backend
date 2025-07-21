package validation

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// IEAgAgRuleValidator implements validation for IEAgAgRule resources
type IEAgAgRuleValidator struct{}

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

// validateMetadata validates the metadata fields
func (v *IEAgAgRuleValidator) validateMetadata(obj *v1beta1.IEAgAgRule) field.ErrorList {
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

// validateSpec validates the IEAgAgRule spec
func (v *IEAgAgRuleValidator) validateSpec(spec v1beta1.IEAgAgRuleSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate description length
	if len(spec.Description) > 512 {
		allErrs = append(allErrs, field.TooLong(fldPath.Child("description"), spec.Description, 512))
	}

	// Validate Transport (required)
	allErrs = append(allErrs, v.validateTransport(spec.Transport, fldPath.Child("transport"))...)

	// Validate Traffic (required)
	allErrs = append(allErrs, v.validateTraffic(spec.Traffic, fldPath.Child("traffic"))...)

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
func (v *IEAgAgRuleValidator) validateTransport(transport string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if transport == "" {
		allErrs = append(allErrs, field.Required(fldPath, "transport is required"))
		return allErrs
	}

	// Validate enum values
	switch strings.ToUpper(transport) {
	case "TCP", "UDP", "SCTP":
		// Valid values
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath, transport,
			[]string{"TCP", "UDP", "SCTP"}))
	}

	return allErrs
}

// validateTraffic validates the traffic direction enum
func (v *IEAgAgRuleValidator) validateTraffic(traffic string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if traffic == "" {
		allErrs = append(allErrs, field.Required(fldPath, "traffic is required"))
		return allErrs
	}

	// Validate enum values
	switch traffic {
	case "Ingress", "Egress":
		// Valid values (note: different case from RuleS2S)
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath, traffic,
			[]string{"Ingress", "Egress"}))
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

// validateAddressGroupLocal validates the local address group reference
func (v *IEAgAgRuleValidator) validateAddressGroupLocal(ref v1beta1.ObjectReference, fldPath *field.Path) field.ErrorList {
	return v.validateObjectReference(ref, fldPath, "AddressGroup")
}

// validateAddressGroup validates the remote address group reference
func (v *IEAgAgRuleValidator) validateAddressGroup(ref v1beta1.ObjectReference, fldPath *field.Path) field.ErrorList {
	return v.validateObjectReference(ref, fldPath, "AddressGroup")
}

// validateObjectReference validates an object reference
func (v *IEAgAgRuleValidator) validateObjectReference(ref v1beta1.ObjectReference, fldPath *field.Path, expectedKind string) field.ErrorList {
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
	return &IEAgAgRuleValidator{}
}
