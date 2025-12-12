package readers

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/utils"
	"strings"
	"time"
)

func (r *Reader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	query := `
		SELECT agb.namespace, agb.name, agb.service_namespace, agb.service_name,
			   agb.address_group_namespace, agb.address_group_name, agb.comment,
			   m.resource_version, m.uid::text, m.labels, m.annotations, m.conditions,
		       m.created_at, m.updated_at, m.deletion_timestamp
		FROM address_group_bindings agb
		INNER JOIN k8s_metadata m ON agb.resource_version = m.resource_version
		WHERE m.deletion_timestamp IS NULL`
	whereClause, args, err := utils.BuildScopeFilterWithTable(scope, "address_group_bindings", "agb")
	if err != nil {
		return errors.Wrap(err, "failed to build scope filter")
	}

	if whereClause != "" {
		query += " AND " + whereClause
	} else {
	}
	query += " ORDER BY agb.namespace, agb.name"
	var rows pgx.Rows
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
func (r *Reader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	query := `
		SELECT agb.namespace, agb.name, agb.service_namespace, agb.service_name,
			   agb.address_group_namespace, agb.address_group_name, agb.comment,
			   m.resource_version, m.uid::text, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
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
func (r *Reader) scanAddressGroupBinding(rows pgx.Rows) (models.AddressGroupBinding, error) {
	var binding models.AddressGroupBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var uid string
	var serviceNamespace, serviceName string
	var addressGroupNamespace, addressGroupName string
	err := rows.Scan(
		&binding.Namespace,
		&binding.Name,
		&serviceNamespace,
		&serviceName,
		&addressGroupNamespace,
		&addressGroupName,
		&binding.Comment,
		&resourceVersion,
		&uid,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
		&deletionTS,
	)
	if err != nil {
		return binding, err
	}
	binding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return binding, err
	}
	binding.Meta.UID = uid
	binding.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(binding.Name, models.WithNamespace(binding.Namespace)))
	binding.ServiceRef = models.NewServiceRef(serviceName, models.WithNamespace(serviceNamespace))
	binding.AddressGroupRef = models.NewAddressGroupRef(addressGroupName, models.WithNamespace(addressGroupNamespace))
	return binding, nil
}
func (r *Reader) scanAddressGroupBindingRow(row pgx.Row) (*models.AddressGroupBinding, error) {
	var binding models.AddressGroupBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var uid string
	var serviceNamespace, serviceName string
	var addressGroupNamespace, addressGroupName string
	err := row.Scan(
		&binding.Namespace,
		&binding.Name,
		&serviceNamespace,
		&serviceName,
		&addressGroupNamespace,
		&addressGroupName,
		&binding.Comment,
		&resourceVersion,
		&uid,
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
	binding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return nil, err
	}
	binding.Meta.UID = uid
	binding.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(binding.Name, models.WithNamespace(binding.Namespace)))
	binding.ServiceRef = models.NewServiceRef(serviceName, models.WithNamespace(serviceNamespace))
	binding.AddressGroupRef = models.NewAddressGroupRef(addressGroupName, models.WithNamespace(addressGroupNamespace))
	return &binding, nil
}
