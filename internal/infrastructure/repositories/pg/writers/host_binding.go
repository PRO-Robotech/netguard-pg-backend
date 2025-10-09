package writers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// SyncHostBindings syncs host bindings to PostgreSQL with K8s metadata support
func (w *Writer) SyncHostBindings(ctx context.Context, hostBindings []models.HostBinding, scope ports.Scope, options ...ports.Option) error {
	// Extract sync operation from options
	syncOp := models.SyncOpUpsert // Default operation
	for _, opt := range options {
		if syncOption, ok := opt.(ports.SyncOption); ok {
			syncOp = syncOption.Operation
			break
		}
	}

	// Handle scoped sync - delete existing resources in scope first (for non-DELETE operations)
	if !scope.IsEmpty() && syncOp != models.SyncOpDelete {
		if err := w.deleteHostBindingsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete host bindings in scope")
		}
	}

	// Handle operations based on sync operation
	switch syncOp {
	case models.SyncOpDelete:
		// For DELETE operations, delete the specific bindings
		var identifiers []models.ResourceIdentifier
		for _, binding := range hostBindings {
			identifiers = append(identifiers, models.ResourceIdentifier{
				Namespace: binding.Namespace,
				Name:      binding.Name,
			})
		}
		if err := w.DeleteHostBindingsByIDs(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete host bindings")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		// For UPSERT/FULLSYNC operations, upsert all provided bindings
		for i := range hostBindings {
			if err := w.upsertHostBinding(ctx, &hostBindings[i]); err != nil {
				// Check for unique constraint violation (one binding per host)
				if isUniqueViolation(err, "host_bindings_host_namespace_host_name_key") {
					return errors.Errorf("host %s/%s is already bound to another address group", hostBindings[i].HostRef.Namespace, hostBindings[i].HostRef.Name)
				}
				return errors.Wrapf(err, "failed to upsert host binding %s/%s", hostBindings[i].Namespace, hostBindings[i].Name)
			}
		}
	default:
		return errors.Errorf("unsupported sync operation: %v", syncOp)
	}

	return nil
}

// upsertHostBinding inserts or updates a host binding with K8s metadata
func (w *Writer) upsertHostBinding(ctx context.Context, hostBinding *models.HostBinding) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(hostBinding.Meta.Labels, hostBinding.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(hostBinding.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// ALWAYS INSERT new K8s metadata (to get new ResourceVersion)
	// PostgreSQL BIGSERIAL primary key only increments on INSERT, not UPDATE
	var resourceVersion int64
	metadataQuery := `
		INSERT INTO k8s_metadata (labels, annotations, conditions)
		VALUES ($1, $2, $3)
		RETURNING resource_version`
	err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to insert K8s metadata for host binding %s/%s", hostBinding.Namespace, hostBinding.Name)
	}

	// Update domain model with new ResourceVersion from DB
	hostBinding.Meta.TouchOnWrite(strconv.FormatInt(resourceVersion, 10))

	// UPSERT host binding record with NEW resource version
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

	// First, mark objects as being deleted in k8s_metadata to prevent re-creation by ListWatch
	markDeleteQuery := `
		UPDATE k8s_metadata m
		SET deletion_timestamp = NOW()
		FROM host_bindings hb
		WHERE hb.resource_version = m.resource_version
		  AND (hb.namespace, hb.name) IN (%s)
		  AND m.deletion_timestamp IS NULL`

	markQuery := fmt.Sprintf(markDeleteQuery, strings.Join(values, ","))
	_, err := w.tx.Exec(ctx, markQuery, args...)
	if err != nil {
		// Log but don't fail - deletion_timestamp is optional for now
		klog.V(4).InfoS("Failed to mark host bindings as deleting in k8s_metadata", "error", err.Error())
	}

	// Then delete from host_bindings table
	query := fmt.Sprintf(`DELETE FROM host_bindings WHERE (namespace, name) IN (%s)`, strings.Join(values, ","))
	_, err = w.tx.Exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete host bindings by IDs")
	}

	return nil
}
