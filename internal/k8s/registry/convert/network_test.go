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

func TestNetworkConverter_ToDomain(t *testing.T) {
	converter := NewNetworkConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *netguardv1beta1.Network
		expected    *models.Network
		expectError bool
	}{
		{
			name: "valid_network_with_minimal_fields",
			input: &netguardv1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-network",
					Namespace: "default",
					UID:       types.UID("test-uid-123"),
				},
				Spec: netguardv1beta1.NetworkSpec{
					CIDR: "10.0.0.0/24",
				},
				Status: netguardv1beta1.NetworkStatus{
					NetworkName: "test-network-internal",
					IsBound:     false,
				},
			},
			expected: &models.Network{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-network",
						Namespace: "default",
					},
				},
				CIDR:        "10.0.0.0/24",
				NetworkName: "test-network-internal",
				IsBound:     false,
				Meta: models.Meta{
					UID: "test-uid-123",
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
				assert.Equal(t, tc.expected.CIDR, result.CIDR)
				assert.Equal(t, tc.expected.NetworkName, result.NetworkName)
				assert.Equal(t, tc.expected.IsBound, result.IsBound)
				assert.Equal(t, tc.expected.Meta.UID, result.Meta.UID)
			}
		})
	}
}

func TestNetworkConverter_FromDomain(t *testing.T) {
	converter := NewNetworkConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *models.Network
		expectError bool
	}{
		{
			name: "valid_network",
			input: &models.Network{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-network",
						Namespace: "default",
					},
				},
				CIDR:        "10.0.0.0/24",
				NetworkName: "test-network-internal",
				IsBound:     true,
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
				assert.Equal(t, "Network", result.Kind)
				assert.Equal(t, tc.input.Name, result.Name)
				assert.Equal(t, tc.input.Namespace, result.Namespace)
				assert.Equal(t, tc.input.CIDR, result.Spec.CIDR)
				assert.Equal(t, tc.input.NetworkName, result.Status.NetworkName)
				assert.Equal(t, tc.input.IsBound, result.Status.IsBound)
			}
		})
	}
}

func TestNetworkConverter_RoundTrip(t *testing.T) {
	converter := NewNetworkConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		original *models.Network
	}{
		{
			name: "basic_network",
			original: &models.Network{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-network",
						Namespace: "default",
					},
				},
				CIDR:        "10.0.0.0/24",
				NetworkName: "test-network-internal",
				IsBound:     false,
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
			assert.Equal(t, tc.original.CIDR, restored.CIDR)
			assert.Equal(t, tc.original.NetworkName, restored.NetworkName)
			assert.Equal(t, tc.original.IsBound, restored.IsBound)
			assert.Equal(t, tc.original.Meta.UID, restored.Meta.UID)
		})
	}
}

func TestNetworkConverter_ToList(t *testing.T) {
	converter := NewNetworkConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		input    []*models.Network
		expected int
	}{
		{
			name:     "empty_list",
			input:    []*models.Network{},
			expected: 0,
		},
		{
			name: "single_item",
			input: []*models.Network{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "network1",
							Namespace: "default",
						},
					},
					CIDR:        "10.0.0.0/24",
					NetworkName: "network1-internal",
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

			networkList, ok := result.(*netguardv1beta1.NetworkList)
			require.True(t, ok)
			assert.Equal(t, "netguard.sgroups.io/v1beta1", networkList.APIVersion)
			assert.Equal(t, "NetworkList", networkList.Kind)
			assert.Len(t, networkList.Items, tc.expected)
		})
	}
}
