package validation

import (
	"context"
	"regexp"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

var (
	// UUID validation regex - standard UUID format
	uuidRegex = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
)

// HostValidator validates Host resources
type HostValidator struct {
	helpers *ValidationHelpers
}

// NewHostValidator creates a new HostValidator
func NewHostValidator() *HostValidator {
	return &HostValidator{
		helpers: NewValidationHelpers(),
	}
}

// ValidateCreate validates a new Host being created
func (v *HostValidator) ValidateCreate(ctx context.Context, obj *v1beta1.Host) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "host object cannot be nil"))
	}

	// Validate metadata
	allErrs = append(allErrs, v.validateMetadata(obj)...)

	// Validate spec
	allErrs = append(allErrs, v.validateSpec(obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

// ValidateUpdate validates a Host being updated
func (v *HostValidator) ValidateUpdate(ctx context.Context, obj, oldObj *v1beta1.Host) field.ErrorList {
	allErrs := v.ValidateCreate(ctx, obj)

	if obj == nil || oldObj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "host object cannot be nil for update"))
	}

	// Validate that immutable fields haven't changed
	allErrs = append(allErrs, v.validateImmutableFields(obj, oldObj)...)

	return allErrs
}

// ValidateStatusUpdate validates a Host status being updated
func (v *HostValidator) ValidateStatusUpdate(ctx context.Context, obj, oldObj *v1beta1.Host) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "host object cannot be nil"))
	}

	// Validate status fields
	allErrs = append(allErrs, v.validateStatus(obj.Status, field.NewPath("status"))...)

	return allErrs
}

// validateMetadata validates the Host metadata
func (v *HostValidator) validateMetadata(obj *v1beta1.Host) field.ErrorList {
	allErrs := field.ErrorList{}
	path := field.NewPath("metadata")

	// Standard metadata validation
	allErrs = append(allErrs, ValidateStandardObjectMeta(&obj.ObjectMeta, path)...)

	return allErrs
}

// validateSpec validates the Host spec
func (v *HostValidator) validateSpec(spec v1beta1.HostSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate UUID - required and must be valid UUID format
	if spec.UUID == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("uuid"), "UUID is required"))
	} else if !uuidRegex.MatchString(spec.UUID) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("uuid"), spec.UUID, "UUID must be in valid UUID format"))
	}

	return allErrs
}

// validateStatus validates the Host status
func (v *HostValidator) validateStatus(status v1beta1.HostStatus, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate HostName if present
	if status.HostName != "" {
		// HostName should be reasonable length
		if len(status.HostName) > 255 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("hostName"), status.HostName, "hostName cannot exceed 255 characters"))
		}
	}

	// Validate binding consistency
	if status.IsBound {
		if status.BindingRef == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("bindingRef"), "bindingRef is required when isBound is true"))
		}
		if status.AddressGroupRef == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("addressGroupRef"), "addressGroupRef is required when isBound is true"))
		}
	} else {
		if status.BindingRef != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("bindingRef"), "bindingRef must be nil when isBound is false"))
		}
		if status.AddressGroupRef != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("addressGroupRef"), "addressGroupRef must be nil when isBound is false"))
		}
	}

	// Conditions are typically managed by the system and don't need validation during create/update operations

	return allErrs
}

// validateImmutableFields validates that immutable fields haven't changed
func (v *HostValidator) validateImmutableFields(obj, oldObj *v1beta1.Host) field.ErrorList {
	allErrs := field.ErrorList{}
	fldPath := field.NewPath("spec")

	// UUID is immutable
	if obj.Spec.UUID != oldObj.Spec.UUID {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("uuid"), "UUID is immutable"))
	}

	return allErrs
}

// ValidateDelete validates a Host being deleted
func (v *HostValidator) ValidateDelete(ctx context.Context, obj *v1beta1.Host) field.ErrorList {
	allErrs := field.ErrorList{}

	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "host object cannot be nil"))
	}

	return allErrs
}
