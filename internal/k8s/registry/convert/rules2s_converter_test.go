package convert_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/convert"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRuleS2SConverter_IEAgAgRuleRefs_Conversion(t *testing.T) {
	ctx := context.Background()
	converter := convert.NewRuleS2SConverter()

	t.Run("ToDomain_WithIEAgAgRuleRefs_Success", func(t *testing.T) {
		// Arrange
		k8sRuleS2S := &netguardv1beta1.RuleS2S{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rule",
				Namespace: "default",
			},
			Spec: netguardv1beta1.RuleS2SSpec{
				Traffic: netguardv1beta1.INGRESS,
				ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{Name: "backend"},
					Namespace:       "default",
				},
				ServiceRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{Name: "frontend"},
					Namespace:       "default",
				},
				Trace: true, // Test trace field
			},
			Status: netguardv1beta1.RuleS2SStatus{
				IEAgAgRuleRefs: []netguardv1beta1.NamespacedObjectReference{
					{
						ObjectReference: netguardv1beta1.ObjectReference{Name: "ieagag-rule-1"},
						Namespace:       "default",
					},
					{
						ObjectReference: netguardv1beta1.ObjectReference{Name: "ieagag-rule-2"},
						Namespace:       "default",
					},
				},
			},
		}

		// Act
		domainRule, err := converter.ToDomain(ctx, k8sRuleS2S)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, domainRule)

		// Check trace field conversion
		assert.True(t, domainRule.Trace, "Trace field should be converted correctly")

		// Ключевая проверка: IEAgAgRuleRefs должны быть конвертированы
		assert.Len(t, domainRule.IEAgAgRuleRefs, 2, "Should convert both IEAgAgRuleRefs")

		expected1 := models.ResourceIdentifier{Name: "ieagag-rule-1", Namespace: "default"}
		expected2 := models.ResourceIdentifier{Name: "ieagag-rule-2", Namespace: "default"}

		assert.Contains(t, domainRule.IEAgAgRuleRefs, expected1, "Should contain first IEAgAgRuleRef")
		assert.Contains(t, domainRule.IEAgAgRuleRefs, expected2, "Should contain second IEAgAgRuleRef")
	})

	t.Run("ToDomain_WithEmptyIEAgAgRuleRefs_Success", func(t *testing.T) {
		// Arrange
		k8sRuleS2S := &netguardv1beta1.RuleS2S{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rule",
				Namespace: "default",
			},
			Spec: netguardv1beta1.RuleS2SSpec{
				Traffic: netguardv1beta1.EGRESS,
				ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{Name: "backend"},
					Namespace:       "default",
				},
				ServiceRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{Name: "frontend"},
					Namespace:       "default",
				},
			},
			Status: netguardv1beta1.RuleS2SStatus{
				IEAgAgRuleRefs: []netguardv1beta1.NamespacedObjectReference{}, // Пустые рефы
			},
		}

		// Act
		domainRule, err := converter.ToDomain(ctx, k8sRuleS2S)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, domainRule)
		assert.Empty(t, domainRule.IEAgAgRuleRefs, "Should handle empty IEAgAgRuleRefs")
	})

	t.Run("FromDomain_WithIEAgAgRuleRefs_Success", func(t *testing.T) {
		// Arrange
		domainRule := &models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{Name: "test-rule", Namespace: "default"},
			},
			Traffic: models.INGRESS,
			Trace:   true, // Set trace to true for the test
			ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					Name: "backend",
				},
				Namespace: "default",
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					Name: "frontend",
				},
				Namespace: "default",
			},
			IEAgAgRuleRefs: []netguardv1beta1.NamespacedObjectReference{
				{ObjectReference: netguardv1beta1.ObjectReference{Name: "ieagag-rule-1"}, Namespace: "default"},
				{ObjectReference: netguardv1beta1.ObjectReference{Name: "ieagag-rule-2"}, Namespace: "default"},
			},
		}

		// Act
		k8sRule, err := converter.FromDomain(ctx, domainRule)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, k8sRule)

		// Check trace field conversion
		assert.True(t, k8sRule.Spec.Trace, "Trace field should be converted correctly")

		// Ключевая проверка: IEAgAgRuleRefs должны быть конвертированы в статус
		assert.Len(t, k8sRule.Status.IEAgAgRuleRefs, 2, "Should convert both IEAgAgRuleRefs to status")

		expected1 := netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "IEAgAgRule",
				Name:       "ieagag-rule-1",
			},
			Namespace: "default",
		}
		expected2 := netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "IEAgAgRule",
				Name:       "ieagag-rule-2",
			},
			Namespace: "default",
		}

		assert.Contains(t, k8sRule.Status.IEAgAgRuleRefs, expected1, "Should contain first ref in status")
		assert.Contains(t, k8sRule.Status.IEAgAgRuleRefs, expected2, "Should contain second ref in status")
	})

	t.Run("FromDomain_WithEmptyIEAgAgRuleRefs_Success", func(t *testing.T) {
		// Arrange
		domainRule := &models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{Name: "test-rule", Namespace: "default"},
			},
			Traffic: models.EGRESS,
			ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					Name: "backend",
				},
				Namespace: "default",
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					Name: "frontend",
				},
				Namespace: "default",
			},
			IEAgAgRuleRefs: []netguardv1beta1.NamespacedObjectReference{}, // Пустые рефы
		}

		// Act
		k8sRule, err := converter.FromDomain(ctx, domainRule)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, k8sRule)
		assert.Empty(t, k8sRule.Status.IEAgAgRuleRefs, "Should handle empty IEAgAgRuleRefs")
	})
}

func TestRuleS2SConverter_TrafficEnum_Conversion(t *testing.T) {
	ctx := context.Background()
	converter := convert.NewRuleS2SConverter()

	t.Run("ToDomain_TrafficEnum_Success", func(t *testing.T) {
		testCases := []struct {
			name           string
			k8sTraffic     netguardv1beta1.Traffic
			expectedDomain models.Traffic
		}{
			{
				name:           "INGRESS conversion",
				k8sTraffic:     netguardv1beta1.INGRESS,
				expectedDomain: models.INGRESS,
			},
			{
				name:           "EGRESS conversion",
				k8sTraffic:     netguardv1beta1.EGRESS,
				expectedDomain: models.EGRESS,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Arrange
				k8sRuleS2S := &netguardv1beta1.RuleS2S{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rule",
						Namespace: "default",
					},
					Spec: netguardv1beta1.RuleS2SSpec{
						Traffic: tc.k8sTraffic,
						ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{Name: "backend"},
							Namespace:       "default",
						},
						ServiceRef: netguardv1beta1.NamespacedObjectReference{
							ObjectReference: netguardv1beta1.ObjectReference{Name: "frontend"},
							Namespace:       "default",
						},
					},
				}

				// Act
				domainRule, err := converter.ToDomain(ctx, k8sRuleS2S)

				// Assert
				require.NoError(t, err)
				assert.Equal(t, tc.expectedDomain, domainRule.Traffic,
					"Traffic should be converted correctly")
			})
		}
	})

	t.Run("FromDomain_TrafficEnum_Success", func(t *testing.T) {
		testCases := []struct {
			name          string
			domainTraffic models.Traffic
			expectedK8s   netguardv1beta1.Traffic
		}{
			{
				name:          "INGRESS conversion",
				domainTraffic: models.INGRESS,
				expectedK8s:   netguardv1beta1.INGRESS,
			},
			{
				name:          "EGRESS conversion",
				domainTraffic: models.EGRESS,
				expectedK8s:   netguardv1beta1.EGRESS,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Arrange
				domainRule := &models.RuleS2S{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{Name: "test-rule", Namespace: "default"},
					},
					Traffic: tc.domainTraffic,
					ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "backend",
						},
						Namespace: "default",
					},
					ServiceRef: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "frontend",
						},
						Namespace: "default",
					},
				}

				// Act
				k8sRule, err := converter.FromDomain(ctx, domainRule)

				// Assert
				require.NoError(t, err)
				assert.Equal(t, tc.expectedK8s, k8sRule.Spec.Traffic,
					"Traffic should be converted correctly")
			})
		}
	})

	t.Run("ToDomain_InvalidTraffic_ReturnsError", func(t *testing.T) {
		// Arrange
		k8sRuleS2S := &netguardv1beta1.RuleS2S{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rule",
				Namespace: "default",
			},
			Spec: netguardv1beta1.RuleS2SSpec{
				Traffic: "INVALID_TRAFFIC", // Невалидное значение
				ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{Name: "backend"},
					Namespace:       "default",
				},
				ServiceRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{Name: "frontend"},
					Namespace:       "default",
				},
			},
		}

		// Act
		_, err := converter.ToDomain(ctx, k8sRuleS2S)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown traffic type",
			"Should return error for invalid traffic type")
	})

	t.Run("FromDomain_InvalidTraffic_ReturnsError", func(t *testing.T) {
		// Arrange
		domainRule := &models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{Name: "test-rule", Namespace: "default"},
			},
			Traffic: "INVALID_TRAFFIC", // Невалидное значение
			ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					Name: "backend",
				},
				Namespace: "default",
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					Name: "frontend",
				},
				Namespace: "default",
			},
		}

		// Act
		_, err := converter.FromDomain(ctx, domainRule)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown traffic type",
			"Should return error for invalid traffic type")
	})
}

func TestRuleS2SConverter_RoundTrip_PreservesData(t *testing.T) {
	ctx := context.Background()
	converter := convert.NewRuleS2SConverter()

	t.Run("DomainToK8sToDomain_PreservesIEAgAgRuleRefs", func(t *testing.T) {
		// Arrange
		originalRule := &models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{Name: "test-rule", Namespace: "default"},
			},
			Traffic: models.INGRESS,
			ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					Name: "backend",
				},
				Namespace: "default",
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					Name: "frontend",
				},
				Namespace: "default",
			},
			IEAgAgRuleRefs: []netguardv1beta1.NamespacedObjectReference{
				{ObjectReference: netguardv1beta1.ObjectReference{Name: "ieagag-rule-1"}, Namespace: "default"},
				{ObjectReference: netguardv1beta1.ObjectReference{Name: "ieagag-rule-2"}, Namespace: "default"},
				{ObjectReference: netguardv1beta1.ObjectReference{Name: "ieagag-rule-3"}, Namespace: "test"},
			},
		}

		// Act: Domain -> K8s -> Domain
		k8sRule, err := converter.FromDomain(ctx, originalRule)
		require.NoError(t, err)

		convertedRule, err := converter.ToDomain(ctx, k8sRule)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, originalRule.ResourceIdentifier, convertedRule.ResourceIdentifier,
			"ResourceIdentifier should be preserved")
		assert.Equal(t, originalRule.Traffic, convertedRule.Traffic,
			"Traffic should be preserved")
		assert.Equal(t, len(originalRule.IEAgAgRuleRefs), len(convertedRule.IEAgAgRuleRefs),
			"Number of IEAgAgRuleRefs should be preserved")

		// Проверяем каждую ссылку
		for _, originalRef := range originalRule.IEAgAgRuleRefs {
			assert.Contains(t, convertedRule.IEAgAgRuleRefs, originalRef,
				"IEAgAgRuleRef %v should be preserved", originalRef)
		}
	})
}

func TestRuleS2SConverter_Trace_Conversion(t *testing.T) {
	ctx := context.Background()
	converter := convert.NewRuleS2SConverter()

	t.Run("ToDomain_WithTrace_Success", func(t *testing.T) {
		// Arrange
		k8sRuleS2S := &netguardv1beta1.RuleS2S{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rule-trace",
				Namespace: "default",
			},
			Spec: netguardv1beta1.RuleS2SSpec{
				Traffic: netguardv1beta1.INGRESS,
				ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{Name: "backend"},
					Namespace:       "default",
				},
				ServiceRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{Name: "frontend"},
					Namespace:       "default",
				},
				Trace: true, // Test trace enabled
			},
		}

		// Act
		domainRule, err := converter.ToDomain(ctx, k8sRuleS2S)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, domainRule)
		assert.True(t, domainRule.Trace, "Trace field should be true when enabled in K8s spec")
	})

	t.Run("FromDomain_WithTrace_Success", func(t *testing.T) {
		// Arrange
		domainRule := &models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{Name: "test-rule-trace", Namespace: "default"},
			},
			Traffic: models.INGRESS,
			ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					Name: "backend",
				},
				Namespace: "default",
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					Name: "frontend",
				},
				Namespace: "default",
			},
			Trace: true, // Test trace enabled
		}

		// Act
		k8sRule, err := converter.FromDomain(ctx, domainRule)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, k8sRule)
		assert.True(t, k8sRule.Spec.Trace, "Trace field should be true when enabled in domain model")
	})
}
