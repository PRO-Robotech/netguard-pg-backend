package readers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/utils"
)

// ListRuleS2S lists RuleS2S resources with K8s metadata support
func (r *Reader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	query := `
		SELECT rs.namespace, rs.name, rs.traffic,
		       rs.service_local_ref, rs.service_ref, rs.ieagag_rule_refs, rs.trace,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
		FROM rule_s2s rs
		INNER JOIN k8s_metadata m ON rs.resource_version = m.resource_version`

	// Apply scope filtering and deletion_timestamp filter
	whereClause, args := utils.BuildScopeFilter(scope, "rs")

	// Always filter out objects being deleted
	deletionFilter := "m.deletion_timestamp IS NULL"
	if whereClause != "" {
		query += " WHERE " + whereClause + " AND " + deletionFilter
	} else {
		query += " WHERE " + deletionFilter
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

// GetRuleS2SByID gets a RuleS2S resource by ID
func (r *Reader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	query := `
		SELECT rs.namespace, rs.name, rs.traffic,
		       rs.service_local_ref, rs.service_ref, rs.ieagag_rule_refs, rs.trace,
			   m.resource_version, m.labels, m.annotations, m.conditions,
			   m.created_at, m.updated_at
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

// scanRuleS2S scans a RuleS2S resource from pgx.Rows
func (r *Reader) scanRuleS2S(rows pgx.Rows) (models.RuleS2S, error) {
	var ruleS2S models.RuleS2S
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// RuleS2S-specific fields - JSONB columns
	var traffic string                             // Traffic enum as string
	var serviceLocalRefJSON, serviceRefJSON []byte // JSONB columns
	var ieagagRuleRefsJSON []byte                  // IEAgAg rule refs array
	var trace bool                                 // Trace field

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
	)
	if err != nil {
		return ruleS2S, err
	}

	// Convert K8s metadata (convert int64 to string)
	ruleS2S.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return ruleS2S, err
	}

	ruleS2S.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(ruleS2S.Name, models.WithNamespace(ruleS2S.Namespace)))
	ruleS2S.Traffic = models.Traffic(traffic)
	ruleS2S.Trace = trace

	// Unmarshal JSONB ObjectReferences
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

	// Unmarshal IEAgAg rule refs array
	if len(ieagagRuleRefsJSON) > 0 {
		if err := json.Unmarshal(ieagagRuleRefsJSON, &ruleS2S.IEAgAgRuleRefs); err != nil {
			return ruleS2S, errors.Wrap(err, "failed to unmarshal ieagag_rule_refs")
		}
	}

	return ruleS2S, nil
}

// scanRuleS2SRow scans a RuleS2S resource from pgx.Row
func (r *Reader) scanRuleS2SRow(row pgx.Row) (*models.RuleS2S, error) {
	var ruleS2S models.RuleS2S
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time // Temporary variables for timestamps
	var resourceVersion int64          // Scan as int64 from database

	// RuleS2S-specific fields - JSONB columns
	var traffic string                             // Traffic enum as string
	var serviceLocalRefJSON, serviceRefJSON []byte // JSONB columns
	var ieagagRuleRefsJSON []byte                  // IEAgAg rule refs array
	var trace bool                                 // Trace field

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
	)
	if err != nil {
		return nil, err
	}

	// Convert K8s metadata (convert int64 to string)
	ruleS2S.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}

	ruleS2S.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(ruleS2S.Name, models.WithNamespace(ruleS2S.Namespace)))
	ruleS2S.Traffic = models.Traffic(traffic)
	ruleS2S.Trace = trace

	// Unmarshal JSONB ObjectReferences
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

	// Unmarshal IEAgAg rule refs array
	if len(ieagagRuleRefsJSON) > 0 {
		if err := json.Unmarshal(ieagagRuleRefsJSON, &ruleS2S.IEAgAgRuleRefs); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal ieagag_rule_refs")
		}
	}

	return &ruleS2S, nil
}
