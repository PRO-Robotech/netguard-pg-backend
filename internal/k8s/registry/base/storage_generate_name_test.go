package base

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestHandleGeneratedName(t *testing.T) {
	// Create a test storage instance
	storage := &BaseStorage[*v1beta1.Service, any]{}

	tests := []struct {
		name        string
		object      runtime.Object
		expectName  bool
		expectError bool
	}{
		{
			name: "generateName set, name empty - should generate name",
			object: &v1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-service-",
					Namespace:    "default",
				},
			},
			expectName:  true,
			expectError: false,
		},
		{
			name: "both generateName and name set - should keep existing name",
			object: &v1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:         "existing-service",
					GenerateName: "test-service-",
					Namespace:    "default",
				},
			},
			expectName:  true,
			expectError: false,
		},
		{
			name: "neither generateName nor name set - should not generate name",
			object: &v1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
			},
			expectName:  false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get the object's metadata before processing
			accessor, err := meta.Accessor(tt.object)
			if err != nil {
				t.Fatalf("Failed to get object accessor: %v", err)
			}

			originalName := accessor.GetName()
			originalGenerateName := accessor.GetGenerateName()

			// Process the object
			err = storage.handleGeneratedName(tt.object)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check name expectation
			finalName := accessor.GetName()

			if tt.expectName && finalName == "" {
				t.Error("Expected name to be set but it's empty")
			}

			// Specific test cases
			switch tt.name {
			case "generateName set, name empty - should generate name":
				if finalName == "" {
					t.Error("Expected generated name but got empty")
				}
				if finalName == originalGenerateName {
					t.Error("Generated name should not be exactly the same as generateName prefix")
				}
				if len(finalName) <= len(originalGenerateName) {
					t.Error("Generated name should be longer than the prefix")
				}

			case "both generateName and name set - should keep existing name":
				if finalName != originalName {
					t.Errorf("Expected to keep original name %q but got %q", originalName, finalName)
				}

			case "neither generateName nor name set - should not generate name":
				if finalName != "" {
					t.Errorf("Expected empty name but got %q", finalName)
				}
			}
		})
	}
}

func TestHandleGeneratedNameUniqueness(t *testing.T) {
	storage := &BaseStorage[*v1beta1.Service, any]{}

	// Create multiple objects with the same generateName
	objects := make([]*v1beta1.Service, 5)
	names := make([]string, 5)

	for i := 0; i < 5; i++ {
		objects[i] = &v1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-service-",
				Namespace:    "default",
			},
		}

		err := storage.handleGeneratedName(objects[i])
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		accessor, _ := meta.Accessor(objects[i])
		names[i] = accessor.GetName()
	}

	// Check that all generated names are unique
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[i] == names[j] {
				t.Errorf("Generated names are not unique: %q appears multiple times", names[i])
			}
		}
	}

	// Check that all names start with the prefix
	for _, name := range names {
		if len(name) <= len("test-service-") || name[:len("test-service-")] != "test-service-" {
			t.Errorf("Generated name %q does not start with expected prefix", name)
		}
	}
}
