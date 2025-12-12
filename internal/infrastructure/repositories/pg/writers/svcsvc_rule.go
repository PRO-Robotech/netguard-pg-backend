package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// serviceRefJSON represents the JSON structure for service references in the database
// This avoids importing K8s types in the repository layer
type svcSvcRuleServiceRefJSON struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

// SyncSvcSvcRules syncs SvcSvcRule resources to PostgreSQL with K8s metadata support
func (w *Writer) SyncSvcSvcRules(ctx context.Context, rules []models.SvcSvcRule, scope ports.Scope, options ...ports.Option) error {
	// Check for sync operation
	var syncOp models.SyncOp = models.SyncOpUpsert // Default to upsert
	isConditionOnly := false

	for _, opt := range options {
		if syncOpOpt, ok := opt.(ports.SyncOption); ok {
			syncOp = syncOpOpt.Operation
		}
		if _, ok := opt.(ports.ConditionOnlyOperation); ok {
			isConditionOnly = true
		}
	}

	// Handle condition-only operations - update ONLY conditions, NO outbox entry
	if isConditionOnly {
		for _, rule := range rules {
			if err := w.updateSvcSvcRuleConditionsOnly(ctx, rule); err != nil {
				return errors.Wrapf(err, "failed to update conditions for rule %s/%s", rule.Namespace, rule.Name)
			}
		}
		return nil
	}

	// Handle DELETE operations
	if syncOp == models.SyncOpDelete {
		var idsToDelete []models.ResourceIdentifier
		for _, rule := range rules {
			idsToDelete = append(idsToDelete, rule.ResourceIdentifier)
		}

		if err := w.deleteSvcSvcRulesByIdentifiers(ctx, idsToDelete); err != nil {
			return errors.Wrap(err, "failed to delete svc svc rules by IDs")
		}

		return nil
	}

	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteSvcSvcRulesInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete svc svc rules in scope")
		}
	}

	// Upsert all provided rules (for CREATE/UPDATE operations)
	for i := range rules {
		// Initialize metadata fields if not set
		if rules[i].Meta.UID == "" {
			// Check if rule exists and preserve UID
			existingUID, err := w.getExistingSvcSvcRuleUID(ctx, rules[i].Namespace, rules[i].Name)
			if err == nil && existingUID != "" {
				rules[i].Meta.UID = existingUID
			} else {
				rules[i].Meta.TouchOnCreate()
			}
		}

		if err := w.upsertSvcSvcRule(ctx, rules[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert svc svc rule %s/%s", rules[i].Namespace, rules[i].Name)
		}
	}

	return nil
}

// upsertSvcSvcRule inserts or updates a SvcSvcRule with full K8s metadata support
func (w *Writer) upsertSvcSvcRule(ctx context.Context, rule models.SvcSvcRule) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(rule.Meta.Labels, rule.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	// Check if rule exists to determine if conditions need initialization
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM svc_svc_rules WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, existingQuery, rule.Namespace, rule.Name).Scan(&existingResourceVersion)
	if err != nil && !isNoRowsError(err) {
		return errors.Wrapf(err, "failed to check existing svc svc rule %s/%s", rule.Namespace, rule.Name)
	}

	conditionsJSON, err := w.prepareConditionsJSON(ctx, existingResourceVersion, rule.Meta.Conditions, conditionMergeOptions{
		ForcePendingReady: true,
	})
	if err != nil {
		return errors.Wrap(err, "failed to prepare merged conditions")
	}

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
			return errors.Wrapf(err, "failed to update K8s metadata for svc svc rule %s/%s", rule.Namespace, rule.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (uid, labels, annotations, finalizers, conditions)
			VALUES ($1, $2, $3, '{}', $4)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, rule.Meta.UID, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for svc svc rule %s/%s", rule.Namespace, rule.Name)
		}
	}
	// Marshal service references to JSON
	serviceFromRefJSON := svcSvcRuleServiceRefJSON{
		APIVersion: rule.ServiceFromRef.APIVersion,
		Kind:       rule.ServiceFromRef.Kind,
		Name:       rule.ServiceFromRef.Name,
		Namespace:  rule.ServiceFromRef.Namespace,
	}
	serviceFromJSON, err := json.Marshal(serviceFromRefJSON)
	if err != nil {
		return errors.Wrap(err, "failed to marshal service_from_ref")
	}

	serviceToRefJSON := svcSvcRuleServiceRefJSON{
		APIVersion: rule.ServiceToRef.APIVersion,
		Kind:       rule.ServiceToRef.Kind,
		Name:       rule.ServiceToRef.Name,
		Namespace:  rule.ServiceToRef.Namespace,
	}
	serviceToJSON, err := json.Marshal(serviceToRefJSON)
	if err != nil {
		return errors.Wrap(err, "failed to marshal service_to_ref")
	}

	// Convert action to string
	actionStr := string(rule.Action)

	// Upsert the rule
	ruleQuery := `
		INSERT INTO svc_svc_rules (namespace, name, service_from_ref, service_to_ref, action, priority, logs, trace, description, comment, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (namespace, name) DO UPDATE SET
			action = $5,
			priority = $6,
			logs = $7,
			trace = $8,
			description = $9,
			comment = $10,
			resource_version = $11`

	_, err = w.tx.Exec(ctx, ruleQuery,
		rule.Namespace,
		rule.Name,
		serviceFromJSON,
		serviceToJSON,
		actionStr,
		rule.Priority,
		rule.Logs,
		rule.Trace,
		rule.Description,
		rule.Comment,
		resourceVersion,
	)
	if err != nil {
		return errors.Wrapf(err, "failed to upsert svc svc rule %s/%s", rule.Namespace, rule.Name)
	}

	return nil
}

// updateSvcSvcRuleConditionsOnly updates ONLY conditions in k8s_metadata without triggering outbox entry
// This is used by OutboxWorker after successful SGROUP sync to mark resource as Ready
// Updates k8s_metadata directly, bypassing the svc_svc_rules table, so triggers do not fire and no outbox entry is created.
func (w *Writer) updateSvcSvcRuleConditionsOnly(ctx context.Context, rule models.SvcSvcRule) error {
	// Find the existing rule's resource_version by namespace/name
	var resourceVersion int64
	findQuery := `SELECT resource_version FROM svc_svc_rules WHERE namespace = $1 AND name = $2`
	err := w.tx.QueryRow(ctx, findQuery, rule.Namespace, rule.Name).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to find svcsvc rule %s/%s for condition update", rule.Namespace, rule.Name)
	}

	mergedVersion := sql.NullInt64{Int64: resourceVersion, Valid: true}

	conditionsJSON, err := w.prepareConditionsJSON(ctx, mergedVersion, rule.Meta.Conditions, conditionMergeOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to prepare merged conditions")
	}

	// Update only the conditions in k8s_metadata using the resource_version
	// IMPORTANT: We update k8s_metadata directly, NOT svc_svc_rules table
	// This means the AFTER INSERT OR UPDATE trigger on svc_svc_rules will NOT fire
	conditionUpdateQuery := `
		UPDATE k8s_metadata
		SET conditions = $1, updated_at = NOW()
		WHERE resource_version = $2
		RETURNING resource_version`

	var newResourceVersion int64
	if err := w.tx.QueryRow(ctx, conditionUpdateQuery, conditionsJSON, resourceVersion).Scan(&newResourceVersion); err != nil {
		return errors.Wrapf(err, "failed to update conditions for svcsvc rule %s/%s", rule.Namespace, rule.Name)
	}

	// If metadata bump trigger produced a new resourceVersion, keep svc_svc_rules row in sync
	if newResourceVersion != resourceVersion {
		updateRuleRVQuery := `
			UPDATE svc_svc_rules
			SET resource_version = $1
			WHERE namespace = $2 AND name = $3`
		if _, err := w.tx.Exec(ctx, updateRuleRVQuery, newResourceVersion, rule.Namespace, rule.Name); err != nil {
			return errors.Wrapf(err, "failed to sync resource_version for svcsvc rule %s/%s", rule.Namespace, rule.Name)
		}
	}

	return nil
}

// deleteSvcSvcRulesInScope deletes all SvcSvcRules in the given scope
func (w *Writer) deleteSvcSvcRulesInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	// Build WHERE clause from scope using shared utility
	whereClause, args := w.buildScopeFilter(scope, "sr")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM svc_svc_rules sr WHERE %s`, whereClause)

	_, err := w.tx.Exec(ctx, query, args...)
	return err
}

// deleteSvcSvcRulesByIdentifiers deletes specific SvcSvcRules by their identifiers
func (w *Writer) deleteSvcSvcRulesByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
	if len(identifiers) == 0 {
		return nil
	}

	// Build DELETE query with multiple namespace/name pairs
	query := "DELETE FROM svc_svc_rules WHERE "
	var conditions []string
	var args []interface{}

	for i, id := range identifiers {
		conditions = append(conditions, "(namespace = $"+string(rune('0'+2*i+1))+" AND name = $"+string(rune('0'+2*i+2))+")")
		args = append(args, id.Namespace, id.Name)
	}

	query += conditions[0]
	for i := 1; i < len(conditions); i++ {
		query += " OR " + conditions[i]
	}

	_, err := w.tx.Exec(ctx, query, args...)
	return errors.Wrap(err, "failed to delete svc svc rules")
}

// getExistingSvcSvcRuleUID gets the UID of an existing SvcSvcRule
func (w *Writer) getExistingSvcSvcRuleUID(ctx context.Context, namespace, name string) (string, error) {
	var uid string
	query := `
		SELECT m.uid
		FROM svc_svc_rules sr
		INNER JOIN k8s_metadata m ON sr.resource_version = m.resource_version
		WHERE sr.namespace = $1 AND sr.name = $2`
	err := w.tx.QueryRow(ctx, query, namespace, name).Scan(&uid)
	if err != nil {
		return "", err
	}
	return uid, nil
}

// DeleteSvcSvcRulesByIDs deletes specific SvcSvcRules by their ResourceIdentifiers
// This is the public method required by ports.Writer interface
func (w *Writer) DeleteSvcSvcRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.deleteSvcSvcRulesByIdentifiers(ctx, ids)
}
