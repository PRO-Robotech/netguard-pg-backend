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

type ieCidrSvcRuleServiceRefJSON struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

type ieCidrSvcRulePortJSON struct {
	S string `json:"s,omitempty"` // Source port expression
	D string `json:"d"`           // Destination port expression
}

// SyncIECidrSvcRules synchronizes ingress/egress CIDR-to-service rules with PostgreSQL storage
func (w *Writer) SyncIECidrSvcRules(ctx context.Context, rules []models.IECidrSvcRule, scope ports.Scope, options ...ports.Option) error {
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
			if err := w.updateIECidrSvcRuleConditionsOnly(ctx, rule); err != nil {
				return errors.Wrapf(err, "failed to update conditions for ie cidr svc rule %s/%s", rule.Namespace, rule.Name)
			}
		}
		return nil
	}

	if syncOp == models.SyncOpDelete {
		var ids []models.ResourceIdentifier
		for _, rule := range rules {
			ids = append(ids, rule.ResourceIdentifier)
		}
		if err := w.deleteIECidrSvcRulesByIdentifiers(ctx, ids); err != nil {
			return errors.Wrap(err, "failed to delete ie cidr svc rules")
		}
		return nil
	}

	if !scope.IsEmpty() {
		if err := w.deleteIECidrSvcRulesInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete ie cidr svc rules in scope")
		}
	}

	for i := range rules {
		if rules[i].Meta.UID == "" {
			existingUID, err := w.getExistingIECidrSvcRuleUID(ctx, rules[i].Namespace, rules[i].Name)
			if err == nil && existingUID != "" {
				rules[i].Meta.UID = existingUID
			} else {
				rules[i].Meta.TouchOnCreate()
			}
		}

		if err := w.upsertIECidrSvcRule(ctx, rules[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert ie cidr svc rule %s/%s", rules[i].Namespace, rules[i].Name)
		}
	}

	return nil
}

func (w *Writer) upsertIECidrSvcRule(ctx context.Context, rule models.IECidrSvcRule) error {
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(rule.Meta.Labels, rule.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal metadata labels/annotations")
	}

	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM ie_cidr_svc_rules WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, existingQuery, rule.Namespace, rule.Name).Scan(&existingResourceVersion)
	if err != nil && !isNoRowsError(err) {
		return errors.Wrapf(err, "failed to check existing ie cidr svc rule %s/%s", rule.Namespace, rule.Name)
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
			return errors.Wrapf(err, "failed to update metadata for ie cidr svc rule %s/%s", rule.Namespace, rule.Name)
		}
	} else {
		metadataQuery := `
            INSERT INTO k8s_metadata (uid, labels, annotations, finalizers, conditions)
            VALUES ($1, $2, $3, '{}', $4)
            RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, rule.Meta.UID, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to insert metadata for ie cidr svc rule %s/%s", rule.Namespace, rule.Name)
		}
	}

	serviceRefJSON, err := json.Marshal(ieCidrSvcRuleServiceRefJSON{
		APIVersion: rule.ServiceRef.APIVersion,
		Kind:       rule.ServiceRef.Kind,
		Name:       rule.ServiceRef.Name,
		Namespace:  rule.ServiceRef.Namespace,
	})
	if err != nil {
		return errors.Wrap(err, "failed to marshal service_ref")
	}

	portsJSON := make([]ieCidrSvcRulePortJSON, 0, len(rule.Ports))
	for _, port := range rule.Ports {
		portsJSON = append(portsJSON, ieCidrSvcRulePortJSON{
			S: port.S,
			D: port.D,
		})
	}
	portsBytes, err := json.Marshal(portsJSON)
	if err != nil {
		return errors.Wrap(err, "failed to marshal ports")
	}

	transportStr := string(rule.Transport)
	trafficStr := string(rule.Traffic)
	actionStr := string(rule.Action)

	query := `
        INSERT INTO ie_cidr_svc_rules (
            namespace, name, transport, cidr, service_ref, traffic, ports,
            logs, trace, action, priority, description, comment, resource_version
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
        ON CONFLICT (namespace, name) DO UPDATE SET
            transport = $3,
            ports = $7,
            logs = $8,
            trace = $9,
            action = $10,
            priority = $11,
            description = $12,
            comment = $13,
            resource_version = $14`

	if _, err := w.tx.Exec(ctx, query,
		rule.Namespace,
		rule.Name,
		transportStr,
		rule.CIDR,
		serviceRefJSON,
		trafficStr,
		portsBytes,
		rule.Logs,
		rule.Trace,
		actionStr,
		rule.Priority,
		rule.Description,
		rule.Comment,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert ie cidr svc rule %s/%s", rule.Namespace, rule.Name)
	}

	return nil
}

func (w *Writer) updateIECidrSvcRuleConditionsOnly(ctx context.Context, rule models.IECidrSvcRule) error {
	var resourceVersion int64
	findQuery := `SELECT resource_version FROM ie_cidr_svc_rules WHERE namespace = $1 AND name = $2`
	if err := w.tx.QueryRow(ctx, findQuery, rule.Namespace, rule.Name).Scan(&resourceVersion); err != nil {
		return errors.Wrapf(err, "failed to find ie cidr svc rule %s/%s for condition update", rule.Namespace, rule.Name)
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
		return errors.Wrapf(err, "failed to update conditions for ie cidr svc rule %s/%s", rule.Namespace, rule.Name)
	}

	if newResourceVersion != resourceVersion {
		if _, err := w.tx.Exec(ctx, `
            UPDATE ie_cidr_svc_rules
            SET resource_version = $1
            WHERE namespace = $2 AND name = $3`,
			newResourceVersion, rule.Namespace, rule.Name); err != nil {
			return errors.Wrapf(err, "failed to sync resource_version for ie cidr svc rule %s/%s", rule.Namespace, rule.Name)
		}
	}

	return nil
}

func (w *Writer) deleteIECidrSvcRulesInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "ir")
	if whereClause == "" {
		return nil
	}

	query := "DELETE FROM ie_cidr_svc_rules ir WHERE " + whereClause
	_, err := w.tx.Exec(ctx, query, args...)
	return err
}

func (w *Writer) deleteIECidrSvcRulesByIdentifiers(ctx context.Context, ids []models.ResourceIdentifier) error {
	if len(ids) == 0 {
		return nil
	}

	query := "DELETE FROM ie_cidr_svc_rules WHERE "
	conditions := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids)*2)
	for i, id := range ids {
		conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", 2*i+1, 2*i+2))
		args = append(args, id.Namespace, id.Name)
	}
	query += strings.Join(conditions, " OR ")

	_, err := w.tx.Exec(ctx, query, args...)
	return errors.Wrap(err, "failed to delete ie cidr svc rules by identifiers")
}

func (w *Writer) getExistingIECidrSvcRuleUID(ctx context.Context, namespace, name string) (string, error) {
	var uid string
	query := `
        SELECT m.uid
        FROM ie_cidr_svc_rules ir
        INNER JOIN k8s_metadata m ON ir.resource_version = m.resource_version
        WHERE ir.namespace = $1 AND ir.name = $2`
	if err := w.tx.QueryRow(ctx, query, namespace, name).Scan(&uid); err != nil {
		return "", err
	}
	return uid, nil
}

// DeleteIECidrSvcRulesByIDs deletes rules by identifiers (ports.Writer contract)
func (w *Writer) DeleteIECidrSvcRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.deleteIECidrSvcRulesByIdentifiers(ctx, ids)
}
