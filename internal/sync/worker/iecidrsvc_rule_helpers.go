package worker

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// IECidrSvcRuleInfo holds metadata about a IECidrSvcRule resource
type IECidrSvcRuleInfo struct {
	Namespace         string
	Name              string
	ResourceVersion   int64
	DeletionTimestamp *time.Time
}

// findIECidrSvcRulesForService returns rules where the Service participates.
func (w *OutboxWorker) findIECidrSvcRulesForService(
	ctx context.Context,
	serviceNamespace string,
	serviceName string,
) ([]IECidrSvcRuleInfo, error) {
	query := `
	        SELECT
	            r.namespace,
	            r.name,
	            r.resource_version,
	            m.deletion_timestamp
	        FROM ie_cidr_svc_rules r
	        JOIN k8s_metadata m ON m.resource_version = r.resource_version
	        WHERE r.svc_ref->>'namespace' = $1
	          AND r.svc_ref->>'name' = $2`

	rows, err := w.pool.Query(ctx, query, serviceNamespace, serviceName)
	if err != nil {
		return nil, fmt.Errorf("query ie_cidr_svc_rules for service %s/%s failed: %w", serviceNamespace, serviceName, err)
	}
	defer rows.Close()

	var rules []IECidrSvcRuleInfo
	for rows.Next() {
		var rule IECidrSvcRuleInfo
		if err := rows.Scan(&rule.Namespace, &rule.Name, &rule.ResourceVersion, &rule.DeletionTimestamp); err != nil {
			return nil, fmt.Errorf("scan ie_cidr_svc_rule failed: %w", err)
		}
		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ie_cidr_svc_rules failed: %w", err)
	}

	return rules, nil
}

// markIECidrSvcRuleForDeletion issues a soft delete for the rule, creating outbox entry via triggers.
func (w *OutboxWorker) markIECidrSvcRuleForDeletion(
	ctx context.Context,
	rule IECidrSvcRuleInfo,
) error {
	if rule.DeletionTimestamp != nil {
		w.logger.Debug("IECidrSvcRule already marked for deletion",
			zap.String("namespace", rule.Namespace),
			zap.String("name", rule.Name))
		return nil
	}

	w.logger.Info("marking IECidrSvcRule for deletion (soft delete)",
		zap.String("namespace", rule.Namespace),
		zap.String("name", rule.Name),
		zap.Int64("resource_version", rule.ResourceVersion))

	cmd := `DELETE FROM ie_cidr_svc_rules WHERE namespace = $1 AND name = $2`
	if _, err := w.pool.Exec(ctx, cmd, rule.Namespace, rule.Name); err != nil {
		return fmt.Errorf("delete ie_cidr_svc_rule %s/%s failed: %w", rule.Namespace, rule.Name, err)
	}

	return nil
}

// checkAllIECidrSvcRulesDeleted returns true when every rule has been physically removed.
func (w *OutboxWorker) checkAllIECidrSvcRulesDeleted(
	ctx context.Context,
	rules []IECidrSvcRuleInfo,
) (bool, error) {
	if len(rules) == 0 {
		return true, nil
	}

	for _, rule := range rules {
		var count int
		query := `SELECT COUNT(*) FROM ie_cidr_svc_rules WHERE namespace = $1 AND name = $2`
		if err := w.pool.QueryRow(ctx, query, rule.Namespace, rule.Name).Scan(&count); err != nil {
			return false, fmt.Errorf("check ie_cidr_svc_rule %s/%s failed: %w", rule.Namespace, rule.Name, err)
		}

		if count > 0 {
			w.logger.Debug("IECidrSvcRule still present", zap.String("namespace", rule.Namespace), zap.String("name", rule.Name))
			return false, nil
		}
	}

	return true, nil
}
