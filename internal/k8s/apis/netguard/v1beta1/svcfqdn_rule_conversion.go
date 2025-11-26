package v1beta1

import "fmt"

// SvcFqdnRuleFieldLabelConversion validates field labels for SvcFqdnRule resources.
// Supported selectors:
//   - metadata.name / metadata.namespace
//   - spec.serviceFrom.name / spec.serviceFrom.namespace
//   - spec.fqdn
func SvcFqdnRuleFieldLabelConversion(label, value string) (string, string, error) {
	switch label {
	case "metadata.name",
		"metadata.namespace",
		"spec.serviceFrom.name",
		"spec.serviceFrom.namespace",
		"spec.fqdn":
		return label, value, nil
	default:
		return "", "", fmt.Errorf("field label %q not supported for SvcFqdnRule", label)
	}
}
