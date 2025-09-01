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

// ListAgentBindings lists agent bindings with K8s metadata support
func (r *Reader) ListAgentBindings(ctx context.Context, consume func(models.AgentBinding) error, scope ports.Scope) error {
	query := `
		SELECT ab.namespace, ab.name,
		       ab.agent_namespace, ab.agent_name,
		       ab.address_group_namespace, ab.address_group_name,
		       ab.agent_item,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM agent_bindings ab
		INNER JOIN k8s_metadata m ON ab.resource_version = m.resource_version`

	// Apply scope filtering
	whereClause, args := utils.BuildScopeFilter(scope, "ab")
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	query += " ORDER BY ab.namespace, ab.name"

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query agent bindings")
	}
	defer rows.Close()

	for rows.Next() {
		agentBinding, err := r.scanAgentBinding(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan agent binding")
		}

		if err := consume(agentBinding); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetAgentBindingByID gets an agent binding by ID
func (r *Reader) GetAgentBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AgentBinding, error) {
	query := `
		SELECT ab.namespace, ab.name,
		       ab.agent_namespace, ab.agent_name,
		       ab.address_group_namespace, ab.address_group_name,
		       ab.agent_item,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM agent_bindings ab
		INNER JOIN k8s_metadata m ON ab.resource_version = m.resource_version
		WHERE ab.namespace = $1 AND ab.name = $2`

	row := r.queryRow(ctx, query, id.Namespace, id.Name)

	agentBinding, err := r.scanAgentBindingRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan agent binding")
	}

	return agentBinding, nil
}

// scanAgentBinding scans an agent binding from pgx.Rows
func (r *Reader) scanAgentBinding(rows pgx.Rows) (models.AgentBinding, error) {
	var agentBinding models.AgentBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// AgentBinding-specific fields
	var agentNamespace, agentName string               // Agent reference
	var addressGroupNamespace, addressGroupName string // AddressGroup reference
	var agentItemJSON []byte                           // JSONB for AgentItem

	err := rows.Scan(
		&agentBinding.Namespace,
		&agentBinding.Name,
		&agentNamespace,
		&agentName,
		&addressGroupNamespace,
		&addressGroupName,
		&agentItemJSON,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return agentBinding, errors.Wrap(err, "failed to scan agent binding row")
	}

	// Set agent reference
	agentBinding.AgentRef = netguardv1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "Agent",
		Name:       agentName,
	}

	// Set address group reference
	agentBinding.AddressGroupRef = netguardv1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "AddressGroup",
		Name:       addressGroupName,
	}

	// Parse AgentItem from JSONB
	if len(agentItemJSON) > 0 {
		var agentItem models.AgentItem
		if err := json.Unmarshal(agentItemJSON, &agentItem); err != nil {
			return agentBinding, errors.Wrap(err, "failed to unmarshal agent item")
		}
		agentBinding.AgentItem = agentItem
	}

	// Convert K8s metadata (convert int64 to string)
	agentBinding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return agentBinding, errors.Wrap(err, "failed to convert K8s metadata")
	}

	// Set SelfRef
	agentBinding.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(agentBinding.Name, models.WithNamespace(agentBinding.Namespace)))

	return agentBinding, nil
}

// scanAgentBindingRow scans an agent binding from pgx.Row
func (r *Reader) scanAgentBindingRow(row pgx.Row) (*models.AgentBinding, error) {
	var agentBinding models.AgentBinding
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// AgentBinding-specific fields
	var agentNamespace, agentName string               // Agent reference
	var addressGroupNamespace, addressGroupName string // AddressGroup reference
	var agentItemJSON []byte                           // JSONB for AgentItem

	err := row.Scan(
		&agentBinding.Namespace,
		&agentBinding.Name,
		&agentNamespace,
		&agentName,
		&addressGroupNamespace,
		&addressGroupName,
		&agentItemJSON,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to scan agent binding row")
	}

	// Set agent reference
	agentBinding.AgentRef = netguardv1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "Agent",
		Name:       agentName,
	}

	// Set address group reference
	agentBinding.AddressGroupRef = netguardv1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "AddressGroup",
		Name:       addressGroupName,
	}

	// Parse AgentItem from JSONB
	if len(agentItemJSON) > 0 {
		var agentItem models.AgentItem
		if err := json.Unmarshal(agentItemJSON, &agentItem); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal agent item")
		}
		agentBinding.AgentItem = agentItem
	}

	// Convert K8s metadata (convert int64 to string)
	agentBinding.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert K8s metadata")
	}

	// Set SelfRef
	agentBinding.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(agentBinding.Name, models.WithNamespace(agentBinding.Namespace)))

	return &agentBinding, nil
}
