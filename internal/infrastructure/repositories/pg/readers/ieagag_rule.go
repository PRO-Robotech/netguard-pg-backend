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
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// ListIEAgAgRules lists IEAgAgRule resources with K8s metadata support
func (r *Reader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	query := `
		SELECT ier.namespace, ier.name, ier.transport, ier.traffic, ier.action,
		       ier.address_group_local_namespace, ier.address_group_local_name,
		       ier.address_group_namespace, ier.address_group_name, ier.ports,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM ie_ag_ag_rules ier
		INNER JOIN k8s_metadata m ON ier.resource_version = m.resource_version`

	// Apply scope filtering
	whereClause, args := utils.BuildScopeFilter(scope, "ier")
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	query += " ORDER BY ier.namespace, ier.name"

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query ieagag rules")
	}
	defer rows.Close()

	for rows.Next() {
		ieagagRule, err := r.scanIEAgAgRule(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan ieagag rule")
		}

		if err := consume(ieagagRule); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetIEAgAgRuleByID gets an IEAgAgRule resource by ID
func (r *Reader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	query := `
		SELECT ier.namespace, ier.name, ier.transport, ier.traffic, ier.action,
		       ier.address_group_local_namespace, ier.address_group_local_name,
		       ier.address_group_namespace, ier.address_group_name, ier.ports,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM ie_ag_ag_rules ier
		INNER JOIN k8s_metadata m ON ier.resource_version = m.resource_version
		WHERE ier.namespace = $1 AND ier.name = $2`

	row := r.queryRow(ctx, query, id.Namespace, id.Name)

	ieagagRule, err := r.scanIEAgAgRuleRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan ieagag rule")
	}

	return ieagagRule, nil
}

// scanIEAgAgRule scans an IEAgAgRule resource from pgx.Rows
func (r *Reader) scanIEAgAgRule(rows pgx.Rows) (models.IEAgAgRule, error) {
	var ieagagRule models.IEAgAgRule
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// IEAgAgRule-specific fields - separate namespace/name columns
	var transport string                                         // TransportProtocol enum as string
	var traffic string                                           // Traffic enum as string
	var action string                                            // RuleAction enum as string
	var addressGroupLocalNamespace, addressGroupLocalName string // AddressGroupLocal fields
	var addressGroupNamespace, addressGroupName string           // AddressGroup fields
	var portsJSON []byte                                         // JSONB for array of PortSpec

	err := rows.Scan(
		&ieagagRule.Namespace,
		&ieagagRule.Name,
		&transport,
		&traffic,
		&action,
		&addressGroupLocalNamespace,
		&addressGroupLocalName,
		&addressGroupNamespace,
		&addressGroupName,
		&portsJSON,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return ieagagRule, err
	}

	// Convert K8s metadata (convert int64 to string)
	ieagagRule.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return ieagagRule, err
	}

	// Set SelfRef
	ieagagRule.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(ieagagRule.Name, models.WithNamespace(ieagagRule.Namespace)))

	// Set enum fields
	ieagagRule.Transport = models.TransportProtocol(transport)
	ieagagRule.Traffic = models.Traffic(traffic)
	ieagagRule.Action = models.RuleAction(action)

	// Build NamespacedObjectReference from separate namespace/name columns
	if addressGroupLocalNamespace != "" && addressGroupLocalName != "" {
		ieagagRule.AddressGroupLocal = netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       addressGroupLocalName,
			},
			Namespace: addressGroupLocalNamespace,
		}
	}

	if addressGroupNamespace != "" && addressGroupName != "" {
		ieagagRule.AddressGroup = netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       addressGroupName,
			},
			Namespace: addressGroupNamespace,
		}
	}

	// Parse array of PortSpec from JSONB
	if len(portsJSON) > 0 && string(portsJSON) != "null" {
		if err := json.Unmarshal(portsJSON, &ieagagRule.Ports); err != nil {
			return ieagagRule, errors.Wrap(err, "failed to unmarshal ports")
		}
	}

	return ieagagRule, nil
}

// scanIEAgAgRuleRow scans an IEAgAgRule resource from pgx.Row
func (r *Reader) scanIEAgAgRuleRow(row pgx.Row) (*models.IEAgAgRule, error) {
	var ieagagRule models.IEAgAgRule
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// IEAgAgRule-specific fields - separate namespace/name columns
	var transport string                                         // TransportProtocol enum as string
	var traffic string                                           // Traffic enum as string
	var action string                                            // RuleAction enum as string
	var addressGroupLocalNamespace, addressGroupLocalName string // AddressGroupLocal fields
	var addressGroupNamespace, addressGroupName string           // AddressGroup fields
	var portsJSON []byte                                         // JSONB for array of PortSpec

	err := row.Scan(
		&ieagagRule.Namespace,
		&ieagagRule.Name,
		&transport,
		&traffic,
		&action,
		&addressGroupLocalNamespace,
		&addressGroupLocalName,
		&addressGroupNamespace,
		&addressGroupName,
		&portsJSON,
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
	ieagagRule.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}

	// Set SelfRef
	ieagagRule.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(ieagagRule.Name, models.WithNamespace(ieagagRule.Namespace)))

	// Set enum fields
	ieagagRule.Transport = models.TransportProtocol(transport)
	ieagagRule.Traffic = models.Traffic(traffic)
	ieagagRule.Action = models.RuleAction(action)

	// Build NamespacedObjectReference from separate namespace/name columns
	if addressGroupLocalNamespace != "" && addressGroupLocalName != "" {
		ieagagRule.AddressGroupLocal = netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       addressGroupLocalName,
			},
			Namespace: addressGroupLocalNamespace,
		}
	}

	if addressGroupNamespace != "" && addressGroupName != "" {
		ieagagRule.AddressGroup = netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       addressGroupName,
			},
			Namespace: addressGroupNamespace,
		}
	}

	// Parse array of PortSpec from JSONB
	if len(portsJSON) > 0 && string(portsJSON) != "null" {
		if err := json.Unmarshal(portsJSON, &ieagagRule.Ports); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal ports")
		}
	}

	return &ieagagRule, nil
}
