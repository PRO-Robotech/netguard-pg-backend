package worker

import "errors"

// Permanent errors that should not be retried beyond validation attempts
var (
	// ErrResourceDeleted indicates that the resource was deleted before sync could complete
	// This is a permanent error - the resource no longer exists in the database
	// and cannot be synced to SGROUP. The outbox entry should be marked as FAILED_PERMANENT.
	ErrResourceDeleted = errors.New("resource not found (deleted)")
)
