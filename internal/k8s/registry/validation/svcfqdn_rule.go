package validation

import (
	"context"
	"fmt"

	appvalidation "netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// SvcFqdnRuleValidator implements Kubernetes-level validation for SvcFqdnRule resources.
// It focuses on fast structural checks that should happen before hitting the backend layer.
type SvcFqdnRuleValidator struct {
	helpers *ValidationHelpers
}

var _ base.Validator[*netguardv1beta1.SvcFqdnRule] = &SvcFqdnRuleValidator{}

// NewSvcFqdnRuleValidator creates a new SvcFqdnRuleValidator instance.
func NewSvcFqdnRuleValidator() *SvcFqdnRuleValidator {
	return &SvcFqdnRuleValidator{helpers: NewValidationHelpers()}
}

// ValidateCreate validates a SvcFqdnRule on creation.
func (v *SvcFqdnRuleValidator) ValidateCreate(ctx context.Context, obj *netguardv1beta1.SvcFqdnRule) field.ErrorList {
	return v.validate(ctx, obj, nil)
}

// ValidateUpdate validates a SvcFqdnRule on update and enforces immutability rules.
func (v *SvcFqdnRuleValidator) ValidateUpdate(ctx context.Context, obj *netguardv1beta1.SvcFqdnRule, old *netguardv1beta1.SvcFqdnRule) field.ErrorList {
	allErrs := v.validate(ctx, obj, old)
	if obj == nil || old == nil {
		return allErrs
	}

	// Immutable fields: ServiceFrom (fully qualified), FQDN, Transport.
	if obj.Spec.ServiceFrom != old.Spec.ServiceFrom {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "serviceFrom"), obj.Spec.ServiceFrom,
			"serviceFrom is immutable once the rule is created"))
	}
	if obj.Spec.FQDN != old.Spec.FQDN {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "fqdn"), obj.Spec.FQDN,
			"fqdn is immutable once the rule is created"))
	}
	if obj.Spec.Transport != old.Spec.Transport {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "transport"), obj.Spec.Transport,
			"transport is immutable once the rule is created"))
	}

	return allErrs
}

// ValidateDelete currently does not impose additional constraints.
func (v *SvcFqdnRuleValidator) ValidateDelete(ctx context.Context, obj *netguardv1beta1.SvcFqdnRule) field.ErrorList {
	return field.ErrorList{}
}

func (v *SvcFqdnRuleValidator) validate(ctx context.Context, obj *netguardv1beta1.SvcFqdnRule, old *netguardv1beta1.SvcFqdnRule) field.ErrorList {
	allErrs := field.ErrorList{}
	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "svcfqdn_rule object cannot be nil"))
	}

	// Metadata validation (name / generateName / labels / annotations / namespace).
	allErrs = append(allErrs, ValidateStandardObjectMeta(&obj.ObjectMeta, field.NewPath("metadata"))...)

	// Spec validation.
	allErrs = append(allErrs, v.validateSpec(obj, field.NewPath("spec"))...)

	return allErrs
}

func (v *SvcFqdnRuleValidator) validateSpec(obj *netguardv1beta1.SvcFqdnRule, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	spec := obj.Spec

	// ServiceFrom validation.
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&spec.ServiceFrom, fldPath.Child("serviceFrom"))...)
	if spec.ServiceFrom.Kind != "Service" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("serviceFrom").Child("kind"), spec.ServiceFrom.Kind,
			"serviceFrom.kind must be 'Service'"))
	}
	if spec.ServiceFrom.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("serviceFrom").Child("apiVersion"), spec.ServiceFrom.APIVersion,
			"serviceFrom.apiVersion must be 'netguard.sgroups.io/v1beta1'"))
	}
	if spec.ServiceFrom.Namespace != "" && spec.ServiceFrom.Namespace != obj.Namespace {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("serviceFrom").Child("namespace"), spec.ServiceFrom.Namespace,
			fmt.Sprintf("serviceFrom.namespace must match SvcFqdnRule namespace (%s)", obj.Namespace)))
	}

	// FQDN validation. CRD enforces pattern, but we still guard emptiness for clarity.
	allErrs = append(allErrs, v.helpers.ValidateRequiredString(spec.FQDN, fldPath.Child("fqdn"), "fqdn")...)

	// Transport validation (Enum is handled in CRD, but we keep consistent check here).
	allErrs = append(allErrs, ValidateTransportProtocol(spec.Transport, fldPath.Child("transport"))...)

	// Ports validation: ensure every entry parses and there are no overlaps in destinations.
	allErrs = append(allErrs, v.validatePorts(spec.Ports, fldPath.Child("ports"))...)

	// Action validation (optional but must be valid if provided).
	if spec.Action != "" {
		allErrs = append(allErrs, ValidateRuleAction(spec.Action, fldPath.Child("action"))...)
	}

	// Priority bounds already enforced by CRD annotations; no extra logic.

	// Description length guard (512 chars to align with other resources).
	allErrs = append(allErrs, ValidateDescription(spec.Description, fldPath.Child("description"), 512)...)

	return allErrs
}

func (v *SvcFqdnRuleValidator) validatePorts(ports []netguardv1beta1.SvcFqdnPortSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(ports) == 0 {
		return allErrs
	}

	var destinationRanges []models.PortRange
	for i, port := range ports {
		portPath := fldPath.Index(i)
		if errList := v.helpers.ValidateRequiredString(port.Destination, portPath.Child("destination"), "destination"); len(errList) > 0 {
			allErrs = append(allErrs, errList...)
			continue
		}

		ranges, err := appvalidation.ParsePortRanges(port.Destination)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(portPath.Child("destination"), port.Destination, err.Error()))
			continue
		}
		destinationRanges = append(destinationRanges, ranges...)

		if port.Source != "" {
			if _, err := appvalidation.ParsePortRanges(port.Source); err != nil {
				allErrs = append(allErrs, field.Invalid(portPath.Child("source"), port.Source, err.Error()))
			}
		}
	}

	if len(destinationRanges) > 0 {
		if err := appvalidation.CheckPortRangeOverlapsOptimized(destinationRanges, "destination"); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath, "destination", err.Error()))
		}
	}

	return allErrs
}
