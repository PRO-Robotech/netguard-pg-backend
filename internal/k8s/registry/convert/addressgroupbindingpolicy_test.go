package convert

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestAddressGroupBindingPolicyConverter_ToDomain(t *testing.T) {
	converter := NewAddressGroupBindingPolicyConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *netguardv1beta1.AddressGroupBindingPolicy
		expected    *models.AddressGroupBindingPolicy
		expectError bool
	}{
		{
			name: "valid_policy_with_minimal_fields",
			input: &netguardv1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: "default",
					UID:       types.UID("test-uid-123"),
				},
				Spec: netguardv1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "test-addressgroup",
						},
						Namespace: "default",
					},
					ServiceRef: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "test-service",
						},
						Namespace: "default",
					},
				},
				Status: netguardv1beta1.AddressGroupBindingPolicyStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "PolicyActive",
						},
					},
				},
			},
			expected: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-policy",
						Namespace: "default",
					},
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-addressgroup",
						Namespace: "default",
					},
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service",
						Namespace: "default",
					},
				},
				Meta: models.Meta{
					UID: "test-uid-123",
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "PolicyActive",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid_policy_cross_namespace",
			input: &netguardv1beta1.AddressGroupBindingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-policy-cross",
					Namespace:       "policy-namespace",
					UID:             types.UID("test-uid-456"),
					ResourceVersion: "789",
					Generation:      2,
					CreationTimestamp: metav1.Time{
						Time: time.Now(),
					},
				},
				Spec: netguardv1beta1.AddressGroupBindingPolicySpec{
					AddressGroupRef: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "cross-addressgroup",
						},
						Namespace: "addressgroup-namespace",
					},
					ServiceRef: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "cross-service",
						},
						Namespace: "service-namespace",
					},
				},
				Status: netguardv1beta1.AddressGroupBindingPolicyStatus{
					ObservedGeneration: 2,
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionFalse,
							Reason: "CrossNamespacePolicy",
						},
					},
				},
			},
			expected: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-policy-cross",
						Namespace: "policy-namespace",
					},
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "cross-addressgroup",
						Namespace: "addressgroup-namespace",
					},
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "cross-service",
						Namespace: "service-namespace",
					},
				},
				Meta: models.Meta{
					UID: "test-uid-456",
				},
			},
			expectError: false,
		},
		{
			name:        "nil input",
			input:       nil,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.ToDomain(ctx, tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tc.expected.Name, result.Name)
				assert.Equal(t, tc.expected.Namespace, result.Namespace)
				assert.Equal(t, tc.expected.AddressGroupRef.Name, result.AddressGroupRef.Name)
				assert.Equal(t, tc.expected.AddressGroupRef.Namespace, result.AddressGroupRef.Namespace)
				assert.Equal(t, tc.expected.ServiceRef.Name, result.ServiceRef.Name)
				assert.Equal(t, tc.expected.ServiceRef.Namespace, result.ServiceRef.Namespace)
				if tc.expected.Meta.UID != "" {
					assert.Equal(t, tc.expected.Meta.UID, result.Meta.UID)
				}
				if tc.expected.Meta.Conditions != nil {
					assert.Equal(t, len(tc.expected.Meta.Conditions), len(result.Meta.Conditions))
				}
			}
		})
	}
}

func TestAddressGroupBindingPolicyConverter_FromDomain(t *testing.T) {
	converter := NewAddressGroupBindingPolicyConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *models.AddressGroupBindingPolicy
		expectError bool
	}{
		{
			name: "valid_policy",
			input: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-policy",
						Namespace: "default",
					},
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-addressgroup",
						Namespace: "default",
					},
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service",
						Namespace: "default",
					},
				},
				Meta: models.Meta{
					UID:             "test-uid-123",
					ResourceVersion: "123",
					Generation:      1,
					CreationTS:      metav1.NewTime(time.Now()),
				},
			},
			expectError: false,
		},
		{
			name: "cross_namespace_policy",
			input: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-policy-cross",
						Namespace: "policy-namespace",
					},
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "cross-addressgroup",
						Namespace: "addressgroup-namespace",
					},
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "cross-service",
						Namespace: "service-namespace",
					},
				},
				Meta: models.Meta{
					UID:             "test-uid-456",
					ResourceVersion: "456",
					Generation:      1,
					CreationTS:      metav1.NewTime(time.Now()),
				},
			},
			expectError: false,
		},
		{
			name:        "nil input",
			input:       nil,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.FromDomain(ctx, tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, "netguard.sgroups.io/v1beta1", result.APIVersion)
				assert.Equal(t, "AddressGroupBindingPolicy", result.Kind)
				assert.Equal(t, tc.input.Name, result.Name)
				assert.Equal(t, tc.input.Namespace, result.Namespace)
				assert.Equal(t, tc.input.AddressGroupRef.Name, result.Spec.AddressGroupRef.Name)
				assert.Equal(t, tc.input.AddressGroupRef.Namespace, result.Spec.AddressGroupRef.Namespace)
				assert.Equal(t, tc.input.ServiceRef.Name, result.Spec.ServiceRef.Name)
				assert.Equal(t, tc.input.ServiceRef.Namespace, result.Spec.ServiceRef.Namespace)
			}
		})
	}
}

func TestAddressGroupBindingPolicyConverter_RoundTrip(t *testing.T) {
	converter := NewAddressGroupBindingPolicyConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		original *models.AddressGroupBindingPolicy
	}{
		{
			name: "basic_policy",
			original: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-policy",
						Namespace: "default",
					},
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-addressgroup",
						Namespace: "default",
					},
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service",
						Namespace: "default",
					},
				},
				Meta: models.Meta{
					UID:             "test-uid-123",
					ResourceVersion: "123",
					Generation:      1,
					CreationTS:      metav1.NewTime(time.Now()),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Domain -> K8s -> Domain
			k8sObj, err := converter.FromDomain(ctx, tc.original)
			require.NoError(t, err)
			require.NotNil(t, k8sObj)

			restored, err := converter.ToDomain(ctx, k8sObj)
			require.NoError(t, err)
			require.NotNil(t, restored)

			// Verify round-trip preserves core fields
			assert.Equal(t, tc.original.Name, restored.Name)
			assert.Equal(t, tc.original.Namespace, restored.Namespace)
			assert.Equal(t, tc.original.AddressGroupRef.Name, restored.AddressGroupRef.Name)
			assert.Equal(t, tc.original.AddressGroupRef.Namespace, restored.AddressGroupRef.Namespace)
			assert.Equal(t, tc.original.ServiceRef.Name, restored.ServiceRef.Name)
			assert.Equal(t, tc.original.ServiceRef.Namespace, restored.ServiceRef.Namespace)
			assert.Equal(t, tc.original.Meta.UID, restored.Meta.UID)
		})
	}
}

func TestAddressGroupBindingPolicyConverter_ToList(t *testing.T) {
	converter := NewAddressGroupBindingPolicyConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		input    []*models.AddressGroupBindingPolicy
		expected int
	}{
		{
			name:     "empty_list",
			input:    []*models.AddressGroupBindingPolicy{},
			expected: 0,
		},
		{
			name: "single_item",
			input: []*models.AddressGroupBindingPolicy{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "policy1",
							Namespace: "default",
						},
					},
					AddressGroupRef: models.AddressGroupRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "addressgroup1",
							Namespace: "default",
						},
					},
					ServiceRef: models.ServiceRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "service1",
							Namespace: "default",
						},
					},
					Meta: models.Meta{
						UID: "uid-1",
					},
				},
			},
			expected: 1,
		},
		{
			name: "multiple_items",
			input: []*models.AddressGroupBindingPolicy{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "policy1",
							Namespace: "default",
						},
					},
					AddressGroupRef: models.AddressGroupRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "addressgroup1",
							Namespace: "default",
						},
					},
					ServiceRef: models.ServiceRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "service1",
							Namespace: "default",
						},
					},
					Meta: models.Meta{
						UID: "uid-1",
					},
				},
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "policy2",
							Namespace: "test",
						},
					},
					AddressGroupRef: models.AddressGroupRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "addressgroup2",
							Namespace: "test",
						},
					},
					ServiceRef: models.ServiceRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "service2",
							Namespace: "test",
						},
					},
					Meta: models.Meta{
						UID: "uid-2",
					},
				},
			},
			expected: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.ToList(ctx, tc.input)

			assert.NoError(t, err)
			require.NotNil(t, result)

			policyList, ok := result.(*netguardv1beta1.AddressGroupBindingPolicyList)
			require.True(t, ok)
			assert.Equal(t, "netguard.sgroups.io/v1beta1", policyList.APIVersion)
			assert.Equal(t, "AddressGroupBindingPolicyList", policyList.Kind)
			assert.Len(t, policyList.Items, tc.expected)
		})
	}
}
