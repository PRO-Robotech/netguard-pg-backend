package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// SyncRuleS2S syncs RuleS2S resources to PostgreSQL with K8s metadata support
func (w *Writer) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, options ...ports.Option) error {
	// Check for condition-only operation and sync operation
	isConditionOnly := false
	var syncOp models.SyncOp = models.SyncOpUpsert // Default to upsert

	for _, opt := range options {
		if _, ok := opt.(ports.ConditionOnlyOperation); ok {
			isConditionOnly = true
		}
		if syncOpOpt, ok := opt.(ports.SyncOption); ok {
			syncOp = syncOpOpt.Operation
		}
	}

	// For condition-only operations, only update k8s_metadata conditions
	if isConditionOnly {
		for _, rule := range rules {
			if err := w.updateRuleS2SConditionsOnly(ctx, rule); err != nil {
				return errors.Wrapf(err, "failed to update conditions for rule s2s %s/%s", rule.Namespace, rule.Name)
			}
		}
		return nil
	}

	// Handle DELETE operations properly by calling DeleteRuleS2SByIDs
	if syncOp == models.SyncOpDelete {
		var idsToDelete []models.ResourceIdentifier
		for _, rule := range rules {
			idsToDelete = append(idsToDelete, rule.ResourceIdentifier)
		}

		if err := w.DeleteRuleS2SByIDs(ctx, idsToDelete); err != nil {
			return errors.Wrap(err, "failed to delete rule s2s by IDs")
		}

		return nil
	}

	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteRuleS2SInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete rule s2s in scope")
		}
	}

	// Upsert all provided rules (for CREATE/UPDATE operations)
	for i := range rules {
		// Initialize metadata fields if not set
		if rules[i].Meta.UID == "" {
			rules[i].Meta.TouchOnCreate()
		}

		if err := w.upsertRuleS2S(ctx, rules[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert rule s2s %s/%s", rules[i].Namespace, rules[i].Name)
		}
	}

	return nil
}

// upsertRuleS2S inserts or updates a rule s2s with full K8s metadata support
func (w *Writer) upsertRuleS2S(ctx context.Context, rule models.RuleS2S) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(rule.Meta.Labels, rule.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(rule.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if rule s2s exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM rule_s2s WHERE namespace = $1 AND name = $2`
	_ = w.tx.QueryRow(ctx, existingQuery, rule.Namespace, rule.Name).Scan(&existingResourceVersion)

	var resourceVersion int64
	if existingResourceVersion.Valid {
		// UPDATE existing K8s metadata
		metadataQuery := `
			UPDATE k8s_metadata
			SET labels = $1, annotations = $2, conditions = $3, updated_at = NOW()
			WHERE resource_version = $4
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, existingResourceVersion.Int64).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to update K8s metadata for rule s2s %s/%s", rule.Namespace, rule.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, '{}', $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for rule s2s %s/%s", rule.Namespace, rule.Name)
		}
	}

	// Marshal reference fields to JSON
	serviceLocalRefJSON, err := json.Marshal(rule.ServiceLocalRef)
	if err != nil {
		return errors.Wrap(err, "failed to marshal service_local_ref")
	}

	serviceRefJSON, err := json.Marshal(rule.ServiceRef)
	if err != nil {
		return errors.Wrap(err, "failed to marshal service_ref")
	}

	// Marshal IEAgAgRuleRefs array to JSON
	var ieagagRuleRefsJSON []byte
	if len(rule.IEAgAgRuleRefs) > 0 {
		ieagagRuleRefsJSON, err = json.Marshal(rule.IEAgAgRuleRefs)
		if err != nil {
			return errors.Wrap(err, "failed to marshal ieagag_rule_refs")
		}
	} else {
		ieagagRuleRefsJSON = []byte("[]")
	}

	// Then, upsert the rule s2s using the resource version
	ruleQuery := `
		INSERT INTO rule_s2s (namespace, name, traffic, service_local_ref, service_ref, ieagag_rule_refs, trace, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (namespace, name) DO UPDATE SET
			traffic = $3,
			service_local_ref = $4,
			service_ref = $5,
			ieagag_rule_refs = $6,
			trace = $7,
			resource_version = $8`

	if err := w.exec(ctx, ruleQuery,
		rule.Namespace,
		rule.Name,
		string(rule.Traffic),
		serviceLocalRefJSON,
		serviceRefJSON,
		ieagagRuleRefsJSON,
		rule.Trace,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert rule s2s %s/%s", rule.Namespace, rule.Name)
	}

	return nil
}

// deleteRuleS2SInScope deletes rule s2s that match the provided scope
func (w *Writer) deleteRuleS2SInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "rs")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM rule_s2s rs WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete rule s2s in scope")
	}

	return nil
}

// DeleteRuleS2SByIDs deletes RuleS2S resources by their identifiers
func (w *Writer) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	if len(ids) == 0 {
		return nil
	}

	// Build parameter placeholders and collect args
	var conditions []string
	var args []interface{}
	argIndex := 1

	for _, id := range ids {
		conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	// First, mark objects as being deleted in k8s_metadata to prevent re-creation by ListWatch
	markDeleteQuery := `
		UPDATE k8s_metadata m
		SET deletion_timestamp = NOW()
		FROM rule_s2s rs
		WHERE rs.resource_version = m.resource_version
		  AND (%s)
		  AND m.deletion_timestamp IS NULL`

	markQuery := fmt.Sprintf(markDeleteQuery, strings.Join(conditions, " OR "))
	_, err := w.tx.Exec(ctx, markQuery, args...)
	if err != nil {
		// Log but don't fail - deletion_timestamp is optional for now
		klog.V(4).InfoS("Failed to mark rule s2s as deleting in k8s_metadata", "error", err.Error())
	}

	// Then delete from rule_s2s table
	query := fmt.Sprintf(`
		DELETE FROM rule_s2s WHERE %s`,
		strings.Join(conditions, " OR "))

	// Execute the delete and capture the result
	result, err := w.tx.Exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete rule s2s by identifiers")
	}

	rowsAffected := result.RowsAffected()

	// Still track affected rows in the writer
	w.addAffectedRows(rowsAffected)

	return nil
}

// updateRuleS2SConditionsOnly updates only the conditions in k8s_metadata for condition-only operations
// This avoids the UID conflict issues when ConditionManager runs after main transaction commit
func (w *Writer) updateRuleS2SConditionsOnly(ctx context.Context, rule models.RuleS2S) error {
	// Marshal only the conditions we need to update
	conditionsJSON, err := json.Marshal(rule.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// Find the existing rule's resource_version by namespace/name
	var resourceVersion int64
	findQuery := `SELECT resource_version FROM rule_s2s WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, findQuery, rule.Namespace, rule.Name).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to find rule s2s %s/%s for condition update", rule.Namespace, rule.Name)
	}

	// Use simpler approach with shorter timeout and better error handling
	conditionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Update only the conditions in k8s_metadata using the resource_version
	conditionUpdateQuery := `
		UPDATE k8s_metadata
		SET conditions = $1, updated_at = NOW()
		WHERE resource_version = $2`

	if err := w.exec(conditionCtx, conditionUpdateQuery, conditionsJSON, resourceVersion); err != nil {
		return errors.Wrapf(err, "failed to update conditions for rule s2s %s/%s", rule.Namespace, rule.Name)
	}

	return nil
}
