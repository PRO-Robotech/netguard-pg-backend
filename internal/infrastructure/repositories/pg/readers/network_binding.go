package readers

import (
	"context"
	"fmt"
	"time"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/utils"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
)

func (r *Reader) ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope ports.Scope) error {
	query := `
		SELECT nb.namespace, nb.name,
		       nb.network_namespace, nb.network_name,
		       nb.address_group_namespace, nb.address_group_name,
			   m.resource_version, m.uid, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
		FROM network_bindings nb
		INNER JOIN k8s_metadata m ON nb.resource_version = m.resource_version`
	whereClause, args, err := utils.BuildScopeFilterWithTable(scope, "network_bindings", "nb")
	if err != nil {
		return errors.Wrap(err, "failed to build scope filter")
	}

	if whereClause != "" {
		query += " WHERE " + whereClause
	} else {
	}
	query += " ORDER BY nb.namespace, nb.name"
	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query network bindings")
	}
	defer rows.Close()
	for rows.Next() {
		networkBinding, err := r.scanNetworkBinding(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan network binding")
		}
		if err := consume(networkBinding); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *Reader) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	query := `
		SELECT nb.namespace, nb.name,
		       nb.network_namespace, nb.network_name,
		       nb.address_group_namespace, nb.address_group_name,
			   m.resource_version, m.uid, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
		FROM network_bindings nb
		INNER JOIN k8s_metadata m ON nb.resource_version = m.resource_version
		WHERE nb.namespace = $1 AND nb.name = $2`
	row := r.queryRow(ctx, query, id.Namespace, id.Name)
	networkBinding, err := r.scanNetworkBindingRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan network binding")
	}
	return networkBinding, nil
}

func (r *Reader) scanNetworkBinding(rows pgx.Rows) (models.NetworkBinding, error) {
	var networkBinding models.NetworkBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var uid string
	var networkNamespace, networkName string
	var addressGroupNamespace, addressGroupName string
	err := rows.Scan(
		&networkBinding.Namespace,
		&networkBinding.Name,
		&networkNamespace,
		&networkName,
		&addressGroupNamespace,
		&addressGroupName,
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
		return networkBinding, err
	}
	networkBinding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return networkBinding, err
	}
	networkBinding.Meta.UID = uid
	networkBinding.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(networkBinding.Name, models.WithNamespace(networkBinding.Namespace)))
	if networkNamespace != "" && networkName != "" {
		networkBinding.NetworkRef = netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Network",
				Name:       networkName,
			},
			Namespace: networkNamespace,
		}
	}
	if addressGroupNamespace != "" && addressGroupName != "" {
		networkBinding.AddressGroupRef = netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       addressGroupName,
			},
			Namespace: addressGroupNamespace,
		}
	}
	return networkBinding, nil
}

func (r *Reader) scanNetworkBindingRow(row pgx.Row) (*models.NetworkBinding, error) {
	var networkBinding models.NetworkBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var uid string
	var networkNamespace, networkName string
	var addressGroupNamespace, addressGroupName string
	err := row.Scan(
		&networkBinding.Namespace,
		&networkBinding.Name,
		&networkNamespace,
		&networkName,
		&addressGroupNamespace,
		&addressGroupName,
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
	networkBinding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return nil, err
	}
	networkBinding.Meta.UID = uid
	networkBinding.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(networkBinding.Name, models.WithNamespace(networkBinding.Namespace)))
	if networkNamespace != "" && networkName != "" {
		networkBinding.NetworkRef = netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Network",
				Name:       networkName,
			},
			Namespace: networkNamespace,
		}
	}
	if addressGroupNamespace != "" && addressGroupName != "" {
		networkBinding.AddressGroupRef = netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       addressGroupName,
			},
			Namespace: addressGroupNamespace,
		}
	}

	return &networkBinding, nil
}
