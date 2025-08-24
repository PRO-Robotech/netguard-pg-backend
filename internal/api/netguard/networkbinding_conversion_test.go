package netguard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

func TestNetworkBindingBackendConversion_RoundTrip(t *testing.T) {
	t.Run("NetworkBinding_ObjectReference_Fields_Preserved", func(t *testing.T) {
		// Arrange - создаем protobuf NetworkBinding с полными ObjectReference полями
		pbBinding := &netguardpb.NetworkBinding{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-binding",
				Namespace: "test-namespace",
			},
			NetworkRef: &netguardpb.ObjectReference{
				ApiVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Network",
				Name:       "test-network",
			},
			AddressGroupRef: &netguardpb.ObjectReference{
				ApiVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "test-addressgroup",
			},
			NetworkItem: &netguardpb.NetworkItem{
				Name: "test-network",
				Cidr: "10.0.0.0/24",
			},
		}

		// Act 1: Protobuf → Domain conversion
		domainBinding := convertNetworkBinding(pbBinding)

		// Assert 1: ObjectReference fields should be preserved in domain model
		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainBinding.NetworkRef.APIVersion)
		assert.Equal(t, "Network", domainBinding.NetworkRef.Kind)
		assert.Equal(t, "test-network", domainBinding.NetworkRef.Name)

		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainBinding.AddressGroupRef.APIVersion)
		assert.Equal(t, "AddressGroup", domainBinding.AddressGroupRef.Kind)
		assert.Equal(t, "test-addressgroup", domainBinding.AddressGroupRef.Name)

		// Act 2: Domain → Protobuf conversion
		pbResult := convertNetworkBindingToPB(domainBinding)

		// Assert 2: Full round-trip should preserve all ObjectReference fields
		require.NotNil(t, pbResult.NetworkRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", pbResult.NetworkRef.ApiVersion)
		assert.Equal(t, "Network", pbResult.NetworkRef.Kind)
		assert.Equal(t, "test-network", pbResult.NetworkRef.Name)

		require.NotNil(t, pbResult.AddressGroupRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", pbResult.AddressGroupRef.ApiVersion)
		assert.Equal(t, "AddressGroup", pbResult.AddressGroupRef.Kind)
		assert.Equal(t, "test-addressgroup", pbResult.AddressGroupRef.Name)
	})

	t.Run("NetworkBinding_EmptyObjectReference_Fields_Preserved", func(t *testing.T) {
		// Arrange - создаем protobuf с пустыми ObjectReference полями
		pbBinding := &netguardpb.NetworkBinding{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-binding-empty",
				Namespace: "test-namespace",
			},
			NetworkRef: &netguardpb.ObjectReference{
				// Пустые APIVersion и Kind - это должно сохраняться
				ApiVersion: "",
				Kind:       "",
				Name:       "test-network",
			},
			AddressGroupRef: &netguardpb.ObjectReference{
				// Пустые APIVersion и Kind - это должно сохраняться
				ApiVersion: "",
				Kind:       "",
				Name:       "test-addressgroup",
			},
		}

		// Act: Full round-trip conversion
		domainBinding := convertNetworkBinding(pbBinding)
		pbResult := convertNetworkBindingToPB(domainBinding)

		// Assert: Empty values should be preserved as empty (not lost or changed)
		require.NotNil(t, pbResult.NetworkRef)
		assert.Equal(t, "", pbResult.NetworkRef.ApiVersion)
		assert.Equal(t, "", pbResult.NetworkRef.Kind)
		assert.Equal(t, "test-network", pbResult.NetworkRef.Name)

		require.NotNil(t, pbResult.AddressGroupRef)
		assert.Equal(t, "", pbResult.AddressGroupRef.ApiVersion)
		assert.Equal(t, "", pbResult.AddressGroupRef.Kind)
		assert.Equal(t, "test-addressgroup", pbResult.AddressGroupRef.Name)
	})

	t.Run("NetworkBinding_NilObjectReference_Handled", func(t *testing.T) {
		// Arrange - protobuf с nil ObjectReference (shouldn't happen in practice)
		pbBinding := &netguardpb.NetworkBinding{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-binding-nil",
				Namespace: "test-namespace",
			},
			NetworkRef:      nil, // nil reference
			AddressGroupRef: nil, // nil reference
		}

		// Act & Assert - не должно паниковать
		require.NotPanics(t, func() {
			domainBinding := convertNetworkBinding(pbBinding)

			// Domain model должна иметь zero-value ObjectReference структуры
			assert.Equal(t, v1beta1.ObjectReference{}, domainBinding.NetworkRef)
			assert.Equal(t, v1beta1.ObjectReference{}, domainBinding.AddressGroupRef)
		})
	})
}

func TestNetworkBindingBackendConversion_Regression(t *testing.T) {
	t.Run("ConvertNetworkBinding_DoesNotLose_APIVersion_Kind", func(t *testing.T) {
		// Regression test для бага где convertNetworkBinding копировал только Name

		// Arrange - полный протобуф объект
		pbBinding := &netguardpb.NetworkBinding{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "regression-test",
				Namespace: "test-ns",
			},
			NetworkRef: &netguardpb.ObjectReference{
				ApiVersion: "netguard.sgroups.io/v1beta1", // ЭТО НЕ ДОЛЖНО ТЕРЯТЬСЯ
				Kind:       "Network",                     // ЭТО НЕ ДОЛЖНО ТЕРЯТЬСЯ
				Name:       "network-name",
			},
			AddressGroupRef: &netguardpb.ObjectReference{
				ApiVersion: "netguard.sgroups.io/v1beta1", // ЭТО НЕ ДОЛЖНО ТЕРЯТЬСЯ
				Kind:       "AddressGroup",                // ЭТО НЕ ДОЛЖНО ТЕРЯТЬСЯ
				Name:       "ag-name",
			},
		}

		// Act
		result := convertNetworkBinding(pbBinding)

		// Assert - раньше эти поля терялись, теперь должны сохраняться
		assert.Equal(t, "netguard.sgroups.io/v1beta1", result.NetworkRef.APIVersion, "APIVersion НЕ должна теряться!")
		assert.Equal(t, "Network", result.NetworkRef.Kind, "Kind НЕ должен теряться!")
		assert.Equal(t, "network-name", result.NetworkRef.Name)

		assert.Equal(t, "netguard.sgroups.io/v1beta1", result.AddressGroupRef.APIVersion, "APIVersion НЕ должна теряться!")
		assert.Equal(t, "AddressGroup", result.AddressGroupRef.Kind, "Kind НЕ должен теряться!")
		assert.Equal(t, "ag-name", result.AddressGroupRef.Name)
	})
}
