package pg

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/errors"
)

// PostgreSQL error codes for serialization failures
const (
	// PgSerializationFailure is SQLSTATE 40001 - serialization_failure
	PgSerializationFailure = "40001"
	// PgDeadlockDetected is SQLSTATE 40P01 - deadlock_detected
	PgDeadlockDetected = "40P01"
)

// IsSerializationError checks if the error is a PostgreSQL serialization failure (SQLSTATE 40001)
// or deadlock detected (SQLSTATE 40P01). These errors indicate that the transaction
// should be retried.
func IsSerializationError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == PgSerializationFailure || pgErr.Code == PgDeadlockDetected
	}
	return false
}

// SerializationError wraps a serialization failure that occurred after multiple retry attempts
type SerializationError struct {
	Err      error
	Attempts int
}

// Error implements the error interface
func (e *SerializationError) Error() string {
	return fmt.Sprintf("serialization failure after %d attempts: %v", e.Attempts, e.Err)
}

// Unwrap returns the underlying error for errors.Is/As compatibility
func (e *SerializationError) Unwrap() error {
	return e.Err
}
