package v1beta1

import (
	"fmt"
)

// HostFieldLabelConversion validates and converts field labels for Host resources.
// It ensures that only supported field selectors are allowed by Kubernetes APIServer.
//
// This function is called by the Kubernetes APIServer when processing field selectors
// in kubectl queries like: kubectl get hosts --field-selector=status.isBound=true
//
// Supported fields:
//   - metadata.name (standard Kubernetes field)
//   - metadata.namespace (standard Kubernetes field)
//   - status.isBound (custom field - filters by binding status)
//   - status.addressGroupRef.name (custom field - filters by address group name)
//   - status.addressGroupRef.namespace (custom field - filters by address group namespace)
//   - status.bindingRef.name (custom field - filters by binding name)
//   - status.bindingRef.namespace (custom field - filters by binding namespace)
//
// The actual filtering is performed by:
// 1. APIServer parses field selector and calls this function for validation
// 2. BaseStorage.List() receives validated field selector
// 3. selector_parser.go converts to protobuf format
// 4. gRPC client sends to backend
// 5. Backend handlers create FieldSelectorScope
// 6. SQL Builder generates WHERE clause using field_mapping.go
// 7. PostgreSQL executes filtered query
func HostFieldLabelConversion(label, value string) (string, string, error) {
	switch label {
	case "metadata.name",
		"metadata.namespace",
		"status.isBound",
		"status.addressGroupRef.name",
		"status.addressGroupRef.namespace",
		"status.bindingRef.name",
		"status.bindingRef.namespace":
		// Field is supported - return as-is
		return label, value, nil
	default:
		// Field is not supported - return error
		return "", "", fmt.Errorf("field label %q not supported for Host: only metadata.name, metadata.namespace, status.isBound, status.addressGroupRef.name, status.addressGroupRef.namespace, status.bindingRef.name, status.bindingRef.namespace are supported", label)
	}
}
