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
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// ListNetworks lists networks with K8s metadata support
func (r *Reader) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	query := `
		SELECT n.namespace, n.name, n.network_items, n.is_bound,
		       n.binding_ref_namespace, n.binding_ref_name,
		       n.address_group_ref_namespace, n.address_group_ref_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM networks n
		INNER JOIN k8s_metadata m ON n.resource_version = m.resource_version`

	// Apply scope filtering
	whereClause, args := utils.BuildScopeFilter(scope, "n")
	if whereClause != "" {
		query += " WHERE " + whereClause
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

// GetNetworkByID gets a network by ID
func (r *Reader) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	query := `
		SELECT n.namespace, n.name, n.network_items, n.is_bound,
		       n.binding_ref_namespace, n.binding_ref_name,
		       n.address_group_ref_namespace, n.address_group_ref_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
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

// scanNetwork scans a network from pgx.Rows
func (r *Reader) scanNetwork(rows pgx.Rows) (models.Network, error) {
	var network models.Network
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// Network-specific fields
	var networkItemsJSON []byte                               // JSONB for NetworkItem[]
	var isBound bool                                          // Boolean field
	var bindingRefNamespace, bindingRefName *string           // Nullable references
	var addressGroupRefNamespace, addressGroupRefName *string // Nullable references

	err := rows.Scan(
		&network.Namespace,
		&network.Name,
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
	)
	if err != nil {
		return network, err
	}

	// Convert K8s metadata (convert int64 to string)
	network.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return network, err
	}

	// Set SelfRef
	network.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(network.Name, models.WithNamespace(network.Namespace)))

	// Parse network items from JSONB - extract CIDR from first item
	if len(networkItemsJSON) > 0 {
		networkItems, err := utils.ParseNetworkItems(networkItemsJSON)
		if err != nil {
			return network, errors.Wrap(err, "failed to unmarshal network_items")
		}
		if len(networkItems) > 0 {
			network.CIDR = networkItems[0].CIDR // Use CIDR from first NetworkItem
		}
	}

	// Set Network-specific fields
	network.IsBound = isBound

	// Build ObjectReferences from separate namespace/name columns
	if bindingRefNamespace != nil && bindingRefName != nil {
		network.BindingRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "NetworkBinding",
			Name:       *bindingRefName,
		}
	}

	if addressGroupRefNamespace != nil && addressGroupRefName != nil {
		network.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       *addressGroupRefName,
		}
	}

	return network, nil
}

// scanNetworkRow scans a network from pgx.Row
func (r *Reader) scanNetworkRow(row pgx.Row) (*models.Network, error) {
	var network models.Network
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// Network-specific fields
	var networkItemsJSON []byte                               // JSONB for NetworkItem[]
	var isBound bool                                          // Boolean field
	var bindingRefNamespace, bindingRefName *string           // Nullable references
	var addressGroupRefNamespace, addressGroupRefName *string // Nullable references

	err := row.Scan(
		&network.Namespace,
		&network.Name,
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
	)
	if err != nil {
		return nil, err
	}

	// Convert K8s metadata (convert int64 to string)
	network.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}

	// Set SelfRef
	network.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(network.Name, models.WithNamespace(network.Namespace)))

	// Parse network items from JSONB - extract CIDR from first item
	if len(networkItemsJSON) > 0 {
		networkItems, err := utils.ParseNetworkItems(networkItemsJSON)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal network_items")
		}
		if len(networkItems) > 0 {
			network.CIDR = networkItems[0].CIDR // Use CIDR from first NetworkItem
		}
	}

	// Set Network-specific fields
	network.IsBound = isBound

	// Build ObjectReferences from separate namespace/name columns
	if bindingRefNamespace != nil && bindingRefName != nil {
		network.BindingRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "NetworkBinding",
			Name:       *bindingRefName,
		}
	}

	if addressGroupRefNamespace != nil && addressGroupRefName != nil {
		network.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       *addressGroupRefName,
		}
	}

	return &network, nil
}
