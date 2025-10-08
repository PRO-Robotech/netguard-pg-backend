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
)

// ListAddressGroups lists address groups with K8s metadata support
func (r *Reader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	query := `
		SELECT ag.namespace, ag.name, ag.default_action, ag.logs, ag.trace, ag.description, ag.networks, ag.hosts, ag.aggregated_hosts,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM address_groups ag
		INNER JOIN k8s_metadata m ON ag.resource_version = m.resource_version`

	// Apply scope filtering and deletion_timestamp filter
	whereClause, args := utils.BuildScopeFilter(scope, "ag")

	// Always filter out objects being deleted
	deletionFilter := "m.deletion_timestamp IS NULL"
	if whereClause != "" {
		query += " WHERE " + whereClause + " AND " + deletionFilter
	} else {
		query += " WHERE " + deletionFilter
	}

	query += " ORDER BY ag.namespace, ag.name"

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query address groups")
	}
	defer rows.Close()

	for rows.Next() {
		addressGroup, err := r.scanAddressGroup(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan address group")
		}

		if err := consume(addressGroup); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetAddressGroupByID gets an address group by ID
func (r *Reader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	query := `
		SELECT ag.namespace, ag.name, ag.default_action, ag.logs, ag.trace, ag.description, ag.networks, ag.hosts, ag.aggregated_hosts,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM address_groups ag
		INNER JOIN k8s_metadata m ON ag.resource_version = m.resource_version
		WHERE ag.namespace = $1 AND ag.name = $2`

	row := r.queryRow(ctx, query, id.Namespace, id.Name)

	addressGroup, err := r.scanAddressGroupRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan address group")
	}

	return addressGroup, nil
}

// scanAddressGroup scans an address group from pgx.Rows
func (r *Reader) scanAddressGroup(rows pgx.Rows) (models.AddressGroup, error) {
	var addressGroup models.AddressGroup
	var labelsJSON, annotationsJSON, conditionsJSON, networksJSON, hostsJSON, aggregatedHostsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database
	var description string

	err := rows.Scan(
		&addressGroup.Namespace,
		&addressGroup.Name,
		&addressGroup.DefaultAction,
		&addressGroup.Logs,
		&addressGroup.Trace,
		&description,
		&networksJSON,
		&hostsJSON,
		&aggregatedHostsJSON,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return addressGroup, err
	}

	// Unmarshal Networks field (critical fix for Networks field persistence)
	if len(networksJSON) > 0 {
		if err := json.Unmarshal(networksJSON, &addressGroup.Networks); err != nil {
			return addressGroup, errors.Wrap(err, "failed to unmarshal networks")
		}
		// Ensure Kind and ApiVersion are set for all NetworkItems
		for i := range addressGroup.Networks {
			if addressGroup.Networks[i].Kind == "" {
				addressGroup.Networks[i].Kind = "Network"
			}
			if addressGroup.Networks[i].ApiVersion == "" {
				addressGroup.Networks[i].ApiVersion = "netguard.sgroups.io/v1beta1"
			}
		}
	}

	if len(hostsJSON) > 0 {
		if err := json.Unmarshal(hostsJSON, &addressGroup.Hosts); err != nil {
			return addressGroup, errors.Wrap(err, "failed to unmarshal hosts")
		}
	}

	if len(aggregatedHostsJSON) > 0 {
		if err := json.Unmarshal(aggregatedHostsJSON, &addressGroup.AggregatedHosts); err != nil {
			return addressGroup, errors.Wrap(err, "failed to unmarshal aggregated_hosts")
		}
	}

	// Convert K8s metadata (convert int64 to string)
	addressGroup.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return addressGroup, err
	}

	// Set SelfRef
	addressGroup.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(addressGroup.Name, models.WithNamespace(addressGroup.Namespace)))

	if addressGroup.Namespace != "" {
		addressGroup.AddressGroupName = fmt.Sprintf("%s/%s", addressGroup.Namespace, addressGroup.Name)
	} else {
		addressGroup.AddressGroupName = addressGroup.Name
	}

	return addressGroup, nil
}

// scanAddressGroupRow scans an address group from pgx.Row
func (r *Reader) scanAddressGroupRow(row pgx.Row) (*models.AddressGroup, error) {
	var addressGroup models.AddressGroup
	var labelsJSON, annotationsJSON, conditionsJSON, networksJSON, hostsJSON, aggregatedHostsJSON []byte
	var createdAt, updatedAt time.Time
	var resourceVersion int64
	var description string

	err := row.Scan(
		&addressGroup.Namespace,
		&addressGroup.Name,
		&addressGroup.DefaultAction,
		&addressGroup.Logs,
		&addressGroup.Trace,
		&description,
		&networksJSON,
		&hostsJSON,
		&aggregatedHostsJSON,
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

	// Unmarshal Networks field (critical fix for Networks field persistence)
	if len(networksJSON) > 0 {
		if err := json.Unmarshal(networksJSON, &addressGroup.Networks); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal networks")
		}
		// Ensure Kind and ApiVersion are set for all NetworkItems
		for i := range addressGroup.Networks {
			if addressGroup.Networks[i].Kind == "" {
				addressGroup.Networks[i].Kind = "Network"
			}
			if addressGroup.Networks[i].ApiVersion == "" {
				addressGroup.Networks[i].ApiVersion = "netguard.sgroups.io/v1beta1"
			}
		}
	}

	if len(hostsJSON) > 0 {
		if err := json.Unmarshal(hostsJSON, &addressGroup.Hosts); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal hosts")
		}
	}

	if len(aggregatedHostsJSON) > 0 {
		if err := json.Unmarshal(aggregatedHostsJSON, &addressGroup.AggregatedHosts); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal aggregated_hosts")
		}
	}

	// Convert K8s metadata (convert int64 to string)
	addressGroup.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}

	// Set SelfRef
	addressGroup.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(addressGroup.Name, models.WithNamespace(addressGroup.Namespace)))

	if addressGroup.Namespace != "" {
		addressGroup.AddressGroupName = fmt.Sprintf("%s/%s", addressGroup.Namespace, addressGroup.Name)
	} else {
		addressGroup.AddressGroupName = addressGroup.Name
	}

	return &addressGroup, nil
}
