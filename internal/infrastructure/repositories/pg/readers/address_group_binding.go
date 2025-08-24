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
)

// ListAddressGroupBindings lists address group bindings with K8s metadata support
func (r *Reader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	query := `
		SELECT agb.namespace, agb.name, agb.service_namespace, agb.service_name,
			   agb.address_group_namespace, agb.address_group_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM address_group_bindings agb
		INNER JOIN k8s_metadata m ON agb.resource_version = m.resource_version`

	// Apply scope filtering
	whereClause, args := utils.BuildScopeFilter(scope, "agb")
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	query += " ORDER BY agb.namespace, agb.name"

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query address group bindings")
	}
	defer rows.Close()

	for rows.Next() {
		binding, err := r.scanAddressGroupBinding(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan address group binding")
		}

		if err := consume(binding); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetAddressGroupBindingByID gets an address group binding by ID
func (r *Reader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	query := `
		SELECT agb.namespace, agb.name, agb.service_namespace, agb.service_name,
			   agb.address_group_namespace, agb.address_group_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM address_group_bindings agb
		INNER JOIN k8s_metadata m ON agb.resource_version = m.resource_version
		WHERE agb.namespace = $1 AND agb.name = $2`

	row := r.queryRow(ctx, query, id.Namespace, id.Name)

	binding, err := r.scanAddressGroupBindingRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan address group binding")
	}

	return binding, nil
}

// scanAddressGroupBinding scans an address group binding from pgx.Rows
func (r *Reader) scanAddressGroupBinding(rows pgx.Rows) (models.AddressGroupBinding, error) {
	var binding models.AddressGroupBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database
	var serviceNamespace, serviceName string
	var addressGroupNamespace, addressGroupName string

	err := rows.Scan(
		&binding.Namespace,
		&binding.Name,
		&serviceNamespace,
		&serviceName,
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
		return binding, err
	}

	// Convert K8s metadata (convert int64 to string)
	binding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return binding, err
	}

	// Set SelfRef
	binding.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(binding.Name, models.WithNamespace(binding.Namespace)))

	// Set references
	binding.ServiceRef = models.NewServiceRef(serviceName, models.WithNamespace(serviceNamespace))
	binding.AddressGroupRef = models.NewAddressGroupRef(addressGroupName, models.WithNamespace(addressGroupNamespace))

	return binding, nil
}

// scanAddressGroupBindingRow scans an address group binding from pgx.Row
func (r *Reader) scanAddressGroupBindingRow(row pgx.Row) (*models.AddressGroupBinding, error) {
	var binding models.AddressGroupBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database
	var serviceNamespace, serviceName string
	var addressGroupNamespace, addressGroupName string

	err := row.Scan(
		&binding.Namespace,
		&binding.Name,
		&serviceNamespace,
		&serviceName,
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
	binding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}

	// Set SelfRef
	binding.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(binding.Name, models.WithNamespace(binding.Namespace)))

	// Set references
	binding.ServiceRef = models.NewServiceRef(serviceName, models.WithNamespace(serviceNamespace))
	binding.AddressGroupRef = models.NewAddressGroupRef(addressGroupName, models.WithNamespace(addressGroupNamespace))

	return &binding, nil
}
