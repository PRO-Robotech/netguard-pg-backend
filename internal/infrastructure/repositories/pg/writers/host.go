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

// UUIDAlreadyExistsError represents a UUID uniqueness violation error
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
	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteHostsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete hosts in scope")
		}
	}

	// Upsert each host
	for _, host := range hosts {
		if err := w.upsertHost(ctx, host); err != nil {
			// Check for UUID uniqueness violation
			if isUniqueViolation(err, "hosts_uuid_key") {
				return &UUIDAlreadyExistsError{
					UUID:     host.UUID,
					HostName: fmt.Sprintf("%s/%s", host.Namespace, host.Name),
					Err:      err,
				}
			}
			return errors.Wrapf(err, "failed to upsert host %s/%s", host.Namespace, host.Name)
		}
	}

	return nil
}

// upsertHost inserts or updates a host with K8s metadata
func (w *Writer) upsertHost(ctx context.Context, host models.Host) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(host.Meta.Labels, host.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(host.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if host exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM hosts WHERE namespace = $1 AND name = $2`
	_ = w.tx.QueryRow(ctx, existingQuery, host.Namespace, host.Name).Scan(&existingResourceVersion)

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
			return errors.Wrapf(err, "failed to update K8s metadata for host %s/%s", host.Namespace, host.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, conditions)
			VALUES ($1, $2, $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to insert K8s metadata for host %s/%s", host.Namespace, host.Name)
		}
	}

	// Prepare nullable status fields
	var hostNameSync, addressGroupName *string
	if host.HostName != "" {
		hostNameSync = &host.HostName
	}
	if host.AddressGroupName != "" {
		addressGroupName = &host.AddressGroupName
	}

	// Prepare nullable reference fields
	var bindingRefNamespace, bindingRefName *string
	if host.BindingRef != nil {
		// For HostBinding, we need to get namespace from the host itself as ObjectReference has no namespace field
		bindingRefNamespace = &host.Namespace
		bindingRefName = &host.BindingRef.Name
	}

	var addressGroupRefNamespace, addressGroupRefName *string
	if host.AddressGroupRef != nil {
		// For AddressGroup, we need to get namespace from the host itself as ObjectReference has no namespace field
		addressGroupRefNamespace = &host.Namespace
		addressGroupRefName = &host.AddressGroupRef.Name
	}

	// UPSERT host record
	hostQuery := `
		INSERT INTO hosts (
			namespace, name, uuid, 
			host_name_sync, address_group_name, is_bound,
			binding_ref_namespace, binding_ref_name,
			address_group_ref_namespace, address_group_ref_name,
			resource_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (namespace, name) DO UPDATE SET
			uuid = EXCLUDED.uuid,
			host_name_sync = EXCLUDED.host_name_sync,
			address_group_name = EXCLUDED.address_group_name,
			is_bound = EXCLUDED.is_bound,
			binding_ref_namespace = EXCLUDED.binding_ref_namespace,
			binding_ref_name = EXCLUDED.binding_ref_name,
			address_group_ref_namespace = EXCLUDED.address_group_ref_namespace,
			address_group_ref_name = EXCLUDED.address_group_ref_name,
			resource_version = EXCLUDED.resource_version`

	_, err = w.tx.Exec(ctx, hostQuery,
		host.Namespace, host.Name, host.UUID,
		hostNameSync, addressGroupName, host.IsBound,
		bindingRefNamespace, bindingRefName,
		addressGroupRefNamespace, addressGroupRefName,
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

	query := fmt.Sprintf(`DELETE FROM hosts WHERE (namespace, name) IN (%s)`, strings.Join(values, ","))
	_, err := w.tx.Exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete hosts by IDs")
	}

	return nil
}
