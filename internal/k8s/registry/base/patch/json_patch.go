package patch

import (
	"encoding/json"
	"fmt"
	"reflect"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	"k8s.io/apimachinery/pkg/runtime"
)

// JSONPatchOperation represents a single JSON Patch operation according to RFC 6902
type JSONPatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
	From  string      `json:"from,omitempty"`
}

// SupportedOperations defines the JSON Patch operations supported according to RFC 6902
var SupportedOperations = map[string]bool{
	"add":     true,
	"remove":  true,
	"replace": true,
	"move":    true,
	"copy":    true,
	"test":    true,
}

// ParseJSONPatch parses JSON Patch data and returns a slice of JSONPatchOperation
// It validates the patch according to RFC 6902 specifications
func ParseJSONPatch(data []byte) ([]JSONPatchOperation, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty patch data")
	}

	var operations []JSONPatchOperation
	if err := json.Unmarshal(data, &operations); err != nil {
		return nil, fmt.Errorf("invalid JSON patch format: %w", err)
	}

	if len(operations) == 0 {
		return nil, fmt.Errorf("patch must contain at least one operation")
	}

	// Validate each operation according to RFC 6902
	for i, op := range operations {
		if err := validateOperation(op, i); err != nil {
			return nil, err
		}
	}

	return operations, nil
}

// validateOperation validates a single JSON Patch operation according to RFC 6902
func validateOperation(op JSONPatchOperation, index int) error {
	// Validate operation type
	if !SupportedOperations[op.Op] {
		return fmt.Errorf("operation %d: unsupported operation '%s'", index, op.Op)
	}

	// Validate path is present and starts with '/'
	if op.Path == "" {
		return fmt.Errorf("operation %d: path is required", index)
	}
	if op.Path[0] != '/' {
		return fmt.Errorf("operation %d: path must start with '/'", index)
	}

	// Validate operation-specific requirements
	switch op.Op {
	case "add", "replace", "test":
		if op.Value == nil {
			return fmt.Errorf("operation %d: '%s' operation requires a value", index, op.Op)
		}
	case "move", "copy":
		if op.From == "" {
			return fmt.Errorf("operation %d: '%s' operation requires a 'from' field", index, op.Op)
		}
		if op.From[0] != '/' {
			return fmt.Errorf("operation %d: 'from' field must start with '/'", index)
		}
	case "remove":
		// Remove operation doesn't require value or from
	}

	return nil
}

// ApplyJSONPatch applies a JSON Patch to a runtime.Object and returns the patched object
func ApplyJSONPatch(obj runtime.Object, patchData []byte) (runtime.Object, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot apply patch to nil object")
	}

	// Validate the patch format first
	if _, err := ParseJSONPatch(patchData); err != nil {
		return nil, fmt.Errorf("invalid JSON patch: %w", err)
	}

	// Convert the runtime.Object to JSON using standard marshaling
	originalJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	// Create the JSON patch
	patch, err := jsonpatch.DecodePatch(patchData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON patch: %w", err)
	}

	// Apply the patch
	patchedJSON, err := patch.Apply(originalJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to apply JSON patch: %w", err)
	}

	// Create a fresh object using reflection to properly handle field removals
	objValue := reflect.ValueOf(obj)
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
	}
	objType := objValue.Type()

	// Create a new zero-value instance of the same type
	freshObj := reflect.New(objType).Interface().(runtime.Object)

	// Unmarshal the patched JSON into the fresh object
	if err := json.Unmarshal(patchedJSON, freshObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patched JSON: %w", err)
	}

	return freshObj, nil
}
