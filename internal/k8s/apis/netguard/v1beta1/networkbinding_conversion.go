package v1beta1

import (
	"fmt"
)

// NetworkBindingFieldLabelConversion validates and converts field labels for NetworkBinding resources.
// It ensures that only supported field selectors are allowed by Kubernetes APIServer.
//
// This function is called by the Kubernetes APIServer when processing field selectors
// in kubectl queries like: kubectl get networkbindings --field-selector=spec.addressGroupRef.name=example
//
// Supported fields:
//   - metadata.name (standard Kubernetes field)
//   - metadata.namespace (standard Kubernetes field)
//   - spec.networkRef.name (custom field - filters by network reference name)
//   - spec.networkRef.namespace (custom field - filters by network reference namespace)
//   - spec.addressGroupRef.name (custom field - filters by address group reference name)
//   - spec.addressGroupRef.namespace (custom field - filters by address group reference namespace)
//
// The actual filtering is performed by:
// 1. APIServer parses field selector and calls this function for validation
// 2. BaseStorage.List() receives validated field selector
// 3. selector_parser.go converts to protobuf format
// 4. gRPC client sends to backend
// 5. Backend handlers create FieldSelectorScope
// 6. SQL Builder generates WHERE clause using field_mapping.go
// 7. PostgreSQL executes filtered query
func NetworkBindingFieldLabelConversion(label, value string) (string, string, error) {
	switch label {
	case "metadata.name",
		"metadata.namespace",
		"spec.networkRef.name",
		"spec.networkRef.namespace",
		"spec.addressGroupRef.name",
		"spec.addressGroupRef.namespace":
		// Field is supported - return as-is
		return label, value, nil
	default:
		// Field is not supported - return error
		return "", "", fmt.Errorf("field label %q not supported for NetworkBinding: only metadata.name, metadata.namespace, spec.networkRef.name, spec.networkRef.namespace, spec.addressGroupRef.name, spec.addressGroupRef.namespace are supported", label)
	}
}
