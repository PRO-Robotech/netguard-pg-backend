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

// ListAddressGroupBindingPolicies lists address group binding policies with K8s metadata support
func (r *Reader) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	query := `
		SELECT agbp.namespace, agbp.name, agbp.address_group_ref, agbp.service_ref,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM address_group_binding_policies agbp
		INNER JOIN k8s_metadata m ON agbp.resource_version = m.resource_version`

	// Apply scope filtering and deletion_timestamp filter
	whereClause, args := utils.BuildScopeFilter(scope, "agbp")

	// Always filter out objects being deleted
	deletionFilter := "m.deletion_timestamp IS NULL"
	if whereClause != "" {
		query += " WHERE " + whereClause + " AND " + deletionFilter
	} else {
		query += " WHERE " + deletionFilter
	}

	query += " ORDER BY agbp.namespace, agbp.name"

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query address group binding policies")
	}
	defer rows.Close()

	for rows.Next() {
		policy, err := r.scanAddressGroupBindingPolicy(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan address group binding policy")
		}

		if err := consume(policy); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetAddressGroupBindingPolicyByID gets an address group binding policy by ID
func (r *Reader) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	query := `
		SELECT agbp.namespace, agbp.name, agbp.address_group_ref, agbp.service_ref,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM address_group_binding_policies agbp
		INNER JOIN k8s_metadata m ON agbp.resource_version = m.resource_version
		WHERE agbp.namespace = $1 AND agbp.name = $2`

	row := r.queryRow(ctx, query, id.Namespace, id.Name)

	policy, err := r.scanAddressGroupBindingPolicyRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan address group binding policy")
	}

	return policy, nil
}

// scanAddressGroupBindingPolicy scans an address group binding policy from pgx.Rows
func (r *Reader) scanAddressGroupBindingPolicy(rows pgx.Rows) (models.AddressGroupBindingPolicy, error) {
	var policy models.AddressGroupBindingPolicy
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// Policy-specific fields
	var addressGroupRefJSON []byte // JSONB for NamespacedObjectReference
	var serviceRefJSON []byte      // JSONB for NamespacedObjectReference

	err := rows.Scan(
		&policy.Namespace,
		&policy.Name,
		&addressGroupRefJSON,
		&serviceRefJSON,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return policy, err
	}

	// Convert K8s metadata (convert int64 to string)
	policy.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return policy, err
	}

	// Set SelfRef
	policy.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(policy.Name, models.WithNamespace(policy.Namespace)))

	// Parse NamespacedObjectReference fields from JSONB
	if len(addressGroupRefJSON) > 0 {
		if err := json.Unmarshal(addressGroupRefJSON, &policy.AddressGroupRef); err != nil {
			return policy, errors.Wrap(err, "failed to unmarshal address_group_ref")
		}
	}

	if len(serviceRefJSON) > 0 {
		if err := json.Unmarshal(serviceRefJSON, &policy.ServiceRef); err != nil {
			return policy, errors.Wrap(err, "failed to unmarshal service_ref")
		}
	}

	return policy, nil
}

// scanAddressGroupBindingPolicyRow scans an address group binding policy from pgx.Row
func (r *Reader) scanAddressGroupBindingPolicyRow(row pgx.Row) (*models.AddressGroupBindingPolicy, error) {
	var policy models.AddressGroupBindingPolicy
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// Policy-specific fields
	var addressGroupRefJSON []byte // JSONB for NamespacedObjectReference
	var serviceRefJSON []byte      // JSONB for NamespacedObjectReference

	err := row.Scan(
		&policy.Namespace,
		&policy.Name,
		&addressGroupRefJSON,
		&serviceRefJSON,
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
	policy.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}

	// Set SelfRef
	policy.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(policy.Name, models.WithNamespace(policy.Namespace)))

	// Parse NamespacedObjectReference fields from JSONB
	if len(addressGroupRefJSON) > 0 {
		if err := json.Unmarshal(addressGroupRefJSON, &policy.AddressGroupRef); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal address_group_ref")
		}
	}

	if len(serviceRefJSON) > 0 {
		if err := json.Unmarshal(serviceRefJSON, &policy.ServiceRef); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal service_ref")
		}
	}

	return &policy, nil
}
