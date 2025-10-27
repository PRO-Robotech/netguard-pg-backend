package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/writers"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// TCCreateAddressGroupViaRepositoryWithConnStr creates AddressGroup using Repository Writer
// (version that takes explicit connection string)
func TCCreateAddressGroupViaRepositoryWithConnStr(t *testing.T, connStr, namespace, name string) models.AddressGroup {
	t.Helper()
	ctx := context.Background()

	// Create pgxpool
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to create pgxpool")
	defer pool.Close()

	// Create transaction
	tx, err := pool.Begin(ctx)
	require.NoError(t, err, "failed to begin transaction")
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Logf("Transaction rollback (expected if committed): %v", err)
		}
	}()

	// Create Writer with MockRegistry
	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	// Create AddressGroup model
	ag := models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: namespace,
				Name:      name,
			},
		},
		DefaultAction: models.ActionDrop,
		Logs:          false,
		Trace:         false,
		Networks:      []models.NetworkItem{},
		Hosts:         []netguardv1beta1.NamespacedObjectReference{},
	}

	// Initialize Meta.UID (required by Writer)
	ag.Meta.TouchOnCreate()

	// Sync via Repository (this triggers outbox entry creation!)
	err = writer.SyncAddressGroups(ctx, []models.AddressGroup{ag}, ports.EmptyScope{}, nil)
	require.NoError(t, err, "failed to sync address group via repository")

	// Commit transaction
	err = tx.Commit(ctx)
	require.NoError(t, err, "failed to commit transaction")

	t.Logf("📝 Created AddressGroup via Repository: %s/%s (triggers fired!)", namespace, name)

	return ag
}

// TCDeleteAddressGroupViaRepositoryWithConnStr deletes AddressGroup using Repository Writer
func TCDeleteAddressGroupViaRepositoryWithConnStr(t *testing.T, connStr, namespace, name string) {
	t.Helper()
	ctx := context.Background()

	// Create pgxpool
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to create pgxpool")
	defer pool.Close()

	// Create transaction
	tx, err := pool.Begin(ctx)
	require.NoError(t, err, "failed to begin transaction")
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Logf("Transaction rollback (expected if committed): %v", err)
		}
	}()

	// Create Writer with MockRegistry
	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	// Delete via Repository
	ids := []models.ResourceIdentifier{
		{Namespace: namespace, Name: name},
	}
	err = writer.DeleteAddressGroupsByIDs(ctx, ids)
	require.NoError(t, err, "failed to delete address group via repository")

	// Commit
	err = tx.Commit(ctx)
	require.NoError(t, err, "failed to commit transaction")

	t.Logf("🗑️  Deleted AddressGroup via Repository: %s/%s", namespace, name)
}

// ==================== Host Repository Helpers ====================

// TCCreateHostViaRepositoryWithConnStr creates Host using Repository Writer
func TCCreateHostViaRepositoryWithConnStr(t *testing.T, connStr, namespace, name, ipCIDR string) models.Host {
	t.Helper()
	ctx := context.Background()

	// Create pgxpool
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to create pgxpool")
	defer pool.Close()

	// Create transaction
	tx, err := pool.Begin(ctx)
	require.NoError(t, err, "failed to begin transaction")
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Logf("Transaction rollback (expected if committed): %v", err)
		}
	}()

	// Create Writer with MockRegistry
	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	// Create Host model
	host := models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: namespace,
				Name:      name,
			},
		},
		UUID: uuid.New().String(), // Generate unique UUID
		IpList: []models.IPItem{
			{IP: ipCIDR},
		},
	}

	// Initialize Meta.UID (required by Writer)
	host.Meta.TouchOnCreate()

	// Sync via Repository (this triggers outbox entry creation!)
	err = writer.SyncHosts(ctx, []models.Host{host}, ports.EmptyScope{}, nil)
	require.NoError(t, err, "failed to sync host via repository")

	// Commit transaction
	err = tx.Commit(ctx)
	require.NoError(t, err, "failed to commit transaction")

	t.Logf("📝 Created Host via Repository: %s/%s (triggers fired!)", namespace, name)

	return host
}

// TCDeleteHostViaRepositoryWithConnStr deletes Host using Repository Writer
func TCDeleteHostViaRepositoryWithConnStr(t *testing.T, connStr, namespace, name string) {
	t.Helper()
	ctx := context.Background()

	// Create pgxpool
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to create pgxpool")
	defer pool.Close()

	// Create transaction
	tx, err := pool.Begin(ctx)
	require.NoError(t, err, "failed to begin transaction")
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Logf("Transaction rollback (expected if committed): %v", err)
		}
	}()

	// Create Writer with MockRegistry
	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	// Delete via Repository
	ids := []models.ResourceIdentifier{
		{Namespace: namespace, Name: name},
	}
	err = writer.DeleteHostsByIDs(ctx, ids)
	require.NoError(t, err, "failed to delete host via repository")

	// Commit
	err = tx.Commit(ctx)
	require.NoError(t, err, "failed to commit transaction")

	t.Logf("🗑️  Deleted Host via Repository: %s/%s", namespace, name)
}

// ==================== Network Repository Helpers ====================

// TCCreateNetworkViaRepositoryWithConnStr creates Network using Repository Writer
func TCCreateNetworkViaRepositoryWithConnStr(t *testing.T, connStr, namespace, name, cidr string) models.Network {
	t.Helper()
	ctx := context.Background()

	// Create pgxpool
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to create pgxpool")
	defer pool.Close()

	// Create transaction
	tx, err := pool.Begin(ctx)
	require.NoError(t, err, "failed to begin transaction")
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Logf("Transaction rollback (expected if committed): %v", err)
		}
	}()

	// Create Writer with MockRegistry
	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	// Create Network model
	network := models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: namespace,
				Name:      name,
			},
		},
		CIDR: cidr,
	}

	// Initialize Meta.UID (required by Writer)
	network.Meta.TouchOnCreate()

	// Sync via Repository (this triggers outbox entry creation!)
	err = writer.SyncNetworks(ctx, []models.Network{network}, ports.EmptyScope{}, nil)
	require.NoError(t, err, "failed to sync network via repository")

	// Commit transaction
	err = tx.Commit(ctx)
	require.NoError(t, err, "failed to commit transaction")

	t.Logf("📝 Created Network via Repository: %s/%s (triggers fired!)", namespace, name)

	return network
}

// TCDeleteNetworkViaRepositoryWithConnStr deletes Network using Repository Writer
func TCDeleteNetworkViaRepositoryWithConnStr(t *testing.T, connStr, namespace, name string) {
	t.Helper()
	ctx := context.Background()

	// Create pgxpool
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to create pgxpool")
	defer pool.Close()

	// Create transaction
	tx, err := pool.Begin(ctx)
	require.NoError(t, err, "failed to begin transaction")
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Logf("Transaction rollback (expected if committed): %v", err)
		}
	}()

	// Create Writer with MockRegistry
	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	// Delete via Repository
	ids := []models.ResourceIdentifier{
		{Namespace: namespace, Name: name},
	}
	err = writer.DeleteNetworksByIDs(ctx, ids)
	require.NoError(t, err, "failed to delete network via repository")

	// Commit
	err = tx.Commit(ctx)
	require.NoError(t, err, "failed to commit transaction")

	t.Logf("🗑️  Deleted Network via Repository: %s/%s", namespace, name)
}
