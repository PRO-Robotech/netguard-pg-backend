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

func TestServiceConverter_ManagedFields(t *testing.T) {
	converter := NewServiceConverter()
	ctx := context.Background()

	// Test managedFields conversion in both directions
	t.Run("ManagedFields_ToDomain", func(t *testing.T) {
		// Create k8s Service with managedFields
		now := metav1.NewTime(time.Now())
		k8sService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				UID:       types.UID("test-uid-123"),
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "kubectl-client-side-apply",
						Operation:  metav1.ManagedFieldsOperationUpdate,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:metadata":{"f:labels":{}},"f:spec":{"f:description":{}}}`),
						},
					},
					{
						Manager:    "netguard-apiserver",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:spec":{"f:ingressPorts":{}}}`),
						},
					},
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Test service with managedFields",
			},
		}

		// Convert to domain
		domainService, err := converter.ToDomain(ctx, k8sService)
		require.NoError(t, err)
		require.NotNil(t, domainService)

		// Verify managedFields are preserved
		assert.NotNil(t, domainService.Meta.ManagedFields)
		assert.Len(t, domainService.Meta.ManagedFields, 2)

		// Check first managedFields entry
		entry1 := domainService.Meta.ManagedFields[0]
		assert.Equal(t, "kubectl-client-side-apply", entry1.Manager)
		assert.Equal(t, metav1.ManagedFieldsOperationUpdate, entry1.Operation)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", entry1.APIVersion)
		assert.Equal(t, "FieldsV1", entry1.FieldsType)
		assert.NotNil(t, entry1.FieldsV1)
		assert.Equal(t, `{"f:metadata":{"f:labels":{}},"f:spec":{"f:description":{}}}`, string(entry1.FieldsV1.Raw))

		// Check second managedFields entry
		entry2 := domainService.Meta.ManagedFields[1]
		assert.Equal(t, "netguard-apiserver", entry2.Manager)
		assert.Equal(t, metav1.ManagedFieldsOperationApply, entry2.Operation)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", entry2.APIVersion)
		assert.Equal(t, "FieldsV1", entry2.FieldsType)
		assert.NotNil(t, entry2.FieldsV1)
		assert.Equal(t, `{"f:spec":{"f:ingressPorts":{}}}`, string(entry2.FieldsV1.Raw))
	})

	t.Run("ManagedFields_FromDomain", func(t *testing.T) {
		// Create domain Service with managedFields
		now := metav1.NewTime(time.Now())
		domainService := &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-service",
					Namespace: "default",
				},
			},
			Description: "Test service with managedFields",
			Meta: models.Meta{
				UID:             "test-uid-123",
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
							Raw: []byte(`{"f:spec":{"f:description":{}}}`),
						},
					},
				},
			},
		}

		// Convert from domain
		k8sService, err := converter.FromDomain(ctx, domainService)
		require.NoError(t, err)
		require.NotNil(t, k8sService)

		// Verify managedFields are preserved
		assert.NotNil(t, k8sService.ManagedFields)
		assert.Len(t, k8sService.ManagedFields, 1)

		// Check managedFields entry
		entry := k8sService.ManagedFields[0]
		assert.Equal(t, "kubectl-apply", entry.Manager)
		assert.Equal(t, metav1.ManagedFieldsOperationApply, entry.Operation)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", entry.APIVersion)
		assert.Equal(t, "FieldsV1", entry.FieldsType)
		assert.NotNil(t, entry.FieldsV1)
		assert.Equal(t, `{"f:spec":{"f:description":{}}}`, string(entry.FieldsV1.Raw))
	})

	t.Run("ManagedFields_RoundTrip", func(t *testing.T) {
		// Test round-trip: K8s -> Domain -> K8s
		now := metav1.NewTime(time.Now())
		originalService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "roundtrip-service",
				Namespace: "test",
				UID:       types.UID("roundtrip-uid"),
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "test-manager",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:spec":{"f:description":{},"f:ingressPorts":{}}}`),
						},
					},
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Round-trip test",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Protocol: netguardv1beta1.ProtocolTCP,
						Port:     "80",
					},
				},
			},
		}

		// K8s -> Domain
		domainService, err := converter.ToDomain(ctx, originalService)
		require.NoError(t, err)

		// Domain -> K8s
		convertedService, err := converter.FromDomain(ctx, domainService)
		require.NoError(t, err)

		// Verify managedFields survived round-trip
		assert.Equal(t, len(originalService.ManagedFields), len(convertedService.ManagedFields))
		assert.Equal(t, originalService.ManagedFields[0].Manager, convertedService.ManagedFields[0].Manager)
		assert.Equal(t, originalService.ManagedFields[0].Operation, convertedService.ManagedFields[0].Operation)
		assert.Equal(t, originalService.ManagedFields[0].APIVersion, convertedService.ManagedFields[0].APIVersion)
		assert.Equal(t, originalService.ManagedFields[0].FieldsType, convertedService.ManagedFields[0].FieldsType)
		assert.Equal(t, string(originalService.ManagedFields[0].FieldsV1.Raw), string(convertedService.ManagedFields[0].FieldsV1.Raw))
	})

	t.Run("ManagedFields_EmptyHandling", func(t *testing.T) {
		// Test handling of empty/nil managedFields
		k8sService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:          "empty-managed-fields",
				Namespace:     "default",
				ManagedFields: nil, // nil managedFields
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "No managed fields",
			},
		}

		// Convert to domain
		domainService, err := converter.ToDomain(ctx, k8sService)
		require.NoError(t, err)
		assert.Nil(t, domainService.Meta.ManagedFields)

		// Convert back to k8s
		convertedService, err := converter.FromDomain(ctx, domainService)
		require.NoError(t, err)
		assert.Nil(t, convertedService.ManagedFields)
	})

	t.Run("ManagedFields_MultipleManagers", func(t *testing.T) {
		// Test multiple field managers
		now1 := metav1.NewTime(time.Now())
		now2 := metav1.NewTime(time.Now().Add(1 * time.Minute))
		now3 := metav1.NewTime(time.Now().Add(2 * time.Minute))

		k8sService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-manager-service",
				Namespace: "default",
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "manager-1",
						Operation:  metav1.ManagedFieldsOperationUpdate,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now1,
						FieldsType: "FieldsV1",
						FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:metadata":{"f:labels":{}}}`)},
					},
					{
						Manager:    "manager-2",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now2,
						FieldsType: "FieldsV1",
						FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:spec":{"f:description":{}}}`)},
					},
					{
						Manager:    "manager-3",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now3,
						FieldsType: "FieldsV1",
						FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:spec":{"f:ingressPorts":{}}}`)},
					},
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Multiple managers test",
			},
		}

		// Test conversion
		domainService, err := converter.ToDomain(ctx, k8sService)
		require.NoError(t, err)
		assert.Len(t, domainService.Meta.ManagedFields, 3)

		// Verify all managers are preserved
		managers := make(map[string]bool)
		for _, entry := range domainService.Meta.ManagedFields {
			managers[entry.Manager] = true
		}
		assert.True(t, managers["manager-1"])
		assert.True(t, managers["manager-2"])
		assert.True(t, managers["manager-3"])
	})
}

func TestDomainMeta_ManagedFields(t *testing.T) {
	// Test Meta struct managedFields methods
	t.Run("AddManagedField", func(t *testing.T) {
		meta := &models.Meta{}
		now := metav1.NewTime(time.Now())

		// Add first entry
		entry1 := metav1.ManagedFieldsEntry{
			Manager:    "test-manager",
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: "netguard.sgroups.io/v1beta1",
			Time:       &now,
			FieldsType: "FieldsV1",
			FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:spec":{"f:description":{}}}`)},
		}
		meta.AddManagedField(entry1)

		assert.Len(t, meta.ManagedFields, 1)
		assert.Equal(t, "test-manager", meta.ManagedFields[0].Manager)

		// Add entry for same manager - should update
		entry2 := metav1.ManagedFieldsEntry{
			Manager:    "test-manager",
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: "netguard.sgroups.io/v1beta1",
			Time:       &now,
			FieldsType: "FieldsV1",
			FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:spec":{"f:ingressPorts":{}}}`)},
		}
		meta.AddManagedField(entry2)

		assert.Len(t, meta.ManagedFields, 1) // Still 1 entry
		assert.Equal(t, `{"f:spec":{"f:ingressPorts":{}}}`, string(meta.ManagedFields[0].FieldsV1.Raw))

		// Add different manager - should add new
		entry3 := metav1.ManagedFieldsEntry{
			Manager:    "different-manager",
			Operation:  metav1.ManagedFieldsOperationUpdate,
			APIVersion: "netguard.sgroups.io/v1beta1",
			Time:       &now,
			FieldsType: "FieldsV1",
			FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:metadata":{"f:labels":{}}}`)},
		}
		meta.AddManagedField(entry3)

		assert.Len(t, meta.ManagedFields, 2)
	})

	t.Run("RemoveManagedFieldsByManager", func(t *testing.T) {
		meta := &models.Meta{}
		now := metav1.NewTime(time.Now())

		// Add multiple entries with different managers
		entries := []metav1.ManagedFieldsEntry{
			{
				Manager:    "manager-1",
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: "netguard.sgroups.io/v1beta1",
				Time:       &now,
			},
			{
				Manager:    "manager-2",
				Operation:  metav1.ManagedFieldsOperationUpdate,
				APIVersion: "netguard.sgroups.io/v1beta1",
				Time:       &now,
			},
			{
				Manager:    "manager-1",
				Operation:  metav1.ManagedFieldsOperationUpdate,
				APIVersion: "netguard.sgroups.io/v1beta1",
				Time:       &now,
			},
		}

		for _, entry := range entries {
			meta.AddManagedField(entry)
		}

		assert.Len(t, meta.ManagedFields, 3)

		// Remove manager-1 entries
		meta.RemoveManagedFieldsByManager("manager-1")

		assert.Len(t, meta.ManagedFields, 1)
		assert.Equal(t, "manager-2", meta.ManagedFields[0].Manager)
	})

	t.Run("GetManagedFields", func(t *testing.T) {
		meta := &models.Meta{}
		assert.Nil(t, meta.GetManagedFields())

		now := metav1.NewTime(time.Now())
		entry := metav1.ManagedFieldsEntry{
			Manager:    "test-manager",
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: "netguard.sgroups.io/v1beta1",
			Time:       &now,
		}
		meta.AddManagedField(entry)

		managedFields := meta.GetManagedFields()
		assert.Len(t, managedFields, 1)
		assert.Equal(t, "test-manager", managedFields[0].Manager)
	})

	t.Run("SetManagedFields", func(t *testing.T) {
		meta := &models.Meta{}
		now := metav1.NewTime(time.Now())

		managedFields := []metav1.ManagedFieldsEntry{
			{
				Manager:    "set-manager-1",
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: "netguard.sgroups.io/v1beta1",
				Time:       &now,
			},
			{
				Manager:    "set-manager-2",
				Operation:  metav1.ManagedFieldsOperationUpdate,
				APIVersion: "netguard.sgroups.io/v1beta1",
				Time:       &now,
			},
		}

		meta.SetManagedFields(managedFields)

		assert.Len(t, meta.ManagedFields, 2)
		assert.Equal(t, "set-manager-1", meta.ManagedFields[0].Manager)
		assert.Equal(t, "set-manager-2", meta.ManagedFields[1].Manager)
	})
}
