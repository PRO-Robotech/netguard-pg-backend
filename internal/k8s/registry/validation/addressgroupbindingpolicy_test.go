package validation

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestAddressGroupBindingPolicyValidator_ValidateCreate(t *testing.T) {
	validator := NewAddressGroupBindingPolicyValidator()
	ctx := context.Background()

	tests := []struct {
		name        string
		policy      *v1beta1.AddressGroupBindingPolicy
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid AddressGroupBindingPolicy",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-policy",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			expectError: false,
		},
		{
			name:        "nil AddressGroupBindingPolicy object",
			policy:      nil,
			expectError: true,
			errorMsg:    "addressgroupbindingpolicy object cannot be nil",
		},
		{
			name: "missing name in metadata",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
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
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Policy_Name",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "missing AddressGroupRef fields",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-ag-ref",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						// Missing required fields
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "wrong API version in AddressGroupRef",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-ag-api",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "v1", // Wrong API version
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion must be 'netguard.sgroups.io/v1beta1'",
		},
		{
			name: "wrong kind in AddressGroupRef",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-ag-kind",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service", // Should be AddressGroup
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'AddressGroup'",
		},
		{
			name: "missing ServiceRef fields",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-service-ref",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						// Missing required fields
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "wrong API version in ServiceRef",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-service-api",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "networking.k8s.io/v1", // Wrong API version
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion must be 'netguard.sgroups.io/v1beta1'",
		},
		{
			name: "wrong kind in ServiceRef",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-service-kind",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup", // Should be Service
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'Service'",
		},
		{
			name: "missing namespace in AddressGroupRef",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-ag-ns",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						// Missing namespace
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "namespace is required",
		},
		{
			name: "missing namespace in ServiceRef",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-service-ns",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						// Missing namespace
					},
				},
			},
			expectError: true,
			errorMsg:    "namespace is required",
		},
		{
			name: "invalid name format in AddressGroupRef",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-ag-name",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "Invalid_AddressGroup_Name",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "invalid name format in ServiceRef",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-service-name",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "Invalid_Service_Name",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "cross-namespace references (valid)",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cross-ns-policy",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "addressgroup-in-other-ns",
						},
						Namespace: "other-namespace",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-in-different-ns",
						},
						Namespace: "service-namespace",
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid generateName instead of name",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "auto-policy",
					Namespace:    "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid generateName format",
			policy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "Invalid_GenerateName",
					Namespace:    "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateCreate(ctx, tt.policy)

			if tt.expectError {
				if len(errors) == 0 {
					t.Errorf("expected validation error but got none")
					return
				}

				// Check if the expected error message is found
				found := false
				fullErrorText := errors.ToAggregate().Error()
				if containsString(fullErrorText, tt.errorMsg) {
					found = true
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

func TestAddressGroupBindingPolicyValidator_ValidateUpdate(t *testing.T) {
	validator := NewAddressGroupBindingPolicyValidator()
	ctx := context.Background()

	basePolicy := &v1beta1.AddressGroupBindingPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: v1beta1.AddressGroupBindingPolicySpec{
			AddressGroupRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "my-addressgroup",
				},
				Namespace: "default",
			},
			ServiceRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       "my-service",
				},
				Namespace: "default",
			},
		},
	}

	tests := []struct {
		name        string
		newPolicy   *v1beta1.AddressGroupBindingPolicy
		oldPolicy   *v1beta1.AddressGroupBindingPolicy
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid update (no reference changes)",
			newPolicy:   basePolicy.DeepCopy(),
			oldPolicy:   basePolicy,
			expectError: false,
		},
		{
			name: "attempt to change AddressGroupRef (immutable)",
			newPolicy: func() *v1beta1.AddressGroupBindingPolicy {
				policy := basePolicy.DeepCopy()
				policy.Spec.AddressGroupRef.Name = "different-addressgroup"
				return policy
			}(),
			oldPolicy:   basePolicy,
			expectError: true,
			errorMsg:    "addressGroupRef is immutable and cannot be changed",
		},
		{
			name: "attempt to change ServiceRef (immutable)",
			newPolicy: func() *v1beta1.AddressGroupBindingPolicy {
				policy := basePolicy.DeepCopy()
				policy.Spec.ServiceRef.Name = "different-service"
				return policy
			}(),
			oldPolicy:   basePolicy,
			expectError: true,
			errorMsg:    "serviceRef is immutable and cannot be changed",
		},
		{
			name: "attempt to change AddressGroupRef kind (immutable)",
			newPolicy: func() *v1beta1.AddressGroupBindingPolicy {
				policy := basePolicy.DeepCopy()
				policy.Spec.AddressGroupRef.Kind = "Service"
				return policy
			}(),
			oldPolicy:   basePolicy,
			expectError: true,
			errorMsg:    "addressGroupRef is immutable and cannot be changed",
		},
		{
			name: "attempt to change ServiceRef API version (immutable)",
			newPolicy: func() *v1beta1.AddressGroupBindingPolicy {
				policy := basePolicy.DeepCopy()
				policy.Spec.ServiceRef.APIVersion = "v1"
				return policy
			}(),
			oldPolicy:   basePolicy,
			expectError: true,
			errorMsg:    "serviceRef is immutable and cannot be changed",
		},
		{
			name: "attempt to change AddressGroupRef namespace (immutable)",
			newPolicy: func() *v1beta1.AddressGroupBindingPolicy {
				policy := basePolicy.DeepCopy()
				policy.Spec.AddressGroupRef.Namespace = "different-namespace"
				return policy
			}(),
			oldPolicy:   basePolicy,
			expectError: true,
			errorMsg:    "addressGroupRef is immutable and cannot be changed",
		},
		{
			name: "attempt to change ServiceRef namespace (immutable)",
			newPolicy: func() *v1beta1.AddressGroupBindingPolicy {
				policy := basePolicy.DeepCopy()
				policy.Spec.ServiceRef.Namespace = "different-namespace"
				return policy
			}(),
			oldPolicy:   basePolicy,
			expectError: true,
			errorMsg:    "serviceRef is immutable and cannot be changed",
		},
		{
			name: "valid metadata update (labels, annotations)",
			newPolicy: func() *v1beta1.AddressGroupBindingPolicy {
				policy := basePolicy.DeepCopy()
				policy.Labels = map[string]string{"new-label": "new-value"}
				policy.Annotations = map[string]string{"new-annotation": "new-value"}
				return policy
			}(),
			oldPolicy:   basePolicy,
			expectError: false,
		},
		{
			name: "update with invalid new data",
			newPolicy: &v1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "my-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
				},
			},
			oldPolicy:   basePolicy,
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateUpdate(ctx, tt.newPolicy, tt.oldPolicy)

			if tt.expectError {
				if len(errors) == 0 {
					t.Errorf("expected validation error but got none")
					return
				}

				// Check if the expected error message is found
				found := false
				fullErrorText := errors.ToAggregate().Error()
				if containsString(fullErrorText, tt.errorMsg) {
					found = true
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

func TestAddressGroupBindingPolicyValidator_ValidateDelete(t *testing.T) {
	validator := NewAddressGroupBindingPolicyValidator()
	ctx := context.Background()

	policy := &v1beta1.AddressGroupBindingPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: v1beta1.AddressGroupBindingPolicySpec{
			AddressGroupRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "my-addressgroup",
				},
				Namespace: "default",
			},
			ServiceRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       "my-service",
				},
				Namespace: "default",
			},
		},
	}

	errors := validator.ValidateDelete(ctx, policy)
	if len(errors) > 0 {
		t.Errorf("expected no validation errors for delete but got: %v", errors)
	}
}
