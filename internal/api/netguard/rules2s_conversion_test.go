package netguard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

func TestRuleS2SConversion_RoundTrip(t *testing.T) {
	t.Run("Full_NamespacedObjectReference_Preserved", func(t *testing.T) {
		// Arrange - create protobuf with full NamespacedObjectReference fields
		pbRule := &netguardpb.RuleS2S{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-rule",
				Namespace: "test-namespace",
			},
			Traffic: netguardpb.Traffic_Ingress,
			ServiceLocalRef: &netguardpb.ServiceRef{
				ObjectRef: &netguardpb.NamespacedObjectReference{
					ApiVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       "local-service",
					Namespace:  "local-ns",
				},
			},
			ServiceRef: &netguardpb.ServiceRef{
				ObjectRef: &netguardpb.NamespacedObjectReference{
					ApiVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       "remote-service",
					Namespace:  "remote-ns",
				},
			},
			IeagAgRuleObjectRefs: []*netguardpb.NamespacedObjectReference{
				{
					ApiVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "IEAgAgRule",
					Name:       "rule-1",
					Namespace:  "rule-ns",
				},
				{
					ApiVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "IEAgAgRule",
					Name:       "rule-2",
					Namespace:  "rule-ns",
				},
			},
			Trace: true,
		}

		// Act 1: Protobuf → Domain conversion
		domainRule := convertRuleS2S(pbRule)

		// Assert 1: NamespacedObjectReference fields should be preserved
		assert.Equal(t, models.INGRESS, domainRule.Traffic)
		assert.Equal(t, true, domainRule.Trace)

		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainRule.ServiceLocalRef.APIVersion)
		assert.Equal(t, "ServiceAlias", domainRule.ServiceLocalRef.Kind)
		assert.Equal(t, "local-service", domainRule.ServiceLocalRef.Name)
		assert.Equal(t, "local-ns", domainRule.ServiceLocalRef.Namespace)

		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainRule.ServiceRef.APIVersion)
		assert.Equal(t, "ServiceAlias", domainRule.ServiceRef.Kind)
		assert.Equal(t, "remote-service", domainRule.ServiceRef.Name)
		assert.Equal(t, "remote-ns", domainRule.ServiceRef.Namespace)

		require.Len(t, domainRule.IEAgAgRuleRefs, 2)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainRule.IEAgAgRuleRefs[0].APIVersion)
		assert.Equal(t, "IEAgAgRule", domainRule.IEAgAgRuleRefs[0].Kind)
		assert.Equal(t, "rule-1", domainRule.IEAgAgRuleRefs[0].Name)
		assert.Equal(t, "rule-ns", domainRule.IEAgAgRuleRefs[0].Namespace)

		// Act 2: Domain → Protobuf conversion
		pbResult := convertRuleS2SToPB(domainRule)

		// Assert 2: Full round-trip should preserve all NamespacedObjectReference fields
		require.NotNil(t, pbResult.ServiceLocalRef)
		require.NotNil(t, pbResult.ServiceLocalRef.ObjectRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", pbResult.ServiceLocalRef.ObjectRef.ApiVersion)
		assert.Equal(t, "ServiceAlias", pbResult.ServiceLocalRef.ObjectRef.Kind)
		assert.Equal(t, "local-service", pbResult.ServiceLocalRef.ObjectRef.Name)
		assert.Equal(t, "local-ns", pbResult.ServiceLocalRef.ObjectRef.Namespace)

		require.NotNil(t, pbResult.ServiceRef)
		require.NotNil(t, pbResult.ServiceRef.ObjectRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", pbResult.ServiceRef.ObjectRef.ApiVersion)
		assert.Equal(t, "ServiceAlias", pbResult.ServiceRef.ObjectRef.Kind)
		assert.Equal(t, "remote-service", pbResult.ServiceRef.ObjectRef.Name)
		assert.Equal(t, "remote-ns", pbResult.ServiceRef.ObjectRef.Namespace)

		require.Len(t, pbResult.IeagAgRuleObjectRefs, 2)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", pbResult.IeagAgRuleObjectRefs[0].ApiVersion)
		assert.Equal(t, "IEAgAgRule", pbResult.IeagAgRuleObjectRefs[0].Kind)
		assert.Equal(t, "rule-1", pbResult.IeagAgRuleObjectRefs[0].Name)
		assert.Equal(t, "rule-ns", pbResult.IeagAgRuleObjectRefs[0].Namespace)

		assert.Equal(t, true, pbResult.Trace)
	})

	t.Run("Legacy_Identifier_Backward_Compatibility", func(t *testing.T) {
		// Arrange - protobuf with legacy identifier fields only
		pbRule := &netguardpb.RuleS2S{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "legacy-rule",
				Namespace: "test-namespace",
			},
			Traffic: netguardpb.Traffic_Egress,
			ServiceLocalRef: &netguardpb.ServiceRef{
				Identifier: &netguardpb.ResourceIdentifier{
					Name:      "legacy-local",
					Namespace: "local-ns",
				},
			},
			ServiceRef: &netguardpb.ServiceRef{
				Identifier: &netguardpb.ResourceIdentifier{
					Name:      "legacy-remote",
					Namespace: "remote-ns",
				},
			},
			IeagAgRuleRefs: []*netguardpb.ResourceIdentifier{
				{
					Name:      "legacy-rule-1",
					Namespace: "rule-ns",
				},
			},
		}

		// Act: Convert from legacy format
		domainRule := convertRuleS2S(pbRule)

		// Assert: Should use default apiVersion and kind
		assert.Equal(t, models.EGRESS, domainRule.Traffic)

		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainRule.ServiceLocalRef.APIVersion)
		assert.Equal(t, "ServiceAlias", domainRule.ServiceLocalRef.Kind)
		assert.Equal(t, "legacy-local", domainRule.ServiceLocalRef.Name)
		assert.Equal(t, "local-ns", domainRule.ServiceLocalRef.Namespace)

		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainRule.ServiceRef.APIVersion)
		assert.Equal(t, "ServiceAlias", domainRule.ServiceRef.Kind)
		assert.Equal(t, "legacy-remote", domainRule.ServiceRef.Name)
		assert.Equal(t, "remote-ns", domainRule.ServiceRef.Namespace)

		require.Len(t, domainRule.IEAgAgRuleRefs, 1)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainRule.IEAgAgRuleRefs[0].APIVersion)
		assert.Equal(t, "IEAgAgRule", domainRule.IEAgAgRuleRefs[0].Kind)
		assert.Equal(t, "legacy-rule-1", domainRule.IEAgAgRuleRefs[0].Name)
	})

	t.Run("Regression_Test_No_Data_Loss", func(t *testing.T) {
		// This is a regression test to ensure we don't lose apiVersion/kind again

		// Create a domain object with full reference info
		domainRule := models.RuleS2S{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-rule", models.WithNamespace("test-ns"))),
			Traffic: models.INGRESS,
			ServiceLocalRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       "my-local-service",
				},
				Namespace: "local-ns",
			},
			ServiceRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       "my-remote-service",
				},
				Namespace: "remote-ns",
			},
			IEAgAgRuleRefs: []v1beta1.NamespacedObjectReference{
				{
					ObjectReference: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "IEAgAgRule",
						Name:       "my-rule",
					},
					Namespace: "rule-ns",
				},
			},
			Trace: true,
		}

		// Convert to protobuf
		pbRule := convertRuleS2SToPB(domainRule)

		// Convert back to domain
		resultRule := convertRuleS2S(pbRule)

		// Assert: No data loss in round-trip
		assert.Equal(t, domainRule.ServiceLocalRef.APIVersion, resultRule.ServiceLocalRef.APIVersion, "ServiceLocalRef.APIVersion must not be lost!")
		assert.Equal(t, domainRule.ServiceLocalRef.Kind, resultRule.ServiceLocalRef.Kind, "ServiceLocalRef.Kind must not be lost!")
		assert.Equal(t, domainRule.ServiceLocalRef.Name, resultRule.ServiceLocalRef.Name)
		assert.Equal(t, domainRule.ServiceLocalRef.Namespace, resultRule.ServiceLocalRef.Namespace)

		assert.Equal(t, domainRule.ServiceRef.APIVersion, resultRule.ServiceRef.APIVersion, "ServiceRef.APIVersion must not be lost!")
		assert.Equal(t, domainRule.ServiceRef.Kind, resultRule.ServiceRef.Kind, "ServiceRef.Kind must not be lost!")
		assert.Equal(t, domainRule.ServiceRef.Name, resultRule.ServiceRef.Name)
		assert.Equal(t, domainRule.ServiceRef.Namespace, resultRule.ServiceRef.Namespace)

		require.Len(t, resultRule.IEAgAgRuleRefs, 1)
		assert.Equal(t, domainRule.IEAgAgRuleRefs[0].APIVersion, resultRule.IEAgAgRuleRefs[0].APIVersion, "IEAgAgRuleRef.APIVersion must not be lost!")
		assert.Equal(t, domainRule.IEAgAgRuleRefs[0].Kind, resultRule.IEAgAgRuleRefs[0].Kind, "IEAgAgRuleRef.Kind must not be lost!")

		assert.Equal(t, domainRule.Trace, resultRule.Trace)
	})
}
