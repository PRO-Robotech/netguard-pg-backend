package validation

import (
	"context"
	"fmt"
	"net"

	appvalidation "netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// IECidrSvcRuleValidator implements Kubernetes-level validation for IECidrSvcRule resources.
// It focuses on fast structural checks that should happen before hitting the backend layer.
type IECidrSvcRuleValidator struct {
	helpers *ValidationHelpers
}

var _ base.Validator[*netguardv1beta1.IECidrSvcRule] = &IECidrSvcRuleValidator{}

// NewIECidrSvcRuleValidator creates a new IECidrSvcRuleValidator instance.
func NewIECidrSvcRuleValidator() *IECidrSvcRuleValidator {
	return &IECidrSvcRuleValidator{helpers: NewValidationHelpers()}
}

// ValidateCreate validates an IECidrSvcRule on creation.
func (v *IECidrSvcRuleValidator) ValidateCreate(ctx context.Context, obj *netguardv1beta1.IECidrSvcRule) field.ErrorList {
	return v.validate(ctx, obj, nil)
}

// ValidateUpdate validates an IECidrSvcRule on update and enforces immutability rules.
func (v *IECidrSvcRuleValidator) ValidateUpdate(ctx context.Context, obj *netguardv1beta1.IECidrSvcRule, old *netguardv1beta1.IECidrSvcRule) field.ErrorList {
	allErrs := v.validate(ctx, obj, old)
	if obj == nil || old == nil {
		return allErrs
	}

	// Immutable fields: CIDR, Svc, Traffic
	if obj.Spec.CIDR != old.Spec.CIDR {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "cidr"), obj.Spec.CIDR,
			"cidr is immutable once the rule is created"))
	}
	if obj.Spec.Svc != old.Spec.Svc {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "svc"), obj.Spec.Svc,
			"svc is immutable once the rule is created"))
	}
	if obj.Spec.Traffic != old.Spec.Traffic {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "traffic"), obj.Spec.Traffic,
			"traffic is immutable once the rule is created"))
	}

	return allErrs
}

// ValidateDelete currently does not impose additional constraints.
func (v *IECidrSvcRuleValidator) ValidateDelete(ctx context.Context, obj *netguardv1beta1.IECidrSvcRule) field.ErrorList {
	return field.ErrorList{}
}

func (v *IECidrSvcRuleValidator) validate(ctx context.Context, obj *netguardv1beta1.IECidrSvcRule, old *netguardv1beta1.IECidrSvcRule) field.ErrorList {
	allErrs := field.ErrorList{}
	if obj == nil {
		return append(allErrs, field.Required(field.NewPath(""), "iecidrsvc_rule object cannot be nil"))
	}

	// Metadata validation (name / generateName / labels / annotations / namespace).
	allErrs = append(allErrs, ValidateStandardObjectMeta(&obj.ObjectMeta, field.NewPath("metadata"))...)

	// Spec validation.
	allErrs = append(allErrs, v.validateSpec(obj, field.NewPath("spec"))...)

	return allErrs
}

func (v *IECidrSvcRuleValidator) validateSpec(obj *netguardv1beta1.IECidrSvcRule, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	spec := obj.Spec

	// Transport validation
	allErrs = append(allErrs, ValidateTransportProtocol(spec.Transport, fldPath.Child("transport"))...)

	// CIDR validation - full validation with net.ParseCIDR
	allErrs = append(allErrs, v.validateCIDR(spec.CIDR, fldPath.Child("cidr"))...)

	// Svc (service reference) validation
	allErrs = append(allErrs, ValidateNamespacedObjectReference(&spec.Svc, fldPath.Child("svc"))...)
	if spec.Svc.Kind != "Service" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("svc").Child("kind"), spec.Svc.Kind,
			"svc.kind must be 'Service'"))
	}
	if spec.Svc.APIVersion != "netguard.sgroups.io/v1beta1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("svc").Child("apiVersion"), spec.Svc.APIVersion,
			"svc.apiVersion must be 'netguard.sgroups.io/v1beta1'"))
	}
	if spec.Svc.Namespace != "" && spec.Svc.Namespace != obj.Namespace {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("svc").Child("namespace"), spec.Svc.Namespace,
			fmt.Sprintf("svc.namespace must match IECidrSvcRule namespace (%s)", obj.Namespace)))
	}

	// Traffic validation
	allErrs = append(allErrs, v.validateTraffic(spec.Traffic, fldPath.Child("traffic"))...)

	// Ports validation
	allErrs = append(allErrs, v.validatePorts(spec.Ports, fldPath.Child("ports"))...)

	// Action validation (optional but must be valid if provided)
	if spec.Action != "" {
		allErrs = append(allErrs, ValidateRuleAction(spec.Action, fldPath.Child("action"))...)
	}

	// Description length guard (512 chars to align with other resources)
	allErrs = append(allErrs, ValidateDescription(spec.Description, fldPath.Child("description"), 512)...)

	return allErrs
}

func (v *IECidrSvcRuleValidator) validateCIDR(cidr string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if cidr == "" {
		allErrs = append(allErrs, field.Required(fldPath, "cidr is required"))
		return allErrs
	}

	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, cidr,
			fmt.Sprintf("invalid CIDR notation: %v", err)))
	}

	return allErrs
}

func (v *IECidrSvcRuleValidator) validateTraffic(traffic netguardv1beta1.Traffic, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	switch traffic {
	case netguardv1beta1.INGRESS, netguardv1beta1.EGRESS:
		// Valid
	case "":
		allErrs = append(allErrs, field.Required(fldPath, "traffic direction is required"))
	default:
		allErrs = append(allErrs, field.Invalid(fldPath, traffic,
			"traffic must be either 'INGRESS' or 'EGRESS'"))
	}
	return allErrs
}

func (v *IECidrSvcRuleValidator) validatePorts(ports []netguardv1beta1.IECidrSvcPortSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(ports) == 0 {
		allErrs = append(allErrs, field.Required(fldPath, "ports must contain at least one entry"))
		return allErrs
	}

	var allDstPortRanges []models.PortRange
	for i, port := range ports {
		portPath := fldPath.Index(i)

		// Destination port (D) is required
		if port.D == "" {
			allErrs = append(allErrs, field.Required(portPath.Child("d"), "destination port (d) is required"))
			continue
		}

		// Parse destination port string
		dstRanges, err := appvalidation.ParsePortRanges(port.D)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(portPath.Child("d"), port.D, err.Error()))
		} else {
			allDstPortRanges = append(allDstPortRanges, dstRanges...)
		}

		// Source port (S) is optional, but validate if provided
		if port.S != "" {
			_, err := appvalidation.ParsePortRanges(port.S)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(portPath.Child("s"), port.S, err.Error()))
			}
		}
	}

	// Check for destination port overlaps across all entries
	if len(allDstPortRanges) > 0 {
		if err := appvalidation.CheckPortRangeOverlapsOptimized(allDstPortRanges, "d"); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath, "d", err.Error()))
		}
	}

	return allErrs
}
