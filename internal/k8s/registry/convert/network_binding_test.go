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

func TestNetworkBindingConverter_ToDomain(t *testing.T) {
	converter := NewNetworkBindingConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *netguardv1beta1.NetworkBinding
		expected    *models.NetworkBinding
		expectError bool
	}{
		{
			name: "valid_networkbinding_with_minimal_fields",
			input: &netguardv1beta1.NetworkBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-binding",
					Namespace: "default",
					UID:       types.UID("test-uid-123"),
				},
				Spec: netguardv1beta1.NetworkBindingSpec{
					NetworkRef: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Network",
						Name:       "test-network",
					},
					AddressGroupRef: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "test-addressgroup",
					},
				},
				Status: netguardv1beta1.NetworkBindingStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "AllReady",
						},
					},
				},
			},
			expected: &models.NetworkBinding{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-binding",
						Namespace: "default",
					},
				},
				NetworkRef: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Network",
					Name:       "test-network",
				},
				AddressGroupRef: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "test-addressgroup",
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
				assert.Equal(t, tc.expected.NetworkRef.Name, result.NetworkRef.Name)
				assert.Equal(t, tc.expected.AddressGroupRef.Name, result.AddressGroupRef.Name)
				assert.Equal(t, tc.expected.Meta.UID, result.Meta.UID)
				assert.Equal(t, len(tc.expected.Meta.Conditions), len(result.Meta.Conditions))
			}
		})
	}
}

func TestNetworkBindingConverter_FromDomain(t *testing.T) {
	converter := NewNetworkBindingConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *models.NetworkBinding
		expectError bool
	}{
		{
			name: "valid_networkbinding",
			input: &models.NetworkBinding{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-binding",
						Namespace: "default",
					},
				},
				NetworkRef: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Network",
					Name:       "test-network",
				},
				AddressGroupRef: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "test-addressgroup",
				},
				NetworkItem: models.NetworkItem{
					Name: "test-network",
					CIDR: "10.0.0.0/24",
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
				assert.Equal(t, "NetworkBinding", result.Kind)
				assert.Equal(t, tc.input.Name, result.Name)
				assert.Equal(t, tc.input.Namespace, result.Namespace)
				assert.Equal(t, tc.input.NetworkRef.Name, result.Spec.NetworkRef.Name)
				assert.Equal(t, tc.input.AddressGroupRef.Name, result.Spec.AddressGroupRef.Name)
			}
		})
	}
}

func TestNetworkBindingConverter_RoundTrip(t *testing.T) {
	converter := NewNetworkBindingConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		original *models.NetworkBinding
	}{
		{
			name: "basic_networkbinding",
			original: &models.NetworkBinding{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-binding",
						Namespace: "default",
					},
				},
				NetworkRef: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Network",
					Name:       "test-network",
				},
				AddressGroupRef: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "test-addressgroup",
				},
				NetworkItem: models.NetworkItem{
					Name: "test-network",
					CIDR: "10.0.0.0/24",
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
			assert.Equal(t, tc.original.NetworkRef.Name, restored.NetworkRef.Name)
			assert.Equal(t, tc.original.AddressGroupRef.Name, restored.AddressGroupRef.Name)
			assert.Equal(t, tc.original.Meta.UID, restored.Meta.UID)
		})
	}
}

func TestNetworkBindingConverter_ToList(t *testing.T) {
	converter := NewNetworkBindingConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		input    []*models.NetworkBinding
		expected int
	}{
		{
			name:     "empty_list",
			input:    []*models.NetworkBinding{},
			expected: 0,
		},
		{
			name: "single_item",
			input: []*models.NetworkBinding{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "binding1",
							Namespace: "default",
						},
					},
					NetworkRef: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Network",
						Name:       "network1",
					},
					AddressGroupRef: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "addressgroup1",
					},
					Meta: models.Meta{
						UID: "uid-1",
					},
				},
			},
			expected: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.ToList(ctx, tc.input)

			assert.NoError(t, err)
			require.NotNil(t, result)

			bindingList, ok := result.(*netguardv1beta1.NetworkBindingList)
			require.True(t, ok)
			assert.Equal(t, "netguard.sgroups.io/v1beta1", bindingList.APIVersion)
			assert.Equal(t, "NetworkBindingList", bindingList.Kind)
			assert.Len(t, bindingList.Items, tc.expected)
		})
	}
}
