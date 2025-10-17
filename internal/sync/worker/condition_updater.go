package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// PendingSync condition reason values
const (
	ReasonInitialSync          = "InitialSync"
	ReasonAwaitingSGroupSync   = "AwaitingSGroupSync"
	ReasonRetryingSync         = "RetryingSync"
	ReasonSynced               = "Synced"
	ReasonFailedPermanent      = "FailedPermanent"
	ReasonAwaitingDependencies = "AwaitingDependencies"
	ReasonAllDependenciesReady = "AllDependenciesReady"
	ReasonAwaitingEntityDeps   = "AwaitingEntityDependencies"
)

// updatePendingSyncCondition updates the PendingSync condition in k8s_metadata by namespace+name
// This is the PRIMARY method for updating PendingSync conditions throughout the worker
//
// FIXED (Migration 030): Now queries by (resource_type, namespace, name) instead of non-existent resource_id
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

	condition := map[string]interface{}{
		"type":               "PendingSync",
		"status":             boolToConditionStatus(pending),
		"reason":             reason,
		"message":            message,
		"lastTransitionTime": time.Now().Format(time.RFC3339),
	}

	conditionJSON, err := json.Marshal(condition)
	if err != nil {
		return fmt.Errorf("failed to marshal condition: %w", err)
	}

	// Build entity-specific query with JOIN to entity table
	// This fixes P0-1: Uses (namespace, name) instead of non-existent resource_id column
	var query string
	switch resourceType {
	case "Host":
		query = `
			UPDATE k8s_metadata km
			SET conditions = CASE
				WHEN conditions IS NULL THEN jsonb_build_array($1::jsonb)
				ELSE (
					SELECT jsonb_agg(
						CASE
							WHEN elem->>'type' = 'PendingSync' THEN $1::jsonb
							ELSE elem
						END
					)
					FROM jsonb_array_elements(conditions) AS elem
				) || CASE
					WHEN NOT EXISTS (
						SELECT 1 FROM jsonb_array_elements(conditions) AS elem
						WHERE elem->>'type' = 'PendingSync'
					) THEN jsonb_build_array($1::jsonb)
					ELSE '[]'::jsonb
				END
			END,
			updated_at = NOW()
			FROM hosts h
			WHERE h.namespace = $2 AND h.name = $3
			  AND h.resource_version = km.resource_version
		`
	case "Network":
		query = `
			UPDATE k8s_metadata km
			SET conditions = CASE
				WHEN conditions IS NULL THEN jsonb_build_array($1::jsonb)
				ELSE (
					SELECT jsonb_agg(
						CASE
							WHEN elem->>'type' = 'PendingSync' THEN $1::jsonb
							ELSE elem
						END
					)
					FROM jsonb_array_elements(conditions) AS elem
				) || CASE
					WHEN NOT EXISTS (
						SELECT 1 FROM jsonb_array_elements(conditions) AS elem
						WHERE elem->>'type' = 'PendingSync'
					) THEN jsonb_build_array($1::jsonb)
					ELSE '[]'::jsonb
				END
			END,
			updated_at = NOW()
			FROM networks n
			WHERE n.namespace = $2 AND n.name = $3
			  AND n.resource_version = km.resource_version
		`
	case "AddressGroup":
		query = `
			UPDATE k8s_metadata km
			SET conditions = CASE
				WHEN conditions IS NULL THEN jsonb_build_array($1::jsonb)
				ELSE (
					SELECT jsonb_agg(
						CASE
							WHEN elem->>'type' = 'PendingSync' THEN $1::jsonb
							ELSE elem
						END
					)
					FROM jsonb_array_elements(conditions) AS elem
				) || CASE
					WHEN NOT EXISTS (
						SELECT 1 FROM jsonb_array_elements(conditions) AS elem
						WHERE elem->>'type' = 'PendingSync'
					) THEN jsonb_build_array($1::jsonb)
					ELSE '[]'::jsonb
				END
			END,
			updated_at = NOW()
			FROM address_groups ag
			WHERE ag.namespace = $2 AND ag.name = $3
			  AND ag.resource_version = km.resource_version
		`
	case "Service":
		query = `
			UPDATE k8s_metadata km
			SET conditions = CASE
				WHEN conditions IS NULL THEN jsonb_build_array($1::jsonb)
				ELSE (
					SELECT jsonb_agg(
						CASE
							WHEN elem->>'type' = 'PendingSync' THEN $1::jsonb
							ELSE elem
						END
					)
					FROM jsonb_array_elements(conditions) AS elem
				) || CASE
					WHEN NOT EXISTS (
						SELECT 1 FROM jsonb_array_elements(conditions) AS elem
						WHERE elem->>'type' = 'PendingSync'
					) THEN jsonb_build_array($1::jsonb)
					ELSE '[]'::jsonb
				END
			END,
			updated_at = NOW()
			FROM services s
			WHERE s.namespace = $2 AND s.name = $3
			  AND s.resource_version = km.resource_version
		`
	default:
		return fmt.Errorf("unsupported resource type for condition update: %s", resourceType)
	}

	result, err := w.pool.Exec(ctx, query, conditionJSON, namespace, name)
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
		w.logger.Debug("no k8s_metadata found for resource (might not exist yet)",
			zap.String("resource_type", resourceType),
			zap.String("namespace", namespace),
			zap.String("name", name))
	}

	w.logger.Debug("updated PendingSync condition",
		zap.String("resource_type", resourceType),
		zap.String("namespace", namespace),
		zap.String("name", name),
		zap.Bool("pending", pending),
		zap.String("reason", reason),
		zap.Int64("rows_affected", rowsAffected))

	return nil
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
