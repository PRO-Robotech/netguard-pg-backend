package netguard_test

import (
	"testing"

	"netguard-pg-backend/internal/api/netguard"
	"netguard-pg-backend/internal/domain/models"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuleS2SConverters_IEAgAgRuleRefs_Conversion(t *testing.T) {
	t.Run("ConvertRuleS2SToPB_WithIEAgAgRuleRefs_Success", func(t *testing.T) {
		// Arrange
		domainRule := &models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{Name: "test-rule", Namespace: "default"},
			},
			Traffic: models.INGRESS,
			ServiceLocalRef: models.ServiceAliasRef{
				ResourceIdentifier: models.ResourceIdentifier{Name: "backend", Namespace: "default"},
			},
			ServiceRef: models.ServiceAliasRef{
				ResourceIdentifier: models.ResourceIdentifier{Name: "frontend", Namespace: "default"},
			},
			IEAgAgRuleRefs: []models.ResourceIdentifier{
				{Name: "ieagag-rule-1", Namespace: "default"},
				{Name: "ieagag-rule-2", Namespace: "default"},
				{Name: "ieagag-rule-3", Namespace: "test"},
			},
		}

		// Act
		pbRule, err := netguard.ConvertRuleS2SToPB(domainRule)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, pbRule)

		// Ключевая проверка: IEAgAgRuleRefs должны быть конвертированы
		assert.Len(t, pbRule.IeagAgRuleRefs, 3, "Should convert all IEAgAgRuleRefs")

		// Проверяем конкретные ссылки
		expectedRefs := []*netguardpb.ResourceIdentifier{
			{Name: "ieagag-rule-1", Namespace: "default"},
			{Name: "ieagag-rule-2", Namespace: "default"},
			{Name: "ieagag-rule-3", Namespace: "test"},
		}

		for i, expectedRef := range expectedRefs {
			assert.Equal(t, expectedRef.Name, pbRule.IeagAgRuleRefs[i].Name,
				"IEAgAgRuleRef %d name should match", i)
			assert.Equal(t, expectedRef.Namespace, pbRule.IeagAgRuleRefs[i].Namespace,
				"IEAgAgRuleRef %d namespace should match", i)
		}
	})

	t.Run("ConvertRuleS2SToPB_WithEmptyIEAgAgRuleRefs_Success", func(t *testing.T) {
		// Arrange
		domainRule := &models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{Name: "test-rule", Namespace: "default"},
			},
			Traffic: models.EGRESS,
			ServiceLocalRef: models.ServiceAliasRef{
				ResourceIdentifier: models.ResourceIdentifier{Name: "backend", Namespace: "default"},
			},
			ServiceRef: models.ServiceAliasRef{
				ResourceIdentifier: models.ResourceIdentifier{Name: "frontend", Namespace: "default"},
			},
			IEAgAgRuleRefs: []models.ResourceIdentifier{}, // Пустые рефы
		}

		// Act
		pbRule, err := netguard.ConvertRuleS2SToPB(domainRule)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, pbRule)
		assert.Empty(t, pbRule.IeagAgRuleRefs, "Should handle empty IEAgAgRuleRefs")
	})

	t.Run("ConvertRuleS2SFromPB_WithIEAgAgRuleRefs_Success", func(t *testing.T) {
		// Arrange
		pbRule := &netguardpb.RuleS2S{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-rule",
				Namespace: "default",
			},
			Traffic: netguardpb.Traffic_Ingress,
			ServiceLocalRef: &netguardpb.ServiceRef{
				Identifier: &netguardpb.ResourceIdentifier{
					Name:      "backend",
					Namespace: "default",
				},
			},
			ServiceRef: &netguardpb.ServiceRef{
				Identifier: &netguardpb.ResourceIdentifier{
					Name:      "frontend",
					Namespace: "default",
				},
			},
			IeagAgRuleRefs: []*netguardpb.ResourceIdentifier{
				{Name: "ieagag-rule-1", Namespace: "default"},
				{Name: "ieagag-rule-2", Namespace: "default"},
				{Name: "ieagag-rule-3", Namespace: "test"},
			},
		}

		// Act
		domainRule, err := netguard.ConvertRuleS2SFromPB(pbRule)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, domainRule)

		// Ключевая проверка: IEAgAgRuleRefs должны быть конвертированы
		assert.Len(t, domainRule.IEAgAgRuleRefs, 3, "Should convert all IEAgAgRuleRefs")

		expectedRefs := []models.ResourceIdentifier{
			{Name: "ieagag-rule-1", Namespace: "default"},
			{Name: "ieagag-rule-2", Namespace: "default"},
			{Name: "ieagag-rule-3", Namespace: "test"},
		}

		for _, expectedRef := range expectedRefs {
			assert.Contains(t, domainRule.IEAgAgRuleRefs, expectedRef,
				"Should contain IEAgAgRuleRef %v", expectedRef)
		}
	})

	t.Run("ConvertRuleS2SFromPB_WithEmptyIEAgAgRuleRefs_Success", func(t *testing.T) {
		// Arrange
		pbRule := &netguardpb.RuleS2S{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-rule",
				Namespace: "default",
			},
			Traffic: netguardpb.Traffic_Egress,
			ServiceLocalRef: &netguardpb.ServiceRef{
				Identifier: &netguardpb.ResourceIdentifier{
					Name:      "backend",
					Namespace: "default",
				},
			},
			ServiceRef: &netguardpb.ServiceRef{
				Identifier: &netguardpb.ResourceIdentifier{
					Name:      "frontend",
					Namespace: "default",
				},
			},
			IeagAgRuleRefs: []*netguardpb.ResourceIdentifier{}, // Пустые рефы
		}

		// Act
		domainRule, err := netguard.ConvertRuleS2SFromPB(pbRule)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, domainRule)
		assert.Empty(t, domainRule.IEAgAgRuleRefs, "Should handle empty IEAgAgRuleRefs")
	})
}

func TestRuleS2SConverters_TrafficEnum_Conversion(t *testing.T) {
	t.Run("ConvertRuleS2SToPB_TrafficEnum_Success", func(t *testing.T) {
		testCases := []struct {
			name          string
			domainTraffic models.Traffic
			expectedPB    netguardpb.Traffic
		}{
			{
				name:          "INGRESS conversion",
				domainTraffic: models.INGRESS,
				expectedPB:    netguardpb.Traffic_Ingress,
			},
			{
				name:          "EGRESS conversion",
				domainTraffic: models.EGRESS,
				expectedPB:    netguardpb.Traffic_Egress,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Arrange
				domainRule := &models.RuleS2S{
					ResourceIdentifier: models.ResourceIdentifier{Name: "test-rule", Namespace: "default"},
					Spec: models.RuleS2SSpec{
						Traffic: tc.domainTraffic,
						ServiceLocalRef: models.NamespacedObjectReference{
							ObjectReference: models.ObjectReference{Name: "backend"},
							Namespace:       "default",
						},
						ServiceRef: models.NamespacedObjectReference{
							ObjectReference: models.ObjectReference{Name: "frontend"},
							Namespace:       "default",
						},
					},
				}

				// Act
				pbRule, err := netguard.ConvertRuleS2SToPB(domainRule)

				// Assert
				require.NoError(t, err)
				assert.Equal(t, tc.expectedPB, pbRule.Traffic,
					"Traffic should be converted correctly")
			})
		}
	})

	t.Run("ConvertRuleS2SFromPB_TrafficEnum_Success", func(t *testing.T) {
		testCases := []struct {
			name           string
			pbTraffic      netguardpb.Traffic
			expectedDomain models.Traffic
		}{
			{
				name:           "INGRESS conversion",
				pbTraffic:      netguardpb.Traffic_Ingress,
				expectedDomain: models.INGRESS,
			},
			{
				name:           "EGRESS conversion",
				pbTraffic:      netguardpb.Traffic_Egress,
				expectedDomain: models.EGRESS,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Arrange
				pbRule := &netguardpb.RuleS2S{
					SelfRef: &netguardpb.ResourceIdentifier{
						Name:      "test-rule",
						Namespace: "default",
					},
					Traffic: tc.pbTraffic,
					ServiceLocalRef: &netguardpb.ServiceRef{
						Identifier: &netguardpb.ResourceIdentifier{
							Name:      "backend",
							Namespace: "default",
						},
					},
					ServiceRef: &netguardpb.ServiceRef{
						Identifier: &netguardpb.ResourceIdentifier{
							Name:      "frontend",
							Namespace: "default",
						},
					},
				}

				// Act
				domainRule, err := netguard.ConvertRuleS2SFromPB(pbRule)

				// Assert
				require.NoError(t, err)
				assert.Equal(t, tc.expectedDomain, domainRule.Spec.Traffic,
					"Traffic should be converted correctly")
			})
		}
	})
}

func TestRuleS2SConverters_RoundTrip_PreservesData(t *testing.T) {
	t.Run("DomainToPBToDomain_PreservesIEAgAgRuleRefs", func(t *testing.T) {
		// Arrange
		originalRule := &models.RuleS2S{
			ResourceIdentifier: models.ResourceIdentifier{Name: "test-rule", Namespace: "default"},
			Spec: models.RuleS2SSpec{
				Traffic: models.INGRESS,
				ServiceLocalRef: models.NamespacedObjectReference{
					ObjectReference: models.ObjectReference{Name: "backend"},
					Namespace:       "default",
				},
				ServiceRef: models.NamespacedObjectReference{
					ObjectReference: models.ObjectReference{Name: "frontend"},
					Namespace:       "default",
				},
			},
			IEAgAgRuleRefs: []models.ResourceIdentifier{
				{Name: "ieagag-rule-1", Namespace: "default"},
				{Name: "ieagag-rule-2", Namespace: "different-ns"},
				{Name: "ieagag-rule-3", Namespace: "test"},
			},
		}

		// Act: Domain -> PB -> Domain
		pbRule, err := netguard.ConvertRuleS2SToPB(originalRule)
		require.NoError(t, err)

		convertedRule, err := netguard.ConvertRuleS2SFromPB(pbRule)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, originalRule.ResourceIdentifier, convertedRule.ResourceIdentifier,
			"ResourceIdentifier should be preserved")
		assert.Equal(t, originalRule.Spec.Traffic, convertedRule.Spec.Traffic,
			"Traffic should be preserved")
		assert.Equal(t, len(originalRule.IEAgAgRuleRefs), len(convertedRule.IEAgAgRuleRefs),
			"Number of IEAgAgRuleRefs should be preserved")

		// Проверяем каждую ссылку
		for _, originalRef := range originalRule.IEAgAgRuleRefs {
			assert.Contains(t, convertedRule.IEAgAgRuleRefs, originalRef,
				"IEAgAgRuleRef %v should be preserved", originalRef)
		}
	})

	t.Run("PBToDomainToPB_PreservesIEAgAgRuleRefs", func(t *testing.T) {
		// Arrange
		originalPB := &netguardpb.RuleS2S{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-rule",
				Namespace: "default",
			},
			Traffic: netguardpb.Traffic_Egress,
			ServiceLocalRef: &netguardpb.ServiceRef{
				Identifier: &netguardpb.ResourceIdentifier{
					Name:      "backend",
					Namespace: "default",
				},
			},
			ServiceRef: &netguardpb.ServiceRef{
				Identifier: &netguardpb.ResourceIdentifier{
					Name:      "frontend",
					Namespace: "default",
				},
			},
			IeagAgRuleRefs: []*netguardpb.ResourceIdentifier{
				{Name: "ieagag-rule-1", Namespace: "default"},
				{Name: "ieagag-rule-2", Namespace: "different-ns"},
			},
		}

		// Act: PB -> Domain -> PB
		domainRule, err := netguard.ConvertRuleS2SFromPB(originalPB)
		require.NoError(t, err)

		convertedPB, err := netguard.ConvertRuleS2SToPB(domainRule)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, originalPB.SelfRef.Name, convertedPB.SelfRef.Name,
			"Name should be preserved")
		assert.Equal(t, originalPB.SelfRef.Namespace, convertedPB.SelfRef.Namespace,
			"Namespace should be preserved")
		assert.Equal(t, originalPB.Traffic, convertedPB.Traffic,
			"Traffic should be preserved")
		assert.Equal(t, len(originalPB.IeagAgRuleRefs), len(convertedPB.IeagAgRuleRefs),
			"Number of IEAgAgRuleRefs should be preserved")

		// Проверяем каждую ссылку
		for i, originalRef := range originalPB.IeagAgRuleRefs {
			convertedRef := convertedPB.IeagAgRuleRefs[i]
			assert.Equal(t, originalRef.Name, convertedRef.Name,
				"IEAgAgRuleRef %d name should be preserved", i)
			assert.Equal(t, originalRef.Namespace, convertedRef.Namespace,
				"IEAgAgRuleRef %d namespace should be preserved", i)
		}
	})
}

func TestRuleS2SConverters_NilSafety(t *testing.T) {
	t.Run("ConvertRuleS2SToPB_NilInput_ReturnsError", func(t *testing.T) {
		// Act
		pbRule, err := netguard.ConvertRuleS2SToPB(nil)

		// Assert
		require.Error(t, err)
		assert.Nil(t, pbRule)
	})

	t.Run("ConvertRuleS2SFromPB_NilInput_ReturnsError", func(t *testing.T) {
		// Act
		domainRule, err := netguard.ConvertRuleS2SFromPB(nil)

		// Assert
		require.Error(t, err)
		assert.Nil(t, domainRule)
	})

	t.Run("ConvertRuleS2SToPB_NilIEAgAgRuleRefs_Success", func(t *testing.T) {
		// Arrange
		domainRule := &models.RuleS2S{
			ResourceIdentifier: models.ResourceIdentifier{Name: "test-rule", Namespace: "default"},
			Spec: models.RuleS2SSpec{
				Traffic: models.INGRESS,
				ServiceLocalRef: models.NamespacedObjectReference{
					ObjectReference: models.ObjectReference{Name: "backend"},
					Namespace:       "default",
				},
				ServiceRef: models.NamespacedObjectReference{
					ObjectReference: models.ObjectReference{Name: "frontend"},
					Namespace:       "default",
				},
			},
			IEAgAgRuleRefs: nil, // Nil ссылки
		}

		// Act
		pbRule, err := netguard.ConvertRuleS2SToPB(domainRule)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, pbRule)
		assert.Empty(t, pbRule.IeagAgRuleRefs, "Should handle nil IEAgAgRuleRefs gracefully")
	})
}

// TODO: Add protobuf converter tests for trace field once converter functions are fixed
