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

// ListServiceAliases lists service aliases with K8s metadata support
func (r *Reader) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	query := `
		SELECT sa.namespace, sa.name, sa.service_namespace, sa.service_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM service_aliases sa
		INNER JOIN k8s_metadata m ON sa.resource_version = m.resource_version`

	// Apply scope filtering and deletion_timestamp filter
	whereClause, args := utils.BuildScopeFilter(scope, "sa")

	// Always filter out objects being deleted
	deletionFilter := "m.deletion_timestamp IS NULL"
	if whereClause != "" {
		query += " WHERE " + whereClause + " AND " + deletionFilter
	} else {
		query += " WHERE " + deletionFilter
	}

	query += " ORDER BY sa.namespace, sa.name"

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query service aliases")
	}
	defer rows.Close()

	for rows.Next() {
		serviceAlias, err := r.scanServiceAlias(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan service alias")
		}

		if err := consume(serviceAlias); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetServiceAliasByID gets a service alias by ID
func (r *Reader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	query := `
		SELECT sa.namespace, sa.name, sa.service_namespace, sa.service_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM service_aliases sa
		INNER JOIN k8s_metadata m ON sa.resource_version = m.resource_version
		WHERE sa.namespace = $1 AND sa.name = $2`

	row := r.queryRow(ctx, query, id.Namespace, id.Name)

	serviceAlias, err := r.scanServiceAliasRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan service alias")
	}

	return serviceAlias, nil
}

// scanServiceAlias scans a service alias from pgx.Rows
func (r *Reader) scanServiceAlias(rows pgx.Rows) (models.ServiceAlias, error) {
	var serviceAlias models.ServiceAlias
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database
	var serviceNamespace, serviceName string

	err := rows.Scan(
		&serviceAlias.Namespace,
		&serviceAlias.Name,
		&serviceNamespace,
		&serviceName,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return serviceAlias, err
	}

	// Convert K8s metadata (convert int64 to string)
	serviceAlias.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return serviceAlias, err
	}

	// Set SelfRef
	serviceAlias.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(serviceAlias.Name, models.WithNamespace(serviceAlias.Namespace)))

	// Set ServiceRef using the NamespacedObjectReference pattern
	serviceAlias.ServiceRef = models.NewServiceRef(serviceName, models.WithNamespace(serviceNamespace))

	return serviceAlias, nil
}

// scanServiceAliasRow scans a service alias from pgx.Row
func (r *Reader) scanServiceAliasRow(row pgx.Row) (*models.ServiceAlias, error) {
	var serviceAlias models.ServiceAlias
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database
	var serviceNamespace, serviceName string

	err := row.Scan(
		&serviceAlias.Namespace,
		&serviceAlias.Name,
		&serviceNamespace,
		&serviceName,
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
	serviceAlias.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}

	// Set SelfRef
	serviceAlias.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(serviceAlias.Name, models.WithNamespace(serviceAlias.Namespace)))

	// Set ServiceRef using the NamespacedObjectReference pattern
	serviceAlias.ServiceRef = models.NewServiceRef(serviceName, models.WithNamespace(serviceNamespace))

	return &serviceAlias, nil
}
