package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// SyncHostBindings syncs host bindings to PostgreSQL with K8s metadata support
func (w *Writer) SyncHostBindings(ctx context.Context, hostBindings []models.HostBinding, scope ports.Scope, options ...ports.Option) error {
	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteHostBindingsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete host bindings in scope")
		}
	}

	// Upsert each host binding
	for _, hostBinding := range hostBindings {
		if err := w.upsertHostBinding(ctx, hostBinding); err != nil {
			// Check for unique constraint violation (one binding per host)
			if isUniqueViolation(err, "host_bindings_host_namespace_host_name_key") {
				return errors.Errorf("host %s/%s is already bound to another address group", hostBinding.HostRef.Namespace, hostBinding.HostRef.Name)
			}
			return errors.Wrapf(err, "failed to upsert host binding %s/%s", hostBinding.Namespace, hostBinding.Name)
		}
	}

	return nil
}

// upsertHostBinding inserts or updates a host binding with K8s metadata
func (w *Writer) upsertHostBinding(ctx context.Context, hostBinding models.HostBinding) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(hostBinding.Meta.Labels, hostBinding.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(hostBinding.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if host binding exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM host_bindings WHERE namespace = $1 AND name = $2`
	_ = w.tx.QueryRow(ctx, existingQuery, hostBinding.Namespace, hostBinding.Name).Scan(&existingResourceVersion)

	var resourceVersion int64
	if existingResourceVersion.Valid {
		// UPDATE existing K8s metadata
		metadataQuery := `
			UPDATE k8s_metadata 
			SET labels = $1, annotations = $2, conditions = $3, updated_at = NOW()
			WHERE resource_version = $4
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, existingResourceVersion.Int64).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to update K8s metadata for host binding %s/%s", hostBinding.Namespace, hostBinding.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, conditions)
			VALUES ($1, $2, $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to insert K8s metadata for host binding %s/%s", hostBinding.Namespace, hostBinding.Name)
		}
	}

	// UPSERT host binding record
	hostBindingQuery := `
		INSERT INTO host_bindings (
			namespace, name,
			host_namespace, host_name,
			address_group_namespace, address_group_name,
			resource_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (namespace, name) DO UPDATE SET
			host_namespace = EXCLUDED.host_namespace,
			host_name = EXCLUDED.host_name,
			address_group_namespace = EXCLUDED.address_group_namespace,
			address_group_name = EXCLUDED.address_group_name,
			resource_version = EXCLUDED.resource_version`

	_, err = w.tx.Exec(ctx, hostBindingQuery,
		hostBinding.Namespace, hostBinding.Name,
		hostBinding.HostRef.Namespace, hostBinding.HostRef.Name,
		hostBinding.AddressGroupRef.Namespace, hostBinding.AddressGroupRef.Name,
		resourceVersion,
	)
	if err != nil {
		return errors.Wrapf(err, "failed to upsert host binding %s/%s", hostBinding.Namespace, hostBinding.Name)
	}

	return nil
}

// deleteHostBindingsInScope deletes all host bindings within the given scope
func (w *Writer) deleteHostBindingsInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	switch s := scope.(type) {
	case ports.ResourceIdentifierScope:
		if s.IsEmpty() {
			return nil
		}

		// Build IN clause for (namespace, name) pairs
		var values []string
		var args []interface{}
		argIndex := 1

		for _, id := range s.Identifiers {
			values = append(values, fmt.Sprintf("($%d, $%d)", argIndex, argIndex+1))
			args = append(args, id.Namespace, id.Name)
			argIndex += 2
		}

		query := fmt.Sprintf(`DELETE FROM host_bindings WHERE (namespace, name) IN (%s)`, strings.Join(values, ","))
		_, err := w.tx.Exec(ctx, query, args...)
		return errors.Wrap(err, "failed to delete host bindings by resource identifiers")

	default:
		return errors.Errorf("unsupported scope type for host bindings deletion: %T", scope)
	}
}

// DeleteHostBindingsByIDs deletes host bindings by their resource identifiers
func (w *Writer) DeleteHostBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, options ...ports.Option) error {
	if len(ids) == 0 {
		return nil
	}

	// Build IN clause for (namespace, name) pairs
	var values []string
	var args []interface{}
	argIndex := 1

	for _, id := range ids {
		values = append(values, fmt.Sprintf("($%d, $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	query := fmt.Sprintf(`DELETE FROM host_bindings WHERE (namespace, name) IN (%s)`, strings.Join(values, ","))
	_, err := w.tx.Exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete host bindings by IDs")
	}

	return nil
}
