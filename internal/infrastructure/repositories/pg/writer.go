package pg

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/writers"

	atm "github.com/H-BF/corlib/pkg/atomic"
)

// writer implements the PostgreSQL writer with sgroups-style atomic transaction management
type writer struct {
	registry      *Registry
	tx            pgx.Tx
	ctx           context.Context
	modularWriter *writers.Writer    // Delegate to modular writer
	affectedRows  *int64             // Track affected rows (sgroups pattern)
	txHolder      *atm.Value[pgx.Tx] // Atomic transaction holder (sgroups pattern)
}

// Close closes the writer
func (w *writer) Close() error {
	return nil // Transaction lifecycle managed by Commit/Abort
}

// Commit commits the transaction with sgroups-style affected rows tracking
func (w *writer) Commit() error {
	if w.txHolder == nil {
		return errors.New("writer closed")
	}

	var err error = errors.New("writer closed")

	w.txHolder.Clear(func(tx pgx.Tx) {
		// Track affected rows like sgroups
		if n := atomic.AddInt64(w.affectedRows, 0); n > 0 {
			// TODO: Add sync status update when implemented
			// For now, just commit the transaction
		}

		if err = tx.Commit(w.ctx); err != nil {
			_ = tx.Rollback(w.ctx)
		}
	})

	return err
}

// Abort aborts the transaction with sgroups-style cleanup
func (w *writer) Abort() {
	if w.txHolder != nil {
		w.txHolder.Clear(func(tx pgx.Tx) {
			_ = tx.Rollback(w.ctx)
		})
	}
}

// GetTx returns the underlying transaction (used by ReaderFromWriter)
func (w *writer) GetTx() pgx.Tx {
	return w.tx
}

// Implemented resource methods - delegated to modular writers

// Service methods - delegated to writers/service.go
func (w *writer) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncServices(ctx, services, scope, opts...)
}

func (w *writer) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncServiceAliases(ctx, aliases, scope, opts...)
}

func (w *writer) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteServicesByIDs(ctx, ids, opts...)
}

func (w *writer) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteServiceAliasesByIDs(ctx, ids, opts...)
}

// AddressGroup methods - delegated to writers/address_group.go
func (w *writer) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncAddressGroups(ctx, addressGroups, scope, opts...)
}

func (w *writer) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncAddressGroupBindings(ctx, bindings, scope, opts...)
}

func (w *writer) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncAddressGroupPortMappings(ctx, mappings, scope, opts...)
}

func (w *writer) SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncAddressGroupBindingPolicies(ctx, policies, scope, opts...)
}

func (w *writer) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteAddressGroupsByIDs(ctx, ids, opts...)
}

func (w *writer) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteAddressGroupBindingsByIDs(ctx, ids, opts...)
}

func (w *writer) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteAddressGroupPortMappingsByIDs(ctx, ids, opts...)
}

func (w *writer) DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteAddressGroupBindingPoliciesByIDs(ctx, ids, opts...)
}

// Placeholder methods for not-yet-implemented resources

func (w *writer) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncRuleS2S(ctx, rules, scope, opts...)
}

func (w *writer) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteRuleS2SByIDs(ctx, ids)
}

func (w *writer) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncIEAgAgRules(ctx, rules, scope, opts...)
}

func (w *writer) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteIEAgAgRulesByIDs(ctx, ids)
}

func (w *writer) SyncNetworks(ctx context.Context, networks []models.Network, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncNetworks(ctx, networks, scope, opts...)
}

func (w *writer) SyncNetworkBindings(ctx context.Context, bindings []models.NetworkBinding, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncNetworkBindings(ctx, bindings, scope, opts...)
}

func (w *writer) DeleteNetworksByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteNetworksByIDs(ctx, ids)
}

func (w *writer) DeleteNetworkBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteNetworkBindingsByIDs(ctx, ids)
}

func (w *writer) SyncHosts(ctx context.Context, hosts []models.Host, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncHosts(ctx, hosts, scope, opts...)
}

func (w *writer) DeleteHostsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteHostsByIDs(ctx, ids)
}

func (w *writer) SyncHostBindings(ctx context.Context, hostBindings []models.HostBinding, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncHostBindings(ctx, hostBindings, scope, opts...)
}

func (w *writer) DeleteHostBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteHostBindingsByIDs(ctx, ids)
}

// MarkAsDeleting sets deletion_timestamp for a resource in k8s_metadata
// This prevents the resource from being re-created by backend ListWatch during deletion
func (w *writer) MarkAsDeleting(resourceVersion string) error {
	query := `
		UPDATE k8s_metadata
		SET deletion_timestamp = NOW()
		WHERE resource_version = $1 AND deletion_timestamp IS NULL`

	_, err := w.tx.Exec(w.ctx, query, resourceVersion)
	return err
}

// MarkAsDeletingByName sets deletion_timestamp for a resource by namespace/name
// This is called when Kubernetes sets DeletionTimestamp on an object
func (w *writer) MarkAsDeletingByName(namespace, name, kind string) error {
	query := `
		UPDATE k8s_metadata
		SET deletion_timestamp = NOW()
		WHERE namespace = $1 AND name = $2 AND kind = $3 AND deletion_timestamp IS NULL`

	result, err := w.tx.Exec(w.ctx, query, namespace, name, kind)
	if err != nil {
		return err
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected > 0 {
		klog.InfoS("✅ Marked resource as deleting by name",
			"namespace", namespace,
			"name", name,
			"kind", kind,
			"rowsAffected", rowsAffected)
	}

	return nil
}

// MarkAsDeletingBatch sets deletion_timestamp for multiple resources by resourceVersion
// This is used by DeleteCollection to mark all resources before deletion
func (w *writer) MarkAsDeletingBatch(resourceVersions []string) error {
	if len(resourceVersions) == 0 {
		return nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(resourceVersions))
	args := make([]interface{}, len(resourceVersions))
	for i, rv := range resourceVersions {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = rv
	}

	query := fmt.Sprintf(`
		UPDATE k8s_metadata
		SET deletion_timestamp = NOW()
		WHERE resource_version IN (%s) AND deletion_timestamp IS NULL`,
		strings.Join(placeholders, ", "))

	result, err := w.tx.Exec(w.ctx, query, args...)
	if err != nil {
		return err
	}

	rowsAffected := result.RowsAffected()
	klog.InfoS("✅ Marked resources as deleting in batch",
		"count", len(resourceVersions),
		"rowsAffected", rowsAffected)

	return nil
}
