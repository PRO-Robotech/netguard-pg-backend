package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

type svcFqdnRuleServiceRefJSON struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

type svcFqdnRulePortJSON struct {
	Port string `json:"port"`
}

// SyncSvcFqdnRules synchronizes service-to-FQDN rules with PostgreSQL storage
func (w *Writer) SyncSvcFqdnRules(ctx context.Context, rules []models.SvcFqdnRule, scope ports.Scope, options ...ports.Option) error {
	var syncOp models.SyncOp = models.SyncOpUpsert
	isConditionOnly := false

	for _, opt := range options {
		if syncOpOpt, ok := opt.(ports.SyncOption); ok {
			syncOp = syncOpOpt.Operation
		}
		if _, ok := opt.(ports.ConditionOnlyOperation); ok {
			isConditionOnly = true
		}
	}

	if isConditionOnly {
		for _, rule := range rules {
			if err := w.updateSvcFqdnRuleConditionsOnly(ctx, rule); err != nil {
				return errors.Wrapf(err, "failed to update conditions for svc fqdn rule %s/%s", rule.Namespace, rule.Name)
			}
		}
		return nil
	}

	if syncOp == models.SyncOpDelete {
		var ids []models.ResourceIdentifier
		for _, rule := range rules {
			ids = append(ids, rule.ResourceIdentifier)
		}
		if err := w.deleteSvcFqdnRulesByIdentifiers(ctx, ids); err != nil {
			return errors.Wrap(err, "failed to delete svc fqdn rules")
		}
		return nil
	}

	if !scope.IsEmpty() {
		if err := w.deleteSvcFqdnRulesInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete svc fqdn rules in scope")
		}
	}

	for i := range rules {
		if rules[i].Meta.UID == "" {
			existingUID, err := w.getExistingSvcFqdnRuleUID(ctx, rules[i].Namespace, rules[i].Name)
			if err == nil && existingUID != "" {
				rules[i].Meta.UID = existingUID
			} else {
				rules[i].Meta.TouchOnCreate()
			}
		}

		if err := w.upsertSvcFqdnRule(ctx, rules[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert svc fqdn rule %s/%s", rules[i].Namespace, rules[i].Name)
		}
	}

	return nil
}

func (w *Writer) upsertSvcFqdnRule(ctx context.Context, rule models.SvcFqdnRule) error {
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(rule.Meta.Labels, rule.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal metadata labels/annotations")
	}

	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM svc_fqdn_rules WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, existingQuery, rule.Namespace, rule.Name).Scan(&existingResourceVersion)
	if err != nil && !isNoRowsError(err) {
		return errors.Wrapf(err, "failed to check existing svc fqdn rule %s/%s", rule.Namespace, rule.Name)
	}

	conditionsJSON, err := w.prepareConditionsJSON(ctx, existingResourceVersion, rule.Meta.Conditions, conditionMergeOptions{
		ForcePendingReady: true,
	})
	if err != nil {
		return errors.Wrap(err, "failed to prepare merged conditions")
	}

	var resourceVersion int64
	if existingResourceVersion.Valid {
		metadataQuery := `
            UPDATE k8s_metadata
            SET labels = $1, annotations = $2, conditions = $3, updated_at = NOW()
            WHERE resource_version = $4
            RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, existingResourceVersion.Int64).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to update metadata for svc fqdn rule %s/%s", rule.Namespace, rule.Name)
		}
	} else {
		metadataQuery := `
            INSERT INTO k8s_metadata (uid, labels, annotations, finalizers, conditions)
            VALUES ($1, $2, $3, '{}', $4)
            RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, rule.Meta.UID, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to insert metadata for svc fqdn rule %s/%s", rule.Namespace, rule.Name)
		}
	}
	serviceFromJSON, err := json.Marshal(svcFqdnRuleServiceRefJSON{
		APIVersion: rule.ServiceFromRef.APIVersion,
		Kind:       rule.ServiceFromRef.Kind,
		Name:       rule.ServiceFromRef.Name,
		Namespace:  rule.ServiceFromRef.Namespace,
	})
	if err != nil {
		return errors.Wrap(err, "failed to marshal service_from_ref")
	}

	portsJSON := make([]svcFqdnRulePortJSON, 0, len(rule.Ports))
	for _, port := range rule.Ports {
		portsJSON = append(portsJSON, svcFqdnRulePortJSON{Port: port.Port})
	}
	portsBytes, err := json.Marshal(portsJSON)
	if err != nil {
		return errors.Wrap(err, "failed to marshal ports")
	}

	actionStr := string(rule.Action)
	transportStr := string(rule.Transport)

	query := `
        INSERT INTO svc_fqdn_rules (
            namespace, name, service_from_ref, fqdn, transport, ports,
            logs, trace, action, priority, description, comment, resource_version
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
        ON CONFLICT (namespace, name) DO UPDATE SET
            fqdn = $4,
            transport = $5,
            ports = $6,
            logs = $7,
            trace = $8,
            action = $9,
            priority = $10,
            description = $11,
            comment = $12,
            resource_version = $13`

	if _, err := w.tx.Exec(ctx, query,
		rule.Namespace,
		rule.Name,
		serviceFromJSON,
		rule.FQDN,
		transportStr,
		portsBytes,
		rule.Logs,
		rule.Trace,
		actionStr,
		rule.Priority,
		rule.Description,
		rule.Comment,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert svc fqdn rule %s/%s", rule.Namespace, rule.Name)
	}

	return nil
}

func (w *Writer) updateSvcFqdnRuleConditionsOnly(ctx context.Context, rule models.SvcFqdnRule) error {
	var resourceVersion int64
	findQuery := `SELECT resource_version FROM svc_fqdn_rules WHERE namespace = $1 AND name = $2`
	if err := w.tx.QueryRow(ctx, findQuery, rule.Namespace, rule.Name).Scan(&resourceVersion); err != nil {
		return errors.Wrapf(err, "failed to find svc fqdn rule %s/%s for condition update", rule.Namespace, rule.Name)
	}

	mergedVersion := sql.NullInt64{Int64: resourceVersion, Valid: true}

	conditionsJSON, err := w.prepareConditionsJSON(ctx, mergedVersion, rule.Meta.Conditions, conditionMergeOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to prepare merged conditions")
	}

	updateQuery := `
        UPDATE k8s_metadata
        SET conditions = $1, updated_at = NOW()
        WHERE resource_version = $2
        RETURNING resource_version`

	var newResourceVersion int64
	if err := w.tx.QueryRow(ctx, updateQuery, conditionsJSON, resourceVersion).Scan(&newResourceVersion); err != nil {
		return errors.Wrapf(err, "failed to update conditions for svc fqdn rule %s/%s", rule.Namespace, rule.Name)
	}

	if newResourceVersion != resourceVersion {
		if _, err := w.tx.Exec(ctx, `
            UPDATE svc_fqdn_rules
            SET resource_version = $1
            WHERE namespace = $2 AND name = $3`,
			newResourceVersion, rule.Namespace, rule.Name); err != nil {
			return errors.Wrapf(err, "failed to sync resource_version for svc fqdn rule %s/%s", rule.Namespace, rule.Name)
		}
	}

	return nil
}

func (w *Writer) deleteSvcFqdnRulesInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "fr")
	if whereClause == "" {
		return nil
	}

	query := "DELETE FROM svc_fqdn_rules fr WHERE " + whereClause
	_, err := w.tx.Exec(ctx, query, args...)
	return err
}

func (w *Writer) deleteSvcFqdnRulesByIdentifiers(ctx context.Context, ids []models.ResourceIdentifier) error {
	if len(ids) == 0 {
		return nil
	}

	query := "DELETE FROM svc_fqdn_rules WHERE "
	conditions := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids)*2)
	for i, id := range ids {
		conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", 2*i+1, 2*i+2))
		args = append(args, id.Namespace, id.Name)
	}
	query += strings.Join(conditions, " OR ")

	_, err := w.tx.Exec(ctx, query, args...)
	return errors.Wrap(err, "failed to delete svc fqdn rules by identifiers")
}

func (w *Writer) getExistingSvcFqdnRuleUID(ctx context.Context, namespace, name string) (string, error) {
	var uid string
	query := `
        SELECT m.uid
        FROM svc_fqdn_rules fr
        INNER JOIN k8s_metadata m ON fr.resource_version = m.resource_version
        WHERE fr.namespace = $1 AND fr.name = $2`
	if err := w.tx.QueryRow(ctx, query, namespace, name).Scan(&uid); err != nil {
		return "", err
	}
	return uid, nil
}

// DeleteSvcFqdnRulesByIDs deletes rules by identifiers (ports.Writer contract)
func (w *Writer) DeleteSvcFqdnRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.deleteSvcFqdnRulesByIdentifiers(ctx, ids)
}
