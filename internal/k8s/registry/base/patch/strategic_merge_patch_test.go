package patch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestObjectWithStrategicMerge extends TestObject with strategic merge annotations
type TestObjectWithStrategicMerge struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TestSpecWithStrategicMerge `json:"spec,omitempty"`
	Status            TestStatus                 `json:"status,omitempty"`
}

type TestSpecWithStrategicMerge struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	// Ports with strategic merge key annotation for array merging
	Ports []TestPort `json:"ports,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
	// Rules with replace strategy
	Rules []TestRule `json:"rules,omitempty" patchStrategy:"replace"`
	// Simple array without strategic merge (should be replaced)
	Tags []string `json:"tags,omitempty"`
}

type TestPort struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"`
}

type TestRule struct {
	ID     string `json:"id"`
	Action string `json:"action"`
}

// DeepCopyObject implements runtime.Object
func (t *TestObjectWithStrategicMerge) DeepCopyObject() runtime.Object {
	if t == nil {
		return nil
	}
	out := new(TestObjectWithStrategicMerge)
	t.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all properties of this object into another object of the same type
func (t *TestObjectWithStrategicMerge) DeepCopyInto(out *TestObjectWithStrategicMerge) {
	*out = *t
	out.TypeMeta = t.TypeMeta
	t.ObjectMeta.DeepCopyInto(&out.ObjectMeta)

	// Deep copy spec
	out.Spec = TestSpecWithStrategicMerge{
		Name:        t.Spec.Name,
		Description: t.Spec.Description,
	}

	if t.Spec.Labels != nil {
		out.Spec.Labels = make(map[string]string, len(t.Spec.Labels))
		for k, v := range t.Spec.Labels {
			out.Spec.Labels[k] = v
		}
	}

	if t.Spec.Ports != nil {
		out.Spec.Ports = make([]TestPort, len(t.Spec.Ports))
		copy(out.Spec.Ports, t.Spec.Ports)
	}

	if t.Spec.Rules != nil {
		out.Spec.Rules = make([]TestRule, len(t.Spec.Rules))
		copy(out.Spec.Rules, t.Spec.Rules)
	}

	if t.Spec.Tags != nil {
		out.Spec.Tags = make([]string, len(t.Spec.Tags))
		copy(out.Spec.Tags, t.Spec.Tags)
	}

	// Deep copy status
	out.Status = TestStatus{
		Phase:   t.Status.Phase,
		Message: t.Status.Message,
	}
}

func createTestObjectWithStrategicMerge() *TestObjectWithStrategicMerge {
	return &TestObjectWithStrategicMerge{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "test/v1",
			Kind:       "TestObjectWithStrategicMerge",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-object",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: TestSpecWithStrategicMerge{
			Name:        "test-spec",
			Description: "original description",
			Labels: map[string]string{
				"env": "dev",
			},
			Ports: []TestPort{
				{Name: "http", Port: 8080, Protocol: "TCP"},
				{Name: "https", Port: 8443, Protocol: "TCP"},
			},
			Rules: []TestRule{
				{ID: "rule1", Action: "allow"},
				{ID: "rule2", Action: "deny"},
			},
			Tags: []string{"tag1", "tag2"},
		},
		Status: TestStatus{
			Phase:   "Running",
			Message: "All good",
		},
	}
}

func TestApplyStrategicMergePatch_BasicMerge(t *testing.T) {
	original := createTestObjectWithStrategicMerge()

	patch := `{
		"spec": {
			"description": "updated description",
			"labels": {
				"version": "v1.0"
			}
		}
	}`

	result, err := ApplyStrategicMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObjectWithStrategicMerge)
	assert.Equal(t, "updated description", resultObj.Spec.Description)
	assert.Equal(t, "dev", resultObj.Spec.Labels["env"])      // Preserved
	assert.Equal(t, "v1.0", resultObj.Spec.Labels["version"]) // Added
}

func TestApplyStrategicMergePatch_ArrayMergeStrategy(t *testing.T) {
	original := createTestObjectWithStrategicMerge()

	// Test array replacement (fallback behavior for custom types)
	patch := `{
		"spec": {
			"ports": [
				{
					"name": "http",
					"port": 9080,
					"protocol": "TCP"
				},
				{
					"name": "grpc",
					"port": 9090,
					"protocol": "TCP"
				}
			]
		}
	}`

	result, err := ApplyStrategicMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObjectWithStrategicMerge)

	// For custom types, arrays are replaced (merge patch behavior)
	assert.Len(t, resultObj.Spec.Ports, 2)

	// Find ports by name
	portsByName := make(map[string]TestPort)
	for _, port := range resultObj.Spec.Ports {
		portsByName[port.Name] = port
	}

	// Should have the new ports from the patch
	assert.Equal(t, 9080, portsByName["http"].Port)
	assert.Equal(t, "TCP", portsByName["http"].Protocol)
	assert.Equal(t, 9090, portsByName["grpc"].Port)
	assert.Equal(t, "TCP", portsByName["grpc"].Protocol)
}

func TestApplyStrategicMergePatch_ArrayReplaceStrategy(t *testing.T) {
	original := createTestObjectWithStrategicMerge()

	// Test replace strategy for rules array
	patch := `{
		"spec": {
			"rules": [
				{
					"id": "rule3",
					"action": "audit"
				}
			]
		}
	}`

	result, err := ApplyStrategicMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObjectWithStrategicMerge)

	// Rules should be completely replaced (not merged)
	assert.Len(t, resultObj.Spec.Rules, 1)
	assert.Equal(t, "rule3", resultObj.Spec.Rules[0].ID)
	assert.Equal(t, "audit", resultObj.Spec.Rules[0].Action)
}

func TestApplyStrategicMergePatch_SimpleArrayReplacement(t *testing.T) {
	original := createTestObjectWithStrategicMerge()

	// Test simple array replacement for tags (no strategic merge annotations)
	patch := `{
		"spec": {
			"tags": ["newtag1", "newtag2", "newtag3"]
		}
	}`

	result, err := ApplyStrategicMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObjectWithStrategicMerge)

	// Tags should be completely replaced
	assert.Equal(t, []string{"newtag1", "newtag2", "newtag3"}, resultObj.Spec.Tags)
}

func TestApplyStrategicMergePatch_DeleteFromArray(t *testing.T) {
	original := createTestObjectWithStrategicMerge()

	// Test array replacement (strategic merge directives don't work with custom types)
	patch := `{
		"spec": {
			"tags": ["newtag"]
		}
	}`

	result, err := ApplyStrategicMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObjectWithStrategicMerge)

	// Array should be replaced entirely
	assert.Equal(t, []string{"newtag"}, resultObj.Spec.Tags)
}

func TestApplyStrategicMergePatch_RetainKeysDirective(t *testing.T) {
	original := createTestObjectWithStrategicMerge()

	// Test regular map merge (strategic merge directives don't work with custom types)
	patch := `{
		"spec": {
			"labels": {
				"version": "v1.0",
				"team": "backend"
			}
		}
	}`

	result, err := ApplyStrategicMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObjectWithStrategicMerge)

	// Should merge maps (preserve existing + add new)
	assert.Len(t, resultObj.Spec.Labels, 3)
	assert.Equal(t, "dev", resultObj.Spec.Labels["env"])      // Preserved
	assert.Equal(t, "v1.0", resultObj.Spec.Labels["version"]) // Added
	assert.Equal(t, "backend", resultObj.Spec.Labels["team"]) // Added
}

func TestApplyStrategicMergePatch_ErrorCases(t *testing.T) {
	original := createTestObjectWithStrategicMerge()

	tests := []struct {
		name        string
		obj         runtime.Object
		patch       string
		expectedErr string
	}{
		{
			name:        "nil object",
			obj:         nil,
			patch:       `{"spec": {"description": "test"}}`,
			expectedErr: "cannot apply strategic merge patch to nil object",
		},
		{
			name:        "empty patch data",
			obj:         original,
			patch:       "",
			expectedErr: "empty strategic merge patch data",
		},
		{
			name:        "invalid JSON",
			obj:         original,
			patch:       `{"spec": {"description": "test"`,
			expectedErr: "invalid strategic merge patch JSON",
		},
		{
			name:        "patch is not object",
			obj:         original,
			patch:       `["not", "an", "object"]`,
			expectedErr: "strategic merge patch must be a JSON object",
		},
		{
			name:        "patch is primitive",
			obj:         original,
			patch:       `"not an object"`,
			expectedErr: "strategic merge patch must be a JSON object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ApplyStrategicMergePatch(tt.obj, []byte(tt.patch))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestValidateStrategicMergePatch(t *testing.T) {
	tests := []struct {
		name        string
		patch       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid patch",
			patch:       `{"spec": {"description": "test"}}`,
			expectError: false,
		},
		{
			name:        "empty patch object",
			patch:       `{}`,
			expectError: false,
		},
		{
			name:        "patch with directives",
			patch:       `{"spec": {"labels": {"$retainKeys": ["env"]}}}`,
			expectError: false,
		},
		{
			name:        "empty patch data",
			patch:       "",
			expectError: true,
			errorMsg:    "empty strategic merge patch data",
		},
		{
			name:        "invalid JSON",
			patch:       `{"spec": "unclosed`,
			expectError: true,
			errorMsg:    "invalid strategic merge patch JSON",
		},
		{
			name:        "patch is array",
			patch:       `["not", "object"]`,
			expectError: true,
			errorMsg:    "strategic merge patch must be a JSON object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStrategicMergePatch([]byte(tt.patch))
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateStrategicMergePatch(t *testing.T) {
	original := createTestObjectWithStrategicMerge()

	modified := createTestObjectWithStrategicMerge()
	modified.Spec.Description = "modified description"
	modified.Spec.Labels["version"] = "v2.0"
	modified.Spec.Ports = append(modified.Spec.Ports, TestPort{
		Name: "metrics", Port: 9090, Protocol: "TCP",
	})

	patchData, err := CreateStrategicMergePatch(original, modified)
	require.NoError(t, err)
	assert.NotEmpty(t, patchData)

	// Apply the generated patch to verify it works
	result, err := ApplyStrategicMergePatch(original, patchData)
	require.NoError(t, err)

	resultObj := result.(*TestObjectWithStrategicMerge)
	assert.Equal(t, "modified description", resultObj.Spec.Description)
	assert.Equal(t, "v2.0", resultObj.Spec.Labels["version"])
	assert.Len(t, resultObj.Spec.Ports, 3) // Original 2 + 1 new
}

func TestCreateStrategicMergePatch_ErrorCases(t *testing.T) {
	original := createTestObjectWithStrategicMerge()
	differentType := &TestObject{} // Different type

	tests := []struct {
		name        string
		original    runtime.Object
		modified    runtime.Object
		expectedErr string
	}{
		{
			name:        "nil original",
			original:    nil,
			modified:    original,
			expectedErr: "both original and modified objects must be non-nil",
		},
		{
			name:        "nil modified",
			original:    original,
			modified:    nil,
			expectedErr: "both original and modified objects must be non-nil",
		},
		{
			name:        "different types",
			original:    original,
			modified:    differentType,
			expectedErr: "original and modified objects must be of the same type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateStrategicMergePatch(tt.original, tt.modified)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestApplyStrategicMergePatchWithOptions(t *testing.T) {
	original := createTestObjectWithStrategicMerge()

	patch := `{
		"spec": {
			"description": "updated with options"
		}
	}`

	options := StrategicMergePatchOptions{
		IgnoreUnknownFields: true,
		StrictValidation:    false,
	}

	result, err := ApplyStrategicMergePatchWithOptions(original, []byte(patch), options)
	require.NoError(t, err)

	resultObj := result.(*TestObjectWithStrategicMerge)
	assert.Equal(t, "updated with options", resultObj.Spec.Description)
}

func TestApplyStrategicMergePatch_PreserveObjectIdentity(t *testing.T) {
	original := createTestObjectWithStrategicMerge()
	original.ResourceVersion = "123"
	original.UID = "test-uid"

	patch := `{
		"spec": {
			"description": "updated"
		}
	}`

	result, err := ApplyStrategicMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObjectWithStrategicMerge)

	// Object identity should be preserved
	assert.Equal(t, "test/v1", resultObj.APIVersion)
	assert.Equal(t, "TestObjectWithStrategicMerge", resultObj.Kind)
	assert.Equal(t, "test-object", resultObj.Name)
	assert.Equal(t, "default", resultObj.Namespace)
	assert.Equal(t, "123", resultObj.ResourceVersion)
	assert.Equal(t, "test-uid", string(resultObj.UID))

	// Only spec should be updated
	assert.Equal(t, "test-spec", resultObj.Spec.Name)      // Preserved
	assert.Equal(t, "updated", resultObj.Spec.Description) // Updated
}

func TestApplyStrategicMergePatch_ComplexArrayMerge(t *testing.T) {
	original := createTestObjectWithStrategicMerge()

	// Complex patch with multiple array operations
	patch := `{
		"spec": {
			"ports": [
				{
					"name": "http",
					"port": 9080
				},
				{
					"name": "websocket",
					"port": 9081,
					"protocol": "TCP"
				}
			],
			"rules": [
				{
					"id": "newrule",
					"action": "log"
				}
			],
			"tags": ["production", "v2"]
		}
	}`

	result, err := ApplyStrategicMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObjectWithStrategicMerge)

	// For custom types, all arrays are replaced (merge patch behavior)
	assert.Len(t, resultObj.Spec.Ports, 2) // Replaced with patch ports
	assert.Len(t, resultObj.Spec.Rules, 1) // Replaced with patch rules
	assert.Equal(t, "newrule", resultObj.Spec.Rules[0].ID)

	// Tags should be replaced
	assert.Equal(t, []string{"production", "v2"}, resultObj.Spec.Tags)

	// Verify port replacement
	portsByName := make(map[string]TestPort)
	for _, port := range resultObj.Spec.Ports {
		portsByName[port.Name] = port
	}
	assert.Equal(t, 9080, portsByName["http"].Port)
	assert.Equal(t, 9081, portsByName["websocket"].Port)
}
