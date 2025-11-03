package worker

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// SvcSvcRuleInfo holds metadata about a SvcSvcRule resource
type SvcSvcRuleInfo struct {
	Namespace         string
	Name              string
	ResourceVersion   int64
	DeletionTimestamp *time.Time
}

// findSvcSvcRulesForService returns rules where the Service participates as source or destination.
func (w *OutboxWorker) findSvcSvcRulesForService(
	ctx context.Context,
	serviceNamespace string,
	serviceName string,
) ([]SvcSvcRuleInfo, error) {
	query := `
        SELECT
            sr.namespace,
            sr.name,
            sr.resource_version,
            m.deletion_timestamp
        FROM svc_svc_rules sr
        JOIN k8s_metadata m ON m.resource_version = sr.resource_version
        WHERE (
            sr.service_from_ref->>'namespace' = $1 AND sr.service_from_ref->>'name' = $2
        ) OR (
            sr.service_to_ref->>'namespace' = $1 AND sr.service_to_ref->>'name' = $2
        )`

	rows, err := w.pool.Query(ctx, query, serviceNamespace, serviceName)
	if err != nil {
		return nil, fmt.Errorf("query svc_svc_rules for service %s/%s failed: %w", serviceNamespace, serviceName, err)
	}
	defer rows.Close()

	var rules []SvcSvcRuleInfo
	for rows.Next() {
		var rule SvcSvcRuleInfo
		if err := rows.Scan(&rule.Namespace, &rule.Name, &rule.ResourceVersion, &rule.DeletionTimestamp); err != nil {
			return nil, fmt.Errorf("scan svc_svc_rule failed: %w", err)
		}
		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate svc_svc_rules failed: %w", err)
	}

	return rules, nil
}

// markSvcSvcRuleForDeletion issues a soft delete for the rule, creating outbox entry via triggers.
func (w *OutboxWorker) markSvcSvcRuleForDeletion(
	ctx context.Context,
	rule SvcSvcRuleInfo,
) error {
	if rule.DeletionTimestamp != nil {
		w.logger.Debug("SvcSvcRule already marked for deletion",
			zap.String("namespace", rule.Namespace),
			zap.String("name", rule.Name))
		return nil
	}

	w.logger.Info("marking SvcSvcRule for deletion (soft delete)",
		zap.String("namespace", rule.Namespace),
		zap.String("name", rule.Name),
		zap.Int64("resource_version", rule.ResourceVersion))

	cmd := `DELETE FROM svc_svc_rules WHERE namespace = $1 AND name = $2`
	if _, err := w.pool.Exec(ctx, cmd, rule.Namespace, rule.Name); err != nil {
		return fmt.Errorf("delete svc_svc_rule %s/%s failed: %w", rule.Namespace, rule.Name, err)
	}

	return nil
}

// checkAllSvcSvcRulesDeleted returns true when every rule has been physically removed.
func (w *OutboxWorker) checkAllSvcSvcRulesDeleted(
	ctx context.Context,
	rules []SvcSvcRuleInfo,
) (bool, error) {
	if len(rules) == 0 {
		return true, nil
	}

	for _, rule := range rules {
		var count int
		query := `SELECT COUNT(*) FROM svc_svc_rules WHERE namespace = $1 AND name = $2`
		if err := w.pool.QueryRow(ctx, query, rule.Namespace, rule.Name).Scan(&count); err != nil {
			return false, fmt.Errorf("check svc_svc_rule %s/%s failed: %w", rule.Namespace, rule.Name, err)
		}

		if count > 0 {
			w.logger.Debug("SvcSvcRule still present", zap.String("namespace", rule.Namespace), zap.String("name", rule.Name))
			return false, nil
		}
	}

	return true, nil
}
