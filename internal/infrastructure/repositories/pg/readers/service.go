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

// addressGroupRefJSON is an intermediate structure for JSONB unmarshaling
// This avoids importing K8s types in the repository layer
type addressGroupRefJSON struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

// aggregatedAddressGroupRefJSON is an intermediate structure for aggregated address groups
type aggregatedAddressGroupRefJSON struct {
	Ref    addressGroupRefJSON `json:"ref"`
	Source string              `json:"source"`
}

// ListServices lists services with K8s metadata support and relationship loading
func (r *Reader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	query := `
		SELECT s.namespace, s.name, s.description, s.ingress_ports,
		       s.address_groups, s.aggregated_address_groups,
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

		if err := consume(service); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetServiceByID gets a service by ID with full relationship loading
func (r *Reader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	query := `
		SELECT s.namespace, s.name, s.description, s.ingress_ports,
		       s.address_groups, s.aggregated_address_groups,
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

	return service, nil
}

// scanService scans a service from pgx.Rows
func (r *Reader) scanService(rows pgx.Rows) (models.Service, error) {
	var service models.Service
	var addressGroupsJSON, aggregatedAddressGroupsJSON []byte
	var ingressPortsJSON []byte
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

	// Parse address_groups from JSONB using intermediate structure
	if addressGroupsJSON != nil && len(addressGroupsJSON) > 0 && string(addressGroupsJSON) != "null" {
		var agRefs []addressGroupRefJSON
		if err := json.Unmarshal(addressGroupsJSON, &agRefs); err != nil {
			return service, errors.Wrap(err, "failed to parse address_groups JSON")
		}
		service.AddressGroups = make([]models.AddressGroupRef, len(agRefs))
		for i, ref := range agRefs {
			// Convert intermediate JSON structure to domain model
			service.AddressGroups[i] = models.NewAddressGroupRef(ref.Name, models.WithNamespace(ref.Namespace))
		}
	}

	// Parse aggregated_address_groups from JSONB using intermediate structure
	if aggregatedAddressGroupsJSON != nil && len(aggregatedAddressGroupsJSON) > 0 && string(aggregatedAddressGroupsJSON) != "null" {
		var aggregatedRefs []aggregatedAddressGroupRefJSON
		if err := json.Unmarshal(aggregatedAddressGroupsJSON, &aggregatedRefs); err != nil {
			return service, errors.Wrap(err, "failed to parse aggregated_address_groups JSON")
		}
		service.AggregatedAddressGroups = make([]models.AddressGroupReference, len(aggregatedRefs))
		for i, ref := range aggregatedRefs {
			// Convert intermediate JSON structure to domain model
			// Note: AddressGroupReference contains a NamespacedObjectReference which IS a K8s type
			// but it's created via models.NewAddressGroupRef which handles the conversion
			domainRef := models.NewAddressGroupRef(ref.Ref.Name, models.WithNamespace(ref.Ref.Namespace))
			service.AggregatedAddressGroups[i] = models.AddressGroupReference{
				Ref:    domainRef,
				Source: models.AddressGroupRegistrationSource(ref.Source),
			}
		}
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
	var addressGroupsJSON, aggregatedAddressGroupsJSON []byte
	var ingressPortsJSON []byte
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

	// Parse address_groups from JSONB using intermediate structure
	if addressGroupsJSON != nil && len(addressGroupsJSON) > 0 && string(addressGroupsJSON) != "null" {
		var agRefs []addressGroupRefJSON
		if err := json.Unmarshal(addressGroupsJSON, &agRefs); err != nil {
			return nil, errors.Wrap(err, "failed to parse address_groups JSON")
		}
		service.AddressGroups = make([]models.AddressGroupRef, len(agRefs))
		for i, ref := range agRefs {
			// Convert intermediate JSON structure to domain model
			service.AddressGroups[i] = models.NewAddressGroupRef(ref.Name, models.WithNamespace(ref.Namespace))
		}
	}

	// Parse aggregated_address_groups from JSONB using intermediate structure
	if aggregatedAddressGroupsJSON != nil && len(aggregatedAddressGroupsJSON) > 0 && string(aggregatedAddressGroupsJSON) != "null" {
		var aggregatedRefs []aggregatedAddressGroupRefJSON
		if err := json.Unmarshal(aggregatedAddressGroupsJSON, &aggregatedRefs); err != nil {
			return nil, errors.Wrap(err, "failed to parse aggregated_address_groups JSON")
		}
		service.AggregatedAddressGroups = make([]models.AddressGroupReference, len(aggregatedRefs))
		for i, ref := range aggregatedRefs {
			// Convert intermediate JSON structure to domain model
			domainRef := models.NewAddressGroupRef(ref.Ref.Name, models.WithNamespace(ref.Ref.Namespace))
			service.AggregatedAddressGroups[i] = models.AddressGroupReference{
				Ref:    domainRef,
				Source: models.AddressGroupRegistrationSource(ref.Source),
			}
		}
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
