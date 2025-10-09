package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

// upsertHost inserts or updates a host with K8s metadata
func (w *Writer) upsertHost(ctx context.Context, host *models.Host) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(host.Meta.Labels, host.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(host.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	var ipListJSON []byte
	var existingIpListJSON []byte
	var existingResourceVersion sql.NullInt64
	checkQuery := `SELECT COALESCE(ip_list, '[]'::jsonb), resource_version FROM hosts WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, checkQuery, host.Namespace, host.Name).Scan(&existingIpListJSON, &existingResourceVersion)

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

	return nil
}

// deleteHostsInScope deletes all hosts within the given scope
func (w *Writer) deleteHostsInScope(ctx context.Context, scope ports.Scope) error {
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

		query := fmt.Sprintf(`DELETE FROM hosts WHERE (namespace, name) IN (%s)`, strings.Join(values, ","))
		_, err := w.tx.Exec(ctx, query, args...)
		return errors.Wrap(err, "failed to delete hosts by resource identifiers")

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

	// First, mark objects as being deleted in k8s_metadata to prevent re-creation by ListWatch
	markDeleteQuery := `
		UPDATE k8s_metadata m
		SET deletion_timestamp = NOW()
		FROM hosts h
		WHERE h.resource_version = m.resource_version
		  AND (h.namespace, h.name) IN (%s)
		  AND m.deletion_timestamp IS NULL`

	markQuery := fmt.Sprintf(markDeleteQuery, strings.Join(values, ","))
	_, err := w.tx.Exec(ctx, markQuery, args...)
	if err != nil {
		// Log but don't fail - deletion_timestamp is optional for now
		klog.V(4).InfoS("Failed to mark hosts as deleting in k8s_metadata", "error", err.Error())
	}

	// Then delete from hosts table
	query := fmt.Sprintf(`DELETE FROM hosts WHERE (namespace, name) IN (%s)`, strings.Join(values, ","))
	_, err = w.tx.Exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete hosts by IDs")
	}

	return nil
}
