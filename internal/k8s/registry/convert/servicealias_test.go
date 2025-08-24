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

func TestServiceAliasConverter_ToDomain(t *testing.T) {
	converter := NewServiceAliasConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *netguardv1beta1.ServiceAlias
		expected    *models.ServiceAlias
		expectError bool
	}{
		{
			name: "valid_alias_with_minimal_fields",
			input: &netguardv1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-alias",
					Namespace: "default",
					UID:       types.UID("test-uid-123"),
				},
				Spec: netguardv1beta1.ServiceAliasSpec{
					ServiceRef: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "test-service",
					},
				},
				Status: netguardv1beta1.ServiceAliasStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "ServiceReady",
						},
					},
				},
			},
			expected: &models.ServiceAlias{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-alias",
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
							Reason: "ServiceReady",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid_alias_with_full_metadata",
			input: &netguardv1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-alias-full",
					Namespace:       "test-namespace",
					UID:             types.UID("test-uid-456"),
					ResourceVersion: "789",
					Generation:      3,
					CreationTimestamp: metav1.Time{
						Time: time.Now(),
					},
					ManagedFields: []metav1.ManagedFieldsEntry{
						{
							Manager:   "kubectl-client-side-apply",
							Operation: metav1.ManagedFieldsOperationApply,
						},
					},
				},
				Spec: netguardv1beta1.ServiceAliasSpec{
					ServiceRef: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "full-service",
					},
				},
				Status: netguardv1beta1.ServiceAliasStatus{
					ObservedGeneration: 3,
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "ServiceReady",
						},
					},
				},
			},
			expected: &models.ServiceAlias{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-alias-full",
						Namespace: "test-namespace",
					},
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "full-service",
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

func TestServiceAliasConverter_FromDomain(t *testing.T) {
	converter := NewServiceAliasConverter()
	ctx := context.Background()

	testCases := []struct {
		name        string
		input       *models.ServiceAlias
		expectError bool
	}{
		{
			name: "valid_alias",
			input: &models.ServiceAlias{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-alias",
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
			name: "cross_namespace_alias",
			input: &models.ServiceAlias{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-alias-cross",
						Namespace: "alias-namespace",
					},
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "target-service",
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
				assert.Equal(t, "ServiceAlias", result.Kind)
				assert.Equal(t, tc.input.Name, result.Name)
				assert.Equal(t, tc.input.Namespace, result.Namespace)
				assert.Equal(t, tc.input.ServiceRef.Name, result.Spec.ServiceRef.Name)
			}
		})
	}
}

func TestServiceAliasConverter_RoundTrip(t *testing.T) {
	converter := NewServiceAliasConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		original *models.ServiceAlias
	}{
		{
			name: "basic_alias",
			original: &models.ServiceAlias{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-alias",
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
			assert.Equal(t, tc.original.ServiceRef.Name, restored.ServiceRef.Name)
			assert.Equal(t, tc.original.Meta.UID, restored.Meta.UID)
		})
	}
}

func TestServiceAliasConverter_ToList(t *testing.T) {
	converter := NewServiceAliasConverter()
	ctx := context.Background()

	testCases := []struct {
		name     string
		input    []*models.ServiceAlias
		expected int
	}{
		{
			name:     "empty_list",
			input:    []*models.ServiceAlias{},
			expected: 0,
		},
		{
			name: "single_item",
			input: []*models.ServiceAlias{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "alias1",
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
			input: []*models.ServiceAlias{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "alias1",
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
							Name:      "alias2",
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

			aliasList, ok := result.(*netguardv1beta1.ServiceAliasList)
			require.True(t, ok)
			assert.Equal(t, "netguard.sgroups.io/v1beta1", aliasList.APIVersion)
			assert.Equal(t, "ServiceAliasList", aliasList.Kind)
			assert.Len(t, aliasList.Items, tc.expected)
		})
	}
}
