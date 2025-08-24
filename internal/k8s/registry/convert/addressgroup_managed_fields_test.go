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

func TestAddressGroupConverter_ManagedFields(t *testing.T) {
	converter := NewAddressGroupConverter()
	ctx := context.Background()

	t.Run("ManagedFields_ToDomain", func(t *testing.T) {
		// Create k8s AddressGroup with managedFields
		now := metav1.NewTime(time.Now())
		k8sAddressGroup := &netguardv1beta1.AddressGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-address-group",
				Namespace: "default",
				UID:       types.UID("ag-uid-123"),
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "kubectl-client-side-apply",
						Operation:  metav1.ManagedFieldsOperationUpdate,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:metadata":{"f:labels":{}},"f:spec":{"f:defaultAction":{}}}`),
						},
					},
					{
						Manager:    "netguard-apiserver",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:networks":{},"f:spec":{"f:logs":{},"f:trace":{}}}`),
						},
					},
				},
			},
			Spec: netguardv1beta1.AddressGroupSpec{
				DefaultAction: netguardv1beta1.ActionAccept,
				Logs:          true,
				Trace:         false,
			},
			Networks: []netguardv1beta1.NetworkItem{
				{
					Name: "test-network",
					CIDR: "10.0.0.0/24",
				},
			},
		}

		// Convert to domain
		domainAddressGroup, err := converter.ToDomain(ctx, k8sAddressGroup)
		require.NoError(t, err)
		require.NotNil(t, domainAddressGroup)

		// Verify managedFields are preserved
		assert.NotNil(t, domainAddressGroup.Meta.ManagedFields)
		assert.Len(t, domainAddressGroup.Meta.ManagedFields, 2)

		// Check first managedFields entry
		entry1 := domainAddressGroup.Meta.ManagedFields[0]
		assert.Equal(t, "kubectl-client-side-apply", entry1.Manager)
		assert.Equal(t, metav1.ManagedFieldsOperationUpdate, entry1.Operation)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", entry1.APIVersion)
		assert.Equal(t, "FieldsV1", entry1.FieldsType)
		assert.NotNil(t, entry1.FieldsV1)

		// Check second managedFields entry
		entry2 := domainAddressGroup.Meta.ManagedFields[1]
		assert.Equal(t, "netguard-apiserver", entry2.Manager)
		assert.Equal(t, metav1.ManagedFieldsOperationApply, entry2.Operation)
	})

	t.Run("ManagedFields_FromDomain", func(t *testing.T) {
		// Create domain AddressGroup with managedFields
		now := metav1.NewTime(time.Now())
		domainAddressGroup := &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-address-group",
					Namespace: "default",
				},
			},
			DefaultAction: models.ActionAccept,
			Logs:          true,
			Trace:         false,
			Networks: []models.NetworkItem{
				{
					Name: "test-network",
					CIDR: "10.0.0.0/24",
				},
			},
			Meta: models.Meta{
				UID:             "ag-uid-123",
				ResourceVersion: "12345",
				Generation:      1,
				CreationTS:      now,
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "kubectl-apply",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:spec":{"f:defaultAction":{},"f:logs":{}}}`),
						},
					},
				},
			},
		}

		// Convert from domain
		k8sAddressGroup, err := converter.FromDomain(ctx, domainAddressGroup)
		require.NoError(t, err)
		require.NotNil(t, k8sAddressGroup)

		// Verify managedFields are preserved
		assert.NotNil(t, k8sAddressGroup.ManagedFields)
		assert.Len(t, k8sAddressGroup.ManagedFields, 1)

		// Check managedFields entry
		entry := k8sAddressGroup.ManagedFields[0]
		assert.Equal(t, "kubectl-apply", entry.Manager)
		assert.Equal(t, metav1.ManagedFieldsOperationApply, entry.Operation)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", entry.APIVersion)
		assert.Equal(t, "FieldsV1", entry.FieldsType)
		assert.NotNil(t, entry.FieldsV1)
		assert.Equal(t, `{"f:spec":{"f:defaultAction":{},"f:logs":{}}}`, string(entry.FieldsV1.Raw))
	})

	t.Run("ManagedFields_RoundTrip", func(t *testing.T) {
		// Test round-trip: K8s -> Domain -> K8s
		now := metav1.NewTime(time.Now())
		originalAddressGroup := &netguardv1beta1.AddressGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "roundtrip-address-group",
				Namespace: "test",
				UID:       types.UID("roundtrip-ag-uid"),
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "test-manager",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:spec":{"f:defaultAction":{},"f:logs":{},"f:trace":{}},"f:networks":{}}`),
						},
					},
				},
			},
			Spec: netguardv1beta1.AddressGroupSpec{
				DefaultAction: netguardv1beta1.ActionDrop,
				Logs:          true,
				Trace:         true,
			},
			Networks: []netguardv1beta1.NetworkItem{
				{
					Name:      "network-1",
					CIDR:      "192.168.1.0/24",
					Namespace: "default",
				},
				{
					Name: "network-2",
					CIDR: "192.168.2.0/24",
				},
			},
		}

		// K8s -> Domain
		domainAddressGroup, err := converter.ToDomain(ctx, originalAddressGroup)
		require.NoError(t, err)

		// Domain -> K8s
		convertedAddressGroup, err := converter.FromDomain(ctx, domainAddressGroup)
		require.NoError(t, err)

		// Verify managedFields survived round-trip
		assert.Equal(t, len(originalAddressGroup.ManagedFields), len(convertedAddressGroup.ManagedFields))
		assert.Equal(t, originalAddressGroup.ManagedFields[0].Manager, convertedAddressGroup.ManagedFields[0].Manager)
		assert.Equal(t, originalAddressGroup.ManagedFields[0].Operation, convertedAddressGroup.ManagedFields[0].Operation)
		assert.Equal(t, originalAddressGroup.ManagedFields[0].APIVersion, convertedAddressGroup.ManagedFields[0].APIVersion)
		assert.Equal(t, originalAddressGroup.ManagedFields[0].FieldsType, convertedAddressGroup.ManagedFields[0].FieldsType)
		assert.Equal(t, string(originalAddressGroup.ManagedFields[0].FieldsV1.Raw), string(convertedAddressGroup.ManagedFields[0].FieldsV1.Raw))

		// Verify other fields also survived round-trip
		assert.Equal(t, originalAddressGroup.Spec.DefaultAction, convertedAddressGroup.Spec.DefaultAction)
		assert.Equal(t, originalAddressGroup.Spec.Logs, convertedAddressGroup.Spec.Logs)
		assert.Equal(t, originalAddressGroup.Spec.Trace, convertedAddressGroup.Spec.Trace)
		assert.Equal(t, len(originalAddressGroup.Networks), len(convertedAddressGroup.Networks))
	})

	t.Run("ManagedFields_EmptyHandling", func(t *testing.T) {
		// Test handling of empty/nil managedFields
		k8sAddressGroup := &netguardv1beta1.AddressGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:          "empty-managed-fields-ag",
				Namespace:     "default",
				ManagedFields: nil, // nil managedFields
			},
			Spec: netguardv1beta1.AddressGroupSpec{
				DefaultAction: netguardv1beta1.ActionAccept,
			},
		}

		// Convert to domain
		domainAddressGroup, err := converter.ToDomain(ctx, k8sAddressGroup)
		require.NoError(t, err)
		assert.Nil(t, domainAddressGroup.Meta.ManagedFields)

		// Convert back to k8s
		convertedAddressGroup, err := converter.FromDomain(ctx, domainAddressGroup)
		require.NoError(t, err)
		assert.Nil(t, convertedAddressGroup.ManagedFields)
	})

	t.Run("ManagedFields_WithNetworks", func(t *testing.T) {
		// Test managedFields conversion with complex networks
		now := metav1.NewTime(time.Now())
		k8sAddressGroup := &netguardv1beta1.AddressGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "complex-networks-ag",
				Namespace: "default",
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "network-manager",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:networks":{"k:{\"name\":\"net1\"}":{},"k:{\"name\":\"net2\"}":{}}}`),
						},
					},
				},
			},
			Spec: netguardv1beta1.AddressGroupSpec{
				DefaultAction: netguardv1beta1.ActionAccept,
			},
			Networks: []netguardv1beta1.NetworkItem{
				{
					Name:       "net1",
					CIDR:       "10.1.0.0/16",
					ApiVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Network",
					Namespace:  "default",
				},
				{
					Name:       "net2",
					CIDR:       "10.2.0.0/16",
					ApiVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Network",
					Namespace:  "prod",
				},
			},
		}

		// Convert and verify networks + managedFields
		domainAddressGroup, err := converter.ToDomain(ctx, k8sAddressGroup)
		require.NoError(t, err)

		// Verify both networks and managedFields are preserved
		assert.Len(t, domainAddressGroup.Networks, 2)
		assert.Equal(t, "net1", domainAddressGroup.Networks[0].Name)
		assert.Equal(t, "10.1.0.0/16", domainAddressGroup.Networks[0].CIDR)
		assert.Equal(t, "default", domainAddressGroup.Networks[0].Namespace)

		assert.Equal(t, "net2", domainAddressGroup.Networks[1].Name)
		assert.Equal(t, "10.2.0.0/16", domainAddressGroup.Networks[1].CIDR)
		assert.Equal(t, "prod", domainAddressGroup.Networks[1].Namespace)

		assert.Len(t, domainAddressGroup.Meta.ManagedFields, 1)
		assert.Equal(t, "network-manager", domainAddressGroup.Meta.ManagedFields[0].Manager)
	})
}
