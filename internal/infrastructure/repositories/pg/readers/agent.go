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

// ListAgents lists agents with K8s metadata support
func (r *Reader) ListAgents(ctx context.Context, consume func(models.Agent) error, scope ports.Scope) error {
	query := `
		SELECT a.namespace, a.name, a.uuid, a.hostname, a.is_bound,
		       a.binding_ref_namespace, a.binding_ref_name,
		       a.address_group_ref_namespace, a.address_group_ref_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM agents a
		INNER JOIN k8s_metadata m ON a.resource_version = m.resource_version`

	// Apply scope filtering
	whereClause, args := utils.BuildScopeFilter(scope, "a")
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	query += " ORDER BY a.namespace, a.name"

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query agents")
	}
	defer rows.Close()

	for rows.Next() {
		agent, err := r.scanAgent(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan agent")
		}

		if err := consume(agent); err != nil {
			return err
		}
	}

	return rows.Err()
}

// GetAgentByID gets an agent by ID
func (r *Reader) GetAgentByID(ctx context.Context, id models.ResourceIdentifier) (*models.Agent, error) {
	query := `
		SELECT a.namespace, a.name, a.uuid, a.hostname, a.is_bound,
		       a.binding_ref_namespace, a.binding_ref_name,
		       a.address_group_ref_namespace, a.address_group_ref_name,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM agents a
		INNER JOIN k8s_metadata m ON a.resource_version = m.resource_version
		WHERE a.namespace = $1 AND a.name = $2`

	row := r.queryRow(ctx, query, id.Namespace, id.Name)

	agent, err := r.scanAgentRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan agent")
	}

	return agent, nil
}

// scanAgent scans an agent from pgx.Rows
func (r *Reader) scanAgent(rows pgx.Rows) (models.Agent, error) {
	var agent models.Agent
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// Agent-specific fields
	var uuid, hostname string                                 // Agent identification fields
	var isBound bool                                          // Boolean field
	var bindingRefNamespace, bindingRefName *string           // Nullable references
	var addressGroupRefNamespace, addressGroupRefName *string // Nullable references

	err := rows.Scan(
		&agent.Namespace,
		&agent.Name,
		&uuid,
		&hostname,
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
		return agent, errors.Wrap(err, "failed to scan agent row")
	}

	// Set agent-specific fields
	agent.UUID = uuid
	agent.Name = hostname // Note: hostname is stored as name in database
	agent.IsBound = isBound

	// Handle nullable binding references
	if bindingRefNamespace != nil && bindingRefName != nil {
		agent.BindingRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AgentBinding",
			Name:       *bindingRefName,
		}
	}

	// Handle nullable address group references
	if addressGroupRefNamespace != nil && addressGroupRefName != nil {
		agent.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       *addressGroupRefName,
		}
	}

	// Convert K8s metadata (convert int64 to string)
	agent.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return agent, errors.Wrap(err, "failed to convert K8s metadata")
	}

	// Set SelfRef
	agent.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(agent.Name, models.WithNamespace(agent.Namespace)))

	return agent, nil
}

// scanAgentRow scans an agent from pgx.Row
func (r *Reader) scanAgentRow(row pgx.Row) (*models.Agent, error) {
	var agent models.Agent
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// Agent-specific fields
	var uuid, hostname string                                 // Agent identification fields
	var isBound bool                                          // Boolean field
	var bindingRefNamespace, bindingRefName *string           // Nullable references
	var addressGroupRefNamespace, addressGroupRefName *string // Nullable references

	err := row.Scan(
		&agent.Namespace,
		&agent.Name,
		&uuid,
		&hostname,
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
		return nil, errors.Wrap(err, "failed to scan agent row")
	}

	// Set agent-specific fields
	agent.UUID = uuid
	agent.Name = hostname // Note: hostname is stored as name in database
	agent.IsBound = isBound

	// Handle nullable binding references
	if bindingRefNamespace != nil && bindingRefName != nil {
		agent.BindingRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AgentBinding",
			Name:       *bindingRefName,
		}
	}

	// Handle nullable address group references
	if addressGroupRefNamespace != nil && addressGroupRefName != nil {
		agent.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       *addressGroupRefName,
		}
	}

	// Convert K8s metadata (convert int64 to string)
	agent.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert K8s metadata")
	}

	// Set SelfRef
	agent.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(agent.Name, models.WithNamespace(agent.Namespace)))

	return &agent, nil
}
