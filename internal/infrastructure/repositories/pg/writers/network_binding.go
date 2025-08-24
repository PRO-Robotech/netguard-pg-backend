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

// SyncNetworkBindings syncs network bindings to PostgreSQL with K8s metadata support
func (w *Writer) SyncNetworkBindings(ctx context.Context, networkBindings []models.NetworkBinding, scope ports.Scope, options ...ports.Option) error {
	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteNetworkBindingsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete network bindings in scope")
		}
	}

	// Upsert all provided network bindings
	for i := range networkBindings {
		// 🔧 CRITICAL FIX: Initialize metadata fields (UID, Generation, ObservedGeneration)
		// This is what Memory backend does via ensureMetaFill() -> TouchOnCreate()
		// Without this, PATCH operations fail because objInfo.UpdatedObject() needs UID
		// IMPORTANT: Use index-based loop to modify original, not copy!
		if networkBindings[i].Meta.UID == "" {
			networkBindings[i].Meta.TouchOnCreate()
		}

		if err := w.upsertNetworkBinding(ctx, networkBindings[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert network binding %s/%s", networkBindings[i].Namespace, networkBindings[i].Name)
		}
	}

	return nil
}

// upsertNetworkBinding inserts or updates a network binding with full K8s metadata support
func (w *Writer) upsertNetworkBinding(ctx context.Context, binding models.NetworkBinding) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(binding.Meta.Labels, binding.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(binding.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if network binding exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM network_bindings WHERE namespace = $1 AND name = $2`
	_ = w.tx.QueryRow(ctx, existingQuery, binding.Namespace, binding.Name).Scan(&existingResourceVersion)

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
			return errors.Wrapf(err, "failed to update K8s metadata for network binding %s/%s", binding.Namespace, binding.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, '{}', $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for network binding %s/%s", binding.Namespace, binding.Name)
		}
	}

	// Then, upsert the network binding using the resource version
	bindingQuery := `
		INSERT INTO network_bindings (namespace, name, network_namespace, network_name, address_group_namespace, address_group_name, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (namespace, name) DO UPDATE SET
			network_namespace = $3,
			network_name = $4,
			address_group_namespace = $5,
			address_group_name = $6,
			resource_version = $7`

	if err := w.exec(ctx, bindingQuery,
		binding.Namespace,
		binding.Name,
		binding.Namespace, // NetworkRef is in same namespace as binding
		binding.NetworkRef.Name,
		binding.Namespace, // AddressGroupRef is in same namespace as binding
		binding.AddressGroupRef.Name,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert network binding %s/%s", binding.Namespace, binding.Name)
	}

	return nil
}

// deleteNetworkBindingsInScope deletes network bindings that match the provided scope
func (w *Writer) deleteNetworkBindingsInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "nb")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM network_bindings nb WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete network bindings in scope")
	}

	return nil
}

// DeleteNetworkBindingsByIDs deletes network bindings by their identifiers
func (w *Writer) DeleteNetworkBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	if len(ids) == 0 {
		return nil
	}

	// Build parameter placeholders and collect args
	var conditions []string
	var args []interface{}
	argIndex := 1

	for _, id := range ids {
		conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	query := fmt.Sprintf(`
		DELETE FROM network_bindings WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete network bindings by identifiers")
	}

	return nil
}
