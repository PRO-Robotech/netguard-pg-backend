package v1beta1

import (
	"fmt"
)

// HostBindingFieldLabelConversion validates and converts field labels for HostBinding resources.
// It ensures that only supported field selectors are allowed by Kubernetes APIServer.
//
// This function is called by the Kubernetes APIServer when processing field selectors
// in kubectl queries like: kubectl get hostbindings --field-selector=spec.addressGroupRef.name=example
//
// Supported fields:
//   - metadata.name (standard Kubernetes field)
//   - metadata.namespace (standard Kubernetes field)
//   - spec.hostRef.name (custom field - filters by host reference name)
//   - spec.hostRef.namespace (custom field - filters by host reference namespace)
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
func HostBindingFieldLabelConversion(label, value string) (string, string, error) {
	switch label {
	case "metadata.name",
		"metadata.namespace",
		"spec.hostRef.name",
		"spec.hostRef.namespace",
		"spec.addressGroupRef.name",
		"spec.addressGroupRef.namespace":
		// Field is supported - return as-is
		return label, value, nil
	default:
		// Field is not supported - return error
		return "", "", fmt.Errorf("field label %q not supported for HostBinding: only metadata.name, metadata.namespace, spec.hostRef.name, spec.hostRef.namespace, spec.addressGroupRef.name, spec.addressGroupRef.namespace are supported", label)
	}
}
