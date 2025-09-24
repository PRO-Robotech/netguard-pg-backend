package readers

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/utils"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// ListHostBindings lists host bindings with K8s metadata support
func (r *Reader) ListHostBindings(ctx context.Context, consume func(models.HostBinding) error, scope ports.Scope) error {
	query := `
		SELECT hb.namespace, hb.name, 
		       hb.host_namespace, hb.host_name,
		       hb.address_group_namespace, hb.address_group_name,
		       m.resource_version, m.labels, m.annotations, m.conditions,
		       m.created_at, m.updated_at
		FROM host_bindings hb
		INNER JOIN k8s_metadata m ON hb.resource_version = m.resource_version`

	// Apply scope filtering
	whereClause, args := utils.BuildScopeFilter(scope, "hb")
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	query += " ORDER BY hb.namespace, hb.name"

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query host bindings")
	}
	defer rows.Close()

	for rows.Next() {
		hostBinding, err := r.scanHostBinding(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan host binding")
		}

		if err := consume(hostBinding); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetHostBindingByID gets a host binding by ID
func (r *Reader) GetHostBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error) {
	query := `
		SELECT hb.namespace, hb.name, 
		       hb.host_namespace, hb.host_name,
		       hb.address_group_namespace, hb.address_group_name,
		       m.resource_version, m.labels, m.annotations, m.conditions,
		       m.created_at, m.updated_at
		FROM host_bindings hb
		INNER JOIN k8s_metadata m ON hb.resource_version = m.resource_version
		WHERE hb.namespace = $1 AND hb.name = $2`

	row := r.queryRow(ctx, query, id.Namespace, id.Name)

	hostBinding, err := r.scanHostBindingRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan host binding")
	}

	return hostBinding, nil
}

// scanHostBinding scans a host binding from pgx.Rows
func (r *Reader) scanHostBinding(rows pgx.Rows) (models.HostBinding, error) {
	var hostBinding models.HostBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// HostBinding-specific fields
	var hostNamespace, hostName string                 // Host reference
	var addressGroupNamespace, addressGroupName string // AddressGroup reference

	err := rows.Scan(
		&hostBinding.Namespace,
		&hostBinding.Name,
		&hostNamespace,
		&hostName,
		&addressGroupNamespace,
		&addressGroupName,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return models.HostBinding{}, errors.Wrap(err, "failed to scan host binding row")
	}

	// Set host reference
	hostBinding.HostRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Host",
			Name:       hostName,
		},
		Namespace: hostNamespace,
	}

	// Set address group reference
	hostBinding.AddressGroupRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       addressGroupName,
		},
		Namespace: addressGroupNamespace,
	}

	// Parse and set metadata
	hostBinding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return models.HostBinding{}, errors.Wrap(err, "failed to parse host binding metadata")
	}

	return hostBinding, nil
}

// scanHostBindingRow scans a host binding from pgx.Row
func (r *Reader) scanHostBindingRow(row pgx.Row) (*models.HostBinding, error) {
	var hostBinding models.HostBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// HostBinding-specific fields
	var hostNamespace, hostName string                 // Host reference
	var addressGroupNamespace, addressGroupName string // AddressGroup reference

	err := row.Scan(
		&hostBinding.Namespace,
		&hostBinding.Name,
		&hostNamespace,
		&hostName,
		&addressGroupNamespace,
		&addressGroupName,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to scan host binding row")
	}

	// Set host reference
	hostBinding.HostRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Host",
			Name:       hostName,
		},
		Namespace: hostNamespace,
	}

	// Set address group reference
	hostBinding.AddressGroupRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       addressGroupName,
		},
		Namespace: addressGroupNamespace,
	}

	// Parse and set metadata
	hostBinding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse host binding metadata")
	}

	return &hostBinding, nil
}
