package fieldmanager

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Note: TestObject, TestSpec, TestStatus, and createTestObject are defined in managed_fields_test.go

func TestNewServerSideFieldManager(t *testing.T) {
	tests := []struct {
		name           string
		defaultManager string
	}{
		{
			name:           "with custom default manager",
			defaultManager: "custom-manager",
		},
		{
			name:           "with empty default manager",
			defaultManager: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := NewServerSideFieldManager(tt.defaultManager)
			assert.NotNil(t, fm)
			assert.NotNil(t, fm.managedFieldsManager)
		})
	}
}

func TestServerSideFieldManager_Apply(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")
	ctx := context.Background()
	obj := createTestObject()

	patch := `{
		"spec": {
			"description": "updated via server-side apply",
			"labels": {
				"version": "v1.0"
			}
		}
	}`

	result, err := fm.Apply(ctx, obj, []byte(patch), "kubectl", false)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify the patch was applied
	resultObj := result.(*TestObject)
	assert.Equal(t, "updated via server-side apply", resultObj.Spec.Description)
	assert.Equal(t, "dev", resultObj.Spec.Labels["env"])      // Preserved
	assert.Equal(t, "v1.0", resultObj.Spec.Labels["version"]) // Added

	// Verify managed fields were added
	managedFields := resultObj.GetManagedFields()
	require.Len(t, managedFields, 1)
	assert.Equal(t, "kubectl", managedFields[0].Manager)
	assert.Equal(t, metav1.ManagedFieldsOperationApply, managedFields[0].Operation)
}

func TestServerSideFieldManager_Apply_ForceApply(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")
	ctx := context.Background()
	obj := createTestObject()

	// First, add managed fields for another manager
	fieldsV1 := &metav1.FieldsV1{Raw: []byte(`{"spec":{"description":{}}}`)}
	err := fm.managedFieldsManager.AddManagedFields(obj, "helm", metav1.ManagedFieldsOperationApply, fieldsV1, "")
	require.NoError(t, err)

	patch := `{
		"spec": {
			"description": "force applied"
		}
	}`

	// Apply with force=true should succeed even with conflicts
	result, err := fm.Apply(ctx, obj, []byte(patch), "kubectl", true)
	require.NoError(t, err)
	assert.NotNil(t, result)

	resultObj := result.(*TestObject)
	assert.Equal(t, "force applied", resultObj.Spec.Description)

	// Should have managed fields for both managers
	managedFields := resultObj.GetManagedFields()
	assert.GreaterOrEqual(t, len(managedFields), 1)

	// Verify helm no longer owns spec.description
	var helmFields map[string]interface{}
	for _, e := range managedFields {
		if e.Manager == "helm" && e.Operation == metav1.ManagedFieldsOperationApply {
			if len(e.FieldsV1.Raw) > 0 {
				_ = json.Unmarshal(e.FieldsV1.Raw, &helmFields)
			}
		}
	}
	if helmFields != nil {
		if spec, ok := helmFields["spec"].(map[string]interface{}); ok {
			_, has := spec["description"]
			assert.False(t, has, "helm should not own spec.description after force apply")
		}
	}
}

func TestServerSideFieldManager_Apply_ErrorCases(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")
	ctx := context.Background()
	obj := createTestObject()

	tests := []struct {
		name         string
		obj          runtime.Object
		patch        []byte
		fieldManager string
		force        bool
		expectedErr  string
	}{
		{
			name:         "nil object",
			obj:          nil,
			patch:        []byte(`{"spec":{"description":"test"}}`),
			fieldManager: "kubectl",
			force:        false,
			expectedErr:  "cannot apply to nil object",
		},
		{
			name:         "empty field manager",
			obj:          obj,
			patch:        []byte(`{"spec":{"description":"test"}}`),
			fieldManager: "",
			force:        false,
			expectedErr:  "fieldManager cannot be empty",
		},
		{
			name:         "empty patch",
			obj:          obj,
			patch:        []byte{},
			fieldManager: "kubectl",
			force:        false,
			expectedErr:  "patch cannot be empty",
		},
		{
			name:         "invalid JSON patch",
			obj:          obj,
			patch:        []byte(`{"spec":{"description":"test"`),
			fieldManager: "kubectl",
			force:        false,
			expectedErr:  "failed to parse patch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fm.Apply(ctx, tt.obj, tt.patch, tt.fieldManager, tt.force)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestServerSideFieldManager_Update(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")
	ctx := context.Background()
	obj := createTestObject()

	result, err := fm.Update(ctx, obj, "kubectl")
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify managed fields were added for update operation
	managedFields := result.(*TestObject).GetManagedFields()
	require.Len(t, managedFields, 1)
	assert.Equal(t, "kubectl", managedFields[0].Manager)
	assert.Equal(t, metav1.ManagedFieldsOperationUpdate, managedFields[0].Operation)
}

func TestServerSideFieldManager_Update_DefaultManager(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")
	ctx := context.Background()
	obj := createTestObject()

	// Test with empty field manager - should use default
	result, err := fm.Update(ctx, obj, "")
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify managed fields were added with default manager
	managedFields := result.(*TestObject).GetManagedFields()
	require.Len(t, managedFields, 1)
	assert.Equal(t, "netguard-apiserver", managedFields[0].Manager)
	assert.Equal(t, metav1.ManagedFieldsOperationUpdate, managedFields[0].Operation)
}

func TestServerSideFieldManager_Update_ErrorCases(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")
	ctx := context.Background()

	tests := []struct {
		name         string
		obj          runtime.Object
		fieldManager string
		expectedErr  string
	}{
		{
			name:         "nil object",
			obj:          nil,
			fieldManager: "kubectl",
			expectedErr:  "cannot update nil object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fm.Update(ctx, tt.obj, tt.fieldManager)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestServerSideFieldManager_DetectConflicts(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")
	obj := createTestObject()

	// Add managed fields for another manager
	fieldsV1 := &metav1.FieldsV1{Raw: []byte(`{"spec":{"description":{}}}`)}
	err := fm.managedFieldsManager.AddManagedFields(obj, "helm", metav1.ManagedFieldsOperationApply, fieldsV1, "")
	require.NoError(t, err)

	// Detect conflicts with kubectl
	conflicts, err := fm.DetectConflicts(obj, nil, "kubectl")
	require.NoError(t, err)

	// Should detect conflict with helm manager
	require.Len(t, conflicts, 1)
	assert.Equal(t, "helm", conflicts[0].Manager)
	assert.Contains(t, conflicts[0].Message, "conflicts with kubectl")
}

func TestServerSideFieldManager_DetectConflicts_NoConflicts(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")
	obj := createTestObject()

	// Add managed fields for update operation (should not conflict)
	fieldsV1 := &metav1.FieldsV1{Raw: []byte(`{"spec":{"description":{}}}`)}
	err := fm.managedFieldsManager.AddManagedFields(obj, "helm", metav1.ManagedFieldsOperationUpdate, fieldsV1, "")
	require.NoError(t, err)

	// Detect conflicts with kubectl
	conflicts, err := fm.DetectConflicts(obj, nil, "kubectl")
	require.NoError(t, err)

	// Should not detect conflicts with update operations
	assert.Empty(t, conflicts)
}

func TestServerSideFieldManager_DetectConflicts_SameManager(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")
	obj := createTestObject()

	// Add managed fields for same manager
	fieldsV1 := &metav1.FieldsV1{Raw: []byte(`{"spec":{"description":{}}}`)}
	err := fm.managedFieldsManager.AddManagedFields(obj, "kubectl", metav1.ManagedFieldsOperationApply, fieldsV1, "")
	require.NoError(t, err)

	// Detect conflicts with same manager
	conflicts, err := fm.DetectConflicts(obj, nil, "kubectl")
	require.NoError(t, err)

	// Should not detect conflicts with same manager
	assert.Empty(t, conflicts)
}

func TestServerSideFieldManager_DetectConflicts_ErrorCases(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")

	tests := []struct {
		name         string
		current      runtime.Object
		desired      runtime.Object
		fieldManager string
		expectedErr  string
	}{
		{
			name:         "nil current object",
			current:      nil,
			desired:      nil,
			fieldManager: "kubectl",
			expectedErr:  "cannot detect conflicts with nil current object",
		},
		{
			name:         "empty field manager",
			current:      createTestObject(),
			desired:      nil,
			fieldManager: "",
			expectedErr:  "fieldManager cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fm.DetectConflicts(tt.current, tt.desired, tt.fieldManager)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestServerSideFieldManager_ApplyPatch(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")
	obj := createTestObject()

	patch := []byte(`{
		"spec": {
			"description": "patched description",
			"labels": {
				"new-label": "new-value"
			}
		}
	}`)

	result, err := fm.applyPatch(obj, patch)
	require.NoError(t, err)
	assert.NotNil(t, result)

	resultObj := result.(*TestObject)
	assert.Equal(t, "patched description", resultObj.Spec.Description)
	assert.Equal(t, "dev", resultObj.Spec.Labels["env"])             // Preserved
	assert.Equal(t, "new-value", resultObj.Spec.Labels["new-label"]) // Added
}

func TestServerSideFieldManager_CreateFieldsV1FromPatch(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")

	patchObj := map[string]interface{}{
		"spec": map[string]interface{}{
			"description": "test",
			"labels": map[string]interface{}{
				"env": "prod",
			},
		},
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"app": "test",
			},
		},
	}

	fieldsV1, err := fm.createFieldsV1FromPatch(patchObj)
	require.NoError(t, err)
	assert.NotNil(t, fieldsV1)
	assert.NotEmpty(t, fieldsV1.Raw)

	// Verify the structure contains expected fields
	var fieldsMap map[string]interface{}
	err = json.Unmarshal(fieldsV1.Raw, &fieldsMap)
	require.NoError(t, err)

	assert.Contains(t, fieldsMap, "spec")
	assert.Contains(t, fieldsMap, "metadata")
}

func TestServerSideFieldManager_MergeMaps(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")

	original := map[string]interface{}{
		"spec": map[string]interface{}{
			"name":        "original",
			"description": "original desc",
			"labels": map[string]interface{}{
				"env": "dev",
				"app": "test",
			},
		},
	}

	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"description": "updated desc",
			"labels": map[string]interface{}{
				"env":     "prod",
				"version": "v1.0",
			},
		},
	}

	result := fm.mergeMaps(original, patch)

	// Verify merge results
	spec := result["spec"].(map[string]interface{})
	assert.Equal(t, "original", spec["name"])            // Preserved
	assert.Equal(t, "updated desc", spec["description"]) // Updated

	labels := spec["labels"].(map[string]interface{})
	assert.Equal(t, "test", labels["app"])     // Preserved
	assert.Equal(t, "prod", labels["env"])     // Updated
	assert.Equal(t, "v1.0", labels["version"]) // Added
}

func TestServerSideFieldManager_MergeMaps_NullValues(t *testing.T) {
	fm := NewServerSideFieldManager("test-manager")

	original := map[string]interface{}{
		"spec": map[string]interface{}{
			"name":        "original",
			"description": "original desc",
		},
	}

	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"description": nil, // Should delete this field
			"newField":    "new value",
		},
	}

	result := fm.mergeMaps(original, patch)

	spec := result["spec"].(map[string]interface{})
	assert.Equal(t, "original", spec["name"])      // Preserved
	assert.NotContains(t, spec, "description")     // Deleted by null
	assert.Equal(t, "new value", spec["newField"]) // Added
}
