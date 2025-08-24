package patch

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// ApplyStrategicMergePatch applies a Strategic Merge Patch to a runtime.Object
// using Kubernetes strategic merge patch semantics. Strategic merge patch provides
// more sophisticated merging for arrays and objects based on struct tags and
// merge strategies defined in the Kubernetes API types.
//
// Strategic merge patch supports:
// - patchMergeKey: specifies the key to use for merging array elements
// - patchStrategy: defines merge strategy (merge, replace, retainKeys)
// - Array merging based on merge keys rather than replacement
// - Directive handling ($patch, $retainKeys, $deleteFromPrimitiveList)
func ApplyStrategicMergePatch(obj runtime.Object, patchData []byte) (runtime.Object, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot apply strategic merge patch to nil object")
	}

	if len(patchData) == 0 {
		return nil, fmt.Errorf("empty strategic merge patch data")
	}

	// Validate that patch data is valid JSON
	var patchObj interface{}
	if err := json.Unmarshal(patchData, &patchObj); err != nil {
		return nil, fmt.Errorf("invalid strategic merge patch JSON: %w", err)
	}

	// Strategic merge patch must be a JSON object, not array or primitive
	if _, ok := patchObj.(map[string]interface{}); !ok {
		return nil, fmt.Errorf("strategic merge patch must be a JSON object")
	}

	// Convert the runtime.Object to JSON
	originalJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	// Get the type of the object for strategic merge patch
	objValue := reflect.ValueOf(obj)
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
	}
	objType := objValue.Type()

	// Apply strategic merge patch using k8s.io/apimachinery
	// For custom types that don't have strategic merge metadata, fall back to merge patch behavior
	patchedJSON, err := strategicpatch.StrategicMergePatch(originalJSON, patchData, objType)
	if err != nil {
		// If strategic merge patch fails due to missing metadata, fall back to merge patch
		if isStrategicMergeUnsupportedError(err) {
			return applyFallbackMergePatch(obj, patchData)
		}
		return nil, fmt.Errorf("failed to apply strategic merge patch: %w", err)
	}

	// Create a fresh object using reflection to properly handle field changes
	freshObj := reflect.New(objType).Interface().(runtime.Object)

	// Unmarshal the patched JSON into the fresh object
	if err := json.Unmarshal(patchedJSON, freshObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patched JSON: %w", err)
	}

	return freshObj, nil
}

// ValidateStrategicMergePatch validates that the provided data is a valid strategic merge patch
func ValidateStrategicMergePatch(patchData []byte) error {
	if len(patchData) == 0 {
		return fmt.Errorf("empty strategic merge patch data")
	}

	var patchObj interface{}
	if err := json.Unmarshal(patchData, &patchObj); err != nil {
		return fmt.Errorf("invalid strategic merge patch JSON: %w", err)
	}

	// Strategic merge patch must be a JSON object
	if _, ok := patchObj.(map[string]interface{}); !ok {
		return fmt.Errorf("strategic merge patch must be a JSON object")
	}

	return nil
}

// CreateStrategicMergePatch creates a strategic merge patch between two objects
// This is useful for generating patches programmatically
func CreateStrategicMergePatch(original, modified runtime.Object) ([]byte, error) {
	if original == nil || modified == nil {
		return nil, fmt.Errorf("both original and modified objects must be non-nil")
	}

	// Ensure both objects are of the same type
	originalType := reflect.TypeOf(original)
	modifiedType := reflect.TypeOf(modified)
	if originalType != modifiedType {
		return nil, fmt.Errorf("original and modified objects must be of the same type")
	}

	// Convert objects to JSON
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal original object: %w", err)
	}

	modifiedJSON, err := json.Marshal(modified)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal modified object: %w", err)
	}

	// Get the struct type for strategic merge patch
	objValue := reflect.ValueOf(original)
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
	}
	objType := objValue.Type()

	// Create strategic merge patch
	patchData, err := strategicpatch.CreateTwoWayMergePatch(originalJSON, modifiedJSON, objType)
	if err != nil {
		// If strategic merge patch creation fails, fall back to simple JSON diff
		if isStrategicMergeUnsupportedError(err) {
			return createFallbackMergePatch(originalJSON, modifiedJSON)
		}
		return nil, fmt.Errorf("failed to create strategic merge patch: %w", err)
	}

	return patchData, nil
}

// ApplyStrategicMergePatchWithOptions applies a strategic merge patch with additional options
// This provides more control over the merge process
func ApplyStrategicMergePatchWithOptions(obj runtime.Object, patchData []byte, options StrategicMergePatchOptions) (runtime.Object, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot apply strategic merge patch to nil object")
	}

	if len(patchData) == 0 {
		return nil, fmt.Errorf("empty strategic merge patch data")
	}

	// For now, delegate to the standard implementation
	// In the future, this could support additional options like:
	// - Custom merge strategies
	// - Validation hooks
	// - Transformation callbacks
	return ApplyStrategicMergePatch(obj, patchData)
}

// StrategicMergePatchOptions provides additional configuration for strategic merge patch operations
type StrategicMergePatchOptions struct {
	// IgnoreUnknownFields controls whether unknown fields should be ignored
	IgnoreUnknownFields bool

	// StrictValidation enables strict validation of patch operations
	StrictValidation bool

	// CustomMergeStrategies allows overriding default merge strategies
	CustomMergeStrategies map[string]string
}

// isStrategicMergeUnsupportedError checks if the error is due to missing strategic merge metadata
func isStrategicMergeUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Check for common strategic merge patch errors indicating missing metadata
	return strings.Contains(errStr, "unable to find api field") ||
		strings.Contains(errStr, "no kind is registered") ||
		strings.Contains(errStr, "strategic merge patch format is not supported")
}

// applyFallbackMergePatch applies a fallback merge patch when strategic merge patch is not supported
func applyFallbackMergePatch(obj runtime.Object, patchData []byte) (runtime.Object, error) {
	// Use the existing merge patch implementation as fallback
	return ApplyMergePatch(obj, patchData)
}

// createFallbackMergePatch creates a simple merge patch when strategic merge patch creation fails
func createFallbackMergePatch(originalJSON, modifiedJSON []byte) ([]byte, error) {
	// Parse both JSON objects
	var original, modified map[string]interface{}

	if err := json.Unmarshal(originalJSON, &original); err != nil {
		return nil, fmt.Errorf("failed to unmarshal original JSON: %w", err)
	}

	if err := json.Unmarshal(modifiedJSON, &modified); err != nil {
		return nil, fmt.Errorf("failed to unmarshal modified JSON: %w", err)
	}

	// Create a simple diff patch
	patch := createSimpleDiff(original, modified)

	return json.Marshal(patch)
}

// createSimpleDiff creates a simple diff between two maps
func createSimpleDiff(original, modified map[string]interface{}) map[string]interface{} {
	patch := make(map[string]interface{})

	// Find changed and new fields
	for key, modifiedValue := range modified {
		originalValue, exists := original[key]
		if !exists || !deepEqual(originalValue, modifiedValue) {
			patch[key] = modifiedValue
		}
	}

	// Find deleted fields (set to null)
	for key := range original {
		if _, exists := modified[key]; !exists {
			patch[key] = nil
		}
	}

	return patch
}

// deepEqual performs deep equality check for interface{} values
func deepEqual(a, b interface{}) bool {
	aJSON, err1 := json.Marshal(a)
	bJSON, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}
