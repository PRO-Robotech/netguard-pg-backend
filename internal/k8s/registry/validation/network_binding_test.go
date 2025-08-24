package validation

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestNetworkBindingValidator_ValidateCreate(t *testing.T) {
	validator := NewNetworkBindingValidator()
	ctx := context.Background()

	tests := []struct {
		name        string
		binding     *v1beta1.NetworkBinding
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid NetworkBinding",
			binding: &v1beta1.NetworkBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-binding",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkBindingSpec{
					NetworkRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Network",
						Name:       "my-network",
					},
					AddressGroupRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "my-addressgroup",
					},
				},
			},
			expectError: false,
		},
		{
			name:        "nil NetworkBinding object",
			binding:     nil,
			expectError: true,
			errorMsg:    "networkbinding object cannot be nil",
		},
		{
			name: "missing name in metadata",
			binding: &v1beta1.NetworkBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: v1beta1.NetworkBindingSpec{
					NetworkRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Network",
						Name:       "my-network",
					},
					AddressGroupRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "my-addressgroup",
					},
				},
			},
			expectError: true,
			errorMsg:    "name or generateName is required",
		},
		{
			name: "invalid name format",
			binding: &v1beta1.NetworkBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Binding_Name",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkBindingSpec{
					NetworkRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Network",
						Name:       "my-network",
					},
					AddressGroupRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "my-addressgroup",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "missing NetworkRef fields",
			binding: &v1beta1.NetworkBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-network-ref",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkBindingSpec{
					NetworkRef: v1beta1.ObjectReference{
						// Missing required fields
					},
					AddressGroupRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "my-addressgroup",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "wrong kind in NetworkRef",
			binding: &v1beta1.NetworkBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-network-kind",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkBindingSpec{
					NetworkRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service", // Should be Network
						Name:       "my-network",
					},
					AddressGroupRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "my-addressgroup",
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'Network'",
		},
		{
			name: "wrong kind in AddressGroupRef",
			binding: &v1beta1.NetworkBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-ag-kind",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkBindingSpec{
					NetworkRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Network",
						Name:       "my-network",
					},
					AddressGroupRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service", // Should be AddressGroup
						Name:       "my-addressgroup",
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'AddressGroup'",
		},
		{
			name: "wrong API version in NetworkRef",
			binding: &v1beta1.NetworkBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-api-version",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkBindingSpec{
					NetworkRef: v1beta1.ObjectReference{
						APIVersion: "v1", // Wrong API version
						Kind:       "Network",
						Name:       "my-network",
					},
					AddressGroupRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "my-addressgroup",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion must be 'netguard.sgroups.io/v1beta1'",
		},
		{
			name: "invalid name format in NetworkRef",
			binding: &v1beta1.NetworkBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-ref-name",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkBindingSpec{
					NetworkRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Network",
						Name:       "Invalid_Network_Name",
					},
					AddressGroupRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "my-addressgroup",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
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

func TestNetworkBindingValidator_ValidateUpdate(t *testing.T) {
	validator := NewNetworkBindingValidator()
	ctx := context.Background()

	baseBinding := &v1beta1.NetworkBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-binding",
			Namespace: "default",
		},
		Spec: v1beta1.NetworkBindingSpec{
			NetworkRef: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Network",
				Name:       "my-network",
			},
			AddressGroupRef: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "my-addressgroup",
			},
		},
	}

	baseBindingReady := &v1beta1.NetworkBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ready-binding",
			Namespace: "default",
		},
		Spec: v1beta1.NetworkBindingSpec{
			NetworkRef: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Network",
				Name:       "my-network",
			},
			AddressGroupRef: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "my-addressgroup",
			},
		},
		Status: v1beta1.NetworkBindingStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: "True", // Ready
				},
			},
		},
	}

	tests := []struct {
		name        string
		newBinding  *v1beta1.NetworkBinding
		oldBinding  *v1beta1.NetworkBinding
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
			name:        "valid update when ready (no reference changes)",
			newBinding:  baseBindingReady.DeepCopy(),
			oldBinding:  baseBindingReady,
			expectError: false,
		},
		{
			name: "attempt to change networkRef when ready (immutable)",
			newBinding: func() *v1beta1.NetworkBinding {
				binding := baseBindingReady.DeepCopy()
				binding.Spec.NetworkRef.Name = "different-network"
				return binding
			}(),
			oldBinding:  baseBindingReady,
			expectError: true,
			errorMsg:    "cannot change networkRef when Ready condition is true",
		},
		{
			name: "attempt to change addressGroupRef when ready (immutable)",
			newBinding: func() *v1beta1.NetworkBinding {
				binding := baseBindingReady.DeepCopy()
				binding.Spec.AddressGroupRef.Name = "different-addressgroup"
				return binding
			}(),
			oldBinding:  baseBindingReady,
			expectError: true,
			errorMsg:    "cannot change addressGroupRef when Ready condition is true",
		},
		{
			name: "update with invalid new data",
			newBinding: &v1beta1.NetworkBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkBindingSpec{
					NetworkRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Network",
						Name:       "my-network",
					},
					AddressGroupRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "my-addressgroup",
					},
				},
			},
			oldBinding:  baseBinding,
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
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

func TestNetworkBindingValidator_ValidateDelete(t *testing.T) {
	validator := NewNetworkBindingValidator()
	ctx := context.Background()

	binding := &v1beta1.NetworkBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-binding",
			Namespace: "default",
		},
		Spec: v1beta1.NetworkBindingSpec{
			NetworkRef: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Network",
				Name:       "my-network",
			},
			AddressGroupRef: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "my-addressgroup",
			},
		},
	}

	errors := validator.ValidateDelete(ctx, binding)
	if len(errors) > 0 {
		t.Errorf("expected no validation errors for delete but got: %v", errors)
	}
}
