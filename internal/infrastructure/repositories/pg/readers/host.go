package readers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/utils"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// ListHosts lists hosts with K8s metadata support
func (r *Reader) ListHosts(ctx context.Context, consume func(models.Host) error, scope ports.Scope) error {
	query := `
		SELECT h.namespace, h.name, h.uuid,
		       h.host_name_sync, h.address_group_name, h.is_bound,
		       h.binding_ref_namespace, h.binding_ref_name,
		       h.address_group_ref_namespace, h.address_group_ref_name,
		       h.ip_list,
		       m.resource_version, m.labels, m.annotations, m.conditions,
		       m.created_at, m.updated_at
		FROM hosts h
		INNER JOIN k8s_metadata m ON h.resource_version = m.resource_version`

	// Apply scope filtering
	whereClause, args := utils.BuildScopeFilter(scope, "h")
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	query += " ORDER BY h.namespace, h.name"

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query hosts")
	}
	defer rows.Close()

	for rows.Next() {
		host, err := r.scanHost(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan host")
		}

		if err := consume(host); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetHostByID gets a host by ID
func (r *Reader) GetHostByID(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error) {
	query := `
		SELECT h.namespace, h.name, h.uuid,
		       h.host_name_sync, h.address_group_name, h.is_bound,
		       h.binding_ref_namespace, h.binding_ref_name,
		       h.address_group_ref_namespace, h.address_group_ref_name,
		       h.ip_list,
		       m.resource_version, m.labels, m.annotations, m.conditions,
		       m.created_at, m.updated_at
		FROM hosts h
		INNER JOIN k8s_metadata m ON h.resource_version = m.resource_version
		WHERE h.namespace = $1 AND h.name = $2`

	row := r.queryRow(ctx, query, id.Namespace, id.Name)

	host, err := r.scanHostRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan host")
	}

	return host, nil
}

// scanHost scans a host from pgx.Rows
func (r *Reader) scanHost(rows pgx.Rows) (models.Host, error) {
	var host models.Host
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var ipListJSON []byte              // JSON field for ip_list
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// Host-specific fields
	var uuid string                                           // Required spec field
	var hostNameSync, addressGroupName *string                // Nullable status fields
	var isBound bool                                          // Boolean field
	var bindingRefNamespace, bindingRefName *string           // Nullable references
	var addressGroupRefNamespace, addressGroupRefName *string // Nullable references

	err := rows.Scan(
		&host.Namespace,
		&host.Name,
		&uuid,
		&hostNameSync,
		&addressGroupName,
		&isBound,
		&bindingRefNamespace,
		&bindingRefName,
		&addressGroupRefNamespace,
		&addressGroupRefName,
		&ipListJSON,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return models.Host{}, errors.Wrap(err, "failed to scan host row")
	}

	// Set spec fields
	host.UUID = uuid

	// Set status fields
	if hostNameSync != nil {
		host.HostName = *hostNameSync
	}
	if addressGroupName != nil {
		host.AddressGroupName = *addressGroupName
	}
	host.IsBound = isBound

	// Set binding ref if exists (ObjectReference doesn't include namespace)
	if bindingRefNamespace != nil && bindingRefName != nil {
		host.BindingRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "HostBinding",
			Name:       *bindingRefName,
		}
	}

	// Set address group ref if exists (ObjectReference doesn't include namespace)
	if addressGroupRefNamespace != nil && addressGroupRefName != nil {
		host.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       *addressGroupRefName,
		}
	}

	// Parse IP list from JSON if present
	if ipListJSON != nil {
		var ipItems []models.IPItem
		if err := json.Unmarshal(ipListJSON, &ipItems); err != nil {
			return models.Host{}, errors.Wrap(err, "failed to parse ip_list JSON")
		}
		host.IpList = ipItems
	}

	// Parse and set metadata
	host.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return models.Host{}, errors.Wrap(err, "failed to parse host metadata")
	}

	return host, nil
}

// scanHostRow scans a host from pgx.Row
func (r *Reader) scanHostRow(row pgx.Row) (*models.Host, error) {
	var host models.Host
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var ipListJSON []byte              // JSON field for ip_list
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// Host-specific fields
	var uuid string                                           // Required spec field
	var hostNameSync, addressGroupName *string                // Nullable status fields
	var isBound bool                                          // Boolean field
	var bindingRefNamespace, bindingRefName *string           // Nullable references
	var addressGroupRefNamespace, addressGroupRefName *string // Nullable references

	err := row.Scan(
		&host.Namespace,
		&host.Name,
		&uuid,
		&hostNameSync,
		&addressGroupName,
		&isBound,
		&bindingRefNamespace,
		&bindingRefName,
		&addressGroupRefNamespace,
		&addressGroupRefName,
		&ipListJSON,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to scan host row")
	}

	// Set spec fields
	host.UUID = uuid

	// Set status fields
	if hostNameSync != nil {
		host.HostName = *hostNameSync
	}
	if addressGroupName != nil {
		host.AddressGroupName = *addressGroupName
	}
	host.IsBound = isBound

	// Set binding ref if exists
	if bindingRefNamespace != nil && bindingRefName != nil {
		host.BindingRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "HostBinding",
			Name:       *bindingRefName,
		}
	}

	// Set address group ref if exists
	if addressGroupRefNamespace != nil && addressGroupRefName != nil {
		host.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       *addressGroupRefName,
		}
	}

	// Parse IP list from JSON if present
	if ipListJSON != nil {
		var ipItems []models.IPItem
		if err := json.Unmarshal(ipListJSON, &ipItems); err != nil {
			return nil, errors.Wrap(err, "failed to parse ip_list JSON")
		}

		host.IpList = ipItems
	}

	// Parse and set metadata
	host.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse host metadata")
	}

	return &host, nil
}
