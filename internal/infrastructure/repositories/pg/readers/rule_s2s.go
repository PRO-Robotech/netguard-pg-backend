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
	"time"
)

func (r *Reader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	query := `
		SELECT rs.namespace, rs.name, rs.traffic,
		       rs.service_local_ref, rs.service_ref, rs.ieagag_rule_refs, rs.trace,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
		FROM rule_s2s rs
		INNER JOIN k8s_metadata m ON rs.resource_version = m.resource_version`
	whereClause, args := utils.BuildScopeFilter(scope, "rs")
	if whereClause != "" {
		query += " WHERE " + whereClause
	} else {
	}
	query += " ORDER BY rs.namespace, rs.name"
	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to query rule s2s")
	}
	defer rows.Close()
	for rows.Next() {
		ruleS2S, err := r.scanRuleS2S(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan rule s2s")
		}
		if err := consume(ruleS2S); err != nil {
			return err
		}
	}
	return rows.Err()
}
func (r *Reader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	query := `
		SELECT rs.namespace, rs.name, rs.traffic,
		       rs.service_local_ref, rs.service_ref, rs.ieagag_rule_refs, rs.trace,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at, m.deletion_timestamp
		FROM rule_s2s rs
		INNER JOIN k8s_metadata m ON rs.resource_version = m.resource_version
		WHERE rs.namespace = $1 AND rs.name = $2`
	row := r.queryRow(ctx, query, id.Namespace, id.Name)
	ruleS2S, err := r.scanRuleS2SRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan rule s2s")
	}
	return ruleS2S, nil
}
func (r *Reader) scanRuleS2S(rows pgx.Rows) (models.RuleS2S, error) {
	var ruleS2S models.RuleS2S
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var traffic string
	var serviceLocalRefJSON, serviceRefJSON []byte
	var ieagagRuleRefsJSON []byte
	var trace bool
	err := rows.Scan(
		&ruleS2S.Namespace,
		&ruleS2S.Name,
		&traffic,
		&serviceLocalRefJSON,
		&serviceRefJSON,
		&ieagagRuleRefsJSON,
		&trace,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
		&deletionTS,
	)
	if err != nil {
		return ruleS2S, err
	}
	ruleS2S.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return ruleS2S, err
	}
	ruleS2S.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(ruleS2S.Name, models.WithNamespace(ruleS2S.Namespace)))
	ruleS2S.Traffic = models.Traffic(traffic)
	ruleS2S.Trace = trace
	if len(serviceLocalRefJSON) > 0 {
		if err := json.Unmarshal(serviceLocalRefJSON, &ruleS2S.ServiceLocalRef); err != nil {
			return ruleS2S, errors.Wrap(err, "failed to unmarshal service_local_ref")
		}
	}
	if len(serviceRefJSON) > 0 {
		if err := json.Unmarshal(serviceRefJSON, &ruleS2S.ServiceRef); err != nil {
			return ruleS2S, errors.Wrap(err, "failed to unmarshal service_ref")
		}
	}
	if len(ieagagRuleRefsJSON) > 0 {
		if err := json.Unmarshal(ieagagRuleRefsJSON, &ruleS2S.IEAgAgRuleRefs); err != nil {
			return ruleS2S, errors.Wrap(err, "failed to unmarshal ieagag_rule_refs")
		}
	}
	return ruleS2S, nil
}
func (r *Reader) scanRuleS2SRow(row pgx.Row) (*models.RuleS2S, error) {
	var ruleS2S models.RuleS2S
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	var traffic string
	var serviceLocalRefJSON, serviceRefJSON []byte
	var ieagagRuleRefsJSON []byte
	var trace bool
	err := row.Scan(
		&ruleS2S.Namespace,
		&ruleS2S.Name,
		&traffic,
		&serviceLocalRefJSON,
		&serviceRefJSON,
		&ieagagRuleRefsJSON,
		&trace,
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
	ruleS2S.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return nil, err
	}
	ruleS2S.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(ruleS2S.Name, models.WithNamespace(ruleS2S.Namespace)))
	ruleS2S.Traffic = models.Traffic(traffic)
	ruleS2S.Trace = trace
	if len(serviceLocalRefJSON) > 0 {
		if err := json.Unmarshal(serviceLocalRefJSON, &ruleS2S.ServiceLocalRef); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal service_local_ref")
		}
	}
	if len(serviceRefJSON) > 0 {
		if err := json.Unmarshal(serviceRefJSON, &ruleS2S.ServiceRef); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal service_ref")
		}
	}
	if len(ieagagRuleRefsJSON) > 0 {
		if err := json.Unmarshal(ieagagRuleRefsJSON, &ruleS2S.IEAgAgRuleRefs); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal ieagag_rule_refs")
		}
	}
	return &ruleS2S, nil
}
