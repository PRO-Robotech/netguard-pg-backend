package readers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/utils"
	"time"
)

func (r *Reader) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	query := `
		SELECT agbp.namespace, agbp.name, agbp.address_group_ref, agbp.service_ref,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
		FROM address_group_binding_policies agbp
		INNER JOIN k8s_metadata m ON agbp.resource_version = m.resource_version`
	whereClause, args, err := utils.BuildScopeFilterWithTable(scope, "address_group_binding_policies", "agbp")
	if err != nil {
		return errors.Wrap(err, "failed to build scope filter")
	}

	if whereClause != "" {
		query += " WHERE " + whereClause
	} else {
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
func (r *Reader) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	query := `
		SELECT agbp.namespace, agbp.name, agbp.address_group_ref, agbp.service_ref,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
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
func (r *Reader) scanAddressGroupBindingPolicy(rows pgx.Rows) (models.AddressGroupBindingPolicy, error) {
	var policy models.AddressGroupBindingPolicy
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var addressGroupRefJSON []byte
	var serviceRefJSON []byte
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
		&deletionTS,
	)
	if err != nil {
		return policy, err
	}
	policy.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return policy, err
	}
	policy.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(policy.Name, models.WithNamespace(policy.Namespace)))
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
func (r *Reader) scanAddressGroupBindingPolicyRow(row pgx.Row) (*models.AddressGroupBindingPolicy, error) {
	var policy models.AddressGroupBindingPolicy
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var addressGroupRefJSON []byte
	var serviceRefJSON []byte
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
		&deletionTS,
	)
	if err != nil {
		return nil, err
	}
	policy.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return nil, err
	}
	policy.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(policy.Name, models.WithNamespace(policy.Namespace)))
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
