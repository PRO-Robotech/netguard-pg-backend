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

// ListIECidrSvcRules iterates IECidrSvcRule rows within optional scope
func (r *Reader) ListIECidrSvcRules(ctx context.Context, consume func(models.IECidrSvcRule) error, scope ports.Scope) error {
	query := `
        SELECT ir.namespace, ir.name, ir.transport, ir.cidr, ir.service_ref, ir.traffic,
               ir.ports, ir.logs, ir.trace, ir.action, ir.priority, ir.description, ir.comment,
               m.resource_version, m.labels, m.annotations, m.conditions,
               m.created_at, m.updated_at, m.deletion_timestamp
        FROM ie_cidr_svc_rules ir
        INNER JOIN k8s_metadata m ON ir.resource_version = m.resource_version`

	whereClause, args, err := utils.BuildScopeFilterWithTable(scope, "ie_cidr_svc_rules", "ir")
	if err != nil {
		return errors.Wrap(err, "failed to build scope filter")
	}

	if whereClause != "" {
		query += " WHERE " + whereClause
	}
	query += " ORDER BY ir.namespace, ir.name"

	var rows pgx.Rows
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
		return errors.Wrap(err, "failed to query ie cidr svc rules")
	}
	defer rows.Close()

	for rows.Next() {
		rule, err := r.scanIECidrSvcRule(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan ie cidr svc rule")
		}
		if err := consume(rule); err != nil {
			return err
		}
	}
	return rows.Err()
}

// GetIECidrSvcRuleByID fetches single rule by namespace/name
func (r *Reader) GetIECidrSvcRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IECidrSvcRule, error) {
	query := `
        SELECT ir.namespace, ir.name, ir.transport, ir.cidr, ir.service_ref, ir.traffic,
               ir.ports, ir.logs, ir.trace, ir.action, ir.priority, ir.description, ir.comment,
               m.resource_version, m.labels, m.annotations, m.conditions,
               m.created_at, m.updated_at, m.deletion_timestamp
        FROM ie_cidr_svc_rules ir
        INNER JOIN k8s_metadata m ON ir.resource_version = m.resource_version
        WHERE ir.namespace = $1 AND ir.name = $2`

	var rule *models.IECidrSvcRule
	var err error
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		row := r.queryRow(ctx, query, id.Namespace, id.Name)
		rule, err = r.scanIECidrSvcRuleRow(row)
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
		return nil, errors.Wrap(err, "failed to scan ie cidr svc rule")
	}
	return rule, nil
}

// FindIECidrSvcRulesByService returns rules referencing given service
func (r *Reader) FindIECidrSvcRulesByService(ctx context.Context, namespace, serviceName string) ([]models.IECidrSvcRule, error) {
	query := `
        SELECT ir.namespace, ir.name, ir.transport, ir.cidr, ir.service_ref, ir.traffic,
               ir.ports, ir.logs, ir.trace, ir.action, ir.priority, ir.description, ir.comment,
               m.resource_version, m.labels, m.annotations, m.conditions,
               m.created_at, m.updated_at, m.deletion_timestamp
        FROM ie_cidr_svc_rules ir
        INNER JOIN k8s_metadata m ON ir.resource_version = m.resource_version
        WHERE ir.service_ref->>'namespace' = $1
          AND ir.service_ref->>'name' = $2
        ORDER BY ir.namespace, ir.name`

	rows, err := r.query(ctx, query, namespace, serviceName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query ie cidr svc rules by service")
	}
	defer rows.Close()

	var rules []models.IECidrSvcRule
	for rows.Next() {
		rule, err := r.scanIECidrSvcRule(rows)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan ie cidr svc rule")
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *Reader) scanIECidrSvcRule(rows pgx.Rows) (models.IECidrSvcRule, error) {
	var rule models.IECidrSvcRule
	var serviceRefJSON []byte
	var portsJSON []byte
	var transportStr string
	var trafficStr string
	var actionStr string
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64

	err := rows.Scan(
		&rule.Namespace,
		&rule.Name,
		&transportStr,
		&rule.CIDR,
		&serviceRefJSON,
		&trafficStr,
		&portsJSON,
		&rule.Logs,
		&rule.Trace,
		&actionStr,
		&rule.Priority,
		&rule.Description,
		&rule.Comment,
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

	if err := r.populateIECidrSvcRule(&rule, serviceRefJSON, portsJSON, transportStr, trafficStr, actionStr, resourceVersion, labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS); err != nil {
		return rule, err
	}

	return rule, nil
}

func (r *Reader) scanIECidrSvcRuleRow(row pgx.Row) (*models.IECidrSvcRule, error) {
	var rule models.IECidrSvcRule
	var serviceRefJSON []byte
	var portsJSON []byte
	var transportStr string
	var trafficStr string
	var actionStr string
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64

	err := row.Scan(
		&rule.Namespace,
		&rule.Name,
		&transportStr,
		&rule.CIDR,
		&serviceRefJSON,
		&trafficStr,
		&portsJSON,
		&rule.Logs,
		&rule.Trace,
		&actionStr,
		&rule.Priority,
		&rule.Description,
		&rule.Comment,
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

	if err := r.populateIECidrSvcRule(&rule, serviceRefJSON, portsJSON, transportStr, trafficStr, actionStr, resourceVersion, labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS); err != nil {
		return nil, err
	}

	return &rule, nil
}

func (r *Reader) populateIECidrSvcRule(
	rule *models.IECidrSvcRule,
	serviceRefJSON []byte,
	portsJSON []byte,
	transportStr string,
	trafficStr string,
	actionStr string,
	resourceVersion int64,
	labelsJSON []byte,
	annotationsJSON []byte,
	conditionsJSON []byte,
	createdAt time.Time,
	updatedAt time.Time,
	deletionTS *time.Time,
) error {
	var svcRef ieCidrSvcRuleServiceRefJSON
	if err := json.Unmarshal(serviceRefJSON, &svcRef); err != nil {
		return errors.Wrap(err, "failed to unmarshal service_ref")
	}
	rule.ServiceRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: svcRef.APIVersion,
			Kind:       svcRef.Kind,
			Name:       svcRef.Name,
		},
		Namespace: svcRef.Namespace,
	}

	if len(portsJSON) > 0 {
		var ports []ieCidrSvcRulePortJSON
		if err := json.Unmarshal(portsJSON, &ports); err != nil {
			return errors.Wrap(err, "failed to unmarshal ports")
		}
		if len(ports) > 0 {
			rule.Ports = make([]models.IECidrSvcPortSpec, len(ports))
			for i, p := range ports {
				rule.Ports[i] = models.IECidrSvcPortSpec{
					S: p.S,
					D: p.D,
				}
			}
		}
	}

	rule.Transport = models.TransportProtocol(transportStr)
	rule.Traffic = models.Traffic(trafficStr)
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
