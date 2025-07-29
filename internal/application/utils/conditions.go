package utils

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain/models"
)

const (
	// DefaultMaxRetries is the default number of retries for operations
	DefaultMaxRetries = 5

	// DefaultRetryInterval is the default interval between retries
	DefaultRetryInterval = 100 * time.Millisecond
)

// SetReadyCondition sets the Ready condition on a resource
func SetReadyCondition(obj models.Resource, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: obj.GetGeneration(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	obj.GetMeta().SetCondition(condition)
}

// SetLinkedCondition sets the Linked condition on a NetworkBinding resource
func SetLinkedCondition(obj models.Resource, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               "Linked",
		Status:             status,
		ObservedGeneration: obj.GetGeneration(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	obj.GetMeta().SetCondition(condition)
}

// SetSyncSuccessCondition sets the Ready condition to True with a success message
func SetSyncSuccessCondition(obj models.Resource) {
	SetReadyCondition(obj, metav1.ConditionTrue, "SyncSucceeded", "Successfully synced with external source")
}

// SetSyncFailedCondition sets the Ready condition to False with an error message
func SetSyncFailedCondition(obj models.Resource, err error) {
	SetReadyCondition(obj, metav1.ConditionFalse, "SyncFailed", fmt.Sprintf("Failed to sync with external source: %v", err))
}

// SetDeletionFailedCondition sets the Ready condition to False with a deletion error message
func SetDeletionFailedCondition(obj models.Resource, err error) {
	SetReadyCondition(obj, metav1.ConditionFalse, "DeletionFailed", fmt.Sprintf("Failed to sync deletion with external source: %v", err))
}

// SetFinalizerRemovalFailedCondition sets the Ready condition to False with a finalizer removal error message
func SetFinalizerRemovalFailedCondition(obj models.Resource, err error) {
	SetReadyCondition(obj, metav1.ConditionFalse, "FinalizerRemovalFailed", fmt.Sprintf("Failed to remove finalizer: %v", err))
}

// UpdateStatusWithRetry updates a resource's status with retries on conflict
func UpdateStatusWithRetry(ctx context.Context, repo models.Repository, obj models.Resource, maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		err := repo.Update(ctx, obj)
		if err == nil {
			return nil
		}

		// Check if it's a conflict error (we'll need to implement this check)
		if isConflictError(err) {
			// Wait before retrying with exponential backoff
			backoff := DefaultRetryInterval * time.Duration(1<<uint(i))
			time.Sleep(backoff)
		} else {
			return err
		}
	}

	return fmt.Errorf("failed to update resource status after %d retries", maxRetries)
}

// UpdateStatusWithCondition updates the status of an object with the given update function
// and logs any errors. Returns true if the update was successful, false otherwise.
func UpdateStatusWithCondition(ctx context.Context, repo models.Repository, obj models.Resource, updateFunc func(models.Resource) error) bool {
	if err := updateFunc(obj); err != nil {
		return false
	}

	// Use UpdateStatusWithRetry for robust retry handling
	if err := UpdateStatusWithRetry(ctx, repo, obj, DefaultMaxRetries); err != nil {
		return false
	}

	return true
}

// UpdateStatusWithReadyCondition sets the Ready condition and updates the status in a single operation
func UpdateStatusWithReadyCondition(ctx context.Context, repo models.Repository, obj models.Resource,
	status metav1.ConditionStatus, reason, message string, additionalUpdates func(models.Resource) error) bool {

	updateFunc := func(o models.Resource) error {
		// Set the Ready condition
		SetReadyCondition(o, status, reason, message)

		// Apply any additional updates if provided
		if additionalUpdates != nil {
			if err := additionalUpdates(o); err != nil {
				return err
			}
		}

		return nil
	}

	if err := updateFunc(obj); err != nil {
		return false
	}

	if err := repo.Update(ctx, obj); err != nil {
		return false
	}

	return true
}

// UpdateStatusWithSyncSuccess sets a success Ready condition and updates the status
func UpdateStatusWithSyncSuccess(ctx context.Context, repo models.Repository, obj models.Resource,
	additionalUpdates func(models.Resource) error) bool {

	return UpdateStatusWithReadyCondition(ctx, repo, obj,
		metav1.ConditionTrue, "SyncSucceeded", "Successfully synced with external source", additionalUpdates)
}

// UpdateStatusWithSyncFailure sets a failure Ready condition with the error and updates the status
func UpdateStatusWithSyncFailure(ctx context.Context, repo models.Repository, obj models.Resource,
	err error, additionalUpdates func(models.Resource) error) bool {

	return UpdateStatusWithReadyCondition(ctx, repo, obj,
		metav1.ConditionFalse, "SyncFailed", fmt.Sprintf("Failed to sync with external source: %v", err), additionalUpdates)
}

// UpdateStatusWithDeletionFailure sets a deletion failure Ready condition and updates the status
func UpdateStatusWithDeletionFailure(ctx context.Context, repo models.Repository, obj models.Resource,
	err error, additionalUpdates func(models.Resource) error) bool {

	return UpdateStatusWithReadyCondition(ctx, repo, obj,
		metav1.ConditionFalse, "DeletionFailed", fmt.Sprintf("Failed to sync deletion with external source: %v", err), additionalUpdates)
}

// UpdateStatusWithFinalizerRemovalFailure sets a finalizer removal failure Ready condition and updates the status
func UpdateStatusWithFinalizerRemovalFailure(ctx context.Context, repo models.Repository, obj models.Resource,
	err error, additionalUpdates func(models.Resource) error) bool {

	return UpdateStatusWithReadyCondition(ctx, repo, obj,
		metav1.ConditionFalse, "FinalizerRemovalFailed", fmt.Sprintf("Failed to remove finalizer: %v", err), additionalUpdates)
}

// IsReadyConditionTrue checks if the Ready condition is true for the given object
func IsReadyConditionTrue(obj models.Resource) bool {
	conditions := obj.GetMeta().GetConditions()
	for _, condition := range conditions {
		if condition.Type == "Ready" {
			return condition.Status == metav1.ConditionTrue
		}
	}
	// If the condition is not found, assume it's not ready
	return false
}

// Helper functions

// isConflictError checks if the error is a conflict error
func isConflictError(err error) bool {
	// This is a simplified check - in a real implementation, you'd check for specific error types
	// that indicate a conflict (e.g., version mismatch, concurrent modification)
	return err != nil && (err.Error() == "conflict" || err.Error() == "version mismatch")
}
