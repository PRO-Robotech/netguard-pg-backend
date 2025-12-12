package readers

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/utils"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"time"
)

func (r *Reader) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	query := `
		SELECT n.namespace, n.name, n.cidr::text, n.comment, n.network_items, n.is_bound,
		       n.binding_ref_namespace, n.binding_ref_name,
		       n.address_group_ref_namespace, n.address_group_ref_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
		FROM networks n
		INNER JOIN k8s_metadata m ON n.resource_version = m.resource_version`
	whereClause, args, err := utils.BuildScopeFilterWithTable(scope, "networks", "n")
	if err != nil {
		return errors.Wrap(err, "failed to build scope filter")
	}

	if whereClause != "" {
		query += " WHERE " + whereClause
	} else {
	}
	query += " ORDER BY n.namespace, n.name"
	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query networks")
	}
	defer rows.Close()
	for rows.Next() {
		network, err := r.scanNetwork(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan network")
		}
		if err := consume(network); err != nil {
			return err
		}
	}
	return rows.Err()
}
func (r *Reader) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	query := `
		SELECT n.namespace, n.name, n.cidr::text, n.comment, n.network_items, n.is_bound,
		       n.binding_ref_namespace, n.binding_ref_name,
		       n.address_group_ref_namespace, n.address_group_ref_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
		FROM networks n
		INNER JOIN k8s_metadata m ON n.resource_version = m.resource_version
		WHERE n.namespace = $1 AND n.name = $2`
	row := r.queryRow(ctx, query, id.Namespace, id.Name)
	network, err := r.scanNetworkRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan network")
	}
	return network, nil
}
func (r *Reader) scanNetwork(rows pgx.Rows) (models.Network, error) {
	var network models.Network
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var cidr string
	var networkItemsJSON []byte
	var isBound bool
	var bindingRefNamespace, bindingRefName *string
	var addressGroupRefNamespace, addressGroupRefName *string
	err := rows.Scan(
		&network.Namespace,
		&network.Name,
		&cidr,
		&network.Comment,
		&networkItemsJSON,
		&isBound,
		&bindingRefNamespace,
		&bindingRefName,
		&addressGroupRefNamespace,
		&addressGroupRefName,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
		&deletionTS,
	)
	if err != nil {
		return network, err
	}
	network.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return network, err
	}
	network.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(network.Name, models.WithNamespace(network.Namespace)))
	network.CIDR = cidr
	network.IsBound = isBound
	if bindingRefNamespace != nil && bindingRefName != nil {
		network.BindingRef = &v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "NetworkBinding",
				Name:       *bindingRefName,
			},
			Namespace: *bindingRefNamespace,
		}
	}
	if addressGroupRefNamespace != nil && addressGroupRefName != nil {
		network.AddressGroupRef = &v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       *addressGroupRefName,
			},
			Namespace: *addressGroupRefNamespace,
		}
	}
	return network, nil
}
func (r *Reader) scanNetworkRow(row pgx.Row) (*models.Network, error) {
	var network models.Network
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var cidr string
	var networkItemsJSON []byte
	var isBound bool
	var bindingRefNamespace, bindingRefName *string
	var addressGroupRefNamespace, addressGroupRefName *string
	err := row.Scan(
		&network.Namespace,
		&network.Name,
		&cidr,
		&network.Comment,
		&networkItemsJSON,
		&isBound,
		&bindingRefNamespace,
		&bindingRefName,
		&addressGroupRefNamespace,
		&addressGroupRefName,
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
	network.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return nil, err
	}
	network.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(network.Name, models.WithNamespace(network.Namespace)))
	network.CIDR = cidr
	network.IsBound = isBound
	if bindingRefNamespace != nil && bindingRefName != nil {
		network.BindingRef = &v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "NetworkBinding",
				Name:       *bindingRefName,
			},
			Namespace: *bindingRefNamespace,
		}
	}
	if addressGroupRefNamespace != nil && addressGroupRefName != nil {
		network.AddressGroupRef = &v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       *addressGroupRefName,
			},
			Namespace: *addressGroupRefNamespace,
		}
	}
	return &network, nil
}
func (r *Reader) GetNetworkByCIDR(ctx context.Context, cidr string) (*models.Network, error) {
	query := `
		SELECT n.namespace, n.name, n.cidr::text, n.comment, n.network_items, n.is_bound,
		       n.binding_ref_namespace, n.binding_ref_name,
		       n.address_group_ref_namespace, n.address_group_ref_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
		FROM networks n
		INNER JOIN k8s_metadata m ON n.resource_version = m.resource_version
		WHERE n.cidr = $1::CIDR`
	row := r.queryRow(ctx, query, cidr)
	network, err := r.scanNetworkRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan network by CIDR")
	}
	return network, nil
}
func (r *Reader) GetNetworksOverlappingCIDR(ctx context.Context, cidr string) ([]*models.Network, error) {
	query := `
		SELECT
			n.namespace,
			n.name,
			n.cidr::text,
			n.comment,
			n.network_items,
			n.is_bound,
			n.binding_ref_namespace,
			n.binding_ref_name,
			n.address_group_ref_namespace,
			n.address_group_ref_name,
			m.resource_version,
			m.labels,
			m.annotations,
			m.conditions,
			m.created_at,
			m.updated_at
		FROM networks n
		INNER JOIN k8s_metadata m ON n.resource_version = m.resource_version
		WHERE n.cidr && $1::CIDR
		  AND m.deletion_timestamp IS NULL
		ORDER BY n.namespace, n.name`
	rows, err := r.query(ctx, query, cidr)
	if err != nil {
		return nil, fmt.Errorf("failed to query overlapping networks for CIDR %s: %w", cidr, err)
	}
	defer rows.Close()
	var networks []*models.Network
	for rows.Next() {
		network, err := r.scanNetwork(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan network: %w", err)
		}
		networks = append(networks, &network)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating networks: %w", err)
	}
	return networks, nil
}
