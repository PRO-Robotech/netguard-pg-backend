package patch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	return &TestObject{
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
}

func TestParseJSONPatch(t *testing.T) {
	tests := []struct {
		name        string
		patchData   string
		expectError bool
		errorMsg    string
		expected    []JSONPatchOperation
	}{
		{
			name:      "valid add operation",
			patchData: `[{"op": "add", "path": "/spec/newField", "value": "newValue"}]`,
			expected: []JSONPatchOperation{
				{Op: "add", Path: "/spec/newField", Value: "newValue"},
			},
		},
		{
			name:      "valid remove operation",
			patchData: `[{"op": "remove", "path": "/spec/description"}]`,
			expected: []JSONPatchOperation{
				{Op: "remove", Path: "/spec/description"},
			},
		},
		{
			name:      "valid replace operation",
			patchData: `[{"op": "replace", "path": "/spec/description", "value": "updated description"}]`,
			expected: []JSONPatchOperation{
				{Op: "replace", Path: "/spec/description", Value: "updated description"},
			},
		},
		{
			name:      "valid move operation",
			patchData: `[{"op": "move", "from": "/spec/name", "path": "/spec/newName"}]`,
			expected: []JSONPatchOperation{
				{Op: "move", Path: "/spec/newName", From: "/spec/name"},
			},
		},
		{
			name:      "valid copy operation",
			patchData: `[{"op": "copy", "from": "/spec/name", "path": "/spec/copyName"}]`,
			expected: []JSONPatchOperation{
				{Op: "copy", Path: "/spec/copyName", From: "/spec/name"},
			},
		},
		{
			name:      "valid test operation",
			patchData: `[{"op": "test", "path": "/spec/name", "value": "test-spec"}]`,
			expected: []JSONPatchOperation{
				{Op: "test", Path: "/spec/name", Value: "test-spec"},
			},
		},
		{
			name:      "multiple operations",
			patchData: `[{"op": "replace", "path": "/spec/description", "value": "new desc"}, {"op": "add", "path": "/spec/newField", "value": "value"}]`,
			expected: []JSONPatchOperation{
				{Op: "replace", Path: "/spec/description", Value: "new desc"},
				{Op: "add", Path: "/spec/newField", Value: "value"},
			},
		},
		{
			name:        "empty patch data",
			patchData:   "",
			expectError: true,
			errorMsg:    "empty patch data",
		},
		{
			name:        "invalid JSON",
			patchData:   `[{"op": "add", "path": "/spec/field", "value":}]`,
			expectError: true,
			errorMsg:    "invalid JSON patch format",
		},
		{
			name:        "empty operations array",
			patchData:   `[]`,
			expectError: true,
			errorMsg:    "patch must contain at least one operation",
		},
		{
			name:        "unsupported operation",
			patchData:   `[{"op": "invalid", "path": "/spec/field", "value": "value"}]`,
			expectError: true,
			errorMsg:    "unsupported operation 'invalid'",
		},
		{
			name:        "missing path",
			patchData:   `[{"op": "add", "value": "value"}]`,
			expectError: true,
			errorMsg:    "path is required",
		},
		{
			name:        "path not starting with slash",
			patchData:   `[{"op": "add", "path": "spec/field", "value": "value"}]`,
			expectError: true,
			errorMsg:    "path must start with '/'",
		},
		{
			name:        "add operation missing value",
			patchData:   `[{"op": "add", "path": "/spec/field"}]`,
			expectError: true,
			errorMsg:    "'add' operation requires a value",
		},
		{
			name:        "replace operation missing value",
			patchData:   `[{"op": "replace", "path": "/spec/field"}]`,
			expectError: true,
			errorMsg:    "'replace' operation requires a value",
		},
		{
			name:        "test operation missing value",
			patchData:   `[{"op": "test", "path": "/spec/field"}]`,
			expectError: true,
			errorMsg:    "'test' operation requires a value",
		},
		{
			name:        "move operation missing from",
			patchData:   `[{"op": "move", "path": "/spec/field"}]`,
			expectError: true,
			errorMsg:    "'move' operation requires a 'from' field",
		},
		{
			name:        "copy operation missing from",
			patchData:   `[{"op": "copy", "path": "/spec/field"}]`,
			expectError: true,
			errorMsg:    "'copy' operation requires a 'from' field",
		},
		{
			name:        "move operation from not starting with slash",
			patchData:   `[{"op": "move", "path": "/spec/field", "from": "spec/source"}]`,
			expectError: true,
			errorMsg:    "'from' field must start with '/'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseJSONPatch([]byte(tt.patchData))

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestApplyJSONPatch(t *testing.T) {
	tests := []struct {
		name        string
		obj         runtime.Object
		patchData   string
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, result runtime.Object)
	}{
		{
			name:      "add existing field to spec",
			obj:       createTestObject(),
			patchData: `[{"op": "add", "path": "/spec/labels/newLabel", "value": "newValue"}]`,
			validate: func(t *testing.T, result runtime.Object) {
				testObj := result.(*TestObject)
				assert.Equal(t, "newValue", testObj.Spec.Labels["newLabel"])
			},
		},
		{
			name:      "replace description",
			obj:       createTestObject(),
			patchData: `[{"op": "replace", "path": "/spec/description", "value": "updated description"}]`,
			validate: func(t *testing.T, result runtime.Object) {
				testObj := result.(*TestObject)
				assert.Equal(t, "updated description", testObj.Spec.Description)
			},
		},
		{
			name:      "remove description",
			obj:       createTestObject(),
			patchData: `[{"op": "remove", "path": "/spec/description"}]`,
			validate: func(t *testing.T, result runtime.Object) {
				testObj := result.(*TestObject)
				assert.Empty(t, testObj.Spec.Description)
			},
		},
		{
			name:      "add to array",
			obj:       createTestObject(),
			patchData: `[{"op": "add", "path": "/spec/ports/-", "value": 3000}]`,
			validate: func(t *testing.T, result runtime.Object) {
				testObj := result.(*TestObject)
				assert.Contains(t, testObj.Spec.Ports, 3000)
				assert.Len(t, testObj.Spec.Ports, 3)
			},
		},
		{
			name:      "test operation success",
			obj:       createTestObject(),
			patchData: `[{"op": "test", "path": "/spec/name", "value": "test-spec"}]`,
			validate: func(t *testing.T, result runtime.Object) {
				testObj := result.(*TestObject)
				assert.Equal(t, "test-spec", testObj.Spec.Name)
			},
		},
		{
			name:        "nil object",
			obj:         nil,
			patchData:   `[{"op": "add", "path": "/spec/field", "value": "value"}]`,
			expectError: true,
			errorMsg:    "cannot apply patch to nil object",
		},
		{
			name:        "invalid patch format",
			obj:         createTestObject(),
			patchData:   `[{"op": "invalid", "path": "/spec/field", "value": "value"}]`,
			expectError: true,
			errorMsg:    "invalid JSON patch",
		},
		{
			name:        "test operation failure",
			obj:         createTestObject(),
			patchData:   `[{"op": "test", "path": "/spec/name", "value": "wrong-value"}]`,
			expectError: true,
			errorMsg:    "failed to apply JSON patch",
		},
		{
			name:        "invalid path",
			obj:         createTestObject(),
			patchData:   `[{"op": "add", "path": "/nonexistent/deeply/nested/path", "value": "value"}]`,
			expectError: true,
			errorMsg:    "failed to apply JSON patch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ApplyJSONPatch(tt.obj, []byte(tt.patchData))

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestValidateOperation(t *testing.T) {
	tests := []struct {
		name        string
		op          JSONPatchOperation
		index       int
		expectError bool
		errorMsg    string
	}{
		{
			name:  "valid add operation",
			op:    JSONPatchOperation{Op: "add", Path: "/spec/field", Value: "value"},
			index: 0,
		},
		{
			name:  "valid remove operation",
			op:    JSONPatchOperation{Op: "remove", Path: "/spec/field"},
			index: 0,
		},
		{
			name:  "valid move operation",
			op:    JSONPatchOperation{Op: "move", Path: "/spec/dest", From: "/spec/source"},
			index: 0,
		},
		{
			name:        "unsupported operation",
			op:          JSONPatchOperation{Op: "invalid", Path: "/spec/field"},
			index:       1,
			expectError: true,
			errorMsg:    "operation 1: unsupported operation 'invalid'",
		},
		{
			name:        "empty path",
			op:          JSONPatchOperation{Op: "add", Path: "", Value: "value"},
			index:       2,
			expectError: true,
			errorMsg:    "operation 2: path is required",
		},
		{
			name:        "path not starting with slash",
			op:          JSONPatchOperation{Op: "add", Path: "spec/field", Value: "value"},
			index:       3,
			expectError: true,
			errorMsg:    "operation 3: path must start with '/'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOperation(tt.op, tt.index)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSupportedOperations(t *testing.T) {
	expectedOps := []string{"add", "remove", "replace", "move", "copy", "test"}

	for _, op := range expectedOps {
		assert.True(t, SupportedOperations[op], "Operation %s should be supported", op)
	}

	// Test unsupported operation
	assert.False(t, SupportedOperations["invalid"], "Invalid operation should not be supported")
}
