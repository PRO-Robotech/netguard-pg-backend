package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestNetworkBindingGRPCConversion_RoundTrip(t *testing.T) {
	// Test the complete round-trip: Domain -> Protobuf -> Domain
	// This test would have caught the original issue with missing APIVersion/Kind

	original := models.NetworkBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-binding",
				Namespace: "test-namespace",
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
		},
	}

	// Convert to protobuf
	protobuf := convertNetworkBindingToPB(original)
	require.NotNil(t, protobuf)

	// Verify protobuf contains all required fields
	assert.Equal(t, "test-binding", protobuf.SelfRef.Name)
	assert.Equal(t, "test-namespace", protobuf.SelfRef.Namespace)

	// The critical test - ensure APIVersion and Kind are preserved
	assert.Equal(t, "netguard.sgroups.io/v1beta1", protobuf.NetworkRef.ApiVersion)
	assert.Equal(t, "Network", protobuf.NetworkRef.Kind)
	assert.Equal(t, "test-network", protobuf.NetworkRef.Name)

	assert.Equal(t, "netguard.sgroups.io/v1beta1", protobuf.AddressGroupRef.ApiVersion)
	assert.Equal(t, "AddressGroup", protobuf.AddressGroupRef.Kind)
	assert.Equal(t, "test-addressgroup", protobuf.AddressGroupRef.Name)

	// Convert back to domain model
	restored := convertNetworkBindingFromProto(protobuf)

	// Verify complete round-trip preservation
	assert.Equal(t, original.Name, restored.Name)
	assert.Equal(t, original.Namespace, restored.Namespace)

	// The critical assertion - APIVersion and Kind must be preserved
	assert.Equal(t, original.NetworkRef.APIVersion, restored.NetworkRef.APIVersion)
	assert.Equal(t, original.NetworkRef.Kind, restored.NetworkRef.Kind)
	assert.Equal(t, original.NetworkRef.Name, restored.NetworkRef.Name)

	assert.Equal(t, original.AddressGroupRef.APIVersion, restored.AddressGroupRef.APIVersion)
	assert.Equal(t, original.AddressGroupRef.Kind, restored.AddressGroupRef.Kind)
	assert.Equal(t, original.AddressGroupRef.Name, restored.AddressGroupRef.Name)

	assert.Equal(t, original.NetworkItem.Name, restored.NetworkItem.Name)
	assert.Equal(t, original.NetworkItem.CIDR, restored.NetworkItem.CIDR)
}

func TestNetworkBindingGRPCConversion_EmptyObjectReference(t *testing.T) {
	// Test what happens with empty/partial ObjectReferences
	// This should be handled gracefully (empty values preserved)

	bindingWithEmptyRefs := models.NetworkBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-binding-empty",
				Namespace: "test-namespace",
			},
		},
		// ObjectReferences with only names - APIVersion and Kind are empty
		NetworkRef: netguardv1beta1.ObjectReference{
			Name: "test-network",
			// APIVersion and Kind intentionally empty
		},
		AddressGroupRef: netguardv1beta1.ObjectReference{
			Name: "test-addressgroup",
			// APIVersion and Kind intentionally empty
		},
	}

	// Convert to protobuf and back
	protobuf := convertNetworkBindingToPB(bindingWithEmptyRefs)
	restored := convertNetworkBindingFromProto(protobuf)

	// Empty values should be preserved as empty (not filled or lost)
	assert.Equal(t, "", restored.NetworkRef.APIVersion)
	assert.Equal(t, "", restored.NetworkRef.Kind)
	assert.Equal(t, "test-network", restored.NetworkRef.Name)

	assert.Equal(t, "", restored.AddressGroupRef.APIVersion)
	assert.Equal(t, "", restored.AddressGroupRef.Kind)
	assert.Equal(t, "test-addressgroup", restored.AddressGroupRef.Name)
}
