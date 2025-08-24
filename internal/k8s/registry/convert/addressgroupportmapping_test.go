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

func TestAddressGroupPortMappingConverter_ToDomain(t *testing.T) {
	converter := NewAddressGroupPortMappingConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *netguardv1beta1.AddressGroupPortMapping
		expected    *models.AddressGroupPortMapping
		expectError bool
	}{
		{
			name: "valid_mapping_with_minimal_fields",
			input: &netguardv1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mapping",
					Namespace: "default",
					UID:       types.UID("test-uid-123"),
				},
				AccessPorts: netguardv1beta1.AccessPortsSpec{
					Items: []netguardv1beta1.ServicePortsRef{},
				},
				Status: netguardv1beta1.AddressGroupPortMappingStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "AllReady",
						},
					},
				},
			},
			expected: &models.AddressGroupPortMapping{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-mapping",
						Namespace: "default",
					},
				},
				AccessPorts: map[models.ServiceRef]models.ServicePorts{},
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
			name: "valid_mapping_with_ports",
			input: &netguardv1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mapping-ports",
					Namespace: "default",
					UID:       types.UID("test-uid-456"),
				},
				AccessPorts: netguardv1beta1.AccessPortsSpec{
					Items: []netguardv1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: netguardv1beta1.NamespacedObjectReference{
								ObjectReference: netguardv1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "test-service",
								},
								Namespace: "default",
							},
							Ports: netguardv1beta1.ProtocolPorts{
								TCP: []netguardv1beta1.PortConfig{
									{
										Port: "80",
									},
									{
										Port: "8080-8090",
									},
								},
								UDP: []netguardv1beta1.PortConfig{
									{
										Port: "53",
									},
								},
							},
						},
					},
				},
				Status: netguardv1beta1.AddressGroupPortMappingStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "AllReady",
						},
					},
				},
			},
			expected: &models.AddressGroupPortMapping{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-mapping-ports",
						Namespace: "default",
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
				assert.NotNil(t, result.AccessPorts)
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

func TestAddressGroupPortMappingConverter_FromDomain(t *testing.T) {
	converter := NewAddressGroupPortMappingConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *models.AddressGroupPortMapping
		expectError bool
	}{
		{
			name: "valid_mapping",
			input: &models.AddressGroupPortMapping{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-mapping",
						Namespace: "default",
					},
				},
				AccessPorts: map[models.ServiceRef]models.ServicePorts{
					{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "test-service",
							Namespace: "default",
						},
					}: {
						Ports: models.ProtocolPorts{
							models.TCP: []models.PortRange{
								{Start: 80, End: 80},
								{Start: 8080, End: 8090},
							},
							models.UDP: []models.PortRange{
								{Start: 53, End: 53},
							},
						},
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
			name: "empty_access_ports",
			input: &models.AddressGroupPortMapping{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-mapping-empty",
						Namespace: "default",
					},
				},
				AccessPorts: map[models.ServiceRef]models.ServicePorts{},
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
				assert.Equal(t, "AddressGroupPortMapping", result.Kind)
				assert.Equal(t, tc.input.Name, result.Name)
				assert.Equal(t, tc.input.Namespace, result.Namespace)
				assert.NotNil(t, result.AccessPorts)
			}
		})
	}
}

func TestAddressGroupPortMappingConverter_RoundTrip(t *testing.T) {
	converter := NewAddressGroupPortMappingConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		original *models.AddressGroupPortMapping
	}{
		{
			name: "basic_mapping",
			original: &models.AddressGroupPortMapping{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-mapping",
						Namespace: "default",
					},
				},
				AccessPorts: map[models.ServiceRef]models.ServicePorts{},
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
			assert.Equal(t, tc.original.Meta.UID, restored.Meta.UID)
			assert.NotNil(t, restored.AccessPorts)
		})
	}
}

func TestAddressGroupPortMappingConverter_ToList(t *testing.T) {
	converter := NewAddressGroupPortMappingConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		input    []*models.AddressGroupPortMapping
		expected int
	}{
		{
			name:     "empty_list",
			input:    []*models.AddressGroupPortMapping{},
			expected: 0,
		},
		{
			name: "single_item",
			input: []*models.AddressGroupPortMapping{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "mapping1",
							Namespace: "default",
						},
					},
					AccessPorts: map[models.ServiceRef]models.ServicePorts{},
					Meta: models.Meta{
						UID: "uid-1",
					},
				},
			},
			expected: 1,
		},
		{
			name: "multiple_items",
			input: []*models.AddressGroupPortMapping{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "mapping1",
							Namespace: "default",
						},
					},
					AccessPorts: map[models.ServiceRef]models.ServicePorts{},
					Meta: models.Meta{
						UID: "uid-1",
					},
				},
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "mapping2",
							Namespace: "test",
						},
					},
					AccessPorts: map[models.ServiceRef]models.ServicePorts{},
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

			mappingList, ok := result.(*netguardv1beta1.AddressGroupPortMappingList)
			require.True(t, ok)
			assert.Equal(t, "netguard.sgroups.io/v1beta1", mappingList.APIVersion)
			assert.Equal(t, "AddressGroupPortMappingList", mappingList.Kind)
			assert.Len(t, mappingList.Items, tc.expected)
		})
	}
}
