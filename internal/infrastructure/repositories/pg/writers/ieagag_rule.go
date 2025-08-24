package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// SyncIEAgAgRules syncs IEAgAgRule resources to PostgreSQL with K8s metadata support
func (w *Writer) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope, options ...ports.Option) error {
	// 🔧 CONDITION FIX: Handle ConditionOnlyOperation like service.go
	var isConditionOnly bool
	for _, opt := range options {
		if _, ok := opt.(ports.ConditionOnlyOperation); ok {
			isConditionOnly = true
			fmt.Printf("🔧 DEBUG: Detected ConditionOnlyOperation for IEAgAgRule sync\n")
		}
	}

	// 🚨 CRITICAL: For condition-only operations, only update k8s_metadata conditions
	// Don't touch the main ieagag_rules table - just update conditions in the existing metadata
	if isConditionOnly {
		fmt.Printf("🚧 DEBUG: ConditionOnly operation detected - updating IEAgAgRule conditions only...\n")

		for _, rule := range rules {
			if err := w.updateIEAgAgRuleConditionsOnly(ctx, rule); err != nil {
				return errors.Wrapf(err, "failed to update conditions for IEAgAgRule %s/%s", rule.Namespace, rule.Name)
			}
		}
		fmt.Printf("✅ DEBUG: IEAgAgRule condition-only update completed successfully\n")
		return nil
	}

	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteIEAgAgRulesInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete ieagag rules in scope")
		}
	}

	// Upsert all provided rules
	for i := range rules {
		// 🔧 CRITICAL FIX: Initialize metadata fields (UID, Generation, ObservedGeneration)
		// This is what Memory backend does via ensureMetaFill() -> TouchOnCreate()
		// Without this, PATCH operations fail because objInfo.UpdatedObject() needs UID
		// IMPORTANT: Use index-based loop to modify original, not copy!
		if rules[i].Meta.UID == "" {
			rules[i].Meta.TouchOnCreate()
		}

		if err := w.upsertIEAgAgRule(ctx, rules[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert ieagag rule %s/%s", rules[i].Namespace, rules[i].Name)
		}
	}

	return nil
}

// upsertIEAgAgRule inserts or updates an ieagag rule with full K8s metadata support
func (w *Writer) upsertIEAgAgRule(ctx context.Context, rule models.IEAgAgRule) error {
	log.Printf("🗄️ POSTGRES DEBUG: upsertIEAgAgRule called for %s/%s", rule.Namespace, rule.Name)

	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(rule.Meta.Labels, rule.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(rule.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if ieagag rule exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM ie_ag_ag_rules WHERE namespace = $1 AND name = $2`
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
			return errors.Wrapf(err, "failed to update K8s metadata for ieagag rule %s/%s", rule.Namespace, rule.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, '{}', $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for ieagag rule %s/%s", rule.Namespace, rule.Name)
		}
	}

	// Marshal ports array to JSON
	var portsJSON []byte
	if len(rule.Ports) > 0 {
		portsJSON, err = json.Marshal(rule.Ports)
		if err != nil {
			return errors.Wrap(err, "failed to marshal ports")
		}
	} else {
		portsJSON = []byte("[]")
	}

	// Then, upsert the ieagag rule using the resource version (table name: ie_ag_ag_rules)
	ruleQuery := `
		INSERT INTO ie_ag_ag_rules (namespace, name, transport, traffic, 
			address_group_local_namespace, address_group_local_name,
			address_group_namespace, address_group_name, 
			ports, action, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (namespace, name) DO UPDATE SET
			transport = $3,
			traffic = $4,
			address_group_local_namespace = $5,
			address_group_local_name = $6,
			address_group_namespace = $7,
			address_group_name = $8,
			ports = $9,
			action = $10,
			resource_version = $11`

	if err := w.exec(ctx, ruleQuery,
		rule.Namespace,
		rule.Name,
		string(rule.Transport),
		string(rule.Traffic),
		rule.AddressGroupLocal.Namespace,
		rule.AddressGroupLocal.Name,
		rule.AddressGroup.Namespace,
		rule.AddressGroup.Name,
		portsJSON,
		string(rule.Action),
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert ieagag rule %s/%s", rule.Namespace, rule.Name)
	}

	return nil
}

// updateIEAgAgRuleConditionsOnly updates only the conditions in k8s_metadata for condition-only operations
// This avoids the UID conflict issues when ConditionManager runs after main transaction commit
func (w *Writer) updateIEAgAgRuleConditionsOnly(ctx context.Context, rule models.IEAgAgRule) error {
	fmt.Printf("🔧 DEBUG: updateIEAgAgRuleConditionsOnly for %s/%s\n", rule.Namespace, rule.Name)

	// Marshal only the conditions we need to update
	conditionsJSON, err := json.Marshal(rule.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// Find the existing rule's resource_version by namespace/name
	var resourceVersion int64
	findQuery := `SELECT resource_version FROM ie_ag_ag_rules WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, findQuery, rule.Namespace, rule.Name).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to find IEAgAgRule %s/%s for condition update", rule.Namespace, rule.Name)
	}

	fmt.Printf("🔍 DEBUG: Found IEAgAgRule %s/%s with resource_version=%d, updating conditions only\n", rule.Namespace, rule.Name, resourceVersion)

	// 🎯 OPTIMIZED_FIX: Use simpler approach with shorter timeout and better error handling
	conditionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	fmt.Printf("🔧 OPTIMIZED_FIX: Starting condition update for IEAgAgRule %s/%s with 3s timeout\n", rule.Namespace, rule.Name)

	// Update only the conditions in k8s_metadata using the resource_version
	conditionUpdateQuery := `
		UPDATE k8s_metadata 
		SET conditions = $1, updated_at = NOW()
		WHERE resource_version = $2`

	result, err := w.tx.Exec(conditionCtx, conditionUpdateQuery, conditionsJSON, resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to update conditions for IEAgAgRule %s/%s", rule.Namespace, rule.Name)
	}

	rowsAffected := result.RowsAffected()
	fmt.Printf("🔍 DEBUG: UPDATE k8s_metadata conditions affected %d rows for resource_version=%d\n", rowsAffected, resourceVersion)

	if rowsAffected == 0 {
		fmt.Printf("⚠️  WARNING: No rows affected when updating conditions for IEAgAgRule %s/%s with resource_version=%d\n", rule.Namespace, rule.Name, resourceVersion)
		// Let's also check if the metadata row exists
		var count int
		checkQuery := `SELECT COUNT(*) FROM k8s_metadata WHERE resource_version = $1`
		if err := w.tx.QueryRow(ctx, checkQuery, resourceVersion).Scan(&count); err == nil {
			fmt.Printf("🔍 DEBUG: Found %d k8s_metadata rows for resource_version=%d\n", count, resourceVersion)
		}
	}

	fmt.Printf("✅ DEBUG: Successfully updated conditions for IEAgAgRule %s/%s\n", rule.Namespace, rule.Name)
	return nil
}

// deleteIEAgAgRulesInScope deletes ieagag rules that match the provided scope
func (w *Writer) deleteIEAgAgRulesInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "ier")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM ie_ag_ag_rules ier WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete ieagag rules in scope")
	}

	return nil
}

// DeleteIEAgAgRulesByIDs deletes IEAgAgRule resources by their identifiers
func (w *Writer) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
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

	query := fmt.Sprintf(`
		DELETE FROM ie_ag_ag_rules WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete ieagag rules by identifiers")
	}

	return nil
}
