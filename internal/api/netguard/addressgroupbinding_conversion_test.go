package netguard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

func TestAddressGroupBindingConversion_RoundTrip(t *testing.T) {
	t.Run("Full_ObjectReference_Preserved", func(t *testing.T) {
		// Arrange - create protobuf with full ObjectReference fields
		pbBinding := &netguardpb.AddressGroupBinding{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-binding",
				Namespace: "test-namespace",
			},
			ServiceRef: &netguardpb.ServiceRef{
				ObjectRef: &netguardpb.ObjectReference{
					ApiVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       "test-service",
				},
			},
			AddressGroupRef: &netguardpb.AddressGroupRef{
				ObjectRef: &netguardpb.NamespacedObjectReference{
					ApiVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "test-addressgroup",
					Namespace:  "ag-namespace",
				},
			},
		}

		// Act 1: Protobuf → Domain conversion
		domainBinding := convertAddressGroupBinding(pbBinding)

		// Assert 1: ObjectReference fields should be preserved
		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainBinding.ServiceRef.APIVersion)
		assert.Equal(t, "Service", domainBinding.ServiceRef.Kind)
		assert.Equal(t, "test-service", domainBinding.ServiceRef.Name)

		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainBinding.AddressGroupRef.APIVersion)
		assert.Equal(t, "AddressGroup", domainBinding.AddressGroupRef.Kind)
		assert.Equal(t, "test-addressgroup", domainBinding.AddressGroupRef.Name)
		assert.Equal(t, "ag-namespace", domainBinding.AddressGroupRef.Namespace)

		// Act 2: Domain → Protobuf conversion
		pbResult := convertAddressGroupBindingToPB(domainBinding)

		// Assert 2: Full round-trip should preserve all ObjectReference fields
		require.NotNil(t, pbResult.ServiceRef)
		require.NotNil(t, pbResult.ServiceRef.ObjectRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", pbResult.ServiceRef.ObjectRef.ApiVersion)
		assert.Equal(t, "Service", pbResult.ServiceRef.ObjectRef.Kind)
		assert.Equal(t, "test-service", pbResult.ServiceRef.ObjectRef.Name)

		require.NotNil(t, pbResult.AddressGroupRef)
		require.NotNil(t, pbResult.AddressGroupRef.ObjectRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", pbResult.AddressGroupRef.ObjectRef.ApiVersion)
		assert.Equal(t, "AddressGroup", pbResult.AddressGroupRef.ObjectRef.Kind)
		assert.Equal(t, "test-addressgroup", pbResult.AddressGroupRef.ObjectRef.Name)
		assert.Equal(t, "ag-namespace", pbResult.AddressGroupRef.ObjectRef.Namespace)
	})

	t.Run("Legacy_Identifier_Backward_Compatibility", func(t *testing.T) {
		// Arrange - protobuf with legacy identifier fields only
		pbBinding := &netguardpb.AddressGroupBinding{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "legacy-binding",
				Namespace: "test-namespace",
			},
			ServiceRef: &netguardpb.ServiceRef{
				Identifier: &netguardpb.ResourceIdentifier{
					Name:      "legacy-service",
					Namespace: "test-namespace",
				},
			},
			AddressGroupRef: &netguardpb.AddressGroupRef{
				Identifier: &netguardpb.ResourceIdentifier{
					Name:      "legacy-ag",
					Namespace: "ag-namespace",
				},
			},
		}

		// Act: Convert from legacy format
		domainBinding := convertAddressGroupBinding(pbBinding)

		// Assert: Should use default apiVersion and kind
		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainBinding.ServiceRef.APIVersion)
		assert.Equal(t, "Service", domainBinding.ServiceRef.Kind)
		assert.Equal(t, "legacy-service", domainBinding.ServiceRef.Name)

		assert.Equal(t, "netguard.sgroups.io/v1beta1", domainBinding.AddressGroupRef.APIVersion)
		assert.Equal(t, "AddressGroup", domainBinding.AddressGroupRef.Kind)
		assert.Equal(t, "legacy-ag", domainBinding.AddressGroupRef.Name)
		assert.Equal(t, "ag-namespace", domainBinding.AddressGroupRef.Namespace)
	})

	t.Run("Regression_Test_No_Data_Loss", func(t *testing.T) {
		// This is a regression test to ensure we don't lose apiVersion/kind again

		// Create a domain object with full reference info
		domainBinding := models.AddressGroupBinding{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-binding", models.WithNamespace("test-ns"))),
			ServiceRef: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "my-service",
			},
			AddressGroupRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "my-ag",
				},
				Namespace: "ag-ns",
			},
		}

		// Convert to protobuf
		pbBinding := convertAddressGroupBindingToPB(domainBinding)

		// Convert back to domain
		resultBinding := convertAddressGroupBinding(pbBinding)

		// Assert: No data loss in round-trip
		assert.Equal(t, domainBinding.ServiceRef.APIVersion, resultBinding.ServiceRef.APIVersion, "ServiceRef.APIVersion must not be lost!")
		assert.Equal(t, domainBinding.ServiceRef.Kind, resultBinding.ServiceRef.Kind, "ServiceRef.Kind must not be lost!")
		assert.Equal(t, domainBinding.ServiceRef.Name, resultBinding.ServiceRef.Name)

		assert.Equal(t, domainBinding.AddressGroupRef.APIVersion, resultBinding.AddressGroupRef.APIVersion, "AddressGroupRef.APIVersion must not be lost!")
		assert.Equal(t, domainBinding.AddressGroupRef.Kind, resultBinding.AddressGroupRef.Kind, "AddressGroupRef.Kind must not be lost!")
		assert.Equal(t, domainBinding.AddressGroupRef.Name, resultBinding.AddressGroupRef.Name)
		assert.Equal(t, domainBinding.AddressGroupRef.Namespace, resultBinding.AddressGroupRef.Namespace)
	})
}
