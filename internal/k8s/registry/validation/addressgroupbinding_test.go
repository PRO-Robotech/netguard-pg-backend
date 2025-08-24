package validation

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestAddressGroupBindingValidator_ValidateCreate(t *testing.T) {
	validator := NewAddressGroupBindingValidator()
	ctx := context.Background()

	tests := []struct {
		name        string
		binding     *v1beta1.AddressGroupBinding
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid AddressGroupBinding",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-binding",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
				},
			},
			expectError: false,
		},
		{
			name:        "nil AddressGroupBinding object",
			binding:     nil,
			expectError: true,
			errorMsg:    "addressgroupbinding object cannot be nil",
		},
		{
			name: "missing name in metadata",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "name or generateName is required",
		},
		{
			name: "invalid name format",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Binding_Name",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "missing ServiceRef fields",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-service-ref",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						// Missing required fields
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "wrong kind in ServiceRef",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-service-kind",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup", // Should be Service
						Name:       "my-service",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'Service'",
		},
		{
			name: "wrong API version in ServiceRef",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-service-api",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "v1", // Wrong API version
						Kind:       "Service",
						Name:       "my-service",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion must be 'netguard.sgroups.io/v1beta1'",
		},
		{
			name: "missing AddressGroupRef fields",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-ag-ref",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						// Missing required fields
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "wrong kind in AddressGroupRef",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-ag-kind",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service", // Should be AddressGroup
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'AddressGroup'",
		},
		{
			name: "wrong API version in AddressGroupRef",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-ag-api",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "networking.k8s.io/v1", // Wrong API version
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion must be 'netguard.sgroups.io/v1beta1'",
		},
		{
			name: "missing namespace in AddressGroupRef",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-ag-ns",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						// Missing namespace
					},
				},
			},
			expectError: true,
			errorMsg:    "namespace is required",
		},
		{
			name: "invalid name format in ServiceRef",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-service-name",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "Invalid_Service_Name",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "invalid name format in AddressGroupRef",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-ag-name",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "Invalid_AddressGroup_Name",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "cross-namespace binding (valid)",
			binding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cross-ns-binding",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "service-in-default",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "addressgroup-in-other-ns",
						},
						Namespace: "other-namespace",
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateCreate(ctx, tt.binding)

			if tt.expectError {
				if len(errors) == 0 {
					t.Errorf("expected validation error but got none")
					return
				}

				// Check if the expected error message is found
				found := false
				for _, err := range errors {
					if err.Detail != "" && containsString(err.Detail, tt.errorMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error message containing '%s', but got: %v", tt.errorMsg, errors)
				}
			} else {
				if len(errors) > 0 {
					t.Errorf("expected no validation errors but got: %v", errors)
				}
			}
		})
	}
}

func TestAddressGroupBindingValidator_ValidateUpdate(t *testing.T) {
	validator := NewAddressGroupBindingValidator()
	ctx := context.Background()

	baseBinding := &v1beta1.AddressGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-binding",
			Namespace: "default",
		},
		Spec: v1beta1.AddressGroupBindingSpec{
			ServiceRef: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "my-service",
			},
			AddressGroupRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "my-addressgroup",
				},
				Namespace: "default",
			},
		},
	}

	tests := []struct {
		name        string
		newBinding  *v1beta1.AddressGroupBinding
		oldBinding  *v1beta1.AddressGroupBinding
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid update (no reference changes)",
			newBinding:  baseBinding.DeepCopy(),
			oldBinding:  baseBinding,
			expectError: false,
		},
		{
			name: "attempt to change serviceRef (immutable)",
			newBinding: func() *v1beta1.AddressGroupBinding {
				binding := baseBinding.DeepCopy()
				binding.Spec.ServiceRef.Name = "different-service"
				return binding
			}(),
			oldBinding:  baseBinding,
			expectError: true,
			errorMsg:    "serviceRef is immutable and cannot be changed",
		},
		{
			name: "attempt to change addressGroupRef (immutable)",
			newBinding: func() *v1beta1.AddressGroupBinding {
				binding := baseBinding.DeepCopy()
				binding.Spec.AddressGroupRef.Name = "different-addressgroup"
				return binding
			}(),
			oldBinding:  baseBinding,
			expectError: true,
			errorMsg:    "addressGroupRef is immutable and cannot be changed",
		},
		{
			name: "attempt to change serviceRef kind (immutable)",
			newBinding: func() *v1beta1.AddressGroupBinding {
				binding := baseBinding.DeepCopy()
				binding.Spec.ServiceRef.Kind = "ServiceAlias"
				return binding
			}(),
			oldBinding:  baseBinding,
			expectError: true,
			errorMsg:    "serviceRef is immutable and cannot be changed",
		},
		{
			name: "attempt to change addressGroupRef namespace (immutable)",
			newBinding: func() *v1beta1.AddressGroupBinding {
				binding := baseBinding.DeepCopy()
				binding.Spec.AddressGroupRef.Namespace = "different-namespace"
				return binding
			}(),
			oldBinding:  baseBinding,
			expectError: true,
			errorMsg:    "addressGroupRef is immutable and cannot be changed",
		},
		{
			name: "update with invalid new data",
			newBinding: &v1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
				},
			},
			oldBinding:  baseBinding,
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "valid metadata update (labels, annotations)",
			newBinding: func() *v1beta1.AddressGroupBinding {
				binding := baseBinding.DeepCopy()
				binding.Labels = map[string]string{"new-label": "new-value"}
				binding.Annotations = map[string]string{"new-annotation": "new-value"}
				return binding
			}(),
			oldBinding:  baseBinding,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateUpdate(ctx, tt.newBinding, tt.oldBinding)

			if tt.expectError {
				if len(errors) == 0 {
					t.Errorf("expected validation error but got none")
					return
				}

				// Check if the expected error message is found
				found := false
				for _, err := range errors {
					if err.Detail != "" && containsString(err.Detail, tt.errorMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error message containing '%s', but got: %v", tt.errorMsg, errors)
				}
			} else {
				if len(errors) > 0 {
					t.Errorf("expected no validation errors but got: %v", errors)
				}
			}
		})
	}
}

func TestAddressGroupBindingValidator_ValidateDelete(t *testing.T) {
	validator := NewAddressGroupBindingValidator()
	ctx := context.Background()

	binding := &v1beta1.AddressGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-binding",
			Namespace: "default",
		},
		Spec: v1beta1.AddressGroupBindingSpec{
			ServiceRef: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "my-service",
			},
			AddressGroupRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "my-addressgroup",
				},
				Namespace: "default",
			},
		},
	}

	errors := validator.ValidateDelete(ctx, binding)
	if len(errors) > 0 {
		t.Errorf("expected no validation errors for delete but got: %v", errors)
	}
}
