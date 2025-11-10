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
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"strings"
	"time"
)

type serviceRefJSON struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

func (r *Reader) ListSvcSvcRules(ctx context.Context, consume func(models.SvcSvcRule) error, scope ports.Scope) error {
	query := `
		SELECT sr.namespace, sr.name, sr.service_from_ref, sr.service_to_ref,
		       sr.action, sr.priority, sr.logs, sr.trace,
		       m.resource_version, m.labels, m.annotations, m.conditions,
		       m.created_at, m.updated_at, m.deletion_timestamp
		FROM svc_svc_rules sr
		INNER JOIN k8s_metadata m ON sr.resource_version = m.resource_version`
	whereClause, args := utils.BuildScopeFilter(scope, "sr")
	if whereClause != "" {
		query += " WHERE " + whereClause
	} else {
	}
	query += " ORDER BY sr.namespace, sr.name"
	var rows pgx.Rows
	var err error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		rows, err = r.query(ctx, query, args...)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "conn busy") && attempt < maxRetries-1 {
			time.Sleep(time.Duration(10*(1<<attempt)) * time.Millisecond)
			continue
		}
		return errors.Wrap(err, "failed to query svc svc rules")
	}
	defer rows.Close()
	for rows.Next() {
		rule, err := r.scanSvcSvcRule(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan svc svc rule")
		}
		if err := consume(rule); err != nil {
			return err
		}
	}
	return rows.Err()
}
func (r *Reader) GetSvcSvcRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error) {
	query := `
		SELECT sr.namespace, sr.name, sr.service_from_ref, sr.service_to_ref,
		       sr.action, sr.priority, sr.logs, sr.trace,
		       m.resource_version, m.labels, m.annotations, m.conditions,
		       m.created_at, m.updated_at, m.deletion_timestamp
		FROM svc_svc_rules sr
		INNER JOIN k8s_metadata m ON sr.resource_version = m.resource_version
		WHERE sr.namespace = $1 AND sr.name = $2 AND m.deletion_timestamp IS NULL`
	var rule *models.SvcSvcRule
	var err error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		row := r.queryRow(ctx, query, id.Namespace, id.Name)
		rule, err = r.scanSvcSvcRuleRow(row)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "conn busy") && attempt < maxRetries-1 {
			time.Sleep(time.Duration(10*(1<<attempt)) * time.Millisecond)
			continue
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan svc svc rule")
	}
	return rule, nil
}
func (r *Reader) FindRulesByService(ctx context.Context, namespace, serviceName string) ([]models.SvcSvcRule, error) {
	query := `
		SELECT sr.namespace, sr.name, sr.service_from_ref, sr.service_to_ref,
		       sr.action, sr.priority, sr.logs, sr.trace,
		       m.resource_version, m.labels, m.annotations, m.conditions,
		       m.created_at, m.updated_at, m.deletion_timestamp
		FROM svc_svc_rules sr
		INNER JOIN k8s_metadata m ON sr.resource_version = m.resource_version
		WHERE m.deletion_timestamp IS NULL
		  AND (
		    (sr.service_from_ref->>'namespace' = $1 AND sr.service_from_ref->>'name' = $2)
		    OR
		    (sr.service_to_ref->>'namespace' = $1 AND sr.service_to_ref->>'name' = $2)
		  )
		ORDER BY sr.namespace, sr.name`
	rows, err := r.query(ctx, query, namespace, serviceName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query rules by service")
	}
	defer rows.Close()
	var rules []models.SvcSvcRule
	for rows.Next() {
		rule, err := r.scanSvcSvcRule(rows)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan svc svc rule")
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}
func (r *Reader) FindRuleByServicePair(ctx context.Context, ruleNamespace, serviceFromName, serviceFromNamespace, serviceToName, serviceToNamespace string) (*models.SvcSvcRule, error) {
	query := `
		SELECT sr.namespace, sr.name, sr.service_from_ref, sr.service_to_ref,
		       sr.action, sr.priority, sr.logs, sr.trace,
		       m.resource_version, m.labels, m.annotations, m.conditions,
		       m.created_at, m.updated_at, m.deletion_timestamp
		FROM svc_svc_rules sr
		INNER JOIN k8s_metadata m ON sr.resource_version = m.resource_version
		WHERE sr.namespace = $1
		  AND sr.service_from_ref->>'namespace' = $2
		  AND sr.service_from_ref->>'name' = $3
		  AND sr.service_to_ref->>'namespace' = $4
		  AND sr.service_to_ref->>'name' = $5
		  AND m.deletion_timestamp IS NULL`
	row := r.queryRow(ctx, query, ruleNamespace, serviceFromNamespace, serviceFromName, serviceToNamespace, serviceToName)
	rule, err := r.scanSvcSvcRuleRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan svc svc rule by service pair")
	}
	return rule, nil
}
func (r *Reader) scanSvcSvcRule(rows pgx.Rows) (models.SvcSvcRule, error) {
	var rule models.SvcSvcRule
	var serviceFromRefJSON, serviceToRefJSON []byte
	var actionStr string
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	err := rows.Scan(
		&rule.Namespace,
		&rule.Name,
		&serviceFromRefJSON,
		&serviceToRefJSON,
		&actionStr,
		&rule.Priority,
		&rule.Logs,
		&rule.Trace,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
		&deletionTS,
	)
	if err != nil {
		return rule, err
	}
	rule.Action = models.RuleAction(actionStr)
	var serviceFromRef serviceRefJSON
	if err := json.Unmarshal(serviceFromRefJSON, &serviceFromRef); err != nil {
		return rule, errors.Wrap(err, "failed to unmarshal service_from_ref")
	}
	rule.ServiceFromRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: serviceFromRef.APIVersion,
			Kind:       serviceFromRef.Kind,
			Name:       serviceFromRef.Name,
		},
		Namespace: serviceFromRef.Namespace,
	}
	var serviceToRef serviceRefJSON
	if err := json.Unmarshal(serviceToRefJSON, &serviceToRef); err != nil {
		return rule, errors.Wrap(err, "failed to unmarshal service_to_ref")
	}
	rule.ServiceToRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: serviceToRef.APIVersion,
			Kind:       serviceToRef.Kind,
			Name:       serviceToRef.Name,
		},
		Namespace: serviceToRef.Namespace,
	}
	rule.Meta, err = utils.ConvertK8sMetadata(
		fmt.Sprintf("%d", resourceVersion),
		labelsJSON,
		annotationsJSON,
		conditionsJSON,
		createdAt,
		updatedAt,
		deletionTS,
	)
	if err != nil {
		return rule, err
	}
	return rule, nil
}
func (r *Reader) scanSvcSvcRuleRow(row pgx.Row) (*models.SvcSvcRule, error) {
	var rule models.SvcSvcRule
	var serviceFromRefJSON, serviceToRefJSON []byte
	var actionStr string
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	err := row.Scan(
		&rule.Namespace,
		&rule.Name,
		&serviceFromRefJSON,
		&serviceToRefJSON,
		&actionStr,
		&rule.Priority,
		&rule.Logs,
		&rule.Trace,
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
	rule.Action = models.RuleAction(actionStr)
	var serviceFromRef serviceRefJSON
	if err := json.Unmarshal(serviceFromRefJSON, &serviceFromRef); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal service_from_ref")
	}
	rule.ServiceFromRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: serviceFromRef.APIVersion,
			Kind:       serviceFromRef.Kind,
			Name:       serviceFromRef.Name,
		},
		Namespace: serviceFromRef.Namespace,
	}
	var serviceToRef serviceRefJSON
	if err := json.Unmarshal(serviceToRefJSON, &serviceToRef); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal service_to_ref")
	}
	rule.ServiceToRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: serviceToRef.APIVersion,
			Kind:       serviceToRef.Kind,
			Name:       serviceToRef.Name,
		},
		Namespace: serviceToRef.Namespace,
	}
	rule.Meta, err = utils.ConvertK8sMetadata(
		fmt.Sprintf("%d", resourceVersion),
		labelsJSON,
		annotationsJSON,
		conditionsJSON,
		createdAt,
		updatedAt,
		deletionTS,
	)
	if err != nil {
		return nil, err
	}
	return &rule, nil
}
