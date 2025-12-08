package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
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
	var metaInfoJSON []byte
	var existingIpListJSON []byte
	var existingMetaInfoJSON []byte
	var existingResourceVersion sql.NullInt64
	checkQuery := `SELECT COALESCE(ip_list, '[]'::jsonb), COALESCE(meta_info, '{}'::jsonb), resource_version FROM hosts WHERE namespace = $1 AND name = $2`
	queryErr := w.tx.QueryRow(ctx, checkQuery, host.Namespace, host.Name).Scan(&existingIpListJSON, &existingMetaInfoJSON, &existingResourceVersion)
	if queryErr != nil && !isNoRowsError(queryErr) {
		return errors.Wrapf(queryErr, "failed to check existing host %s/%s", host.Namespace, host.Name)
	}

	if !existingResourceVersion.Valid {
		klog.V(4).InfoS("Forcing PendingSGROUPSync status for new Host",
			"namespace", host.Namespace, "name", host.Name)
	}

	conditionsJSON, err := w.prepareConditionsJSON(ctx, existingResourceVersion, host.Meta.Conditions, conditionMergeOptions{
		ForcePendingReady: true,
	})
	if err != nil {
		return errors.Wrap(err, "failed to prepare merged conditions")
	}

	var existingIpList []models.IPItem
	if queryErr == nil && len(existingIpListJSON) > 0 {
		_ = json.Unmarshal(existingIpListJSON, &existingIpList)
	}

	var existingMetaInfo *models.HostMetaInfo
	if queryErr == nil && len(existingMetaInfoJSON) > 0 && string(existingMetaInfoJSON) != "{}" {
		existingMetaInfo = &models.HostMetaInfo{}
		_ = json.Unmarshal(existingMetaInfoJSON, existingMetaInfo)
	}

	if len(host.IpList) > 0 {
		ipListJSON, err = json.Marshal(host.IpList)
		if err != nil {
			return errors.Wrap(err, "failed to marshal IP list")
		}
	} else if len(existingIpList) > 0 {
		ipListJSON = existingIpListJSON
	}

	// Handle MetaInfo - preserve existing if not set in new host
	if host.MetaInfo != nil {
		metaInfoJSON, err = json.Marshal(host.MetaInfo)
		if err != nil {
			return errors.Wrap(err, "failed to marshal MetaInfo")
		}
	} else if existingMetaInfo != nil {
		metaInfoJSON = existingMetaInfoJSON
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
			ip_list, meta_info,
			resource_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
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
			meta_info = EXCLUDED.meta_info,
			resource_version = EXCLUDED.resource_version`

	_, err = w.tx.Exec(ctx, hostQuery,
		host.Namespace, host.Name, host.UUID,
		hostNameSync, addressGroupName, host.IsBound,
		bindingRefNamespace, bindingRefName,
		addressGroupRefNamespace, addressGroupRefName,
		ipListJSON, metaInfoJSON,
		resourceVersion,
	)
	if err != nil {
		return errors.Wrapf(err, "failed to upsert host %s/%s", host.Namespace, host.Name)
	}

	// Outbox entries for host upserts are created by trigger 'trg_host_upsert_outbox'.

	return nil
}

// DELETE outbox entries are created by BEFORE DELETE trigger 'trigger_host_before_delete'
// (migration 026), so no additional helper is required.

// updateHostConditionsOnly updates ONLY the k8s_metadata.conditions field
// WITHOUT touching the hosts table or creating outbox entries
// This is used by OutboxWorker to update Ready/Synced conditions after successful sync
func (w *Writer) updateHostConditionsOnly(ctx context.Context, host *models.Host) error {
	// First, get the resource_version from the hosts table
	var resourceVersion int64
	getVersionQuery := `SELECT resource_version FROM hosts WHERE namespace = $1 AND name = $2`
	err := w.tx.QueryRow(ctx, getVersionQuery, host.Namespace, host.Name).Scan(&resourceVersion)
	if err != nil {
		if isNoRowsError(err) {
			return errors.Errorf("host %s/%s not found", host.Namespace, host.Name)
		}
		return errors.Wrapf(err, "failed to get resource_version for host %s/%s", host.Namespace, host.Name)
	}

	mergedVersion := sql.NullInt64{Int64: resourceVersion, Valid: true}

	conditionsJSON, err := w.prepareConditionsJSON(ctx, mergedVersion, host.Meta.Conditions, conditionMergeOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to prepare merged conditions")
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
//
// This method triggers the BEFORE DELETE trigger (migration 026: trigger_host_before_delete)
// which automatically handles:
// - Soft delete (UPDATE k8s_metadata SET deletion_timestamp = NOW())
// - DELETE outbox entry creation
// - Prevention of physical deletion (RETURN NULL) until Worker syncs to SGROUP
func (w *Writer) DeleteHostsByIDs(ctx context.Context, ids []models.ResourceIdentifier, options ...ports.Option) error {
	// ===== LOGGING: Get stack trace =====
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	stackTrace := string(buf[:n])

	klog.InfoS("[DELETE_HOSTS] ===== DeleteHostsByIDs CALLED =====",
		"count", len(ids))

	for i, id := range ids {
		klog.InfoS("[DELETE_HOSTS] Host to delete",
			"index", i,
			"namespace", id.Namespace,
			"name", id.Name)
	}

	klog.InfoS("[DELETE_HOSTS] Call stack trace",
		"stack", stackTrace)

	if len(ids) == 0 {
		klog.InfoS("[DELETE_HOSTS] No hosts to delete, returning early")
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

	// Execute DELETE FROM hosts
	// This will trigger the BEFORE DELETE trigger which:
	// 1. Checks if deletion_timestamp is NULL (first delete attempt)
	// 2. If NULL: soft delete + create DELETE outbox entry + prevent physical deletion
	// 3. If NOT NULL: allow physical deletion (Worker already synced to SGROUP)
	deleteQuery := fmt.Sprintf(`
		DELETE FROM hosts
		WHERE (namespace, name) IN (%s)
	`, strings.Join(values, ","))

	klog.InfoS("[DELETE_HOSTS] Executing DELETE query",
		"query", deleteQuery,
		"args", fmt.Sprintf("%v", args))

	result, err := w.tx.Exec(ctx, deleteQuery, args...)
	if err != nil {
		klog.ErrorS(err, "[DELETE_HOSTS] DELETE query FAILED")
		return errors.Wrap(err, "failed to delete hosts")
	}

	rowsAffected := result.RowsAffected()
	klog.InfoS("[DELETE_HOSTS] DELETE query completed",
		"requested", len(ids),
		"rows_affected", rowsAffected)

	if rowsAffected == 0 {
		klog.InfoS("[DELETE_HOSTS] *** NO ROWS AFFECTED *** - Host may have been soft deleted (deletion_timestamp set, physical deletion prevented by trigger)")
	} else {
		klog.InfoS("[DELETE_HOSTS] *** ROWS PHYSICALLY DELETED *** - Check if this was expected (should only happen after SGROUP sync)",
			"rows_deleted", rowsAffected)
	}

	return nil
}
