package writers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// SyncAgents syncs agents to PostgreSQL with K8s metadata support
func (w *Writer) SyncAgents(ctx context.Context, agents []models.Agent, scope ports.Scope, options ...ports.Option) error {
	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteAgentsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete agents in scope")
		}
	}

	// Upsert all provided agents
	for i := range agents {
		// Initialize metadata fields (UID, Generation, ObservedGeneration)
		// This is what Memory backend does via ensureMetaFill() -> TouchOnCreate()
		// Without this, PATCH operations fail because objInfo.UpdatedObject() needs UID
		if agents[i].Meta.UID == "" {
			agents[i].Meta.TouchOnCreate()
		}

		if err := w.upsertAgent(ctx, agents[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert agent %s/%s", agents[i].Namespace, agents[i].Name)
		}
	}

	return nil
}

// upsertAgent inserts or updates an agent with full K8s metadata support
func (w *Writer) upsertAgent(ctx context.Context, agent models.Agent) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(agent.Meta.Labels, agent.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(agent.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// Handle finalizers
	finalizers := agent.Meta.Finalizers
	if finalizers == nil {
		finalizers = []string{}
	}

	// Handle nullable reference fields
	var bindingRefNamespace, bindingRefName *string
	if agent.BindingRef != nil {
		bindingRefNamespace = &agent.Namespace // Bindings are in same namespace
		bindingRefName = &agent.BindingRef.Name
	}

	var addressGroupRefNamespace, addressGroupRefName *string
	if agent.AddressGroupRef != nil {
		addressGroupRefNamespace = &agent.Namespace // AddressGroups are in same namespace
		addressGroupRefName = &agent.AddressGroupRef.Name
	}

	query := `
		WITH metadata AS (
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (resource_version) DO UPDATE SET
				labels = EXCLUDED.labels,
				annotations = EXCLUDED.annotations,
				finalizers = EXCLUDED.finalizers,
				conditions = EXCLUDED.conditions,
				updated_at = NOW()
			RETURNING resource_version
		)
		INSERT INTO agents (
			namespace, name, uuid, hostname, is_bound,
			binding_ref_namespace, binding_ref_name,
			address_group_ref_namespace, address_group_ref_name,
			resource_version
		) SELECT $5, $6, $7, $8, $9, $10, $11, $12, $13, resource_version FROM metadata
		ON CONFLICT (namespace, name) DO UPDATE SET
			uuid = EXCLUDED.uuid,
			hostname = EXCLUDED.hostname,
			is_bound = EXCLUDED.is_bound,
			binding_ref_namespace = EXCLUDED.binding_ref_namespace,
			binding_ref_name = EXCLUDED.binding_ref_name,
			address_group_ref_namespace = EXCLUDED.address_group_ref_namespace,
			address_group_ref_name = EXCLUDED.address_group_ref_name,
			resource_version = EXCLUDED.resource_version`

	err = w.exec(ctx, query,
		labelsJSON,               // $1
		annotationsJSON,          // $2
		finalizers,               // $3
		conditionsJSON,           // $4
		agent.Namespace,          // $5
		agent.Name,               // $6
		agent.UUID,               // $7
		agent.Name,               // $8 (hostname stored as name)
		agent.IsBound,            // $9
		bindingRefNamespace,      // $10
		bindingRefName,           // $11
		addressGroupRefNamespace, // $12
		addressGroupRefName,      // $13
	)
	if err != nil {
		return errors.Wrapf(err, "failed to upsert agent %s/%s", agent.Namespace, agent.Name)
	}

	return nil
}

// DeleteAgentsByIDs deletes agents by their resource identifiers
func (w *Writer) DeleteAgentsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if len(ids) == 0 {
		return nil
	}

	// Build dynamic query for batch delete
	placeholders := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids)*2)

	for i, id := range ids {
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d)", i*2+1, i*2+2))
		args = append(args, id.Namespace, id.Name)
	}

	query := fmt.Sprintf(`
		DELETE FROM agents
		WHERE (namespace, name) IN (%s)`,
		strings.Join(placeholders, ", "))

	err := w.exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete agents")
	}

	return nil
}

// deleteAgentsInScope deletes all agents within the specified scope
func (w *Writer) deleteAgentsInScope(ctx context.Context, scope ports.Scope) error {
	var query string
	var args []interface{}

	// For namespace-scoped deletion
	if scope.String() != "" {
		query = "DELETE FROM agents WHERE namespace = $1"
		args = []interface{}{scope.String()}
	} else {
		// Full scope deletion
		query = "DELETE FROM agents"
	}

	err := w.exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete agents in scope")
	}

	return nil
}
