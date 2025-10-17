package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories"
)

type UUIDAlreadyExistsError struct {
	UUID     string
	HostName string
	Err      error
}

func (e *UUIDAlreadyExistsError) Error() string {
	return fmt.Sprintf("UUID '%s' already exists (attempted to create/update host %s)", e.UUID, e.HostName)
}

func (e *UUIDAlreadyExistsError) Unwrap() error {
	return e.Err
}

// SyncHosts syncs hosts to PostgreSQL with K8s metadata support
func (w *Writer) SyncHosts(ctx context.Context, hosts []models.Host, scope ports.Scope, options ...ports.Option) error {
	isConditionOnly := false
	for _, opt := range options {
		if _, ok := opt.(ports.ConditionOnlyOperation); ok {
			isConditionOnly = true
			break
		}
	}

	if isConditionOnly {
		// Directly update conditions WITHOUT creating outbox entries
		for i := range hosts {
			if err := w.updateHostConditionsOnly(ctx, &hosts[i]); err != nil {
				return errors.Wrapf(err, "failed to update conditions for host %s/%s", hosts[i].Namespace, hosts[i].Name)
			}
		}
		return nil
	}

	// EXISTING LOGIC BELOW (for normal sync operations)
	syncOp := models.SyncOpUpsert // Default operation
	for _, opt := range options {
		if syncOption, ok := opt.(ports.SyncOption); ok {
			syncOp = syncOption.Operation
			break
		}
	}

	if !scope.IsEmpty() && syncOp != models.SyncOpDelete {
		if err := w.deleteHostsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete hosts in scope")
		}
	}

	switch syncOp {
	case models.SyncOpDelete:
		// For DELETE operations, delete the specific hosts
		var identifiers []models.ResourceIdentifier
		for _, host := range hosts {
			identifiers = append(identifiers, models.ResourceIdentifier{
				Namespace: host.Namespace,
				Name:      host.Name,
			})
		}
		if err := w.DeleteHostsByIDs(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete hosts")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		for i := range hosts {
			if err := w.upsertHost(ctx, &hosts[i]); err != nil {
				// Check for UUID uniqueness violation
				if isUniqueViolation(err, "hosts_uuid_key") {
					return &UUIDAlreadyExistsError{
						UUID:     hosts[i].UUID,
						HostName: fmt.Sprintf("%s/%s", hosts[i].Namespace, hosts[i].Name),
						Err:      err,
					}
				}
				return errors.Wrapf(err, "failed to upsert host %s/%s", hosts[i].Namespace, hosts[i].Name)
			}
		}
	}

	return nil
}

// upsertHost inserts or updates a host with K8s metadata and creates Outbox entry
func (w *Writer) upsertHost(ctx context.Context, host *models.Host) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(host.Meta.Labels, host.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	// Check if host exists to determine if this is INSERT or UPDATE
	var ipListJSON []byte
	var existingIpListJSON []byte
	var existingResourceVersion sql.NullInt64
	checkQuery := `SELECT COALESCE(ip_list, '[]'::jsonb), resource_version FROM hosts WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, checkQuery, host.Namespace, host.Name).Scan(&existingIpListJSON, &existingResourceVersion)

	// Ready will be set to True by Worker after successful SGROUP sync
	// Business Rule: Resources should NOT have Ready=True until synced to SGROUP
	conditions := host.Meta.Conditions
	if !existingResourceVersion.Valid {
		// This is a new resource - force Pending status
		conditions = forcePendingSyncCondition(conditions)
		klog.V(4).InfoS("Forcing PendingSGROUPSync status for new Host",
			"namespace", host.Namespace, "name", host.Name)
	}

	conditionsJSON, err := json.Marshal(conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	var existingIpList []models.IPItem
	if err == nil && existingIpListJSON != nil && len(existingIpListJSON) > 0 {
		_ = json.Unmarshal(existingIpListJSON, &existingIpList)
	}

	if host.IpList != nil && len(host.IpList) > 0 {
		ipListJSON, err = json.Marshal(host.IpList)
		if err != nil {
			return errors.Wrap(err, "failed to marshal IP list")
		}
	} else if len(existingIpList) > 0 {
		ipListJSON = existingIpListJSON
	}

	// Insert new K8s metadata to get new ResourceVersion
	var resourceVersion int64
	metadataQuery := `
		INSERT INTO k8s_metadata (labels, annotations, conditions)
		VALUES ($1, $2, $3)
		RETURNING resource_version`
	err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to insert K8s metadata for host %s/%s", host.Namespace, host.Name)
	}

	host.Meta.TouchOnWrite(strconv.FormatInt(resourceVersion, 10))

	var hostNameSync, addressGroupName *string
	if host.HostName != "" {
		hostNameSync = &host.HostName
	}
	if host.AddressGroupName != "" {
		addressGroupName = &host.AddressGroupName
	}

	var bindingRefNamespace, bindingRefName *string
	if host.BindingRef != nil {
		bindingRefNamespace = &host.Namespace
		bindingRefName = &host.BindingRef.Name
	}

	var addressGroupRefNamespace, addressGroupRefName *string
	if host.AddressGroupRef != nil {
		addressGroupRefNamespace = &host.Namespace
		addressGroupRefName = &host.AddressGroupRef.Name
	}

	hostQuery := `
		INSERT INTO hosts (
			namespace, name, uuid,
			host_name_sync, address_group_name, is_bound,
			binding_ref_namespace, binding_ref_name,
			address_group_ref_namespace, address_group_ref_name,
			ip_list,
			resource_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (namespace, name) DO UPDATE SET
			uuid = EXCLUDED.uuid,
			host_name_sync = EXCLUDED.host_name_sync,
			address_group_name = EXCLUDED.address_group_name,
			is_bound = EXCLUDED.is_bound,
			binding_ref_namespace = EXCLUDED.binding_ref_namespace,
			binding_ref_name = EXCLUDED.binding_ref_name,
			address_group_ref_namespace = EXCLUDED.address_group_ref_namespace,
			address_group_ref_name = EXCLUDED.address_group_ref_name,
			ip_list = EXCLUDED.ip_list,
			resource_version = EXCLUDED.resource_version`

	_, err = w.tx.Exec(ctx, hostQuery,
		host.Namespace, host.Name, host.UUID,
		hostNameSync, addressGroupName, host.IsBound,
		bindingRefNamespace, bindingRefName,
		addressGroupRefNamespace, addressGroupRefName,
		ipListJSON,
		resourceVersion,
	)
	if err != nil {
		return errors.Wrapf(err, "failed to upsert host %s/%s", host.Namespace, host.Name)
	}

	// ========================================
	// ========================================
	if err := w.createHostOutboxEntry(ctx, host); err != nil {
		return errors.Wrap(err, "failed to create outbox entry for host")
	}

	return nil
}

// createHostOutboxEntry creates an outbox entry for Host resource (CREATE/UPDATE)
func (w *Writer) createHostOutboxEntry(ctx context.Context, host *models.Host) error {
	// Build payload
	payload := map[string]interface{}{
		"namespace":          host.Namespace,
		"name":               host.Name,
		"uuid":               host.UUID,
		"hostname":           host.HostName,
		"ip_list":            host.IpList,
		"address_group_name": host.AddressGroupName,
		"is_bound":           host.IsBound,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal host payload")
	}

	// Parse resource UUID
	resourceUUID, err := uuid.Parse(host.UUID)
	if err != nil {
		return errors.Wrapf(err, "invalid host UUID: %s", host.UUID)
	}

	// Create outbox entry
	outboxEntry := &domain.OutboxEntry{
		ResourceType:      "Host",
		ResourceID:        resourceUUID,
		ResourceNamespace: host.Namespace,
		ResourceName:      host.Name,
		Operation:         domain.SyncOperationCreate, // Always CREATE for upsert pattern
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

	klog.V(4).InfoS("Created outbox entry for Host",
		"namespace", host.Namespace,
		"name", host.Name,
		"outbox_id", outboxEntry.ID)

	return nil
}

// createHostDeleteOutboxEntry creates an outbox entry for Host DELETE operation
func (w *Writer) createHostDeleteOutboxEntry(ctx context.Context, host *models.Host) error {
	// Build minimal payload for DELETE (only need identifiers)
	payload := map[string]interface{}{
		"namespace": host.Namespace,
		"name":      host.Name,
		"uuid":      host.UUID,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal host DELETE payload")
	}

	// Parse resource UUID
	resourceUUID, err := uuid.Parse(host.UUID)
	if err != nil {
		return errors.Wrapf(err, "invalid host UUID: %s", host.UUID)
	}

	// Create DELETE outbox entry
	outboxEntry := &domain.OutboxEntry{
		ResourceType:      "Host",
		ResourceID:        resourceUUID,
		ResourceNamespace: host.Namespace,
		ResourceName:      host.Name,
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

	klog.V(4).InfoS("Created DELETE outbox entry for Host",
		"namespace", host.Namespace,
		"name", host.Name,
		"outbox_id", outboxEntry.ID)

	return nil
}

// updateHostConditionsOnly updates ONLY the k8s_metadata.conditions field
// WITHOUT touching the hosts table or creating outbox entries
// This is used by OutboxWorker to update Ready/Synced conditions after successful sync
func (w *Writer) updateHostConditionsOnly(ctx context.Context, host *models.Host) error {
	// First, get the resource_version from the hosts table
	var resourceVersion int64
	getVersionQuery := `SELECT resource_version FROM hosts WHERE namespace = $1 AND name = $2`
	err := w.tx.QueryRow(ctx, getVersionQuery, host.Namespace, host.Name).Scan(&resourceVersion)
	if err != nil {
		if err == sql.ErrNoRows {
			return errors.Errorf("host %s/%s not found", host.Namespace, host.Name)
		}
		return errors.Wrapf(err, "failed to get resource_version for host %s/%s", host.Namespace, host.Name)
	}

	// Marshal only the conditions
	conditionsJSON, err := json.Marshal(host.Meta.Conditions)
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
		return errors.Wrapf(err, "failed to update conditions for host %s/%s", host.Namespace, host.Name)
	}

	klog.V(4).InfoS("Updated conditions only for Host (no outbox entry created)",
		"namespace", host.Namespace,
		"name", host.Name,
		"resource_version", resourceVersion)

	return nil
}

// deleteHostsInScope deletes all hosts within the given scope
// Previous implementation used direct DELETE which bypassed Migration 032 trigger,
// causing Hosts to be deleted before Worker could process them.
func (w *Writer) deleteHostsInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	switch s := scope.(type) {
	case ports.ResourceIdentifierScope:
		if s.IsEmpty() {
			return nil
		}

		klog.V(4).InfoS("deleteHostsInScope: deleting hosts in ResourceIdentifierScope",
			"count", len(s.Identifiers))

		// Use DeleteHostsByIDs() which creates DELETE outbox entries
		// This ensures Sync-First principle: DELETE → Outbox → Worker → SGROUP → DB Delete
		// Migration 032 trigger will prevent immediate deletion and create outbox entry
		if err := w.DeleteHostsByIDs(ctx, s.Identifiers); err != nil {
			return errors.Wrap(err, "failed to delete hosts by IDs")
		}

		klog.V(4).InfoS("deleteHostsInScope: created DELETE outbox entries",
			"count", len(s.Identifiers))

		return nil

	default:
		return errors.Errorf("unsupported scope type for hosts deletion: %T", scope)
	}
}

// DeleteHostsByIDs deletes hosts by their resource identifiers
func (w *Writer) DeleteHostsByIDs(ctx context.Context, ids []models.ResourceIdentifier, options ...ports.Option) error {
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

	// ========================================================================
	// ========================================================================
	// We need Host.UUID and other fields to create DELETE outbox entry
	fetchQuery := fmt.Sprintf(`
		SELECT namespace, name, uuid, host_name_sync, address_group_name, is_bound
		FROM hosts
		WHERE (namespace, name) IN (%s)
	`, strings.Join(values, ","))

	rows, err := w.tx.Query(ctx, fetchQuery, args...)
	if err != nil {
		return errors.Wrap(err, "failed to fetch hosts before deletion")
	}
	defer rows.Close()

	var hostsToDelete []models.Host
	for rows.Next() {
		var host models.Host
		var hostNameSync, addressGroupName sql.NullString

		err := rows.Scan(
			&host.Namespace,
			&host.Name,
			&host.UUID,
			&hostNameSync,
			&addressGroupName,
			&host.IsBound,
		)
		if err != nil {
			return errors.Wrap(err, "failed to scan host data")
		}

		if hostNameSync.Valid {
			host.HostName = hostNameSync.String
		}
		if addressGroupName.Valid {
			host.AddressGroupName = addressGroupName.String
		}

		hostsToDelete = append(hostsToDelete, host)
	}

	if err := rows.Err(); err != nil {
		return errors.Wrap(err, "error iterating host rows")
	}

	klog.V(4).InfoS("Fetched hosts for deletion",
		"count", len(hostsToDelete),
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
		FROM hosts h
		WHERE h.resource_version = m.resource_version
		  AND (h.namespace, h.name) IN (%s)
		  AND m.deletion_timestamp IS NULL`

	markQuery := fmt.Sprintf(markDeleteQuery, strings.Join(values, ","))
	_, err = w.tx.Exec(ctx, markQuery, args...)
	if err != nil {
		return errors.Wrap(err, "failed to mark hosts as pending deletion")
	}

	klog.V(4).InfoS("Marked hosts as pending deletion (NOT deleted from DB yet)",
		"count", len(hostsToDelete))

	// ========================================================================
	// STEP 3 - Create DELETE outbox entries for SGROUP sync
	// ========================================================================
	// Worker will process these, sync to SGROUP, THEN delete from DB
	for i := range hostsToDelete {
		if err := w.createHostDeleteOutboxEntry(ctx, &hostsToDelete[i]); err != nil {
			return errors.Wrapf(err, "failed to create DELETE outbox entry for host %s/%s",
				hostsToDelete[i].Namespace, hostsToDelete[i].Name)
		}
	}

	klog.V(4).InfoS("Created DELETE outbox entries for hosts (resources remain in DB until Worker processes)",
		"count", len(hostsToDelete))

	return nil
}
