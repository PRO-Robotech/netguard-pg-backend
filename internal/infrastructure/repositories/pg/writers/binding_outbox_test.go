package writers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestCreateHostBindingOutboxEntry_AffectsHostOnly(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupHostTestDB(t, pool)

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	writer := NewWriter(&mockRegistry{pool: pool}, tx, ctx)

	binding := &models.HostBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-ns",
				Name:      "binding-host",
			},
		},
		HostRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Host",
				Name:       "demo-host",
			},
			Namespace: "test-ns",
		},
		AddressGroupRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "demo-ag",
			},
			Namespace: "test-ns",
		},
		Meta: models.Meta{UID: uuid.New().String()},
	}

	err = writer.createHostBindingOutboxEntry(ctx, binding)
	require.NoError(t, err)

	var affectsJSON []byte
	query := `SELECT affects_resources FROM sync_outbox
              WHERE resource_type = 'HostBinding'
                AND resource_namespace = $1
                AND resource_name = $2
              ORDER BY created_at DESC
              LIMIT 1`
	err = tx.QueryRow(ctx, query, binding.Namespace, binding.Name).Scan(&affectsJSON)
	require.NoError(t, err)

	var affects []map[string]string
	require.NoError(t, json.Unmarshal(affectsJSON, &affects))
	require.Len(t, affects, 1)

	assert.Equal(t, "Host", affects[0]["type"])
	assert.Equal(t, binding.HostRef.Namespace, affects[0]["namespace"])
	assert.Equal(t, binding.HostRef.Name, affects[0]["name"])
}

func TestCreateNetworkBindingOutboxEntry_AffectsNetworkOnly(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	writer := NewWriter(&mockRegistry{pool: pool}, tx, ctx)

	binding := &models.NetworkBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-ns",
				Name:      "binding-net",
			},
		},
		NetworkRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Network",
				Name:       "demo-net",
			},
			Namespace: "test-ns",
		},
		AddressGroupRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "demo-ag",
			},
			Namespace: "test-ns",
		},
		Meta: models.Meta{UID: uuid.New().String()},
	}

	err = writer.createNetworkBindingOutboxEntry(ctx, binding)
	require.NoError(t, err)

	var affectsJSON []byte
	query := `SELECT affects_resources FROM sync_outbox
              WHERE resource_type = 'NetworkBinding'
                AND resource_namespace = $1
                AND resource_name = $2
              ORDER BY created_at DESC
              LIMIT 1`
	err = tx.QueryRow(ctx, query, binding.Namespace, binding.Name).Scan(&affectsJSON)
	require.NoError(t, err)

	var affects []map[string]string
	require.NoError(t, json.Unmarshal(affectsJSON, &affects))
	require.Len(t, affects, 1)

	assert.Equal(t, "Network", affects[0]["type"])
	assert.Equal(t, binding.NetworkRef.Namespace, affects[0]["namespace"])
	assert.Equal(t, binding.NetworkRef.Name, affects[0]["name"])
}
