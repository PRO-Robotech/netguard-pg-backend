package patch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// E2ETestObject represents a complex object for end-to-end patch testing
type E2ETestObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              E2ETestSpec   `json:"spec,omitempty"`
	Status            E2ETestStatus `json:"status,omitempty"`
}

type E2ETestSpec struct {
	Name         string                 `json:"name,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Labels       map[string]string      `json:"labels,omitempty"`
	Ports        []int                  `json:"ports,omitempty"`
	Config       map[string]interface{} `json:"config,omitempty"`
	NestedObject *NestedConfig          `json:"nestedObject,omitempty"`
}

type E2ETestStatus struct {
	Phase      string            `json:"phase,omitempty"`
	Message    string            `json:"message,omitempty"`
	Conditions []StatusCondition `json:"conditions,omitempty"`
}

type NestedConfig struct {
	Enabled    bool              `json:"enabled"`
	Settings   map[string]string `json:"settings,omitempty"`
	Parameters []string          `json:"parameters,omitempty"`
}

type StatusCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// DeepCopyObject implements runtime.Object
func (e *E2ETestObject) DeepCopyObject() runtime.Object {
	if e == nil {
		return nil
	}
	out := new(E2ETestObject)
	e.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all properties of this object into another object of the same type
func (e *E2ETestObject) DeepCopyInto(out *E2ETestObject) {
	*out = *e
	out.TypeMeta = e.TypeMeta
	e.ObjectMeta.DeepCopyInto(&out.ObjectMeta)

	// Deep copy spec
	out.Spec = E2ETestSpec{
		Name:        e.Spec.Name,
		Description: e.Spec.Description,
	}

	if e.Spec.Labels != nil {
		out.Spec.Labels = make(map[string]string, len(e.Spec.Labels))
		for k, v := range e.Spec.Labels {
			out.Spec.Labels[k] = v
		}
	}

	if e.Spec.Ports != nil {
		out.Spec.Ports = make([]int, len(e.Spec.Ports))
		copy(out.Spec.Ports, e.Spec.Ports)
	}

	if e.Spec.Config != nil {
		out.Spec.Config = make(map[string]interface{}, len(e.Spec.Config))
		for k, v := range e.Spec.Config {
			out.Spec.Config[k] = v
		}
	}

	if e.Spec.NestedObject != nil {
		out.Spec.NestedObject = &NestedConfig{
			Enabled: e.Spec.NestedObject.Enabled,
		}
		if e.Spec.NestedObject.Settings != nil {
			out.Spec.NestedObject.Settings = make(map[string]string, len(e.Spec.NestedObject.Settings))
			for k, v := range e.Spec.NestedObject.Settings {
				out.Spec.NestedObject.Settings[k] = v
			}
		}
		if e.Spec.NestedObject.Parameters != nil {
			out.Spec.NestedObject.Parameters = make([]string, len(e.Spec.NestedObject.Parameters))
			copy(out.Spec.NestedObject.Parameters, e.Spec.NestedObject.Parameters)
		}
	}

	// Deep copy status
	out.Status = E2ETestStatus{
		Phase:   e.Status.Phase,
		Message: e.Status.Message,
	}

	if e.Status.Conditions != nil {
		out.Status.Conditions = make([]StatusCondition, len(e.Status.Conditions))
		for i, condition := range e.Status.Conditions {
			out.Status.Conditions[i] = StatusCondition{
				Type:    condition.Type,
				Status:  condition.Status,
				Reason:  condition.Reason,
				Message: condition.Message,
			}
		}
	}
}

func createE2ETestObject() *E2ETestObject {
	return &E2ETestObject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "E2ETest",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-test-object",
			Namespace: "default",
			Labels: map[string]string{
				"app":     "netguard",
				"version": "v1.0.0",
			},
		},
		Spec: E2ETestSpec{
			Name:        "e2e-test-spec",
			Description: "Original description for e2e testing",
			Labels: map[string]string{
				"env":  "test",
				"type": "e2e",
			},
			Ports: []int{8080, 9090, 3000},
			Config: map[string]interface{}{
				"timeout":    30,
				"retries":    3,
				"enableAuth": true,
				"logLevel":   "info",
				"endpoints":  []string{"api", "health", "metrics"},
				"databases": map[string]interface{}{
					"primary": map[string]interface{}{
						"host": "localhost",
						"port": 5432,
					},
					"replica": map[string]interface{}{
						"host": "replica-host",
						"port": 5433,
					},
				},
			},
			NestedObject: &NestedConfig{
				Enabled: true,
				Settings: map[string]string{
					"mode":     "production",
					"debug":    "false",
					"compress": "true",
				},
				Parameters: []string{"param1", "param2", "param3"},
			},
		},
		Status: E2ETestStatus{
			Phase:   "Running",
			Message: "All systems operational",
			Conditions: []StatusCondition{
				{
					Type:    "Ready",
					Status:  "True",
					Reason:  "AllComponentsReady",
					Message: "All components are ready",
				},
				{
					Type:    "Healthy",
					Status:  "True",
					Reason:  "HealthCheckPassed",
					Message: "Health check passed",
				},
			},
		},
	}
}

// TestE2E_JSONPatch_CompleteWorkflow tests JSON Patch operations in a comprehensive E2E scenario
func TestE2E_JSONPatch_CompleteWorkflow(t *testing.T) {
	original := createE2ETestObject()

	t.Run("Add new field and modify existing", func(t *testing.T) {
		patch := `[
			{"op": "add", "path": "/spec/labels/newLabel", "value": "newValue"},
			{"op": "replace", "path": "/spec/description", "value": "Updated description via JSON Patch"},
			{"op": "add", "path": "/spec/ports/-", "value": 4000},
			{"op": "replace", "path": "/status/phase", "value": "Updated"}
		]`

		result, err := ApplyJSONPatch(original, []byte(patch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		assert.Equal(t, "newValue", e2eObj.Spec.Labels["newLabel"])
		assert.Equal(t, "Updated description via JSON Patch", e2eObj.Spec.Description)
		assert.Contains(t, e2eObj.Spec.Ports, 4000)
		assert.Equal(t, "Updated", e2eObj.Status.Phase)
	})

	t.Run("Remove fields and test operations", func(t *testing.T) {
		patch := `[
			{"op": "test", "path": "/spec/name", "value": "e2e-test-spec"},
			{"op": "remove", "path": "/spec/labels/type"},
			{"op": "remove", "path": "/spec/ports/1"}
		]`

		result, err := ApplyJSONPatch(original, []byte(patch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		_, exists := e2eObj.Spec.Labels["type"]
		assert.False(t, exists, "Label 'type' should be removed")
		assert.Len(t, e2eObj.Spec.Ports, 2, "One port should be removed")
	})

	t.Run("Complex nested operations", func(t *testing.T) {
		patch := `[
			{"op": "replace", "path": "/spec/config/timeout", "value": 60},
			{"op": "add", "path": "/spec/config/newSetting", "value": "newValue"},
			{"op": "replace", "path": "/spec/nestedObject/enabled", "value": false},
			{"op": "add", "path": "/spec/nestedObject/settings/newParam", "value": "value"}
		]`

		result, err := ApplyJSONPatch(original, []byte(patch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		assert.Equal(t, float64(60), e2eObj.Spec.Config["timeout"])
		assert.Equal(t, "newValue", e2eObj.Spec.Config["newSetting"])
		assert.False(t, e2eObj.Spec.NestedObject.Enabled)
		assert.Equal(t, "value", e2eObj.Spec.NestedObject.Settings["newParam"])
	})
}

// TestE2E_MergePatch_CompleteWorkflow tests Merge Patch operations in a comprehensive E2E scenario
func TestE2E_MergePatch_CompleteWorkflow(t *testing.T) {
	original := createE2ETestObject()

	t.Run("Basic field updates and additions", func(t *testing.T) {
		patch := `{
			"spec": {
				"description": "Updated via Merge Patch",
				"labels": {
					"newLabel": "mergeValue",
					"type": "updated"
				},
				"config": {
					"timeout": 45,
					"newConfig": "added"
				}
			},
			"status": {
				"phase": "MergeUpdated",
				"message": "Updated via merge patch"
			}
		}`

		result, err := ApplyMergePatch(original, []byte(patch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		assert.Equal(t, "Updated via Merge Patch", e2eObj.Spec.Description)
		assert.Equal(t, "mergeValue", e2eObj.Spec.Labels["newLabel"])
		assert.Equal(t, "updated", e2eObj.Spec.Labels["type"])
		assert.Equal(t, float64(45), e2eObj.Spec.Config["timeout"])
		assert.Equal(t, "added", e2eObj.Spec.Config["newConfig"])
		assert.Equal(t, "MergeUpdated", e2eObj.Status.Phase)
	})

	t.Run("Null value deletions", func(t *testing.T) {
		patch := `{
			"spec": {
				"labels": {
					"type": null
				},
				"config": {
					"retries": null
				}
			}
		}`

		result, err := ApplyMergePatch(original, []byte(patch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		_, exists := e2eObj.Spec.Labels["type"]
		assert.False(t, exists, "Label 'type' should be deleted")
		_, exists = e2eObj.Spec.Config["retries"]
		assert.False(t, exists, "Config 'retries' should be deleted")
	})

	t.Run("Array replacement", func(t *testing.T) {
		patch := `{
			"spec": {
				"ports": [5000, 6000]
			}
		}`

		result, err := ApplyMergePatch(original, []byte(patch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		assert.Equal(t, []int{5000, 6000}, e2eObj.Spec.Ports)
	})

	t.Run("Nested object merging", func(t *testing.T) {
		patch := `{
			"spec": {
				"nestedObject": {
					"enabled": false,
					"settings": {
						"mode": "development",
						"newSetting": "value"
					}
				}
			}
		}`

		result, err := ApplyMergePatch(original, []byte(patch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		assert.False(t, e2eObj.Spec.NestedObject.Enabled)
		assert.Equal(t, "development", e2eObj.Spec.NestedObject.Settings["mode"])
		assert.Equal(t, "value", e2eObj.Spec.NestedObject.Settings["newSetting"])
		// Original settings should be preserved
		assert.Equal(t, "false", e2eObj.Spec.NestedObject.Settings["debug"])
	})
}

// TestE2E_StrategicMergePatch_CompleteWorkflow tests Strategic Merge Patch operations
func TestE2E_StrategicMergePatch_CompleteWorkflow(t *testing.T) {
	original := createE2ETestObject()

	t.Run("Basic strategic merge operations", func(t *testing.T) {
		patch := `{
			"spec": {
				"description": "Updated via Strategic Merge Patch",
				"labels": {
					"strategicLabel": "strategicValue"
				},
				"config": {
					"timeout": 90,
					"strategicConfig": "added"
				}
			},
			"status": {
				"phase": "StrategicUpdated"
			}
		}`

		result, err := ApplyStrategicMergePatch(original, []byte(patch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		assert.Equal(t, "Updated via Strategic Merge Patch", e2eObj.Spec.Description)
		assert.Equal(t, "strategicValue", e2eObj.Spec.Labels["strategicLabel"])
		assert.Equal(t, float64(90), e2eObj.Spec.Config["timeout"])
		assert.Equal(t, "added", e2eObj.Spec.Config["strategicConfig"])
		assert.Equal(t, "StrategicUpdated", e2eObj.Status.Phase)
	})

	t.Run("Array handling with strategic merge", func(t *testing.T) {
		// Strategic merge patch typically replaces arrays unless there are merge keys
		patch := `{
			"spec": {
				"ports": [7000, 8000]
			}
		}`

		result, err := ApplyStrategicMergePatch(original, []byte(patch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		// For custom types without strategic merge metadata, arrays are replaced
		assert.Equal(t, []int{7000, 8000}, e2eObj.Spec.Ports)
	})

	t.Run("Fallback to merge patch behavior", func(t *testing.T) {
		// When strategic merge patch doesn't have metadata, it falls back to merge patch
		patch := `{
			"spec": {
				"nestedObject": {
					"enabled": false,
					"settings": {
						"fallbackSetting": "value"
					}
				}
			}
		}`

		result, err := ApplyStrategicMergePatch(original, []byte(patch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		assert.False(t, e2eObj.Spec.NestedObject.Enabled)
		assert.Equal(t, "value", e2eObj.Spec.NestedObject.Settings["fallbackSetting"])
	})
}

// TestE2E_AllPatchTypes_Compatibility tests compatibility between different patch types
func TestE2E_AllPatchTypes_Compatibility(t *testing.T) {
	original := createE2ETestObject()

	t.Run("Sequential patch operations", func(t *testing.T) {
		// Apply JSON Patch first
		jsonPatch := `[{"op": "replace", "path": "/spec/description", "value": "JSON Patch applied"}]`
		result1, err := ApplyJSONPatch(original, []byte(jsonPatch))
		require.NoError(t, err)

		// Apply Merge Patch to the result
		mergePatch := `{"spec": {"labels": {"mergeLabel": "applied"}}}`
		result2, err := ApplyMergePatch(result1, []byte(mergePatch))
		require.NoError(t, err)

		// Apply Strategic Merge Patch to the result
		strategicPatch := `{"status": {"phase": "AllPatchesApplied"}}`
		result3, err := ApplyStrategicMergePatch(result2, []byte(strategicPatch))
		require.NoError(t, err)

		// Verify all patches were applied
		final := result3.(*E2ETestObject)
		assert.Equal(t, "JSON Patch applied", final.Spec.Description)
		assert.Equal(t, "applied", final.Spec.Labels["mergeLabel"])
		assert.Equal(t, "AllPatchesApplied", final.Status.Phase)
	})

	t.Run("Patch type validation", func(t *testing.T) {
		// Test that each patch type validates its input format correctly

		// Valid JSON Patch
		jsonPatch := `[{"op": "add", "path": "/test", "value": "value"}]`
		_, err := ParseJSONPatch([]byte(jsonPatch))
		assert.NoError(t, err)

		// Valid Merge Patch
		mergePatch := `{"test": "value"}`
		err = ValidateMergePatch([]byte(mergePatch))
		assert.NoError(t, err)

		// Valid Strategic Merge Patch
		strategicPatch := `{"test": "value"}`
		err = ValidateStrategicMergePatch([]byte(strategicPatch))
		assert.NoError(t, err)

		// Invalid formats should fail
		invalidPatch := `{"test": }`
		_, err = ParseJSONPatch([]byte(invalidPatch))
		assert.Error(t, err)

		err = ValidateMergePatch([]byte(invalidPatch))
		assert.Error(t, err)

		err = ValidateStrategicMergePatch([]byte(invalidPatch))
		assert.Error(t, err)
	})
}

// TestE2E_RealWorldScenarios tests realistic patch scenarios
func TestE2E_RealWorldScenarios(t *testing.T) {
	t.Run("kubectl patch service equivalent operations", func(t *testing.T) {
		original := createE2ETestObject()

		// Equivalent to: kubectl patch service test-service --type='json' -p='[{"op": "replace", "path": "/spec/ports/0", "value": 8081}]'
		jsonPatch := `[{"op": "replace", "path": "/spec/ports/0", "value": 8081}]`
		result, err := ApplyJSONPatch(original, []byte(jsonPatch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		assert.Equal(t, 8081, e2eObj.Spec.Ports[0])
	})

	t.Run("kubectl patch with merge strategy", func(t *testing.T) {
		original := createE2ETestObject()

		// Equivalent to: kubectl patch service test-service --type='merge' -p='{"spec":{"labels":{"version":"v2.0.0"}}}'
		mergePatch := `{"spec": {"labels": {"version": "v2.0.0"}}}`
		result, err := ApplyMergePatch(original, []byte(mergePatch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		assert.Equal(t, "v2.0.0", e2eObj.Spec.Labels["version"])
		// Original labels should be preserved
		assert.Equal(t, "test", e2eObj.Spec.Labels["env"])
	})

	t.Run("kubectl patch with strategic merge (server-side apply style)", func(t *testing.T) {
		original := createE2ETestObject()

		// Equivalent to strategic merge patch or server-side apply
		strategicPatch := `{
			"metadata": {
				"labels": {
					"managed-by": "netguard-controller"
				}
			},
			"spec": {
				"config": {
					"managed": true
				}
			}
		}`

		result, err := ApplyStrategicMergePatch(original, []byte(strategicPatch))
		require.NoError(t, err)

		e2eObj := result.(*E2ETestObject)
		assert.Equal(t, "netguard-controller", e2eObj.ObjectMeta.Labels["managed-by"])
		assert.Equal(t, true, e2eObj.Spec.Config["managed"])
	})
}

// TestE2E_ErrorHandling tests error handling in realistic scenarios
func TestE2E_ErrorHandling(t *testing.T) {
	original := createE2ETestObject()

	t.Run("JSON Patch error scenarios", func(t *testing.T) {
		// Test operation failure
		failPatch := `[{"op": "test", "path": "/spec/name", "value": "wrong-name"}]`
		_, err := ApplyJSONPatch(original, []byte(failPatch))
		assert.Error(t, err)

		// Invalid path
		invalidPatch := `[{"op": "replace", "path": "/nonexistent/path", "value": "value"}]`
		_, err = ApplyJSONPatch(original, []byte(invalidPatch))
		assert.Error(t, err)
	})

	t.Run("Merge Patch error scenarios", func(t *testing.T) {
		// Invalid JSON
		invalidPatch := `{"spec": {"config": }}`
		_, err := ApplyMergePatch(original, []byte(invalidPatch))
		assert.Error(t, err)

		// Non-object patch
		nonObjectPatch := `["array", "patch"]`
		_, err = ApplyMergePatch(original, []byte(nonObjectPatch))
		assert.Error(t, err)
	})

	t.Run("Strategic Merge Patch error scenarios", func(t *testing.T) {
		// Invalid JSON
		invalidPatch := `{"spec": {"config": }}`
		_, err := ApplyStrategicMergePatch(original, []byte(invalidPatch))
		assert.Error(t, err)

		// Non-object patch
		nonObjectPatch := `"string patch"`
		_, err = ApplyStrategicMergePatch(original, []byte(nonObjectPatch))
		assert.Error(t, err)
	})
}

// TestE2E_PatchTypeComparison compares results from different patch types
func TestE2E_PatchTypeComparison(t *testing.T) {
	original := createE2ETestObject()

	t.Run("Equivalent operations produce same results", func(t *testing.T) {
		// JSON Patch to replace description
		jsonPatch := `[{"op": "replace", "path": "/spec/description", "value": "Updated description"}]`
		jsonResult, err := ApplyJSONPatch(original, []byte(jsonPatch))
		require.NoError(t, err)

		// Merge Patch to replace description
		mergePatch := `{"spec": {"description": "Updated description"}}`
		mergeResult, err := ApplyMergePatch(original, []byte(mergePatch))
		require.NoError(t, err)

		// Strategic Merge Patch to replace description
		strategicPatch := `{"spec": {"description": "Updated description"}}`
		strategicResult, err := ApplyStrategicMergePatch(original, []byte(strategicPatch))
		require.NoError(t, err)

		// All should produce the same description
		jsonObj := jsonResult.(*E2ETestObject)
		mergeObj := mergeResult.(*E2ETestObject)
		strategicObj := strategicResult.(*E2ETestObject)

		assert.Equal(t, "Updated description", jsonObj.Spec.Description)
		assert.Equal(t, "Updated description", mergeObj.Spec.Description)
		assert.Equal(t, "Updated description", strategicObj.Spec.Description)
	})
}

// TestE2E_PatchTypes_Integration tests integration between patch types and the patch router
func TestE2E_PatchTypes_Integration(t *testing.T) {
	original := createE2ETestObject()

	testCases := []struct {
		name      string
		patchType types.PatchType
		patchData string
		validate  func(t *testing.T, result runtime.Object)
	}{
		{
			name:      "JSON Patch Type",
			patchType: types.JSONPatchType,
			patchData: `[{"op": "replace", "path": "/spec/description", "value": "JSON updated"}]`,
			validate: func(t *testing.T, result runtime.Object) {
				obj := result.(*E2ETestObject)
				assert.Equal(t, "JSON updated", obj.Spec.Description)
			},
		},
		{
			name:      "Merge Patch Type",
			patchType: types.MergePatchType,
			patchData: `{"spec": {"description": "Merge updated"}}`,
			validate: func(t *testing.T, result runtime.Object) {
				obj := result.(*E2ETestObject)
				assert.Equal(t, "Merge updated", obj.Spec.Description)
			},
		},
		{
			name:      "Strategic Merge Patch Type",
			patchType: types.StrategicMergePatchType,
			patchData: `{"spec": {"description": "Strategic updated"}}`,
			validate: func(t *testing.T, result runtime.Object) {
				obj := result.(*E2ETestObject)
				assert.Equal(t, "Strategic updated", obj.Spec.Description)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the patch router function that would be called by BaseStorage.applyPatch
			var result runtime.Object
			var err error

			switch tc.patchType {
			case types.JSONPatchType:
				result, err = ApplyJSONPatch(original, []byte(tc.patchData))
			case types.MergePatchType:
				result, err = ApplyMergePatch(original, []byte(tc.patchData))
			case types.StrategicMergePatchType:
				result, err = ApplyStrategicMergePatch(original, []byte(tc.patchData))
			default:
				t.Fatalf("Unsupported patch type: %s", tc.patchType)
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			tc.validate(t, result)
		})
	}
}
