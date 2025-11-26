package v1beta1

import "fmt"

// SvcSvcRuleFieldLabelConversion validates and converts field labels for SvcSvcRule resources.
// Enabled selectors:
//   - metadata.name / metadata.namespace
//   - spec.serviceFrom.name / spec.serviceFrom.namespace
//   - spec.serviceTo.name / spec.serviceTo.namespace
func SvcSvcRuleFieldLabelConversion(label, value string) (string, string, error) {
	switch label {
	case "metadata.name",
		"metadata.namespace",
		"spec.serviceFrom.name",
		"spec.serviceFrom.namespace",
		"spec.serviceTo.name",
		"spec.serviceTo.namespace":
		return label, value, nil
	default:
		return "", "", fmt.Errorf("field label %q not supported for SvcSvcRule", label)
	}
}
