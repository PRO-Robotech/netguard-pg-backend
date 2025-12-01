package pg

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	pkgerrors "github.com/pkg/errors"
)

func TestIsSerializationError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "serialization failure 40001",
			err:      &pgconn.PgError{Code: "40001"},
			expected: true,
		},
		{
			name:     "deadlock detected 40P01",
			err:      &pgconn.PgError{Code: "40P01"},
			expected: true,
		},
		{
			name:     "unique violation 23505",
			err:      &pgconn.PgError{Code: "23505"},
			expected: false,
		},
		{
			name:     "foreign key violation 23503",
			err:      &pgconn.PgError{Code: "23503"},
			expected: false,
		},
		{
			name:     "non-pg error",
			err:      errors.New("some generic error"),
			expected: false,
		},
		{
			name:     "wrapped serialization error",
			err:      pkgerrors.Wrap(&pgconn.PgError{Code: "40001"}, "operation failed"),
			expected: true,
		},
		{
			name:     "wrapped deadlock error",
			err:      pkgerrors.Wrap(&pgconn.PgError{Code: "40P01"}, "operation failed"),
			expected: true,
		},
		{
			name:     "double wrapped serialization error",
			err:      pkgerrors.Wrap(pkgerrors.Wrap(&pgconn.PgError{Code: "40001"}, "inner"), "outer"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSerializationError(tt.err)
			if result != tt.expected {
				t.Errorf("IsSerializationError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestSerializationError(t *testing.T) {
	originalErr := &pgconn.PgError{Code: "40001", Message: "could not serialize access"}
	serErr := &SerializationError{
		Err:      originalErr,
		Attempts: 3,
	}

	// Test Error() method
	errMsg := serErr.Error()
	if errMsg == "" {
		t.Error("SerializationError.Error() returned empty string")
	}

	expectedContains := "serialization failure after 3 attempts"
	if !contains(errMsg, expectedContains) {
		t.Errorf("SerializationError.Error() = %q, want it to contain %q", errMsg, expectedContains)
	}

	// Test Unwrap() method
	unwrapped := serErr.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("SerializationError.Unwrap() = %v, want %v", unwrapped, originalErr)
	}

	// Test errors.Is compatibility
	if !errors.Is(serErr, originalErr) {
		t.Error("errors.Is(serErr, originalErr) = false, want true")
	}

	// Test errors.As compatibility
	var pgErr *pgconn.PgError
	if !errors.As(serErr, &pgErr) {
		t.Error("errors.As(serErr, &pgErr) = false, want true")
	}
	if pgErr.Code != "40001" {
		t.Errorf("pgErr.Code = %q, want %q", pgErr.Code, "40001")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
