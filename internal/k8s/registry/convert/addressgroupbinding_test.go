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

func TestAddressGroupBindingConverter_ToDomain(t *testing.T) {
	converter := NewAddressGroupBindingConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *netguardv1beta1.AddressGroupBinding
		expected    *models.AddressGroupBinding
		expectError bool
	}{
		{
			name: "valid_binding_with_minimal_fields",
			input: &netguardv1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-binding",
					Namespace: "default",
					UID:       types.UID("test-uid-123"),
				},
				Spec: netguardv1beta1.AddressGroupBindingSpec{
					ServiceRef: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "test-service",
					},
					AddressGroupRef: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "test-addressgroup",
						},
						Namespace: "default",
					},
				},
				Status: netguardv1beta1.AddressGroupBindingStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "AllReady",
						},
					},
				},
			},
			expected: &models.AddressGroupBinding{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-binding",
						Namespace: "default",
					},
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service",
						Namespace: "default",
					},
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-addressgroup",
						Namespace: "default",
					},
				},
				Meta: models.Meta{
					UID: "test-uid-123",
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "AllReady",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid_binding_with_full_metadata",
			input: &netguardv1beta1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-binding-full",
					Namespace:       "test-namespace",
					UID:             types.UID("test-uid-456"),
					ResourceVersion: "123456",
					Generation:      2,
					CreationTimestamp: metav1.Time{
						Time: time.Now(),
					},
					ManagedFields: []metav1.ManagedFieldsEntry{
						{
							Manager:   "kubectl",
							Operation: metav1.ManagedFieldsOperationApply,
						},
					},
				},
				Spec: netguardv1beta1.AddressGroupBindingSpec{
					ServiceRef: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "full-service",
					},
					AddressGroupRef: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "full-addressgroup",
						},
						Namespace: "test-namespace",
					},
				},
				Status: netguardv1beta1.AddressGroupBindingStatus{
					ObservedGeneration: 2,
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "AllReady",
						},
					},
				},
			},
			expected: &models.AddressGroupBinding{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-binding-full",
						Namespace: "test-namespace",
					},
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "full-service",
						Namespace: "test-namespace",
					},
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "full-addressgroup",
						Namespace: "test-namespace",
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
				assert.Equal(t, tc.expected.ServiceRef.Name, result.ServiceRef.Name)
				assert.Equal(t, tc.expected.AddressGroupRef.Name, result.AddressGroupRef.Name)
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

func TestAddressGroupBindingConverter_FromDomain(t *testing.T) {
	converter := NewAddressGroupBindingConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *models.AddressGroupBinding
		expectError bool
	}{
		{
			name: "valid_binding",
			input: &models.AddressGroupBinding{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-binding",
						Namespace: "default",
					},
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service",
						Namespace: "default",
					},
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-addressgroup",
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
				assert.Equal(t, "AddressGroupBinding", result.Kind)
				assert.Equal(t, tc.input.Name, result.Name)
				assert.Equal(t, tc.input.Namespace, result.Namespace)
				assert.Equal(t, tc.input.ServiceRef.Name, result.Spec.ServiceRef.Name)
				assert.Equal(t, tc.input.AddressGroupRef.Name, result.Spec.AddressGroupRef.Name)
			}
		})
	}
}

func TestAddressGroupBindingConverter_RoundTrip(t *testing.T) {
	converter := NewAddressGroupBindingConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		original *models.AddressGroupBinding
	}{
		{
			name: "basic_binding",
			original: &models.AddressGroupBinding{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-binding",
						Namespace: "default",
					},
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service",
						Namespace: "default",
					},
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-addressgroup",
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
			assert.Equal(t, tc.original.ServiceRef.Name, restored.ServiceRef.Name)
			assert.Equal(t, tc.original.AddressGroupRef.Name, restored.AddressGroupRef.Name)
			assert.Equal(t, tc.original.Meta.UID, restored.Meta.UID)
		})
	}
}

func TestAddressGroupBindingConverter_ToList(t *testing.T) {
	converter := NewAddressGroupBindingConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		input    []*models.AddressGroupBinding
		expected int
	}{
		{
			name:     "empty_list",
			input:    []*models.AddressGroupBinding{},
			expected: 0,
		},
		{
			name: "single_item",
			input: []*models.AddressGroupBinding{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "binding1",
							Namespace: "default",
						},
					},
					ServiceRef: models.ServiceRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "service1",
							Namespace: "default",
						},
					},
					AddressGroupRef: models.AddressGroupRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "addressgroup1",
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
			input: []*models.AddressGroupBinding{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "binding1",
							Namespace: "default",
						},
					},
					ServiceRef: models.ServiceRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "service1",
							Namespace: "default",
						},
					},
					AddressGroupRef: models.AddressGroupRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "addressgroup1",
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
							Name:      "binding2",
							Namespace: "test",
						},
					},
					ServiceRef: models.ServiceRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "service2",
							Namespace: "test",
						},
					},
					AddressGroupRef: models.AddressGroupRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "addressgroup2",
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

			bindingList, ok := result.(*netguardv1beta1.AddressGroupBindingList)
			require.True(t, ok)
			assert.Equal(t, "netguard.sgroups.io/v1beta1", bindingList.APIVersion)
			assert.Equal(t, "AddressGroupBindingList", bindingList.Kind)
			assert.Len(t, bindingList.Items, tc.expected)
		})
	}
}
