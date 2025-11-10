package worker

import (
	"fmt"
	"strings"
	"time"
)

// Message template constants for PendingSync condition
const (
	MsgInitialSync          = "Sync pending"
	MsgAwaitingSGroup       = "Waiting for SGROUP sync"
	MsgSyncComplete         = "All sync operations complete"
	MsgRetryFmt             = "Retry %d/%d in %s: %s"
	MsgPermanentFailureFmt  = "Sync failed permanently after %d attempts: %s"
	MsgAwaitingDependencies = "Waiting for: %s"
	MsgMultiplePendingFmt   = "%d resources pending: %s"
	MsgSyncingToSGroup      = "Syncing to SGROUP (attempt %d)"
	MsgEntityDependencyWait = "Waiting for %d dependencies to be Ready (e.g., %s/%s)"
	MsgAddressGroupWaitFmt  = "Waiting for AddressGroup '%s/%s' to sync to SGROUP before updating Host"
)

// formatRetryMessage formats a retry message with attempt info and error details
func formatRetryMessage(attempt, maxAttempts int, nextRetry time.Duration, err error) string {
	// Truncate error message if too long
	errMsg := err.Error()
	if len(errMsg) > 100 {
		errMsg = errMsg[:97] + "..."
	}
	return fmt.Sprintf(MsgRetryFmt, attempt, maxAttempts, formatDuration(nextRetry), errMsg)
}

// formatPermanentFailureMessage formats a permanent failure message
func formatPermanentFailureMessage(attempts int, err error) string {
	// Truncate error message if too long
	errMsg := err.Error()
	if len(errMsg) > 150 {
		errMsg = errMsg[:147] + "..."
	}
	return fmt.Sprintf(MsgPermanentFailureFmt, attempts, errMsg)
}

// formatDependenciesMessage formats a message listing pending dependencies
func formatDependenciesMessage(pendingResources []string) string {
	// Limit to first 5 dependencies to avoid overly long messages
	if len(pendingResources) > 5 {
		shown := strings.Join(pendingResources[:5], ", ")
		remaining := len(pendingResources) - 5
		return fmt.Sprintf(MsgAwaitingDependencies, shown) + fmt.Sprintf(" (+%d more)", remaining)
	}
	return fmt.Sprintf(MsgAwaitingDependencies, strings.Join(pendingResources, ", "))
}

// formatMultiplePendingMessage formats a message for multiple pending resources with attempt counts
func formatMultiplePendingMessage(pendingDetails []string) string {
	count := len(pendingDetails)
	// Limit to first 3 resources with details
	if count > 3 {
		shown := strings.Join(pendingDetails[:3], ", ")
		remaining := count - 3
		return fmt.Sprintf(MsgMultiplePendingFmt, count, shown) + fmt.Sprintf(" (+%d more)", remaining)
	}
	return fmt.Sprintf(MsgMultiplePendingFmt, count, strings.Join(pendingDetails, ", "))
}

// formatSyncingMessage formats a message for ongoing sync operation
func formatSyncingMessage(attempt int) string {
	return fmt.Sprintf(MsgSyncingToSGroup, attempt)
}

// formatEntityDependencyWaitMessage formats a message for entity dependency waiting
func formatEntityDependencyWaitMessage(depCount int, exampleType, exampleName string) string {
	return fmt.Sprintf(MsgEntityDependencyWait, depCount, exampleType, exampleName)
}

// formatAddressGroupWaitMessage formats a message when Host waits for AddressGroup sync
// This is a specific case for Host→AddressGroup dependency when Host is bound (IsBound=true)
func formatAddressGroupWaitMessage(agNamespace, agName string) string {
	return fmt.Sprintf(MsgAddressGroupWaitFmt, agNamespace, agName)
}

// Note: formatDuration is defined in health.go - we reuse that implementation
