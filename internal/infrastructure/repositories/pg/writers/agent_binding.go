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

// SyncAgentBindings syncs agent bindings to PostgreSQL with K8s metadata support
func (w *Writer) SyncAgentBindings(ctx context.Context, agentBindings []models.AgentBinding, scope ports.Scope, options ...ports.Option) error {
	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteAgentBindingsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete agent bindings in scope")
		}
	}

	// Upsert all provided agent bindings
	for i := range agentBindings {
		// Initialize metadata fields (UID, Generation, ObservedGeneration)
		// This is what Memory backend does via ensureMetaFill() -> TouchOnCreate()
		// Without this, PATCH operations fail because objInfo.UpdatedObject() needs UID
		if agentBindings[i].Meta.UID == "" {
			agentBindings[i].Meta.TouchOnCreate()
		}

		if err := w.upsertAgentBinding(ctx, agentBindings[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert agent binding %s/%s", agentBindings[i].Namespace, agentBindings[i].Name)
		}
	}

	return nil
}

// upsertAgentBinding inserts or updates an agent binding with full K8s metadata support
func (w *Writer) upsertAgentBinding(ctx context.Context, agentBinding models.AgentBinding) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(agentBinding.Meta.Labels, agentBinding.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(agentBinding.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// Handle finalizers
	finalizers := agentBinding.Meta.Finalizers
	if finalizers == nil {
		finalizers = []string{}
	}

	// Marshal AgentItem to JSONB
	agentItemJSON, err := json.Marshal(agentBinding.AgentItem)
	if err != nil {
		return errors.Wrap(err, "failed to marshal agent item")
	}

	// Extract agent and address group references
	agentNamespace := agentBinding.Namespace // Assuming same namespace
	agentName := agentBinding.AgentRef.Name
	addressGroupNamespace := agentBinding.Namespace // Assuming same namespace
	addressGroupName := agentBinding.AddressGroupRef.Name

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
		INSERT INTO agent_bindings (
			namespace, name,
			agent_namespace, agent_name,
			address_group_namespace, address_group_name,
			agent_item, resource_version
		) SELECT $5, $6, $7, $8, $9, $10, $11, resource_version FROM metadata
		ON CONFLICT (namespace, name) DO UPDATE SET
			agent_namespace = EXCLUDED.agent_namespace,
			agent_name = EXCLUDED.agent_name,
			address_group_namespace = EXCLUDED.address_group_namespace,
			address_group_name = EXCLUDED.address_group_name,
			agent_item = EXCLUDED.agent_item,
			resource_version = EXCLUDED.resource_version`

	err = w.exec(ctx, query,
		labelsJSON,             // $1
		annotationsJSON,        // $2
		finalizers,             // $3
		conditionsJSON,         // $4
		agentBinding.Namespace, // $5
		agentBinding.Name,      // $6
		agentNamespace,         // $7
		agentName,              // $8
		addressGroupNamespace,  // $9
		addressGroupName,       // $10
		agentItemJSON,          // $11
	)
	if err != nil {
		return errors.Wrapf(err, "failed to upsert agent binding %s/%s", agentBinding.Namespace, agentBinding.Name)
	}

	return nil
}

// DeleteAgentBindingsByIDs deletes agent bindings by their resource identifiers
func (w *Writer) DeleteAgentBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
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
		DELETE FROM agent_bindings
		WHERE (namespace, name) IN (%s)`,
		strings.Join(placeholders, ", "))

	err := w.exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete agent bindings")
	}

	return nil
}

// deleteAgentBindingsInScope deletes all agent bindings within the specified scope
func (w *Writer) deleteAgentBindingsInScope(ctx context.Context, scope ports.Scope) error {
	var query string
	var args []interface{}

	// For namespace-scoped deletion
	if scope.String() != "" {
		query = "DELETE FROM agent_bindings WHERE namespace = $1"
		args = []interface{}{scope.String()}
	} else {
		// Full scope deletion
		query = "DELETE FROM agent_bindings"
	}

	err := w.exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete agent bindings in scope")
	}

	return nil
}
