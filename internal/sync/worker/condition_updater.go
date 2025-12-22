package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// PendingSync condition reason values
const (
	ReasonInitialSync              = "InitialSync"
	ReasonAwaitingSGroupSync       = "AwaitingSGroupSync"
	ReasonRetryingSync             = "RetryingSync"
	ReasonSynced                   = "Synced"
	ReasonFailedPermanent          = "FailedPermanent"
	ReasonAwaitingDependencies     = "AwaitingDependencies"
	ReasonAllDependenciesReady     = "AllDependenciesReady"
	ReasonAwaitingEntityDeps       = "AwaitingEntityDependencies"
	ReasonAwaitingAddressGroupSync = "AwaitingAddressGroupSync"
)

// updatePendingSyncCondition updates the PendingSync condition in k8s_metadata by namespace+name
// This is the PRIMARY method for updating PendingSync conditions throughout the worker
//
// Queries by (resource_type, namespace, name) to match the schema introduced in migration 030.
//
// Parameters:
//   - resourceType: Type of resource (Host, Network, AddressGroup, Service)
//   - namespace: Kubernetes namespace
//   - name: Kubernetes resource name
//   - pending: true if sync is pending/in-progress, false if complete
//   - message: human-readable status message
//
// The function automatically determines the appropriate reason based on the message content
func (w *OutboxWorker) updatePendingSyncCondition(
	ctx context.Context,
	resourceType string, // NEW: Need entity type for routing
	namespace string, // NEW: K8s namespace
	name string, // NEW: K8s name
	pending bool,
	message string,
) error {
	reason := determinePendingSyncReason(pending, message)
	status := boolToConditionStatus(pending)

	condition := map[string]interface{}{
		"type":               "PendingSync",
		"status":             status,
		"reason":             reason,
		"message":            message,
		"lastTransitionTime": time.Now().Format(time.RFC3339),
	}

	conditionJSON, err := json.Marshal(condition)
	if err != nil {
		return fmt.Errorf("failed to marshal condition: %w", err)
	}

	safeConditionsExpr := "COALESCE(NULLIF(conditions, 'null'::jsonb), '[]'::jsonb)"
	conditionsNullCheck := "conditions IS NULL OR conditions = 'null'::jsonb"

	cfg, err := w.buildPendingSyncSQLConfig(resourceType, conditionsNullCheck, safeConditionsExpr)
	if err != nil {
		return err
	}

	existingCond, fetchErr := w.fetchPendingSyncCondition(ctx, cfg.tableName, namespace, name)
	if fetchErr != nil {
		w.logger.Warn("failed to fetch existing PendingSync condition, proceeding with update",
			zap.String("resource_type", resourceType),
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.Error(fetchErr))
	} else if existingCond != nil {
		if existingCond.Status == status &&
			existingCond.Reason == reason &&
			existingCond.Message == message {
			return nil
		}
	}

	result, err := w.pool.Exec(ctx, cfg.updateQuery, conditionJSON, namespace, name)
	if err != nil {
		w.logger.Error("failed to update PendingSync condition",
			zap.String("resource_type", resourceType),
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.Error(err))
		return fmt.Errorf("failed to update PendingSync condition: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		// This is not necessarily an error - the resource might not have k8s_metadata yet
		// or it might have been deleted
		return nil
	}

	return nil
}

type pendingSyncSQLConfig struct {
	tableName   string
	updateQuery string
}

func (w *OutboxWorker) buildPendingSyncSQLConfig(resourceType, conditionsNullCheck, safeConditionsExpr string) (*pendingSyncSQLConfig, error) {
	switch resourceType {
	case "Host":
		return &pendingSyncSQLConfig{
			tableName: "hosts",
			updateQuery: fmt.Sprintf(`
			UPDATE k8s_metadata km
			SET conditions = CASE
				WHEN %s THEN jsonb_build_array($1::jsonb)
				ELSE (
					SELECT jsonb_agg(
						CASE
							WHEN elem->>'type' = 'PendingSync' THEN $1::jsonb
							ELSE elem
						END
					)
					FROM jsonb_array_elements(%s) AS elem
				) || CASE
					WHEN NOT EXISTS (
						SELECT 1 FROM jsonb_array_elements(%s) AS elem
						WHERE elem->>'type' = 'PendingSync'
					) THEN jsonb_build_array($1::jsonb)
					ELSE '[]'::jsonb
				END
			END,
			updated_at = NOW()
			FROM hosts h
			WHERE h.namespace = $2 AND h.name = $3
			  AND h.resource_version = km.resource_version
		`, conditionsNullCheck, safeConditionsExpr, safeConditionsExpr),
		}, nil
	case "Network":
		return &pendingSyncSQLConfig{
			tableName: "networks",
			updateQuery: fmt.Sprintf(`
			UPDATE k8s_metadata km
			SET conditions = CASE
				WHEN %s THEN jsonb_build_array($1::jsonb)
				ELSE (
					SELECT jsonb_agg(
						CASE
							WHEN elem->>'type' = 'PendingSync' THEN $1::jsonb
							ELSE elem
						END
					)
					FROM jsonb_array_elements(%s) AS elem
				) || CASE
					WHEN NOT EXISTS (
						SELECT 1 FROM jsonb_array_elements(%s) AS elem
						WHERE elem->>'type' = 'PendingSync'
					) THEN jsonb_build_array($1::jsonb)
					ELSE '[]'::jsonb
				END
			END,
			updated_at = NOW()
			FROM networks n
			WHERE n.namespace = $2 AND n.name = $3
			  AND n.resource_version = km.resource_version
		`, conditionsNullCheck, safeConditionsExpr, safeConditionsExpr),
		}, nil
	case "AddressGroup":
		return &pendingSyncSQLConfig{
			tableName: "address_groups",
			updateQuery: fmt.Sprintf(`
			UPDATE k8s_metadata km
			SET conditions = CASE
				WHEN %s THEN jsonb_build_array($1::jsonb)
				ELSE (
					SELECT jsonb_agg(
						CASE
							WHEN elem->>'type' = 'PendingSync' THEN $1::jsonb
							ELSE elem
						END
					)
					FROM jsonb_array_elements(%s) AS elem
				) || CASE
					WHEN NOT EXISTS (
						SELECT 1 FROM jsonb_array_elements(%s) AS elem
						WHERE elem->>'type' = 'PendingSync'
					) THEN jsonb_build_array($1::jsonb)
					ELSE '[]'::jsonb
				END
			END,
			updated_at = NOW()
			FROM address_groups ag
			WHERE ag.namespace = $2 AND ag.name = $3
			  AND ag.resource_version = km.resource_version
		`, conditionsNullCheck, safeConditionsExpr, safeConditionsExpr),
		}, nil
	case "Service":
		return &pendingSyncSQLConfig{
			tableName: "services",
			updateQuery: fmt.Sprintf(`
			UPDATE k8s_metadata km
			SET conditions = CASE
				WHEN %s THEN jsonb_build_array($1::jsonb)
				ELSE (
					SELECT jsonb_agg(
						CASE
							WHEN elem->>'type' = 'PendingSync' THEN $1::jsonb
							ELSE elem
						END
					)
					FROM jsonb_array_elements(%s) AS elem
				) || CASE
					WHEN NOT EXISTS (
						SELECT 1 FROM jsonb_array_elements(%s) AS elem
						WHERE elem->>'type' = 'PendingSync'
					) THEN jsonb_build_array($1::jsonb)
					ELSE '[]'::jsonb
				END
			END,
			updated_at = NOW()
			FROM services s
			WHERE s.namespace = $2 AND s.name = $3
			  AND s.resource_version = km.resource_version
		`, conditionsNullCheck, safeConditionsExpr, safeConditionsExpr),
		}, nil
	case "SvcSvcRule":
		return &pendingSyncSQLConfig{
			tableName: "svc_svc_rules",
			updateQuery: fmt.Sprintf(`
			UPDATE k8s_metadata km
			SET conditions = CASE
				WHEN %s THEN jsonb_build_array($1::jsonb)
				ELSE (
					SELECT jsonb_agg(
						CASE
							WHEN elem->>'type' = 'PendingSync' THEN $1::jsonb
							ELSE elem
						END
					)
					FROM jsonb_array_elements(%s) AS elem
				) || CASE
					WHEN NOT EXISTS (
						SELECT 1 FROM jsonb_array_elements(%s) AS elem
						WHERE elem->>'type' = 'PendingSync'
					) THEN jsonb_build_array($1::jsonb)
					ELSE '[]'::jsonb
				END
			END,
			updated_at = NOW()
			FROM svc_svc_rules sr
			WHERE sr.namespace = $2 AND sr.name = $3
			  AND sr.resource_version = km.resource_version
		`, conditionsNullCheck, safeConditionsExpr, safeConditionsExpr),
		}, nil
	case "SvcFqdnRule":
		return &pendingSyncSQLConfig{
			tableName: "svc_fqdn_rules",
			updateQuery: fmt.Sprintf(`
			UPDATE k8s_metadata km
			SET conditions = CASE
				WHEN %s THEN jsonb_build_array($1::jsonb)
				ELSE (
					SELECT jsonb_agg(
						CASE
							WHEN elem->>'type' = 'PendingSync' THEN $1::jsonb
							ELSE elem
						END
					)
					FROM jsonb_array_elements(%s) AS elem
				) || CASE
					WHEN NOT EXISTS (
						SELECT 1 FROM jsonb_array_elements(%s) AS elem
						WHERE elem->>'type' = 'PendingSync'
					) THEN jsonb_build_array($1::jsonb)
					ELSE '[]'::jsonb
				END
			END,
			updated_at = NOW()
			FROM svc_fqdn_rules fr
			WHERE fr.namespace = $2 AND fr.name = $3
			  AND fr.resource_version = km.resource_version
		`, conditionsNullCheck, safeConditionsExpr, safeConditionsExpr),
		}, nil
	case "IECidrSvcRule":
		return &pendingSyncSQLConfig{
			tableName: "ie_cidr_svc_rules",
			updateQuery: fmt.Sprintf(`
			UPDATE k8s_metadata km
			SET conditions = CASE
				WHEN %s THEN jsonb_build_array($1::jsonb)
				ELSE (
					SELECT jsonb_agg(
						CASE
							WHEN elem->>'type' = 'PendingSync' THEN $1::jsonb
							ELSE elem
						END
					)
					FROM jsonb_array_elements(%s) AS elem
				) || CASE
					WHEN NOT EXISTS (
						SELECT 1 FROM jsonb_array_elements(%s) AS elem
						WHERE elem->>'type' = 'PendingSync'
					) THEN jsonb_build_array($1::jsonb)
					ELSE '[]'::jsonb
				END
			END,
			updated_at = NOW()
			FROM ie_cidr_svc_rules ir
			WHERE ir.namespace = $2 AND ir.name = $3
			  AND ir.resource_version = km.resource_version
		`, conditionsNullCheck, safeConditionsExpr, safeConditionsExpr),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported resource type for condition update: %s", resourceType)
	}
}

// determinePendingSyncReason determines the appropriate reason based on message content
// This provides semantic meaning to the condition beyond just True/False
func determinePendingSyncReason(pending bool, message string) string {
	if !pending {
		// Sync complete
		return ReasonSynced
	}

	msgLower := strings.ToLower(message)

	// Check for specific patterns in message - ORDER MATTERS!
	// More specific patterns first, then more general ones

	// Permanent failure (must check before "failed" or other keywords)
	if strings.Contains(msgLower, "permanently") || strings.Contains(msgLower, "failed permanent") {
		return ReasonFailedPermanent
	}

	// Sync pending (initial state)
	if strings.Contains(msgLower, "sync pending") {
		return ReasonInitialSync
	}

	// Syncing to SGROUP (must check before "attempt" to catch "Syncing to SGROUP (attempt 1)")
	if strings.Contains(msgLower, "syncing to sgroup") {
		return ReasonAwaitingSGroupSync
	}

	// Retry (check after specific "syncing" message)
	if strings.Contains(msgLower, "retry") || (strings.Contains(msgLower, "attempt") && strings.Contains(msgLower, "retry")) {
		return ReasonRetryingSync
	}

	// Waiting for process resource dependencies
	if strings.Contains(msgLower, "waiting for:") {
		return ReasonAwaitingDependencies
	}

	// Waiting for entity dependencies
	if strings.Contains(msgLower, "dependencies to be ready") {
		return ReasonAwaitingEntityDeps
	}

	// Waiting for AddressGroup to sync (Host→AG dependency)
	// This is a specific case when Host is bound and waits for AG to sync to SGROUP
	if strings.Contains(msgLower, "addressgroup") && strings.Contains(msgLower, "to sync to sgroup") {
		return ReasonAwaitingAddressGroupSync
	}

	// Default for pending=true
	return ReasonAwaitingSGroupSync
}

// boolToConditionStatus converts bool to Kubernetes condition status string
func boolToConditionStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

// Helper methods for common condition update scenarios

// updatePendingSyncPending sets PendingSync=True with InitialSync reason
func (w *OutboxWorker) updatePendingSyncPending(ctx context.Context, resourceType, namespace, name string) error {
	return w.updatePendingSyncCondition(ctx, resourceType, namespace, name, true, MsgInitialSync)
}

// updatePendingSyncSyncing sets PendingSync=True with sync in progress message
func (w *OutboxWorker) updatePendingSyncSyncing(ctx context.Context, resourceType, namespace, name string, attempt int) error {
	msg := formatSyncingMessage(attempt)
	return w.updatePendingSyncCondition(ctx, resourceType, namespace, name, true, msg)
}

// updatePendingSyncComplete sets PendingSync=False with success message
func (w *OutboxWorker) updatePendingSyncComplete(ctx context.Context, resourceType, namespace, name string) error {
	return w.updatePendingSyncCondition(ctx, resourceType, namespace, name, false, MsgSyncComplete)
}

// updatePendingSyncRetrying sets PendingSync=True with retry details
func (w *OutboxWorker) updatePendingSyncRetrying(
	ctx context.Context,
	resourceType, namespace, name string,
	attempt, maxAttempts int,
	nextRetry time.Duration,
	err error,
) error {
	msg := formatRetryMessage(attempt, maxAttempts, nextRetry, err)
	return w.updatePendingSyncCondition(ctx, resourceType, namespace, name, true, msg)
}

// updatePendingSyncFailedPermanent sets PendingSync=True with permanent failure message
func (w *OutboxWorker) updatePendingSyncFailedPermanent(
	ctx context.Context,
	resourceType, namespace, name string,
	attempts int,
	err error,
) error {
	msg := formatPermanentFailureMessage(attempts, err)
	return w.updatePendingSyncCondition(ctx, resourceType, namespace, name, true, msg)
}

// updatePendingSyncWaitingDependencies sets PendingSync=True with dependency wait message
func (w *OutboxWorker) updatePendingSyncWaitingDependencies(
	ctx context.Context,
	resourceType, namespace, name string,
	pendingResources []string,
) error {
	msg := formatDependenciesMessage(pendingResources)
	return w.updatePendingSyncCondition(ctx, resourceType, namespace, name, true, msg)
}

// updatePendingSyncWaitingEntityDeps sets PendingSync=True for entity dependencies
func (w *OutboxWorker) updatePendingSyncWaitingEntityDeps(
	ctx context.Context,
	resourceType, namespace, name string,
	depCount int,
	exampleType, exampleName string,
) error {
	msg := formatEntityDependencyWaitMessage(depCount, exampleType, exampleName)
	return w.updatePendingSyncCondition(ctx, resourceType, namespace, name, true, msg)
}

// updatePendingSyncWaitingAddressGroup sets PendingSync=True when Host waits for AddressGroup
// This is a specific wrapper for the common case of Host→AddressGroup dependency
func (w *OutboxWorker) updatePendingSyncWaitingAddressGroup(
	ctx context.Context,
	resourceType, namespace, name string,
	agNamespace, agName string,
) error {
	msg := formatAddressGroupWaitMessage(agNamespace, agName)
	return w.updatePendingSyncCondition(ctx, resourceType, namespace, name, true, msg)
}

type pendingSyncCondition struct {
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func (w *OutboxWorker) fetchPendingSyncCondition(ctx context.Context, tableName, namespace, name string) (*pendingSyncCondition, error) {
	query := fmt.Sprintf(`
		SELECT pending.elem
		FROM %s t
		JOIN k8s_metadata km ON t.resource_version = km.resource_version
		LEFT JOIN LATERAL (
			SELECT elem
			FROM jsonb_array_elements(COALESCE(NULLIF(km.conditions, 'null'::jsonb), '[]'::jsonb)) elem
			WHERE elem->>'type' = 'PendingSync'
			LIMIT 1
		) pending ON true
		WHERE t.namespace = $1 AND t.name = $2
		LIMIT 1
	`, tableName)

	var raw []byte
	err := w.pool.QueryRow(ctx, query, namespace, name).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("fetch pending condition: %w", err)
	}

	if len(raw) == 0 {
		return nil, nil
	}

	var cond pendingSyncCondition
	if err := json.Unmarshal(raw, &cond); err != nil {
		return nil, fmt.Errorf("decode pending condition: %w", err)
	}
	return &cond, nil
}
