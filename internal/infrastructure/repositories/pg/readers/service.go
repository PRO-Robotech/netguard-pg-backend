package readers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/utils"
)

// ListServices lists services with K8s metadata support and relationship loading
func (r *Reader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	query := `
		SELECT s.namespace, s.name, s.description, s.ingress_ports, s.address_groups, s.aggregated_address_groups,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM services s
		INNER JOIN k8s_metadata m ON s.resource_version = m.resource_version`

	// Apply scope filtering
	whereClause, args := utils.BuildScopeFilter(scope, "s")
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	query += " ORDER BY s.namespace, s.name"

	var rows pgx.Rows
	var err error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		rows, err = r.query(ctx, query, args...)
		if err == nil {
			break
		}

		if strings.Contains(err.Error(), "conn busy") && attempt < maxRetries-1 {
			time.Sleep(time.Duration(10*(1<<attempt)) * time.Millisecond)
			continue
		}

		return errors.Wrap(err, "failed to query services")
	}
	defer rows.Close()

	for rows.Next() {
		service, err := r.scanService(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan service")
		}

		var loadErr error
		maxRetries := 3
		for attempt := 0; attempt < maxRetries; attempt++ {
			loadErr = r.loadServiceAddressGroups(ctx, &service)
			if loadErr == nil {
				break
			}

			if strings.Contains(loadErr.Error(), "conn busy") && attempt < maxRetries-1 {
				time.Sleep(time.Duration(10*(1<<attempt)) * time.Millisecond)
				continue
			}

			return errors.Wrap(loadErr, "failed to load service address groups")
		}

		if err := consume(service); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetServiceByID gets a service by ID with full relationship loading
func (r *Reader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	query := `
		SELECT s.namespace, s.name, s.description, s.ingress_ports, s.address_groups, s.aggregated_address_groups,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM services s
		INNER JOIN k8s_metadata m ON s.resource_version = m.resource_version
		WHERE s.namespace = $1 AND s.name = $2`

	// Retry mechanism for "conn busy" errors on main query
	var service *models.Service
	var err error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		row := r.queryRow(ctx, query, id.Namespace, id.Name)
		service, err = r.scanServiceRow(row)
		if err == nil {
			break
		}

		if strings.Contains(err.Error(), "conn busy") && attempt < maxRetries-1 {
			time.Sleep(time.Duration(10*(1<<attempt)) * time.Millisecond)
			continue
		}

		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan service")
	}

	var loadErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		loadErr = r.loadServiceAddressGroups(ctx, service)
		if loadErr == nil {
			break
		}

		if strings.Contains(loadErr.Error(), "conn busy") && attempt < maxRetries-1 {
			time.Sleep(time.Duration(10*(1<<attempt)) * time.Millisecond)
			continue
		}

		return nil, errors.Wrap(loadErr, "failed to load service address groups")
	}

	return service, nil
}

// scanService scans a service from pgx.Rows
func (r *Reader) scanService(rows pgx.Rows) (models.Service, error) {
	var service models.Service
	var ingressPortsJSON, addressGroupsJSON, aggregatedAddressGroupsJSON []byte
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	err := rows.Scan(
		&service.Namespace,
		&service.Name,
		&service.Description,
		&ingressPortsJSON,
		&addressGroupsJSON,
		&aggregatedAddressGroupsJSON,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return service, err
	}

	// Parse ingress ports
	service.IngressPorts, err = utils.ParseIngressPorts(ingressPortsJSON)
	if err != nil {
		return service, err
	}

	service.XAggregatedAddressGroups, err = r.parseAggregatedAddressGroups(aggregatedAddressGroupsJSON)
	if err != nil {
		return service, err
	}

	// Convert K8s metadata (convert int64 to string) - skip finalizers for now
	service.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return service, err
	}

	// Set SelfRef
	service.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(service.Name, models.WithNamespace(service.Namespace)))

	return service, nil
}

// scanServiceRow scans a service from pgx.Row
func (r *Reader) scanServiceRow(row pgx.Row) (*models.Service, error) {
	var service models.Service
	var ingressPortsJSON, addressGroupsJSON, aggregatedAddressGroupsJSON []byte
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	err := row.Scan(
		&service.Namespace,
		&service.Name,
		&service.Description,
		&ingressPortsJSON,
		&addressGroupsJSON,
		&aggregatedAddressGroupsJSON,
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

	// Parse ingress ports
	service.IngressPorts, err = utils.ParseIngressPorts(ingressPortsJSON)
	if err != nil {
		return nil, err
	}

	service.XAggregatedAddressGroups, err = r.parseAggregatedAddressGroups(aggregatedAddressGroupsJSON)
	if err != nil {
		return nil, err
	}

	// Convert K8s metadata (convert int64 to string) - skip finalizers for now
	service.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}

	// Set SelfRef
	service.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(service.Name, models.WithNamespace(service.Namespace)))

	return &service, nil
}

func (r *Reader) parseAggregatedAddressGroups(jsonData []byte) ([]models.AddressGroupReference, error) {
	if len(jsonData) == 0 {
		return nil, nil
	}

	var aggregatedGroups []models.AddressGroupReference
	if err := json.Unmarshal(jsonData, &aggregatedGroups); err != nil {
		return nil, errors.Wrap(err, "failed to parse aggregated address groups")
	}

	return aggregatedGroups, nil
}

func (r *Reader) loadServiceAddressGroups(ctx context.Context, service *models.Service) error {
	query := `
		SELECT address_group_namespace, address_group_name
		FROM address_group_bindings
		WHERE service_namespace = $1 AND service_name = $2`

	var rows pgx.Rows
	var err error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		rows, err = r.query(ctx, query, service.Namespace, service.Name)
		if err == nil {
			break
		}

		if strings.Contains(err.Error(), "conn busy") && attempt < maxRetries-1 {
			time.Sleep(time.Duration(10*(1<<attempt)) * time.Millisecond)
			continue
		}

		return errors.Wrap(err, "failed to query address group bindings")
	}
	defer rows.Close()

	service.AddressGroups = []models.AddressGroupRef{}
	for rows.Next() {
		var namespace, name string
		if err := rows.Scan(&namespace, &name); err != nil {
			return errors.Wrap(err, "failed to scan address group binding")
		}

		service.AddressGroups = append(service.AddressGroups,
			models.NewAddressGroupRef(name, models.WithNamespace(namespace)))
	}

	return rows.Err()
}
