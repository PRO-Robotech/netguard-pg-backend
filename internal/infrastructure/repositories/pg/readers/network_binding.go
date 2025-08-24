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
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// ListNetworkBindings lists network bindings with K8s metadata support
func (r *Reader) ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope ports.Scope) error {
	query := `
		SELECT nb.namespace, nb.name,
		       nb.network_namespace, nb.network_name,
		       nb.address_group_namespace, nb.address_group_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM network_bindings nb
		INNER JOIN k8s_metadata m ON nb.resource_version = m.resource_version`

	// Apply scope filtering
	whereClause, args := utils.BuildScopeFilter(scope, "nb")
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	query += " ORDER BY nb.namespace, nb.name"

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query network bindings")
	}
	defer rows.Close()

	for rows.Next() {
		networkBinding, err := r.scanNetworkBinding(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan network binding")
		}

		if err := consume(networkBinding); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetNetworkBindingByID gets a network binding by ID
func (r *Reader) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	query := `
		SELECT nb.namespace, nb.name,
		       nb.network_namespace, nb.network_name,
		       nb.address_group_namespace, nb.address_group_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM network_bindings nb
		INNER JOIN k8s_metadata m ON nb.resource_version = m.resource_version
		WHERE nb.namespace = $1 AND nb.name = $2`

	row := r.queryRow(ctx, query, id.Namespace, id.Name)

	networkBinding, err := r.scanNetworkBindingRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan network binding")
	}

	return networkBinding, nil
}

// scanNetworkBinding scans a network binding from pgx.Rows
func (r *Reader) scanNetworkBinding(rows pgx.Rows) (models.NetworkBinding, error) {
	var networkBinding models.NetworkBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// NetworkBinding-specific fields - separate namespace/name columns
	var networkNamespace, networkName string           // Network reference fields
	var addressGroupNamespace, addressGroupName string // AddressGroup reference fields

	err := rows.Scan(
		&networkBinding.Namespace,
		&networkBinding.Name,
		&networkNamespace,
		&networkName,
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
		return networkBinding, err
	}

	// Convert K8s metadata (convert int64 to string)
	networkBinding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return networkBinding, err
	}

	// Set SelfRef
	networkBinding.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(networkBinding.Name, models.WithNamespace(networkBinding.Namespace)))

	// Build ObjectReference from separate namespace/name columns
	if networkNamespace != "" && networkName != "" {
		networkBinding.NetworkRef = netguardv1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Network",
			Name:       networkName,
		}
	}

	if addressGroupNamespace != "" && addressGroupName != "" {
		networkBinding.AddressGroupRef = netguardv1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       addressGroupName,
		}
	}

	return networkBinding, nil
}

// scanNetworkBindingRow scans a network binding from pgx.Row
func (r *Reader) scanNetworkBindingRow(row pgx.Row) (*models.NetworkBinding, error) {
	var networkBinding models.NetworkBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// NetworkBinding-specific fields - separate namespace/name columns
	var networkNamespace, networkName string           // Network reference fields
	var addressGroupNamespace, addressGroupName string // AddressGroup reference fields

	err := row.Scan(
		&networkBinding.Namespace,
		&networkBinding.Name,
		&networkNamespace,
		&networkName,
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
		return nil, err
	}

	// Convert K8s metadata (convert int64 to string)
	networkBinding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}

	// Set SelfRef
	networkBinding.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(networkBinding.Name, models.WithNamespace(networkBinding.Namespace)))

	// Build ObjectReference from separate namespace/name columns
	if networkNamespace != "" && networkName != "" {
		networkBinding.NetworkRef = netguardv1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Network",
			Name:       networkName,
		}
	}

	if addressGroupNamespace != "" && addressGroupName != "" {
		networkBinding.AddressGroupRef = netguardv1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       addressGroupName,
		}
	}

	return &networkBinding, nil
}
