package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// AddressGroupBindingValidator implements validation for AddressGroupBinding resources
type AddressGroupBindingValidator struct{}

// Compile-time interface assertion
var _ base.Validator[*v1beta1.AddressGroupBinding] = &AddressGroupBindingValidator{}

// ValidateCreate validates a new AddressGroupBinding being created
func (v *AddressGroupBindingValidator) ValidateCreate(ctx context.Context, obj *v1beta1.AddressGroupBinding) field.ErrorList {
	return v.validate(ctx, obj)
}

// ValidateUpdate validates an AddressGroupBinding being updated
func (v *AddressGroupBindingValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.AddressGroupBinding, old *v1beta1.AddressGroupBinding) field.ErrorList {
	allErrs := v.validate(ctx, obj)

	// Additional update-specific validation
	if old != nil {
		// Check if immutable fields are changed
		if obj.Spec.ServiceRef != old.Spec.ServiceRef {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "serviceRef"), obj.Spec.ServiceRef,
				"serviceRef is immutable and cannot be changed"))
		}
		if obj.Spec.AddressGroupRef != old.Spec.AddressGroupRef {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "addressGroupRef"), obj.Spec.AddressGroupRef,
				"addressGroupRef is immutable and cannot be changed"))
		}
	}

	return allErrs
}

// ValidateDelete validates an AddressGroupBinding being deleted
func (v *AddressGroupBindingValidator) ValidateDelete(ctx context.Context, obj *v1beta1.AddressGroupBinding) field.ErrorList {
	// For delete operations, we might want to check:
	// - If binding is referenced by policies
	// - If there are dependent resources
	return field.ErrorList{}
}

// validate performs comprehensive validation of an AddressGroupBinding object (internal method)
func (v *AddressGroupBindingValidator) validate(ctx context.Context, obj *v1beta1.AddressGroupBinding) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "addressgroupbinding object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// validateMetadata validates the metadata fields
func (v *AddressGroupBindingValidator) validateMetadata(obj *v1beta1.AddressGroupBinding) field.ErrorList {
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

// validateSpec validates the AddressGroupBinding spec
func (v *AddressGroupBindingValidator) validateSpec(spec v1beta1.AddressGroupBindingSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate ServiceRef (required)
	allErrs = append(allErrs, v.validateServiceRef(spec.ServiceRef, fldPath.Child("serviceRef"))...)

	// Validate AddressGroupRef (required)
	allErrs = append(allErrs, v.validateAddressGroupRef(spec.AddressGroupRef, fldPath.Child("addressGroupRef"))...)

	return allErrs
}

// validateServiceRef validates the Service object reference
func (v *AddressGroupBindingValidator) validateServiceRef(ref v1beta1.ObjectReference, fldPath *field.Path) field.ErrorList {
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
	} else if ref.Kind != "Service" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be 'Service'"))
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

// validateAddressGroupRef validates the AddressGroup object reference
func (v *AddressGroupBindingValidator) validateAddressGroupRef(ref v1beta1.NamespacedObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate embedded ObjectReference
	allErrs = append(allErrs, v.validateObjectReference(ref.ObjectReference, fldPath, "AddressGroup")...)

	// Validate Namespace (optional but if present should be valid)
	if ref.Namespace != "" && !isDNS1123Subdomain(ref.Namespace) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("namespace"), ref.Namespace,
			"namespace must be a valid DNS-1123 subdomain"))
	}

	return allErrs
}

// validateObjectReference validates a generic object reference with expected kind
func (v *AddressGroupBindingValidator) validateObjectReference(ref v1beta1.ObjectReference, fldPath *field.Path, expectedKind string) field.ErrorList {
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

// NewAddressGroupBindingValidator creates a new AddressGroupBindingValidator instance
func NewAddressGroupBindingValidator() *AddressGroupBindingValidator {
	return &AddressGroupBindingValidator{}
}
