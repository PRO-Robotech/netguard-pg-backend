package v1beta1

import "fmt"

// IECidrSvcRuleFieldLabelConversion validates field labels for IECidrSvcRule resources.
// Supported selectors:
//   - metadata.name / metadata.namespace
//   - spec.svc.name / spec.svc.namespace
//   - spec.cidr
//   - spec.traffic
func IECidrSvcRuleFieldLabelConversion(label, value string) (string, string, error) {
	switch label {
	case "metadata.name",
		"metadata.namespace",
		"spec.svc.name",
		"spec.svc.namespace",
		"spec.cidr",
		"spec.traffic":
		return label, value, nil
	default:
		return "", "", fmt.Errorf("field label %q not supported for IECidrSvcRule", label)
	}
}
