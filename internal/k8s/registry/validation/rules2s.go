package validation

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// RuleS2SValidator implements validation for RuleS2S resources
type RuleS2SValidator struct{}

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

// validateMetadata validates the metadata fields
func (v *RuleS2SValidator) validateMetadata(obj *v1beta1.RuleS2S) field.ErrorList {
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

// validateSpec validates the RuleS2S spec
func (v *RuleS2SValidator) validateSpec(spec v1beta1.RuleS2SSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate Traffic (required)
	allErrs = append(allErrs, v.validateTraffic(spec.Traffic, fldPath.Child("traffic"))...)

	// Validate ServiceLocalRef (required)
	allErrs = append(allErrs, v.validateServiceLocalRef(spec.ServiceLocalRef, fldPath.Child("serviceLocalRef"))...)

	// Validate ServiceRef (required)
	allErrs = append(allErrs, v.validateServiceRef(spec.ServiceRef, fldPath.Child("serviceRef"))...)

	return allErrs
}

// validateTraffic validates the traffic direction enum
func (v *RuleS2SValidator) validateTraffic(traffic string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if traffic == "" {
		allErrs = append(allErrs, field.Required(fldPath, "traffic is required"))
		return allErrs
	}

	// Validate enum values (case-insensitive)
	switch strings.ToLower(traffic) {
	case "ingress", "egress":
		// Valid values
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath, traffic,
			[]string{"ingress", "egress"}))
	}

	return allErrs
}

// validateServiceLocalRef validates the local service reference
func (v *RuleS2SValidator) validateServiceLocalRef(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// ServiceLocalRef should reference ServiceAlias
	allErrs = append(allErrs, v.validateNamespacedObjectReference(ref, fldPath, "ServiceAlias")...)

	return allErrs
}

// validateServiceRef validates the target service reference
func (v *RuleS2SValidator) validateServiceRef(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// ServiceRef should reference ServiceAlias
	allErrs = append(allErrs, v.validateNamespacedObjectReference(ref, fldPath, "ServiceAlias")...)

	return allErrs
}

// validateNamespacedObjectReference validates a namespaced object reference
func (v *RuleS2SValidator) validateNamespacedObjectReference(ref v1beta1.NamespacedObjectReference, fldPath *field.Path, expectedKind string) field.ErrorList {
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

// NewRuleS2SValidator creates a new RuleS2SValidator instance
func NewRuleS2SValidator() *RuleS2SValidator {
	return &RuleS2SValidator{}
}
