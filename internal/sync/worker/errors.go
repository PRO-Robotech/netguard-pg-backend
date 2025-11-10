package worker

import "errors"

// Permanent errors that should not be retried beyond validation attempts
var (
	// ErrResourceDeleted indicates that the resource was deleted before sync could complete
	// This is a permanent error - the resource no longer exists in the database
	// and cannot be synced to SGROUP. The outbox entry should be marked as FAILED_PERMANENT.
	ErrResourceDeleted = errors.New("resource not found (deleted)")

	// ErrWaitingForSync indicates that a process resource (like HostBinding) is waiting
	// for affected resources to sync to SGROUP before it can be physically deleted.
	// This is NOT an error - the operation will be retried without incrementing attempts.
	ErrWaitingForSync = errors.New("waiting for affected resources sync")
)

// IsWaitingForSync checks if the error is ErrWaitingForSync
func IsWaitingForSync(err error) bool {
	return errors.Is(err, ErrWaitingForSync)
}
