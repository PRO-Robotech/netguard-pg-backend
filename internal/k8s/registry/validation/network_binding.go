package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// NetworkBindingValidator validates NetworkBinding resources
type NetworkBindingValidator struct {
	helpers *ValidationHelpers
}

// NewNetworkBindingValidator creates a new NetworkBindingValidator
func NewNetworkBindingValidator() *NetworkBindingValidator {
	return &NetworkBindingValidator{
		helpers: NewValidationHelpers(),
	}
}

// ValidateCreate validates a new NetworkBinding being created
func (v *NetworkBindingValidator) ValidateCreate(ctx context.Context, obj *v1beta1.NetworkBinding) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "networkbinding object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, obj.Namespace, field.NewPath("spec"))...)

	return allErrs
}

// ValidateUpdate validates a NetworkBinding being updated
func (v *NetworkBindingValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.NetworkBinding, oldObj *v1beta1.NetworkBinding) field.ErrorList {
	allErrs := v.ValidateCreate(ctx, obj)

	if obj == nil || oldObj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "networkBinding object cannot be nil for update"))
	}

	// Validate that immutable fields haven't changed
	allErrs = append(allErrs, v.validateImmutableFields(obj, oldObj)...)

	return allErrs
}

// ValidateDelete validates a NetworkBinding being deleted
func (v *NetworkBindingValidator) ValidateDelete(ctx context.Context, obj *v1beta1.NetworkBinding) field.ErrorList {
	// No specific validation for deletion
	return field.ErrorList{}
}

// validateMetadata validates the metadata fields using standard validation
func (v *NetworkBindingValidator) validateMetadata(obj *v1beta1.NetworkBinding) field.ErrorList {
	metaPath := field.NewPath("metadata")
	return ValidateStandardObjectMeta(&obj.ObjectMeta, metaPath)
}

// validateSpec validates the NetworkBinding spec
func (v *NetworkBindingValidator) validateSpec(spec v1beta1.NetworkBindingSpec, parentNamespace string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate NetworkRef - required
	allErrs = append(allErrs, v.validateObjectReference(spec.NetworkRef, fldPath.Child("networkRef"), "Network")...)

	// Validate AddressGroupRef - required
	allErrs = append(allErrs, v.validateObjectReference(spec.AddressGroupRef, fldPath.Child("addressGroupRef"), "AddressGroup")...)

	// Validate that NetworkRef namespace matches parent namespace
	if spec.NetworkRef.Namespace != "" && spec.NetworkRef.Namespace != parentNamespace {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("networkRef").Child("namespace"), spec.NetworkRef.Namespace,
			fmt.Sprintf("networkRef namespace must match NetworkBinding namespace (%s)", parentNamespace)))
	}

	// Validate that AddressGroupRef namespace matches parent namespace
	if spec.AddressGroupRef.Namespace != "" && spec.AddressGroupRef.Namespace != parentNamespace {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("addressGroupRef").Child("namespace"), spec.AddressGroupRef.Namespace,
			fmt.Sprintf("addressGroupRef namespace must match NetworkBinding namespace (%s)", parentNamespace)))
	}

	return allErrs
}

// validateObjectReference validates a NamespacedObjectReference
func (v *NetworkBindingValidator) validateObjectReference(objRef v1beta1.NamespacedObjectReference, fldPath *field.Path, expectedKind string) field.ErrorList {
	allErrs := field.ErrorList{}

	// Name is required
	if objRef.Name == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name is required"))
	}

	// Namespace will be filled by mutation webhook if empty, so we don't require it here
	// The validateSpec function already checks that namespace matches parent namespace if provided

	// Validate Kind if specified
	if objRef.Kind != "" && objRef.Kind != expectedKind {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), objRef.Kind, fmt.Sprintf("kind should be %s", expectedKind)))
	}

	// Validate APIVersion if specified
	if objRef.APIVersion != "" && objRef.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), objRef.APIVersion, "apiVersion should be netguard.sgroups.io/v1beta1"))
	}

	// Validate name format (basic DNS subdomain validation)
	if objRef.Name != "" {
		if len(objRef.Name) > 253 {
			allErrs = append(allErrs, field.TooLong(fldPath.Child("name"), objRef.Name, 253))
		}
		// Additional name format validation could be added here
	}

	return allErrs
}

// validateImmutableFields validates that immutable fields haven't changed
func (v *NetworkBindingValidator) validateImmutableFields(obj, oldObj *v1beta1.NetworkBinding) field.ErrorList {
	allErrs := field.ErrorList{}
	fldPath := field.NewPath("spec")

	// NetworkRef is immutable
	if !v.objectReferencesEqual(obj.Spec.NetworkRef, oldObj.Spec.NetworkRef) {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("networkRef"), "networkRef is immutable"))
	}

	// AddressGroupRef is immutable
	if !v.objectReferencesEqual(obj.Spec.AddressGroupRef, oldObj.Spec.AddressGroupRef) {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("addressGroupRef"), "addressGroupRef is immutable"))
	}

	return allErrs
}

// objectReferencesEqual compares two NamespacedObjectReferences for equality
func (v *NetworkBindingValidator) objectReferencesEqual(ref1, ref2 v1beta1.NamespacedObjectReference) bool {
	return ref1.Name == ref2.Name &&
		ref1.Namespace == ref2.Namespace &&
		ref1.Kind == ref2.Kind &&
		ref1.APIVersion == ref2.APIVersion
}
