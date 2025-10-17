package writers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories"
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

	// ========================================
	// ========================================
	if err := w.createNetworkOutboxEntry(ctx, network); err != nil {
		return errors.Wrap(err, "failed to create outbox entry for network")
	}

	return nil
}

// createNetworkOutboxEntry creates an outbox entry for Network resource (CREATE/UPDATE)
func (w *Writer) createNetworkOutboxEntry(ctx context.Context, network *models.Network) error {
	// Build payload
	payload := map[string]interface{}{
		"namespace":    network.Namespace,
		"name":         network.Name,
		"cidr":         network.CIDR,
		"network_name": network.NetworkName,
		"is_bound":     network.IsBound,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal network payload")
	}

	// Parse resource UUID from Meta.UID
	resourceUUID, err := uuid.Parse(network.Meta.UID)
	if err != nil {
		return errors.Wrapf(err, "invalid network UID: %s", network.Meta.UID)
	}

	// Create outbox entry
	outboxEntry := &domain.OutboxEntry{
		ResourceType:      "Network",
		ResourceID:        resourceUUID,
		ResourceNamespace: network.Namespace,
		ResourceName:      network.Name,
		Operation:         domain.SyncOperationCreate,
		TargetSystem:      domain.TargetSystemSGROUP,
		Payload:           payloadJSON,
		Status:            domain.OutboxStatusPending,
		MaxRetries:        5,
	}

	// Use OutboxRepository with existing transaction
	outboxRepo := repositories.NewOutboxRepository(w.tx)
	if err := outboxRepo.Create(ctx, outboxEntry); err != nil {
		return errors.Wrap(err, "failed to persist outbox entry")
	}

	klog.V(4).InfoS("Created outbox entry for Network",
		"namespace", network.Namespace,
		"name", network.Name,
		"outbox_id", outboxEntry.ID)

	return nil
}

// createNetworkDeleteOutboxEntry creates an outbox entry for Network DELETE operation
func (w *Writer) createNetworkDeleteOutboxEntry(ctx context.Context, network *models.Network) error {
	// Build minimal payload for DELETE (only need identifiers)
	payload := map[string]interface{}{
		"namespace": network.Namespace,
		"name":      network.Name,
		"cidr":      network.CIDR,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal network DELETE payload")
	}

	// Parse resource UUID from Meta.UID
	resourceUUID, err := uuid.Parse(network.Meta.UID)
	if err != nil {
		return errors.Wrapf(err, "invalid network UID: %s", network.Meta.UID)
	}

	// Create DELETE outbox entry
	outboxEntry := &domain.OutboxEntry{
		ResourceType:      "Network",
		ResourceID:        resourceUUID,
		ResourceNamespace: network.Namespace,
		ResourceName:      network.Name,
		Operation:         domain.SyncOperationDelete, // DELETE operation
		TargetSystem:      domain.TargetSystemSGROUP,
		Payload:           payloadJSON,
		Status:            domain.OutboxStatusPending,
		MaxRetries:        5,
	}

	// Use OutboxRepository with existing transaction
	outboxRepo := repositories.NewOutboxRepository(w.tx)
	if err := outboxRepo.Create(ctx, outboxEntry); err != nil {
		return errors.Wrap(err, "failed to persist DELETE outbox entry")
	}

	klog.V(4).InfoS("Created DELETE outbox entry for Network",
		"namespace", network.Namespace,
		"name", network.Name,
		"outbox_id", outboxEntry.ID)

	return nil
}

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
		conditions = append(conditions, fmt.Sprintf("(n.namespace = $%d AND n.name = $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	// ========================================================================
	// ========================================================================
	// We need Network.Meta.UID and other fields to create DELETE outbox entry
	// Use text(cidr) to force PostgreSQL to return CIDR as text, not binary
	fetchQuery := fmt.Sprintf(`
		SELECT n.namespace, n.name, text(n.cidr), n.network_items, n.is_bound, n.resource_version, m.uid
		FROM networks n
		JOIN k8s_metadata m ON n.resource_version = m.resource_version
		WHERE %s
	`, strings.Join(conditions, " OR "))

	rows, err := w.tx.Query(ctx, fetchQuery, args...)
	if err != nil {
		return errors.Wrap(err, "failed to fetch networks before deletion")
	}
	defer rows.Close()

	var networksToDelete []models.Network
	for rows.Next() {
		var network models.Network
		var networkItemsJSON []byte
		var resourceVersion int64
		var cidrText string // PostgreSQL CIDR as text

		err := rows.Scan(
			&network.Namespace,
			&network.Name,
			&cidrText, // Scan CIDR as string (pgx handles conversion)
			&networkItemsJSON,
			&network.IsBound,
			&resourceVersion,
			&network.Meta.UID,
		)
		if err != nil {
			return errors.Wrap(err, "failed to scan network data")
		}

		// Use CIDR as-is (already a string)
		network.CIDR = cidrText

		// Parse network_items to extract network_name if needed
		var networkItems []map[string]interface{}
		if len(networkItemsJSON) > 0 {
			if err := json.Unmarshal(networkItemsJSON, &networkItems); err == nil {
				if len(networkItems) > 0 {
					if name, ok := networkItems[0]["name"].(string); ok {
						network.NetworkName = name
					}
				}
			}
		}

		networksToDelete = append(networksToDelete, network)
	}

	if err := rows.Err(); err != nil {
		return errors.Wrap(err, "error iterating network rows")
	}

	klog.V(4).InfoS("Fetched networks for deletion",
		"count", len(networksToDelete),
		"requested", len(ids))

	// ========================================================================
	// STEP 2 - Mark as "pending deletion" in k8s_metadata
	// ========================================================================
	// SYNC-FIRST PRINCIPLE: We DON'T delete from DB yet!
	// Worker will sync to SGROUP first, THEN delete from DB
	markDeleteQuery := `
		UPDATE k8s_metadata m
		SET deletion_timestamp = NOW(),
		    conditions = COALESCE(conditions, '[]'::jsonb) ||
		        '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
		FROM networks n
		WHERE n.resource_version = m.resource_version
		  AND (%s)
		  AND m.deletion_timestamp IS NULL`

	markQuery := fmt.Sprintf(markDeleteQuery, strings.Join(conditions, " OR "))
	_, err = w.tx.Exec(ctx, markQuery, args...)
	if err != nil {
		return errors.Wrap(err, "failed to mark networks as pending deletion")
	}

	klog.V(4).InfoS("Marked networks as pending deletion (NOT deleted from DB yet)",
		"count", len(networksToDelete))

	// ========================================================================
	// STEP 3 - Create DELETE outbox entries for SGROUP sync
	// ========================================================================
	// Worker will process these, sync to SGROUP, THEN delete from DB
	for i := range networksToDelete {
		if err := w.createNetworkDeleteOutboxEntry(ctx, &networksToDelete[i]); err != nil {
			return errors.Wrapf(err, "failed to create DELETE outbox entry for network %s/%s",
				networksToDelete[i].Namespace, networksToDelete[i].Name)
		}
	}

	klog.V(4).InfoS("Created DELETE outbox entries for networks (resources remain in DB until Worker processes)",
		"count", len(networksToDelete))

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
