package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// CIDRAlreadyExistsError represents a CIDR uniqueness violation error
type CIDRAlreadyExistsError struct {
	CIDR        string
	NetworkName string
	Err         error
}

func (e *CIDRAlreadyExistsError) Error() string {
	return fmt.Sprintf("CIDR '%s' already exists (attempted to create/update network %s)", e.CIDR, e.NetworkName)
}

func (e *CIDRAlreadyExistsError) Unwrap() error {
	return e.Err
}

// isUniqueViolation checks if the error is a PostgreSQL unique constraint violation
// for the specified constraint name
func isUniqueViolation(err error, constraintName string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// PostgreSQL unique_violation error code is "23505"
		if pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, constraintName) {
			return true
		}
	}
	return false
}

// SyncNetworks syncs networks to PostgreSQL with K8s metadata support
func (w *Writer) SyncNetworks(ctx context.Context, networks []models.Network, scope ports.Scope, options ...ports.Option) error {
	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteNetworksInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete networks in scope")
		}
	}

	// Upsert all provided networks
	for i := range networks {
		// 🔧 CRITICAL FIX: Initialize metadata fields (UID, Generation, ObservedGeneration)
		// This is what Memory backend does via ensureMetaFill() -> TouchOnCreate()
		// Without this, PATCH operations fail because objInfo.UpdatedObject() needs UID
		// IMPORTANT: Use index-based loop to modify original, not copy!
		if networks[i].Meta.UID == "" {
			networks[i].Meta.TouchOnCreate()
		}

		if err := w.upsertNetwork(ctx, networks[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert network %s/%s", networks[i].Namespace, networks[i].Name)
		}
	}

	return nil
}

// upsertNetwork inserts or updates a network with full K8s metadata support
func (w *Writer) upsertNetwork(ctx context.Context, network models.Network) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(network.Meta.Labels, network.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(network.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if network exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM networks WHERE namespace = $1 AND name = $2`
	_ = w.tx.QueryRow(ctx, existingQuery, network.Namespace, network.Name).Scan(&existingResourceVersion)

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
			return errors.Wrapf(err, "failed to update K8s metadata for network %s/%s", network.Namespace, network.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, '{}', $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for network %s/%s", network.Namespace, network.Name)
		}
	}

	// Create network items with single CIDR entry
	networkItems := []map[string]interface{}{
		{"cidr": network.CIDR, "name": network.NetworkName},
	}
	networkItemsJSON, err := json.Marshal(networkItems)
	if err != nil {
		return errors.Wrap(err, "failed to marshal network_items")
	}

	// Extract reference fields from ObjectReference (references are in same namespace)
	var bindingRefNamespace, bindingRefName, agRefNamespace, agRefName interface{}
	if network.BindingRef != nil {
		bindingRefNamespace = network.Namespace // References are in same namespace
		bindingRefName = network.BindingRef.Name
	}
	if network.AddressGroupRef != nil {
		agRefNamespace = network.Namespace // References are in same namespace
		agRefName = network.AddressGroupRef.Name
	}

	// Then, upsert the network using the resource version (including new cidr column)
	networkQuery := `
		INSERT INTO networks (namespace, name, cidr, network_items, is_bound, 
			binding_ref_namespace, binding_ref_name, 
			address_group_ref_namespace, address_group_ref_name, 
			resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (namespace, name) DO UPDATE SET
			cidr = $3,
			network_items = $4,
			is_bound = $5,
			binding_ref_namespace = $6,
			binding_ref_name = $7,
			address_group_ref_namespace = $8,
			address_group_ref_name = $9,
			resource_version = $10`

	if err := w.exec(ctx, networkQuery,
		network.Namespace,
		network.Name,
		network.CIDR, // Add CIDR as separate column
		networkItemsJSON,
		network.IsBound,
		bindingRefNamespace,
		bindingRefName,
		agRefNamespace,
		agRefName,
		resourceVersion,
	); err != nil {
		// Check if this is a UNIQUE constraint violation on CIDR
		if isUniqueViolation(err, "idx_networks_cidr_unique") {
			return &CIDRAlreadyExistsError{
				CIDR:        network.CIDR,
				NetworkName: network.Key(),
				Err:         err,
			}
		}
		return errors.Wrapf(err, "failed to upsert network %s/%s", network.Namespace, network.Name)
	}

	return nil
}

// deleteNetworksInScope deletes networks that match the provided scope
func (w *Writer) deleteNetworksInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "n")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM networks n WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete networks in scope")
	}

	return nil
}

// DeleteNetworksByIDs deletes networks by their identifiers
func (w *Writer) DeleteNetworksByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
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
		DELETE FROM networks WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete networks by identifiers")
	}

	return nil
}
