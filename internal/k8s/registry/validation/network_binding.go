package validation

import (
	"context"

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
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// ValidateUpdate validates a NetworkBinding being updated
func (v *NetworkBindingValidator) ValidateUpdate(ctx context.Context, obj *v1beta1.NetworkBinding, oldObj *v1beta1.NetworkBinding) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "networkbinding object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	// Check if references have changed when Ready condition is true
	if v.isReadyConditionTrue(oldObj) {
		if !v.objectReferencesEqual(obj.Spec.NetworkRef, oldObj.Spec.NetworkRef) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec.networkRef"),
				"cannot change networkRef when Ready condition is true"))
		}
		if !v.objectReferencesEqual(obj.Spec.AddressGroupRef, oldObj.Spec.AddressGroupRef) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec.addressGroupRef"),
				"cannot change addressGroupRef when Ready condition is true"))
		}
	}

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
func (v *NetworkBindingValidator) validateSpec(spec v1beta1.NetworkBindingSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate NetworkRef
	allErrs = append(allErrs, v.validateObjectReference(spec.NetworkRef, fldPath.Child("networkRef"), "Network")...)

	// Validate AddressGroupRef
	allErrs = append(allErrs, v.validateObjectReference(spec.AddressGroupRef, fldPath.Child("addressGroupRef"), "AddressGroup")...)

	return allErrs
}

// validateObjectReference validates an ObjectReference using standard validation
func (v *NetworkBindingValidator) validateObjectReference(ref v1beta1.ObjectReference, fldPath *field.Path, expectedKind string) field.ErrorList {
	allErrs := field.ErrorList{}

	// Convert to pointer for standard validation
	objRef := &v1beta1.ObjectReference{
		APIVersion: ref.APIVersion,
		Kind:       ref.Kind,
		Name:       ref.Name,
	}

	// Use standard validation
	allErrs = append(allErrs, ValidateObjectReference(objRef, fldPath)...)

	// Additional validation for expected kind
	if ref.Kind != "" && ref.Kind != expectedKind {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref.Kind,
			"kind must be '"+expectedKind+"'"))
	}

	// Additional validation for expected API version
	if ref.APIVersion != "" && ref.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), ref.APIVersion,
			"apiVersion must be 'netguard.sgroups.io/v1beta1'"))
	}

	return allErrs
}

// objectReferencesEqual checks if two ObjectReferences are equal
func (v *NetworkBindingValidator) objectReferencesEqual(ref1, ref2 v1beta1.ObjectReference) bool {
	return ref1.APIVersion == ref2.APIVersion &&
		ref1.Kind == ref2.Kind &&
		ref1.Name == ref2.Name
}

// isReadyConditionTrue checks if the Ready condition is true
func (v *NetworkBindingValidator) isReadyConditionTrue(obj *v1beta1.NetworkBinding) bool {
	for _, condition := range obj.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			return true
		}
	}
	return false
}
