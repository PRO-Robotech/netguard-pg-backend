package writers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
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

func isExclusionViolation(err error, constraintName string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23P01" && strings.Contains(pgErr.ConstraintName, constraintName)
	}
	return false
}

// SyncNetworks syncs networks to PostgreSQL with K8s metadata support
func (w *Writer) SyncNetworks(ctx context.Context, networks []models.Network, scope ports.Scope, options ...ports.Option) error {
	isConditionOnly := false
	for _, opt := range options {
		if _, ok := opt.(ports.ConditionOnlyOperation); ok {
			isConditionOnly = true
			break
		}
	}

	if isConditionOnly {
		// Directly update conditions WITHOUT creating outbox entries
		for i := range networks {
			if err := w.updateNetworkConditionsOnly(ctx, &networks[i]); err != nil {
				return errors.Wrapf(err, "failed to update conditions for network %s/%s", networks[i].Namespace, networks[i].Name)
			}
		}
		return nil
	}

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
		if err := w.deleteNetworksInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete networks in scope")
		}
	}

	// Handle operations based on sync operation
	switch syncOp {
	case models.SyncOpDelete:
		// For DELETE operations, delete the specific networks
		var identifiers []models.ResourceIdentifier
		for _, network := range networks {
			identifiers = append(identifiers, models.ResourceIdentifier{
				Namespace: network.Namespace,
				Name:      network.Name,
			})
		}
		if err := w.DeleteNetworksByIDs(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete networks")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		// For UPSERT/FULLSYNC operations, upsert all provided networks
		for i := range networks {
			// Initialize metadata fields if not set
			if networks[i].Meta.UID == "" {
				networks[i].Meta.TouchOnCreate()
			}

			if err := w.upsertNetwork(ctx, &networks[i]); err != nil {
				return errors.Wrapf(err, "failed to upsert network %s/%s", networks[i].Namespace, networks[i].Name)
			}
		}
	}

	return nil
}

// upsertNetwork inserts or updates a network with full K8s metadata support and creates Outbox entry
func (w *Writer) upsertNetwork(ctx context.Context, network *models.Network) error {
	// Check if network exists to determine if this is INSERT or UPDATE
	var existingResourceVersion int64
	checkQuery := `SELECT resource_version FROM networks WHERE namespace = $1 AND name = $2`
	err := w.tx.QueryRow(ctx, checkQuery, network.Namespace, network.Name).Scan(&existingResourceVersion)
	isNewResource := (err != nil) // If query fails, resource doesn't exist

	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(network.Meta.Labels, network.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	// Ready will be set to True by Worker after successful SGROUP sync
	// Business Rule: Resources should NOT have Ready=True until synced to SGROUP
	conditions := network.Meta.Conditions
	if isNewResource {
		// This is a new resource - force Pending status
		conditions = forcePendingSyncCondition(conditions)
		klog.V(4).InfoS("Forcing PendingSGROUPSync status for new Network",
			"namespace", network.Namespace, "name", network.Name)
	}

	conditionsJSON, err := json.Marshal(conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	var resourceVersion int64
	metadataQuery := `
		INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
		VALUES ($1, $2, '{}', $3)
		RETURNING resource_version`
	err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to insert K8s metadata for network %s/%s", network.Namespace, network.Name)
	}

	// Update domain model with new ResourceVersion from DB
	network.Meta.TouchOnWrite(strconv.FormatInt(resourceVersion, 10))

	// Create network items with single CIDR entry
	networkItems := []map[string]interface{}{
		{"cidr": network.CIDR, "name": network.NetworkName},
	}
	networkItemsJSON, err := json.Marshal(networkItems)
	if err != nil {
		return errors.Wrap(err, "failed to marshal network_items")
	}

	// Extract reference fields from NamespacedObjectReference
	var bindingRefNamespace, bindingRefName, agRefNamespace, agRefName interface{}
	if network.BindingRef != nil {
		bindingRefNamespace = network.BindingRef.Namespace
		bindingRefName = network.BindingRef.Name
	}
	if network.AddressGroupRef != nil {
		agRefNamespace = network.AddressGroupRef.Namespace
		agRefName = network.AddressGroupRef.Name
	}

	// Then, upsert the network using the NEW resource version
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
		if isUniqueViolation(err, "idx_networks_cidr_unique") {
			return &CIDRAlreadyExistsError{
				CIDR:        network.CIDR,
				NetworkName: network.Key(),
				Err:         err,
			}
		}

		if isExclusionViolation(err, "prevent_networks_cidr_overlap") {
			return &ports.CIDROverlapError{
				CIDR: network.CIDR,
				Err:  errors.New("CIDR overlaps with existing network"),
			}
		}

		return errors.Wrapf(err, "failed to upsert network %s/%s", network.Namespace, network.Name)
	}

	// Outbox entries for networks are created by trigger 'trg_network_upsert_outbox'
	// using the Kubernetes UID stored in k8s_metadata.

	return nil
}

// DELETE outbox entries are created by BEFORE DELETE trigger 'trigger_network_before_delete'
// (migration 026), so additional helpers are unnecessary.

// deleteNetworksInScope deletes networks that match the provided scope
// Previous implementation used direct DELETE which bypassed Migration 032 trigger,
// causing Networks to be deleted before Worker could process them.
func (w *Writer) deleteNetworksInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "n")
	if whereClause == "" {
		return nil
	}

	// STEP 1: Fetch Networks in scope to get their identifiers
	// We need namespace+name to call DeleteNetworksByIDs()
	fetchQuery := fmt.Sprintf(`
		SELECT namespace, name
		FROM networks n
		WHERE %s`, whereClause)

	rows, err := w.tx.Query(ctx, fetchQuery, args...)
	if err != nil {
		return errors.Wrap(err, "failed to fetch networks in scope")
	}
	defer rows.Close()

	var identifiers []models.ResourceIdentifier
	for rows.Next() {
		var id models.ResourceIdentifier
		if err := rows.Scan(&id.Namespace, &id.Name); err != nil {
			return errors.Wrap(err, "failed to scan network identifier")
		}
		identifiers = append(identifiers, id)
	}

	if err := rows.Err(); err != nil {
		return errors.Wrap(err, "error iterating network rows in scope")
	}

	klog.V(4).InfoS("deleteNetworksInScope: found networks in scope",
		"count", len(identifiers))

	// STEP 2: Use DeleteNetworksByIDs() which creates DELETE outbox entries
	// This ensures Sync-First principle: DELETE → Outbox → Worker → SGROUP → DB Delete
	// Migration 032 trigger will prevent immediate deletion and create outbox entry
	if len(identifiers) > 0 {
		if err := w.DeleteNetworksByIDs(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete networks by IDs")
		}

		klog.V(4).InfoS("deleteNetworksInScope: created DELETE outbox entries",
			"count", len(identifiers))
	}

	return nil
}

// DeleteNetworksByIDs deletes networks by their resource identifiers
//
// This method triggers the BEFORE DELETE trigger (migration 026: trigger_network_before_delete)
// which automatically handles:
// - Soft delete (UPDATE k8s_metadata SET deletion_timestamp = NOW())
// - DELETE outbox entry creation
// - Prevention of physical deletion (RETURN NULL) until Worker syncs to SGROUP
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

	// Execute DELETE FROM networks
	// This will trigger the BEFORE DELETE trigger which:
	// 1. Checks if deletion_timestamp is NULL (first delete attempt)
	// 2. If NULL: soft delete + create DELETE outbox entry + prevent physical deletion
	// 3. If NOT NULL: allow physical deletion (Worker already synced to SGROUP)
	deleteQuery := fmt.Sprintf(`
		DELETE FROM networks
		WHERE %s
	`, strings.Join(conditions, " OR "))

	result, err := w.tx.Exec(ctx, deleteQuery, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete networks")
	}

	rowsAffected := result.RowsAffected()
	klog.V(4).InfoS("Executed DELETE for networks (BEFORE DELETE trigger handles soft delete + outbox)",
		"requested", len(ids),
		"rows_affected", rowsAffected)

	return nil
}

// updateNetworkConditionsOnly updates ONLY the k8s_metadata.conditions field
// WITHOUT touching the networks table or creating outbox entries
// This is used by OutboxWorker to update Ready/Synced conditions after successful sync
func (w *Writer) updateNetworkConditionsOnly(ctx context.Context, network *models.Network) error {
	// First, get the resource_version from the networks table
	var resourceVersion int64
	getVersionQuery := `SELECT resource_version FROM networks WHERE namespace = $1 AND name = $2`
	err := w.tx.QueryRow(ctx, getVersionQuery, network.Namespace, network.Name).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to get resource_version for network %s/%s", network.Namespace, network.Name)
	}

	// Marshal only the conditions
	conditionsJSON, err := json.Marshal(network.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// Update ONLY the conditions in k8s_metadata
	updateQuery := `
		UPDATE k8s_metadata
		SET conditions = $1, updated_at = NOW()
		WHERE resource_version = $2`

	_, err = w.tx.Exec(ctx, updateQuery, conditionsJSON, resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to update conditions for network %s/%s", network.Namespace, network.Name)
	}

	klog.V(4).InfoS("Updated conditions only for Network (no outbox entry created)",
		"namespace", network.Namespace,
		"name", network.Name,
		"resource_version", resourceVersion)

	return nil
}
