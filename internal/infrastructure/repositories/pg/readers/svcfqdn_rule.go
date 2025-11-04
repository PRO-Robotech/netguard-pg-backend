package readers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/utils"
	v1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

type svcFqdnRuleServiceRefJSON struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

type svcFqdnRulePortJSON struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// ListSvcFqdnRules iterates SvcFqdnRule rows within optional scope
func (r *Reader) ListSvcFqdnRules(ctx context.Context, consume func(models.SvcFqdnRule) error, scope ports.Scope) error {
	query := `
        SELECT fr.namespace, fr.name, fr.service_from_ref, fr.fqdn, fr.transport,
               fr.ports, fr.logs, fr.trace, fr.action, fr.priority, fr.description,
               m.resource_version, m.labels, m.annotations, m.conditions,
               m.created_at, m.updated_at, m.deletion_timestamp
        FROM svc_fqdn_rules fr
        INNER JOIN k8s_metadata m ON fr.resource_version = m.resource_version`

	whereClause, args := utils.BuildScopeFilter(scope, "fr")
	if whereClause != "" {
		query += " WHERE " + whereClause
	}
	query += " ORDER BY fr.namespace, fr.name"

	var rows pgx.Rows
	var err error
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		rows, err = r.query(ctx, query, args...)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "conn busy") && attempt < maxRetries-1 {
			time.Sleep(time.Duration(10*(1<<attempt)) * time.Millisecond)
			continue
		}
		return errors.Wrap(err, "failed to query svc fqdn rules")
	}
	defer rows.Close()

	for rows.Next() {
		rule, err := r.scanSvcFqdnRule(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan svc fqdn rule")
		}
		if err := consume(rule); err != nil {
			return err
		}
	}
	return rows.Err()
}

// GetSvcFqdnRuleByID fetches single rule by namespace/name
func (r *Reader) GetSvcFqdnRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcFqdnRule, error) {
	query := `
        SELECT fr.namespace, fr.name, fr.service_from_ref, fr.fqdn, fr.transport,
               fr.ports, fr.logs, fr.trace, fr.action, fr.priority, fr.description,
               m.resource_version, m.labels, m.annotations, m.conditions,
               m.created_at, m.updated_at, m.deletion_timestamp
        FROM svc_fqdn_rules fr
        INNER JOIN k8s_metadata m ON fr.resource_version = m.resource_version
        WHERE fr.namespace = $1 AND fr.name = $2 AND m.deletion_timestamp IS NULL`

	var rule *models.SvcFqdnRule
	var err error
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		row := r.queryRow(ctx, query, id.Namespace, id.Name)
		rule, err = r.scanSvcFqdnRuleRow(row)
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
		return nil, errors.Wrap(err, "failed to scan svc fqdn rule")
	}
	return rule, nil
}

// FindSvcFqdnRulesByService returns rules referencing given service as source
func (r *Reader) FindSvcFqdnRulesByService(ctx context.Context, namespace, serviceName string) ([]models.SvcFqdnRule, error) {
	query := `
        SELECT fr.namespace, fr.name, fr.service_from_ref, fr.fqdn, fr.transport,
               fr.ports, fr.logs, fr.trace, fr.action, fr.priority, fr.description,
               m.resource_version, m.labels, m.annotations, m.conditions,
               m.created_at, m.updated_at, m.deletion_timestamp
        FROM svc_fqdn_rules fr
        INNER JOIN k8s_metadata m ON fr.resource_version = m.resource_version
        WHERE m.deletion_timestamp IS NULL
          AND fr.service_from_ref->>'namespace' = $1
          AND fr.service_from_ref->>'name' = $2
        ORDER BY fr.namespace, fr.name`

	rows, err := r.query(ctx, query, namespace, serviceName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query svc fqdn rules by service")
	}
	defer rows.Close()

	var rules []models.SvcFqdnRule
	for rows.Next() {
		rule, err := r.scanSvcFqdnRule(rows)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan svc fqdn rule")
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *Reader) scanSvcFqdnRule(rows pgx.Rows) (models.SvcFqdnRule, error) {
	var rule models.SvcFqdnRule
	var serviceFromJSON []byte
	var portsJSON []byte
	var transportStr string
	var actionStr string
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64

	err := rows.Scan(
		&rule.Namespace,
		&rule.Name,
		&serviceFromJSON,
		&rule.FQDN,
		&transportStr,
		&portsJSON,
		&rule.Logs,
		&rule.Trace,
		&actionStr,
		&rule.Priority,
		&rule.Description,
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

	if err := r.populateSvcFqdnRule(&rule, serviceFromJSON, portsJSON, transportStr, actionStr, resourceVersion, labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS); err != nil {
		return rule, err
	}

	return rule, nil
}

func (r *Reader) scanSvcFqdnRuleRow(row pgx.Row) (*models.SvcFqdnRule, error) {
	var rule models.SvcFqdnRule
	var serviceFromJSON []byte
	var portsJSON []byte
	var transportStr string
	var actionStr string
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64

	err := row.Scan(
		&rule.Namespace,
		&rule.Name,
		&serviceFromJSON,
		&rule.FQDN,
		&transportStr,
		&portsJSON,
		&rule.Logs,
		&rule.Trace,
		&actionStr,
		&rule.Priority,
		&rule.Description,
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

	if err := r.populateSvcFqdnRule(&rule, serviceFromJSON, portsJSON, transportStr, actionStr, resourceVersion, labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS); err != nil {
		return nil, err
	}

	return &rule, nil
}

func (r *Reader) populateSvcFqdnRule(
	rule *models.SvcFqdnRule,
	serviceFromJSON []byte,
	portsJSON []byte,
	transportStr string,
	actionStr string,
	resourceVersion int64,
	labelsJSON []byte,
	annotationsJSON []byte,
	conditionsJSON []byte,
	createdAt time.Time,
	updatedAt time.Time,
	deletionTS *time.Time,
) error {
	var svcRef svcFqdnRuleServiceRefJSON
	if err := json.Unmarshal(serviceFromJSON, &svcRef); err != nil {
		return errors.Wrap(err, "failed to unmarshal service_from_ref")
	}
	rule.ServiceFromRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: svcRef.APIVersion,
			Kind:       svcRef.Kind,
			Name:       svcRef.Name,
		},
		Namespace: svcRef.Namespace,
	}

	if len(portsJSON) > 0 {
		var ports []svcFqdnRulePortJSON
		if err := json.Unmarshal(portsJSON, &ports); err != nil {
			return errors.Wrap(err, "failed to unmarshal ports")
		}
		if len(ports) > 0 {
			rule.Ports = make([]models.FqdnPortSpec, len(ports))
			for i, p := range ports {
				rule.Ports[i] = models.FqdnPortSpec{
					Source:      p.Source,
					Destination: p.Destination,
				}
			}
		}
	}

	rule.Transport = models.TransportProtocol(transportStr)
	rule.Action = models.RuleAction(actionStr)

	meta, err := utils.ConvertK8sMetadata(
		fmt.Sprintf("%d", resourceVersion),
		labelsJSON,
		annotationsJSON,
		conditionsJSON,
		createdAt,
		updatedAt,
		deletionTS,
	)
	if err != nil {
		return err
	}
	rule.Meta = meta

	return nil
}
