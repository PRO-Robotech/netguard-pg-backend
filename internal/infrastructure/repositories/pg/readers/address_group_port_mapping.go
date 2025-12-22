package readers

import (
	"context"
	"fmt"
	"time"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/utils"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
)

func (r *Reader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	query := `
		SELECT agpm.namespace, agpm.name, agpm.access_ports,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
		FROM address_group_port_mappings agpm
		INNER JOIN k8s_metadata m ON agpm.resource_version = m.resource_version`
	whereClause, args, err := utils.BuildScopeFilterWithTable(scope, "address_group_port_mappings", "agpm")
	if err != nil {
		return errors.Wrap(err, "failed to build scope filter")
	}

	if whereClause != "" {
		query += " WHERE " + whereClause
	} else {
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

func (r *Reader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	query := `
		SELECT agpm.namespace, agpm.name, agpm.access_ports,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
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

func (r *Reader) scanAddressGroupPortMapping(rows pgx.Rows) (models.AddressGroupPortMapping, error) {
	var mapping models.AddressGroupPortMapping
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
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
		&deletionTS,
	)
	if err != nil {
		return mapping, err
	}

	mapping.AccessPorts, err = utils.UnmarshalAccessPorts(accessPortsJSON)
	if err != nil {
		return mapping, err
	}

	mapping.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return mapping, err
	}

	mapping.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(mapping.Name, models.WithNamespace(mapping.Namespace)))

	return mapping, nil
}

func (r *Reader) scanAddressGroupPortMappingRow(row pgx.Row) (*models.AddressGroupPortMapping, error) {
	var mapping models.AddressGroupPortMapping
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
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
		&deletionTS,
	)
	if err != nil {
		return nil, err
	}

	mapping.AccessPorts, err = utils.UnmarshalAccessPorts(accessPortsJSON)
	if err != nil {
		return nil, err
	}

	mapping.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return nil, err
	}
	mapping.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(mapping.Name, models.WithNamespace(mapping.Namespace)))

	return &mapping, nil
}
