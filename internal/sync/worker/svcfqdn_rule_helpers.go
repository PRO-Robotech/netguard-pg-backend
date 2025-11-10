package worker

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// SvcFqdnRuleInfo holds metadata about a SvcFqdnRule resource
type SvcFqdnRuleInfo struct {
	Namespace         string
	Name              string
	ResourceVersion   int64
	DeletionTimestamp *time.Time
}

// findSvcFqdnRulesForService returns rules where the Service participates as the source.
func (w *OutboxWorker) findSvcFqdnRulesForService(
	ctx context.Context,
	serviceNamespace string,
	serviceName string,
) ([]SvcFqdnRuleInfo, error) {
	query := `
	        SELECT
	            fr.namespace,
	            fr.name,
	            fr.resource_version,
	            m.deletion_timestamp
	        FROM svc_fqdn_rules fr
	        JOIN k8s_metadata m ON m.resource_version = fr.resource_version
	        WHERE fr.service_from_ref->>'namespace' = $1
	          AND fr.service_from_ref->>'name' = $2`

	rows, err := w.pool.Query(ctx, query, serviceNamespace, serviceName)
	if err != nil {
		return nil, fmt.Errorf("query svc_fqdn_rules for service %s/%s failed: %w", serviceNamespace, serviceName, err)
	}
	defer rows.Close()

	var rules []SvcFqdnRuleInfo
	for rows.Next() {
		var rule SvcFqdnRuleInfo
		if err := rows.Scan(&rule.Namespace, &rule.Name, &rule.ResourceVersion, &rule.DeletionTimestamp); err != nil {
			return nil, fmt.Errorf("scan svc_fqdn_rule failed: %w", err)
		}
		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate svc_fqdn_rules failed: %w", err)
	}

	return rules, nil
}

// markSvcFqdnRuleForDeletion issues a soft delete for the rule, creating outbox entry via triggers.
func (w *OutboxWorker) markSvcFqdnRuleForDeletion(
	ctx context.Context,
	rule SvcFqdnRuleInfo,
) error {
	if rule.DeletionTimestamp != nil {
		w.logger.Debug("SvcFqdnRule already marked for deletion",
			zap.String("namespace", rule.Namespace),
			zap.String("name", rule.Name))
		return nil
	}

	w.logger.Info("marking SvcFqdnRule for deletion (soft delete)",
		zap.String("namespace", rule.Namespace),
		zap.String("name", rule.Name),
		zap.Int64("resource_version", rule.ResourceVersion))

	cmd := `DELETE FROM svc_fqdn_rules WHERE namespace = $1 AND name = $2`
	if _, err := w.pool.Exec(ctx, cmd, rule.Namespace, rule.Name); err != nil {
		return fmt.Errorf("delete svc_fqdn_rule %s/%s failed: %w", rule.Namespace, rule.Name, err)
	}

	return nil
}

// checkAllSvcFqdnRulesDeleted returns true when every rule has been physically removed.
func (w *OutboxWorker) checkAllSvcFqdnRulesDeleted(
	ctx context.Context,
	rules []SvcFqdnRuleInfo,
) (bool, error) {
	if len(rules) == 0 {
		return true, nil
	}

	for _, rule := range rules {
		var count int
		query := `SELECT COUNT(*) FROM svc_fqdn_rules WHERE namespace = $1 AND name = $2`
		if err := w.pool.QueryRow(ctx, query, rule.Namespace, rule.Name).Scan(&count); err != nil {
			return false, fmt.Errorf("check svc_fqdn_rule %s/%s failed: %w", rule.Namespace, rule.Name, err)
		}

		if count > 0 {
			w.logger.Debug("SvcFqdnRule still present", zap.String("namespace", rule.Namespace), zap.String("name", rule.Name))
			return false, nil
		}
	}

	return true, nil
}
