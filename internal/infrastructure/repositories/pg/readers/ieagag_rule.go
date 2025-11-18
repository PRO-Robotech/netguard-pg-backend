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
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"time"
)

func (r *Reader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	query := `
		SELECT ier.namespace, ier.name, ier.transport, ier.traffic, ier.action,
		       ier.address_group_local_namespace, ier.address_group_local_name,
		       ier.address_group_namespace, ier.address_group_name, ier.ports,
		       ier.trace,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
		FROM ie_ag_ag_rules ier
		INNER JOIN k8s_metadata m ON ier.resource_version = m.resource_version`
	whereClause, args, err := utils.BuildScopeFilterWithTable(scope, "ie_ag_ag_rules", "ier")
	if err != nil {
		return errors.Wrap(err, "failed to build scope filter")
	}

	if whereClause != "" {
		query += " WHERE " + whereClause
	} else {
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
func (r *Reader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	query := `
		SELECT ier.namespace, ier.name, ier.transport, ier.traffic, ier.action,
		       ier.address_group_local_namespace, ier.address_group_local_name,
		       ier.address_group_namespace, ier.address_group_name, ier.ports,
		       ier.trace,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
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
func (r *Reader) scanIEAgAgRule(rows pgx.Rows) (models.IEAgAgRule, error) {
	var ieagagRule models.IEAgAgRule
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var transport string
	var traffic string
	var action string
	var addressGroupLocalNamespace, addressGroupLocalName string
	var addressGroupNamespace, addressGroupName string
	var portsJSON []byte
	var trace bool
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
		&trace,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
		&deletionTS,
	)
	if err != nil {
		return ieagagRule, err
	}
	ieagagRule.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return ieagagRule, err
	}
	ieagagRule.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(ieagagRule.Name, models.WithNamespace(ieagagRule.Namespace)))
	ieagagRule.Transport = models.TransportProtocol(transport)
	ieagagRule.Traffic = models.Traffic(traffic)
	ieagagRule.Action = models.RuleAction(action)
	ieagagRule.Trace = trace
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
	if len(portsJSON) > 0 && string(portsJSON) != "null" {
		if err := json.Unmarshal(portsJSON, &ieagagRule.Ports); err != nil {
			return ieagagRule, errors.Wrap(err, "failed to unmarshal ports")
		}
	}
	return ieagagRule, nil
}
func (r *Reader) scanIEAgAgRuleRow(row pgx.Row) (*models.IEAgAgRule, error) {
	var ieagagRule models.IEAgAgRule
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var transport string
	var traffic string
	var action string
	var addressGroupLocalNamespace, addressGroupLocalName string
	var addressGroupNamespace, addressGroupName string
	var portsJSON []byte
	var trace bool
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
		&trace,
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
	ieagagRule.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return nil, err
	}
	ieagagRule.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(ieagagRule.Name, models.WithNamespace(ieagagRule.Namespace)))
	ieagagRule.Transport = models.TransportProtocol(transport)
	ieagagRule.Traffic = models.Traffic(traffic)
	ieagagRule.Action = models.RuleAction(action)
	ieagagRule.Trace = trace
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
	if len(portsJSON) > 0 && string(portsJSON) != "null" {
		if err := json.Unmarshal(portsJSON, &ieagagRule.Ports); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal ports")
		}
	}
	return &ieagagRule, nil
}
