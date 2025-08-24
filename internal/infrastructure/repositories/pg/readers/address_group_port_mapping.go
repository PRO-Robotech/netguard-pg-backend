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

// ListAddressGroupPortMappings lists address group port mappings with K8s metadata support
func (r *Reader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	query := `
		SELECT agpm.namespace, agpm.name, agpm.access_ports,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM address_group_port_mappings agpm
		INNER JOIN k8s_metadata m ON agpm.resource_version = m.resource_version`

	// Apply scope filtering
	whereClause, args := utils.BuildScopeFilter(scope, "agpm")
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	query += " ORDER BY agpm.namespace, agpm.name"

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query address group port mappings")
	}
	defer rows.Close()

	for rows.Next() {
		mapping, err := r.scanAddressGroupPortMapping(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan address group port mapping")
		}

		if err := consume(mapping); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetAddressGroupPortMappingByID gets an address group port mapping by ID
func (r *Reader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	query := `
		SELECT agpm.namespace, agpm.name, agpm.access_ports,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM address_group_port_mappings agpm
		INNER JOIN k8s_metadata m ON agpm.resource_version = m.resource_version
		WHERE agpm.namespace = $1 AND agpm.name = $2`

	row := r.queryRow(ctx, query, id.Namespace, id.Name)

	mapping, err := r.scanAddressGroupPortMappingRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan address group port mapping")
	}

	return mapping, nil
}

// scanAddressGroupPortMapping scans an address group port mapping from pgx.Rows
func (r *Reader) scanAddressGroupPortMapping(rows pgx.Rows) (models.AddressGroupPortMapping, error) {
	var mapping models.AddressGroupPortMapping
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database
	var accessPortsJSON []byte

	err := rows.Scan(
		&mapping.Namespace,
		&mapping.Name,
		&accessPortsJSON,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return mapping, err
	}

	// Parse access ports
	mapping.AccessPorts, err = utils.UnmarshalAccessPorts(accessPortsJSON)
	if err != nil {
		return mapping, err
	}

	// Convert K8s metadata (convert int64 to string)
	mapping.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return mapping, err
	}

	// Set SelfRef
	mapping.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(mapping.Name, models.WithNamespace(mapping.Namespace)))

	return mapping, nil
}

// scanAddressGroupPortMappingRow scans an address group port mapping from pgx.Row
func (r *Reader) scanAddressGroupPortMappingRow(row pgx.Row) (*models.AddressGroupPortMapping, error) {
	var mapping models.AddressGroupPortMapping
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database
	var accessPortsJSON []byte

	err := row.Scan(
		&mapping.Namespace,
		&mapping.Name,
		&accessPortsJSON,
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

	// Parse access ports
	mapping.AccessPorts, err = utils.UnmarshalAccessPorts(accessPortsJSON)
	if err != nil {
		return nil, err
	}

	// Convert K8s metadata (convert int64 to string)
	mapping.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}

	// Set SelfRef
	mapping.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(mapping.Name, models.WithNamespace(mapping.Namespace)))

	return &mapping, nil
}
