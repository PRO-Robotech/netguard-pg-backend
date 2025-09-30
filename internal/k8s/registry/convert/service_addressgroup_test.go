package convert

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// TestServiceConverter_AddressGroups_ToDomain tests conversion of AddressGroups from K8s Spec to Domain
func TestServiceConverter_AddressGroups_ToDomain(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	testCases := []struct {
		name     string
		input    *netguardv1beta1.Service
		check    func(t *testing.T, result *models.Service)
	}{
		{
			name: "single AddressGroup in spec",
			input: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-1",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					AddressGroups: []netguardv1beta1.NamespacedObjectReference{
						{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-1",
							},
							Namespace: "default",
						},
					},
				},
			},
			check: func(t *testing.T, result *models.Service) {
				require.NotNil(t, result.AddressGroups)
				require.Len(t, result.AddressGroups, 1)
				assert.Equal(t, "ag-1", result.AddressGroups[0].Name)
				assert.Equal(t, "default", result.AddressGroups[0].Namespace)
			},
		},
		{
			name: "multiple AddressGroups in spec",
			input: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-2",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					AddressGroups: []netguardv1beta1.NamespacedObjectReference{
						{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-1",
							},
							Namespace: "default",
						},
						{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-2",
							},
							Namespace: "default",
						},
						{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-3",
							},
							Namespace: "other-ns",
						},
					},
				},
			},
			check: func(t *testing.T, result *models.Service) {
				require.NotNil(t, result.AddressGroups)
				require.Len(t, result.AddressGroups, 3)
				assert.Equal(t, "ag-1", result.AddressGroups[0].Name)
				assert.Equal(t, "ag-2", result.AddressGroups[1].Name)
				assert.Equal(t, "ag-3", result.AddressGroups[2].Name)
				assert.Equal(t, "other-ns", result.AddressGroups[2].Namespace)
			},
		},
		{
			name: "no AddressGroups (backward compatibility)",
			input: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-3",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{},
			},
			check: func(t *testing.T, result *models.Service) {
				assert.Nil(t, result.AddressGroups)
			},
		},
		{
			name: "empty AddressGroups array",
			input: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-4",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					AddressGroups: []netguardv1beta1.NamespacedObjectReference{},
				},
			},
			check: func(t *testing.T, result *models.Service) {
				assert.Nil(t, result.AddressGroups)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.ToDomain(ctx, tc.input)
			require.NoError(t, err)
			require.NotNil(t, result)
			tc.check(t, result)
		})
	}
}

// TestServiceConverter_AggregatedAddressGroups_ToDomain tests conversion of AggregatedAddressGroups from K8s ROOT to Domain
func TestServiceConverter_AggregatedAddressGroups_ToDomain(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	testCases := []struct {
		name  string
		input *netguardv1beta1.Service
		check func(t *testing.T, result *models.Service)
	}{
		{
			name: "AggregatedAddressGroups at ROOT level",
			input: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-1",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{},
				AggregatedAddressGroups: []netguardv1beta1.AddressGroupReference{
					{
						Ref: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-1",
							},
							Namespace: "default",
						},
						Source: netguardv1beta1.AddressGroupSourceSpec,
					},
					{
						Ref: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-2",
							},
							Namespace: "default",
						},
						Source: netguardv1beta1.AddressGroupSourceSpec,
					},
				},
			},
			check: func(t *testing.T, result *models.Service) {
				require.NotNil(t, result.AggregatedAddressGroups)
				require.Len(t, result.AggregatedAddressGroups, 2)

				// Check first AG
				assert.Equal(t, "ag-1", result.AggregatedAddressGroups[0].Ref.Name)
				assert.Equal(t, "default", result.AggregatedAddressGroups[0].Ref.Namespace)
				assert.Equal(t, "AddressGroup", result.AggregatedAddressGroups[0].Ref.Kind)
				assert.Equal(t, models.AddressGroupSourceSpec, result.AggregatedAddressGroups[0].Source)

				// Check second AG
				assert.Equal(t, "ag-2", result.AggregatedAddressGroups[1].Ref.Name)
				assert.Equal(t, "default", result.AggregatedAddressGroups[1].Ref.Namespace)
			},
		},
		{
			name: "no AggregatedAddressGroups",
			input: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-2",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{},
			},
			check: func(t *testing.T, result *models.Service) {
				assert.Nil(t, result.AggregatedAddressGroups)
			},
		},
		{
			name: "empty AggregatedAddressGroups",
			input: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-3",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{},
				AggregatedAddressGroups: []netguardv1beta1.AddressGroupReference{},
			},
			check: func(t *testing.T, result *models.Service) {
				require.NotNil(t, result.AggregatedAddressGroups)
				assert.Len(t, result.AggregatedAddressGroups, 0)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.ToDomain(ctx, tc.input)
			require.NoError(t, err)
			require.NotNil(t, result)
			tc.check(t, result)
		})
	}
}

// TestServiceConverter_AddressGroups_FromDomain tests conversion of AddressGroups from Domain to K8s Spec
func TestServiceConverter_AddressGroups_FromDomain(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	testCases := []struct {
		name  string
		input *models.Service
		check func(t *testing.T, result *netguardv1beta1.Service)
	}{
		{
			name: "single AddressGroup",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "svc-1",
						Namespace: "default",
					},
				},
				AddressGroups: []models.AddressGroupRef{
					models.NewAddressGroupRef("ag-1", models.WithNamespace("default")),
				},
			},
			check: func(t *testing.T, result *netguardv1beta1.Service) {
				require.NotNil(t, result.Spec.AddressGroups)
				require.Len(t, result.Spec.AddressGroups, 1)
				assert.Equal(t, "ag-1", result.Spec.AddressGroups[0].Name)
				assert.Equal(t, "default", result.Spec.AddressGroups[0].Namespace)
			},
		},
		{
			name: "multiple AddressGroups",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "svc-2",
						Namespace: "default",
					},
				},
				AddressGroups: []models.AddressGroupRef{
					models.NewAddressGroupRef("ag-1", models.WithNamespace("default")),
					models.NewAddressGroupRef("ag-2", models.WithNamespace("default")),
					models.NewAddressGroupRef("ag-3", models.WithNamespace("other-ns")),
				},
			},
			check: func(t *testing.T, result *netguardv1beta1.Service) {
				require.NotNil(t, result.Spec.AddressGroups)
				require.Len(t, result.Spec.AddressGroups, 3)
				assert.Equal(t, "ag-1", result.Spec.AddressGroups[0].Name)
				assert.Equal(t, "ag-2", result.Spec.AddressGroups[1].Name)
				assert.Equal(t, "ag-3", result.Spec.AddressGroups[2].Name)
				assert.Equal(t, "other-ns", result.Spec.AddressGroups[2].Namespace)
			},
		},
		{
			name: "no AddressGroups",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "svc-3",
						Namespace: "default",
					},
				},
			},
			check: func(t *testing.T, result *netguardv1beta1.Service) {
				assert.Nil(t, result.Spec.AddressGroups)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.FromDomain(ctx, tc.input)
			require.NoError(t, err)
			require.NotNil(t, result)
			tc.check(t, result)
		})
	}
}

// TestServiceConverter_AggregatedAddressGroups_FromDomain_PlacedAtRoot tests that AggregatedAddressGroups are at ROOT, not in Status
func TestServiceConverter_AggregatedAddressGroups_FromDomain_PlacedAtRoot(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	testCases := []struct {
		name  string
		input *models.Service
		check func(t *testing.T, result *netguardv1beta1.Service)
	}{
		{
			name: "AggregatedAddressGroups from spec source",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "svc-1",
						Namespace: "default",
					},
				},
				AggregatedAddressGroups: []models.AddressGroupReference{
					{
						Ref: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-1",
							},
							Namespace: "default",
						},
						Source: models.AddressGroupSourceSpec,
					},
				},
			},
			check: func(t *testing.T, result *netguardv1beta1.Service) {
				// CRITICAL: Must be at ROOT level
				require.NotNil(t, result.AggregatedAddressGroups)
				require.Len(t, result.AggregatedAddressGroups, 1)
				assert.Equal(t, "ag-1", result.AggregatedAddressGroups[0].Ref.Name)
				assert.Equal(t, "default", result.AggregatedAddressGroups[0].Ref.Namespace)

				// Verify NOT in Status
				// Status only has ObservedGeneration and Conditions
				assert.Empty(t, result.Status.Conditions)
			},
		},
		{
			name: "AggregatedAddressGroups from binding source",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "svc-2",
						Namespace: "default",
					},
				},
				AggregatedAddressGroups: []models.AddressGroupReference{
					{
						Ref: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-2",
							},
							Namespace: "default",
						},
						Source: models.AddressGroupSourceBinding,
					},
				},
			},
			check: func(t *testing.T, result *netguardv1beta1.Service) {
				// CRITICAL: Must be at ROOT level regardless of source
				require.NotNil(t, result.AggregatedAddressGroups)
				require.Len(t, result.AggregatedAddressGroups, 1)
				assert.Equal(t, "ag-2", result.AggregatedAddressGroups[0].Ref.Name)
			},
		},
		{
			name: "Mixed sources in AggregatedAddressGroups",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "svc-3",
						Namespace: "default",
					},
				},
				AggregatedAddressGroups: []models.AddressGroupReference{
					{
						Ref: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-from-spec",
							},
							Namespace: "default",
						},
						Source: models.AddressGroupSourceSpec,
					},
					{
						Ref: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-from-binding",
							},
							Namespace: "default",
						},
						Source: models.AddressGroupSourceBinding,
					},
				},
			},
			check: func(t *testing.T, result *netguardv1beta1.Service) {
				// CRITICAL: Both at ROOT level
				require.NotNil(t, result.AggregatedAddressGroups)
				require.Len(t, result.AggregatedAddressGroups, 2)
				assert.Equal(t, "ag-from-spec", result.AggregatedAddressGroups[0].Ref.Name)
				assert.Equal(t, "ag-from-binding", result.AggregatedAddressGroups[1].Ref.Name)
			},
		},
		{
			name: "no AggregatedAddressGroups",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "svc-4",
						Namespace: "default",
					},
				},
			},
			check: func(t *testing.T, result *netguardv1beta1.Service) {
				assert.Nil(t, result.AggregatedAddressGroups)
			},
		},
		{
			name: "empty AggregatedAddressGroups",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "svc-5",
						Namespace: "default",
					},
				},
				AggregatedAddressGroups: []models.AddressGroupReference{},
			},
			check: func(t *testing.T, result *netguardv1beta1.Service) {
				require.NotNil(t, result.AggregatedAddressGroups)
				assert.Len(t, result.AggregatedAddressGroups, 0)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.FromDomain(ctx, tc.input)
			require.NoError(t, err)
			require.NotNil(t, result)
			tc.check(t, result)
		})
	}
}

// TestServiceConverter_RoundTrip_WithAddressGroups tests bidirectional conversion
func TestServiceConverter_RoundTrip_WithAddressGroups(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	testCases := []struct {
		name                         string
		k8s                          *netguardv1beta1.Service
		expectedAggregatedCount      int // Expected count after round-trip with defensive logic
	}{
		{
			name: "spec AddressGroups only",
			k8s: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-1",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					AddressGroups: []netguardv1beta1.NamespacedObjectReference{
						{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-1",
							},
							Namespace: "default",
						},
						{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-2",
							},
							Namespace: "default",
						},
					},
				},
			},
			// Defensive logic: when AggregatedAddressGroups is empty but Spec has data,
			// populate AggregatedAddressGroups from Spec for consistency
			expectedAggregatedCount: 2,
		},
		{
			name: "aggregated AddressGroups only",
			k8s: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-2",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{},
				AggregatedAddressGroups: []netguardv1beta1.AddressGroupReference{
					{
						Ref: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-1",
							},
							Namespace: "default",
						},
						Source: netguardv1beta1.AddressGroupSourceSpec,
					},
					{
						Ref: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-2",
							},
							Namespace: "other-ns",
						},
						Source: netguardv1beta1.AddressGroupSourceSpec,
					},
				},
			},
			expectedAggregatedCount: 2,
		},
		{
			name: "both spec and aggregated",
			k8s: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-3",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					AddressGroups: []netguardv1beta1.NamespacedObjectReference{
						{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-1",
							},
							Namespace: "default",
						},
					},
				},
				AggregatedAddressGroups: []netguardv1beta1.AddressGroupReference{
					{
						Ref: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-1",
							},
							Namespace: "default",
						},
						Source: netguardv1beta1.AddressGroupSourceSpec,
					},
					{
						Ref: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1beta1",
								Kind:       "AddressGroup",
								Name:       "ag-2",
							},
							Namespace: "default",
						},
						Source: netguardv1beta1.AddressGroupSourceSpec,
					},
				},
			},
			expectedAggregatedCount: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// K8s -> Domain
			domain, err := converter.ToDomain(ctx, tc.k8s)
			require.NoError(t, err)
			require.NotNil(t, domain)

			// Domain -> K8s
			k8sResult, err := converter.FromDomain(ctx, domain)
			require.NoError(t, err)
			require.NotNil(t, k8sResult)

			// Verify Spec.AddressGroups match
			assert.Equal(t, tc.k8s.Spec.AddressGroups, k8sResult.Spec.AddressGroups,
				"Spec.AddressGroups should match after round-trip")

			// Verify AggregatedAddressGroups count with defensive logic
			assert.Equal(t, tc.expectedAggregatedCount, len(k8sResult.AggregatedAddressGroups),
				"AggregatedAddressGroups count should match expected (with defensive logic)")

			// Verify AggregatedAddressGroups is at ROOT and not nil
			assert.NotNil(t, k8sResult.AggregatedAddressGroups,
				"AggregatedAddressGroups should be at root level")
		})
	}
}

// TestServiceConverter_HelperFunctions tests the helper conversion functions
func TestServiceConverter_HelperFunctions(t *testing.T) {
	t.Run("convertAddressGroupReferencesToDomain - nil input", func(t *testing.T) {
		result := convertAddressGroupReferencesToDomain(nil)
		assert.Nil(t, result)
	})

	t.Run("convertAddressGroupReferencesToDomain - empty input", func(t *testing.T) {
		result := convertAddressGroupReferencesToDomain([]netguardv1beta1.AddressGroupReference{})
		assert.NotNil(t, result)
		assert.Len(t, result, 0)
	})

	t.Run("convertAddressGroupReferencesToDomain - valid input", func(t *testing.T) {
		input := []netguardv1beta1.AddressGroupReference{
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "ag-1",
					},
					Namespace: "default",
				},
				Source: netguardv1beta1.AddressGroupSourceSpec,
			},
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "ag-2",
					},
					Namespace: "other-ns",
				},
				Source: netguardv1beta1.AddressGroupSourceSpec,
			},
		}
		result := convertAddressGroupReferencesToDomain(input)
		require.NotNil(t, result)
		require.Len(t, result, 2)
		assert.Equal(t, "ag-1", result[0].Ref.Name)
		assert.Equal(t, "default", result[0].Ref.Namespace)
		assert.Equal(t, "ag-2", result[1].Ref.Name)
		assert.Equal(t, "other-ns", result[1].Ref.Namespace)
	})

	t.Run("convertAddressGroupReferencesToK8s - nil input", func(t *testing.T) {
		result := convertAddressGroupReferencesToK8s(nil)
		assert.Nil(t, result)
	})

	t.Run("convertAddressGroupReferencesToK8s - empty input", func(t *testing.T) {
		result := convertAddressGroupReferencesToK8s([]models.AddressGroupReference{})
		assert.NotNil(t, result)
		assert.Len(t, result, 0)
	})

	t.Run("convertAddressGroupReferencesToK8s - valid input", func(t *testing.T) {
		input := []models.AddressGroupReference{
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name: "ag-1",
					},
					Namespace: "default",
				},
				Source: models.AddressGroupSourceSpec,
			},
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name: "ag-2",
					},
					Namespace: "other-ns",
				},
				Source: models.AddressGroupSourceBinding,
			},
		}
		result := convertAddressGroupReferencesToK8s(input)
		require.NotNil(t, result)
		require.Len(t, result, 2)
		assert.Equal(t, "ag-1", result[0].Ref.Name)
		assert.Equal(t, "default", result[0].Ref.Namespace)
		assert.Equal(t, "ag-2", result[1].Ref.Name)
		assert.Equal(t, "other-ns", result[1].Ref.Namespace)
	})
}
