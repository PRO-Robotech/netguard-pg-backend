package patch

import (
	"encoding/json"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
)

// ApplyMergePatch applies a Merge Patch to a runtime.Object according to RFC 7396
// and returns the patched object. Merge Patch uses simple JSON merge semantics:
// - Fields in the patch replace corresponding fields in the target
// - Null values in the patch delete corresponding fields in the target
// - Objects are merged recursively
// - Arrays are replaced entirely (not merged)
func ApplyMergePatch(obj runtime.Object, patchData []byte) (runtime.Object, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot apply merge patch to nil object")
	}

	if len(patchData) == 0 {
		return nil, fmt.Errorf("empty merge patch data")
	}

	// Validate that patch data is valid JSON
	var patchObj interface{}
	if err := json.Unmarshal(patchData, &patchObj); err != nil {
		return nil, fmt.Errorf("invalid merge patch JSON: %w", err)
	}

	// Merge patch must be a JSON object, not array or primitive
	if _, ok := patchObj.(map[string]interface{}); !ok {
		return nil, fmt.Errorf("merge patch must be a JSON object")
	}

	// Convert the runtime.Object to JSON
	originalJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	// Parse original JSON into a map for merging
	var originalMap map[string]interface{}
	if err := json.Unmarshal(originalJSON, &originalMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal original object: %w", err)
	}

	// Parse patch JSON into a map
	var patchMap map[string]interface{}
	if err := json.Unmarshal(patchData, &patchMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patch data: %w", err)
	}

	// Apply merge patch according to RFC 7396
	mergedMap, err := applyMergeToMap(originalMap, patchMap)
	if err != nil {
		return nil, fmt.Errorf("failed to apply merge patch: %w", err)
	}

	// Convert merged map back to JSON
	mergedJSON, err := json.Marshal(mergedMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged object: %w", err)
	}

	// Create a fresh object using reflection to properly handle field changes
	objValue := reflect.ValueOf(obj)
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
	}
	objType := objValue.Type()

	// Create a new zero-value instance of the same type
	freshObj := reflect.New(objType).Interface().(runtime.Object)

	// Unmarshal the merged JSON into the fresh object
	if err := json.Unmarshal(mergedJSON, freshObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal merged JSON: %w", err)
	}

	return freshObj, nil
}

// applyMergeToMap applies merge patch semantics to maps according to RFC 7396
func applyMergeToMap(original, patch map[string]interface{}) (map[string]interface{}, error) {
	if original == nil {
		original = make(map[string]interface{})
	}

	// Create a copy of the original map to avoid modifying the input
	result := make(map[string]interface{})
	for k, v := range original {
		result[k] = v
	}

	// Apply patch according to RFC 7396 rules
	for key, patchValue := range patch {
		if patchValue == nil {
			// Null values delete the field
			delete(result, key)
		} else if patchMap, isPatchObject := patchValue.(map[string]interface{}); isPatchObject {
			// If patch value is an object, merge recursively
			if originalValue, exists := result[key]; exists {
				if originalMap, isOriginalObject := originalValue.(map[string]interface{}); isOriginalObject {
					// Both are objects, merge recursively
					merged, err := applyMergeToMap(originalMap, patchMap)
					if err != nil {
						return nil, fmt.Errorf("failed to merge nested object at key '%s': %w", key, err)
					}
					result[key] = merged
				} else {
					// Original is not an object, replace with patch object
					result[key] = patchValue
				}
			} else {
				// Key doesn't exist in original, add the patch object
				result[key] = patchValue
			}
		} else {
			// For all other values (primitives, arrays), replace entirely
			result[key] = patchValue
		}
	}

	return result, nil
}

// ValidateMergePatch validates that the provided data is a valid merge patch
func ValidateMergePatch(patchData []byte) error {
	if len(patchData) == 0 {
		return fmt.Errorf("empty merge patch data")
	}

	var patchObj interface{}
	if err := json.Unmarshal(patchData, &patchObj); err != nil {
		return fmt.Errorf("invalid merge patch JSON: %w", err)
	}

	// Merge patch must be a JSON object
	if _, ok := patchObj.(map[string]interface{}); !ok {
		return fmt.Errorf("merge patch must be a JSON object")
	}

	return nil
}
