package patch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestApplyMergePatch_BasicMerge(t *testing.T) {
	original := &TestObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-object",
			Namespace: "default",
		},
		Spec: TestSpec{
			Name:        "original-name",
			Description: "original description",
			Labels: map[string]string{
				"app": "test",
				"env": "dev",
			},
		},
	}

	patch := `{
		"spec": {
			"description": "updated description",
			"labels": {
				"version": "v1.0"
			}
		}
	}`

	result, err := ApplyMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObject)
	assert.Equal(t, "updated description", resultObj.Spec.Description)
	assert.Equal(t, "test", resultObj.Spec.Labels["app"])
	assert.Equal(t, "dev", resultObj.Spec.Labels["env"])
	assert.Equal(t, "v1.0", resultObj.Spec.Labels["version"])
}

func TestApplyMergePatch_NullValueDeletion(t *testing.T) {
	original := &TestObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-object",
			Namespace: "default",
		},
		Spec: TestSpec{
			Name:        "original-name",
			Description: "original description",
			Labels: map[string]string{
				"app": "test",
				"env": "dev",
			},
		},
	}

	// Patch with null value should delete the description field
	patch := `{
		"spec": {
			"description": null,
			"labels": {
				"env": null,
				"version": "v1.0"
			}
		}
	}`

	result, err := ApplyMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObject)
	assert.Empty(t, resultObj.Spec.Description) // Should be deleted/empty
	assert.Equal(t, "test", resultObj.Spec.Labels["app"])
	assert.Empty(t, resultObj.Spec.Labels["env"]) // Should be deleted
	assert.Equal(t, "v1.0", resultObj.Spec.Labels["version"])
}

func TestApplyMergePatch_NestedObjectMerge(t *testing.T) {
	original := &TestObject{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-object",
			Labels: map[string]string{
				"app":     "original",
				"version": "v1.0",
			},
		},
		Spec: TestSpec{
			Name: "original-name",
			Labels: map[string]string{
				"env":  "dev",
				"team": "backend",
			},
		},
	}

	patch := `{
		"metadata": {
			"labels": {
				"version": "v2.0",
				"new-label": "added"
			}
		},
		"spec": {
			"labels": {
				"env": "prod",
				"owner": "devops"
			}
		}
	}`

	result, err := ApplyMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObject)
	// Check metadata labels merge
	assert.Equal(t, "original", resultObj.ObjectMeta.Labels["app"])    // Preserved
	assert.Equal(t, "v2.0", resultObj.ObjectMeta.Labels["version"])    // Updated
	assert.Equal(t, "added", resultObj.ObjectMeta.Labels["new-label"]) // Added

	// Check spec labels merge
	assert.Equal(t, "prod", resultObj.Spec.Labels["env"])     // Updated
	assert.Equal(t, "backend", resultObj.Spec.Labels["team"]) // Preserved
	assert.Equal(t, "devops", resultObj.Spec.Labels["owner"]) // Added
}

func TestApplyMergePatch_ArrayReplacement(t *testing.T) {
	original := &TestObject{
		Spec: TestSpec{
			Ports: []int{80, 443, 8080},
		},
	}

	// Arrays should be replaced entirely, not merged
	patch := `{
		"spec": {
			"ports": [9090, 9091]
		}
	}`

	result, err := ApplyMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObject)
	assert.Equal(t, []int{9090, 9091}, resultObj.Spec.Ports)
}

func TestApplyMergePatch_AddNewFields(t *testing.T) {
	original := &TestObject{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-object",
		},
	}

	patch := `{
		"spec": {
			"name": "new-name",
			"description": "new description",
			"labels": {
				"app": "test"
			}
		},
		"status": {
			"phase": "Running"
		}
	}`

	result, err := ApplyMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObject)
	assert.Equal(t, "new-name", resultObj.Spec.Name)
	assert.Equal(t, "new description", resultObj.Spec.Description)
	assert.Equal(t, "test", resultObj.Spec.Labels["app"])
	assert.Equal(t, "Running", resultObj.Status.Phase)
}

func TestApplyMergePatch_EmptyPatch(t *testing.T) {
	original := &TestObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: TestSpec{
			Name: "original",
		},
	}

	patch := `{}`

	result, err := ApplyMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObject)
	assert.Equal(t, "test", resultObj.Name)
	assert.Equal(t, "original", resultObj.Spec.Name)
}

func TestApplyMergePatch_ErrorCases(t *testing.T) {
	original := &TestObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}

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
			expectedErr: "cannot apply merge patch to nil object",
		},
		{
			name:        "empty patch data",
			obj:         original,
			patch:       "",
			expectedErr: "empty merge patch data",
		},
		{
			name:        "invalid JSON",
			obj:         original,
			patch:       `{"spec": {"description": "test"`,
			expectedErr: "invalid merge patch JSON",
		},
		{
			name:        "patch is not object",
			obj:         original,
			patch:       `["not", "an", "object"]`,
			expectedErr: "merge patch must be a JSON object",
		},
		{
			name:        "patch is primitive",
			obj:         original,
			patch:       `"not an object"`,
			expectedErr: "merge patch must be a JSON object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ApplyMergePatch(tt.obj, []byte(tt.patch))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestApplyMergePatch_ComplexNestedMerge(t *testing.T) {
	original := &TestObject{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-object",
			Labels: map[string]string{
				"app":     "original",
				"version": "v1.0",
			},
		},
		Spec: TestSpec{
			Name:        "original-name",
			Description: "original description",
			Labels: map[string]string{
				"env":  "dev",
				"team": "backend",
			},
			Ports: []int{80, 443},
		},
		Status: TestStatus{
			Phase:   "Running",
			Message: "All good",
		},
	}

	patch := `{
		"metadata": {
			"labels": {
				"app": "updated",
				"version": null,
				"new-label": "added"
			}
		},
		"spec": {
			"description": "updated description",
			"labels": {
				"env": "prod",
				"team": null,
				"owner": "devops"
			},
			"ports": [9090, 9091, 9092]
		},
		"status": {
			"phase": "Updated"
		}
	}`

	result, err := ApplyMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObject)

	// Check metadata labels
	assert.Equal(t, "updated", resultObj.ObjectMeta.Labels["app"])
	assert.Empty(t, resultObj.ObjectMeta.Labels["version"]) // Deleted by null
	assert.Equal(t, "added", resultObj.ObjectMeta.Labels["new-label"])

	// Check spec
	assert.Equal(t, "original-name", resultObj.Spec.Name) // Preserved
	assert.Equal(t, "updated description", resultObj.Spec.Description)
	assert.Equal(t, "prod", resultObj.Spec.Labels["env"])
	assert.Empty(t, resultObj.Spec.Labels["team"]) // Deleted by null
	assert.Equal(t, "devops", resultObj.Spec.Labels["owner"])

	// Check array replacement
	assert.Equal(t, []int{9090, 9091, 9092}, resultObj.Spec.Ports)

	// Check status
	assert.Equal(t, "Updated", resultObj.Status.Phase)
	assert.Equal(t, "All good", resultObj.Status.Message) // Preserved
}

func TestValidateMergePatch(t *testing.T) {
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
			name:        "empty patch data",
			patch:       "",
			expectError: true,
			errorMsg:    "empty merge patch data",
		},
		{
			name:        "invalid JSON",
			patch:       `{"spec": "unclosed`,
			expectError: true,
			errorMsg:    "invalid merge patch JSON",
		},
		{
			name:        "patch is array",
			patch:       `["not", "object"]`,
			expectError: true,
			errorMsg:    "merge patch must be a JSON object",
		},
		{
			name:        "patch is string",
			patch:       `"not object"`,
			expectError: true,
			errorMsg:    "merge patch must be a JSON object",
		},
		{
			name:        "patch is number",
			patch:       `123`,
			expectError: true,
			errorMsg:    "merge patch must be a JSON object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMergePatch([]byte(tt.patch))
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestApplyMergePatch_PreserveObjectIdentity(t *testing.T) {
	original := &TestObject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "test/v1",
			Kind:       "TestObject",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-object",
			Namespace:       "default",
			ResourceVersion: "123",
			UID:             "test-uid",
		},
		Spec: TestSpec{
			Name:        "original",
			Description: "original description",
		},
	}

	patch := `{
		"spec": {
			"description": "updated"
		}
	}`

	result, err := ApplyMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObject)

	// Object identity should be preserved
	assert.Equal(t, "test/v1", resultObj.APIVersion)
	assert.Equal(t, "TestObject", resultObj.Kind)
	assert.Equal(t, "test-object", resultObj.Name)
	assert.Equal(t, "default", resultObj.Namespace)
	assert.Equal(t, "123", resultObj.ResourceVersion)
	assert.Equal(t, "test-uid", string(resultObj.UID))

	// Only spec should be updated
	assert.Equal(t, "original", resultObj.Spec.Name)       // Preserved
	assert.Equal(t, "updated", resultObj.Spec.Description) // Updated
}

func TestApplyMergePatch_NullDeletesEntireObject(t *testing.T) {
	original := &TestObject{
		Spec: TestSpec{
			Labels: map[string]string{
				"app": "test",
				"env": "dev",
			},
		},
	}

	// Setting labels to null should delete the entire labels object
	patch := `{
		"spec": {
			"labels": null
		}
	}`

	result, err := ApplyMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObject)
	assert.Nil(t, resultObj.Spec.Labels) // Entire labels map should be deleted
}

func TestApplyMergePatch_DeepNullDeletion(t *testing.T) {
	original := &TestObject{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app":     "test",
				"version": "v1.0",
				"env":     "dev",
			},
		},
		Spec: TestSpec{
			Name:        "test-name",
			Description: "test description",
			Labels: map[string]string{
				"team":  "backend",
				"owner": "devops",
			},
		},
	}

	// Test multiple null deletions at different levels
	patch := `{
		"metadata": {
			"labels": {
				"version": null,
				"env": null
			}
		},
		"spec": {
			"description": null,
			"labels": {
				"owner": null
			}
		}
	}`

	result, err := ApplyMergePatch(original, []byte(patch))
	require.NoError(t, err)

	resultObj := result.(*TestObject)

	// Check metadata labels
	assert.Equal(t, "test", resultObj.ObjectMeta.Labels["app"]) // Preserved
	assert.Empty(t, resultObj.ObjectMeta.Labels["version"])     // Deleted
	assert.Empty(t, resultObj.ObjectMeta.Labels["env"])         // Deleted

	// Check spec
	assert.Equal(t, "test-name", resultObj.Spec.Name)         // Preserved
	assert.Empty(t, resultObj.Spec.Description)               // Deleted
	assert.Equal(t, "backend", resultObj.Spec.Labels["team"]) // Preserved
	assert.Empty(t, resultObj.Spec.Labels["owner"])           // Deleted
}
