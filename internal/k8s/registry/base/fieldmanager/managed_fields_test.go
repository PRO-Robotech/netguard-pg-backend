package fieldmanager

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestObject is a simple test object that implements runtime.Object
type TestObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TestSpec   `json:"spec,omitempty"`
	Status            TestStatus `json:"status,omitempty"`
}

type TestSpec struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Ports       []int             `json:"ports,omitempty"`
}

type TestStatus struct {
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// DeepCopyObject implements runtime.Object
func (t *TestObject) DeepCopyObject() runtime.Object {
	if t == nil {
		return nil
	}
	out := new(TestObject)
	t.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all properties of this object into another object of the same type
func (t *TestObject) DeepCopyInto(out *TestObject) {
	*out = *t
	out.TypeMeta = t.TypeMeta
	t.ObjectMeta.DeepCopyInto(&out.ObjectMeta)

	// Deep copy spec
	out.Spec = TestSpec{
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
		out.Spec.Ports = make([]int, len(t.Spec.Ports))
		copy(out.Spec.Ports, t.Spec.Ports)
	}

	// Deep copy status
	out.Status = TestStatus{
		Phase:   t.Status.Phase,
		Message: t.Status.Message,
	}
}

func createTestObject() *TestObject {
	obj := &TestObject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "test/v1",
			Kind:       "TestObject",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-object",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: TestSpec{
			Name:        "test-spec",
			Description: "original description",
			Labels: map[string]string{
				"env": "dev",
			},
			Ports: []int{8080, 9090},
		},
		Status: TestStatus{
			Phase:   "Running",
			Message: "All good",
		},
	}

	// Set the GroupVersionKind
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "test",
		Version: "v1",
		Kind:    "TestObject",
	})

	return obj
}

func TestNewManagedFieldsManager(t *testing.T) {
	tests := []struct {
		name            string
		defaultManager  string
		expectedManager string
	}{
		{
			name:            "with custom default manager",
			defaultManager:  "custom-manager",
			expectedManager: "custom-manager",
		},
		{
			name:            "with empty default manager",
			defaultManager:  "",
			expectedManager: "netguard-apiserver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManagedFieldsManager(tt.defaultManager)
			assert.NotNil(t, manager)
			assert.Equal(t, tt.expectedManager, manager.defaultManager)
		})
	}
}

func TestAddManagedFields(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")
	obj := createTestObject()

	// Create test FieldsV1
	fieldsV1 := &metav1.FieldsV1{
		Raw: []byte(`{"spec":{"description":{}}}`),
	}

	err := manager.AddManagedFields(obj, "kubectl", metav1.ManagedFieldsOperationApply, fieldsV1, "")
	require.NoError(t, err)

	// Verify managed fields were added
	managedFields := obj.GetManagedFields()
	require.Len(t, managedFields, 1)

	entry := managedFields[0]
	assert.Equal(t, "kubectl", entry.Manager)
	assert.Equal(t, metav1.ManagedFieldsOperationApply, entry.Operation)
	assert.Equal(t, "test/v1", entry.APIVersion)
	assert.Equal(t, "FieldsV1", entry.FieldsType)
	assert.Equal(t, fieldsV1, entry.FieldsV1)
	assert.Empty(t, entry.Subresource)
	assert.NotNil(t, entry.Time)
}

func TestAddManagedFields_UpdateExisting(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")
	obj := createTestObject()

	// Add first managed fields entry
	fieldsV1_1 := &metav1.FieldsV1{
		Raw: []byte(`{"spec":{"description":{}}}`),
	}
	err := manager.AddManagedFields(obj, "kubectl", metav1.ManagedFieldsOperationApply, fieldsV1_1, "")
	require.NoError(t, err)

	// Add second entry with same manager and operation - should update existing
	fieldsV1_2 := &metav1.FieldsV1{
		Raw: []byte(`{"spec":{"name":{}}}`),
	}
	err = manager.AddManagedFields(obj, "kubectl", metav1.ManagedFieldsOperationApply, fieldsV1_2, "")
	require.NoError(t, err)

	// Should still have only one entry, but updated
	managedFields := obj.GetManagedFields()
	require.Len(t, managedFields, 1)

	entry := managedFields[0]
	assert.Equal(t, "kubectl", entry.Manager)
	assert.Equal(t, fieldsV1_2, entry.FieldsV1)
}

func TestAddManagedFields_MultipleManagers(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")
	obj := createTestObject()

	// Add entry for kubectl
	fieldsV1_kubectl := &metav1.FieldsV1{
		Raw: []byte(`{"spec":{"description":{}}}`),
	}
	err := manager.AddManagedFields(obj, "kubectl", metav1.ManagedFieldsOperationApply, fieldsV1_kubectl, "")
	require.NoError(t, err)

	// Add entry for helm
	fieldsV1_helm := &metav1.FieldsV1{
		Raw: []byte(`{"spec":{"name":{}}}`),
	}
	err = manager.AddManagedFields(obj, "helm", metav1.ManagedFieldsOperationUpdate, fieldsV1_helm, "")
	require.NoError(t, err)

	// Should have two entries
	managedFields := obj.GetManagedFields()
	require.Len(t, managedFields, 2)

	// Find entries by manager
	var kubectlEntry, helmEntry *metav1.ManagedFieldsEntry
	for i := range managedFields {
		if managedFields[i].Manager == "kubectl" {
			kubectlEntry = &managedFields[i]
		} else if managedFields[i].Manager == "helm" {
			helmEntry = &managedFields[i]
		}
	}

	require.NotNil(t, kubectlEntry)
	require.NotNil(t, helmEntry)

	assert.Equal(t, metav1.ManagedFieldsOperationApply, kubectlEntry.Operation)
	assert.Equal(t, metav1.ManagedFieldsOperationUpdate, helmEntry.Operation)
}

func TestAddManagedFields_ErrorCases(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")

	tests := []struct {
		name        string
		obj         runtime.Object
		expectedErr string
	}{
		{
			name:        "nil object",
			obj:         nil,
			expectedErr: "cannot add managed fields to nil object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fieldsV1 := &metav1.FieldsV1{Raw: []byte(`{}`)}
			err := manager.AddManagedFields(tt.obj, "test", metav1.ManagedFieldsOperationApply, fieldsV1, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestRemoveManagedFields(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")
	obj := createTestObject()

	// Add managed fields for multiple managers
	fieldsV1 := &metav1.FieldsV1{Raw: []byte(`{}`)}
	err := manager.AddManagedFields(obj, "kubectl", metav1.ManagedFieldsOperationApply, fieldsV1, "")
	require.NoError(t, err)
	err = manager.AddManagedFields(obj, "helm", metav1.ManagedFieldsOperationUpdate, fieldsV1, "")
	require.NoError(t, err)

	// Verify we have 2 entries
	managedFields := obj.GetManagedFields()
	require.Len(t, managedFields, 2)

	// Remove kubectl's managed fields
	err = manager.RemoveManagedFields(obj, "kubectl")
	require.NoError(t, err)

	// Should have only helm's entry left
	managedFields = obj.GetManagedFields()
	require.Len(t, managedFields, 1)
	assert.Equal(t, "helm", managedFields[0].Manager)
}

func TestGetManagedFields(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")
	obj := createTestObject()

	// Initially should have no managed fields
	managedFields, err := manager.GetManagedFields(obj)
	require.NoError(t, err)
	assert.Nil(t, managedFields)

	// Add managed fields
	fieldsV1 := &metav1.FieldsV1{Raw: []byte(`{}`)}
	err = manager.AddManagedFields(obj, "kubectl", metav1.ManagedFieldsOperationApply, fieldsV1, "")
	require.NoError(t, err)

	// Should now have managed fields
	managedFields, err = manager.GetManagedFields(obj)
	require.NoError(t, err)
	require.Len(t, managedFields, 1)
	assert.Equal(t, "kubectl", managedFields[0].Manager)
}

func TestGetFieldsForManager(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")
	obj := createTestObject()

	fieldsV1 := &metav1.FieldsV1{Raw: []byte(`{"spec":{"description":{}}}`)}
	err := manager.AddManagedFields(obj, "kubectl", metav1.ManagedFieldsOperationApply, fieldsV1, "")
	require.NoError(t, err)

	// Get fields for existing manager
	fields, err := manager.GetFieldsForManager(obj, "kubectl")
	require.NoError(t, err)
	assert.Equal(t, fieldsV1, fields)

	// Get fields for non-existing manager
	fields, err = manager.GetFieldsForManager(obj, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, fields)
}

func TestHasManager(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")
	obj := createTestObject()

	// Initially should not have any managers
	hasManager, err := manager.HasManager(obj, "kubectl")
	require.NoError(t, err)
	assert.False(t, hasManager)

	// Add managed fields
	fieldsV1 := &metav1.FieldsV1{Raw: []byte(`{}`)}
	err = manager.AddManagedFields(obj, "kubectl", metav1.ManagedFieldsOperationApply, fieldsV1, "")
	require.NoError(t, err)

	// Should now have the manager
	hasManager, err = manager.HasManager(obj, "kubectl")
	require.NoError(t, err)
	assert.True(t, hasManager)

	// Should not have other managers
	hasManager, err = manager.HasManager(obj, "helm")
	require.NoError(t, err)
	assert.False(t, hasManager)
}

func TestGetManagers(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")
	obj := createTestObject()

	// Initially should have no managers
	managers, err := manager.GetManagers(obj)
	require.NoError(t, err)
	assert.Empty(t, managers)

	// Add managed fields for multiple managers
	fieldsV1 := &metav1.FieldsV1{Raw: []byte(`{}`)}
	err = manager.AddManagedFields(obj, "kubectl", metav1.ManagedFieldsOperationApply, fieldsV1, "")
	require.NoError(t, err)
	err = manager.AddManagedFields(obj, "helm", metav1.ManagedFieldsOperationUpdate, fieldsV1, "")
	require.NoError(t, err)
	err = manager.AddManagedFields(obj, "kubectl", metav1.ManagedFieldsOperationUpdate, fieldsV1, "")
	require.NoError(t, err)

	// Should have unique managers
	managers, err = manager.GetManagers(obj)
	require.NoError(t, err)
	assert.Len(t, managers, 2)
	assert.Contains(t, managers, "kubectl")
	assert.Contains(t, managers, "helm")
}

func TestSerializeDeserializeManagedFields(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")

	// Create test managed fields
	now := metav1.NewTime(time.Now())
	managedFields := []metav1.ManagedFieldsEntry{
		{
			Manager:     "kubectl",
			Operation:   metav1.ManagedFieldsOperationApply,
			APIVersion:  "test/v1",
			Time:        &now,
			FieldsType:  "FieldsV1",
			FieldsV1:    &metav1.FieldsV1{Raw: []byte(`{"spec":{"description":{}}}`)},
			Subresource: "",
		},
	}

	// Serialize
	data, err := manager.SerializeManagedFields(managedFields)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Deserialize
	deserializedFields, err := manager.DeserializeManagedFields(data)
	require.NoError(t, err)
	require.Len(t, deserializedFields, 1)

	entry := deserializedFields[0]
	assert.Equal(t, "kubectl", entry.Manager)
	assert.Equal(t, metav1.ManagedFieldsOperationApply, entry.Operation)
	assert.Equal(t, "test/v1", entry.APIVersion)
	assert.Equal(t, "FieldsV1", entry.FieldsType)
	assert.NotNil(t, entry.FieldsV1)
}

func TestValidateManagedFields(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")

	tests := []struct {
		name          string
		managedFields []metav1.ManagedFieldsEntry
		expectError   bool
		errorMsg      string
	}{
		{
			name: "valid managed fields",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Manager:     "kubectl",
					Operation:   metav1.ManagedFieldsOperationApply,
					APIVersion:  "test/v1",
					Time:        &metav1.Time{Time: time.Now()},
					FieldsType:  "FieldsV1",
					FieldsV1:    &metav1.FieldsV1{Raw: []byte(`{}`)},
					Subresource: "",
				},
			},
			expectError: false,
		},
		{
			name: "empty manager",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Manager:    "",
					Operation:  metav1.ManagedFieldsOperationApply,
					APIVersion: "test/v1",
					Time:       &metav1.Time{Time: time.Now()},
				},
			},
			expectError: true,
			errorMsg:    "manager cannot be empty",
		},
		{
			name: "invalid operation",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Manager:    "kubectl",
					Operation:  "InvalidOperation",
					APIVersion: "test/v1",
					Time:       &metav1.Time{Time: time.Now()},
				},
			},
			expectError: true,
			errorMsg:    "invalid operation",
		},
		{
			name: "empty api version",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Manager:    "kubectl",
					Operation:  metav1.ManagedFieldsOperationApply,
					APIVersion: "",
					Time:       &metav1.Time{Time: time.Now()},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion cannot be empty",
		},
		{
			name: "nil time",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Manager:    "kubectl",
					Operation:  metav1.ManagedFieldsOperationApply,
					APIVersion: "test/v1",
					Time:       nil,
				},
			},
			expectError: true,
			errorMsg:    "time cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateManagedFields(tt.managedFields)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateFieldsV1FromObject(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")
	obj := createTestObject()

	fieldsV1, err := manager.CreateFieldsV1FromObject(obj)
	require.NoError(t, err)
	assert.NotNil(t, fieldsV1)
	assert.NotEmpty(t, fieldsV1.Raw)

	// Verify the fields structure
	var fieldsMap map[string]interface{}
	err = json.Unmarshal(fieldsV1.Raw, &fieldsMap)
	require.NoError(t, err)

	// Should have spec and metadata fields
	assert.Contains(t, fieldsMap, "spec")
	assert.Contains(t, fieldsMap, "metadata")
}

func TestMergeFieldsV1(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")

	base := &metav1.FieldsV1{
		Raw: []byte(`{"spec":{"description":{},"name":{}}}`),
	}
	overlay := &metav1.FieldsV1{
		Raw: []byte(`{"spec":{"labels":{},"ports":{}}}`),
	}

	merged, err := manager.MergeFieldsV1(base, overlay)
	require.NoError(t, err)
	assert.NotNil(t, merged)

	// Verify merged structure contains fields from both
	var mergedMap map[string]interface{}
	err = json.Unmarshal(merged.Raw, &mergedMap)
	require.NoError(t, err)

	spec, ok := mergedMap["spec"].(map[string]interface{})
	require.True(t, ok)

	// Should have fields from both base and overlay
	assert.Contains(t, spec, "description") // from base
	assert.Contains(t, spec, "name")        // from base
	assert.Contains(t, spec, "labels")      // from overlay
	assert.Contains(t, spec, "ports")       // from overlay
}

func TestMergeFieldsV1_NilCases(t *testing.T) {
	manager := NewManagedFieldsManager("test-manager")

	// Both nil
	merged, err := manager.MergeFieldsV1(nil, nil)
	require.NoError(t, err)
	assert.Nil(t, merged)

	// Base nil
	overlay := &metav1.FieldsV1{Raw: []byte(`{"spec":{}}`)}
	merged, err = manager.MergeFieldsV1(nil, overlay)
	require.NoError(t, err)
	assert.Equal(t, overlay, merged)

	// Overlay nil
	base := &metav1.FieldsV1{Raw: []byte(`{"spec":{}}`)}
	merged, err = manager.MergeFieldsV1(base, nil)
	require.NoError(t, err)
	assert.Equal(t, base, merged)
}
