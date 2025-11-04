package worker

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_determinePendingSyncReason(t *testing.T) {
	tests := []struct {
		name     string
		pending  bool
		message  string
		expected string
	}{
		{
			name:     "Synced - pending false",
			pending:  false,
			message:  "All sync operations complete",
			expected: ReasonSynced,
		},
		{
			name:     "Initial sync",
			pending:  true,
			message:  "Sync pending",
			expected: ReasonInitialSync,
		},
		{
			name:     "Retrying sync with retry keyword",
			pending:  true,
			message:  "Retry 3/20 in 30s: SGROUP timeout",
			expected: ReasonRetryingSync,
		},
		{
			name:     "Retrying with lowercase",
			pending:  true,
			message:  "retry 5/20 in 1m: connection refused",
			expected: ReasonRetryingSync,
		},
		{
			name:     "Permanent failure",
			pending:  true,
			message:  "Sync failed permanently after 20 attempts: validation error",
			expected: ReasonFailedPermanent,
		},
		{
			name:     "Permanent failure alternate wording",
			pending:  true,
			message:  "Failed permanent: invalid resource",
			expected: ReasonFailedPermanent,
		},
		{
			name:     "Waiting for dependencies (process resource)",
			pending:  true,
			message:  "Waiting for: Host-1, AG-1",
			expected: ReasonAwaitingDependencies,
		},
		{
			name:     "Waiting for entity dependencies",
			pending:  true,
			message:  "Waiting for 2 dependencies to be Ready (e.g., Host/Host-1)",
			expected: ReasonAwaitingEntityDeps,
		},
		{
			name:     "Syncing to SGROUP",
			pending:  true,
			message:  "Syncing to SGROUP (attempt 1)",
			expected: ReasonAwaitingSGroupSync,
		},
		{
			name:     "Default awaiting SGROUP",
			pending:  true,
			message:  "Some other message",
			expected: ReasonAwaitingSGroupSync,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determinePendingSyncReason(tt.pending, tt.message)
			assert.Equal(t, tt.expected, result,
				"Expected reason %s for message: %s", tt.expected, tt.message)
		})
	}
}

func Test_boolToConditionStatus(t *testing.T) {
	tests := []struct {
		name     string
		input    bool
		expected string
	}{
		{
			name:     "True",
			input:    true,
			expected: "True",
		},
		{
			name:     "False",
			input:    false,
			expected: "False",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := boolToConditionStatus(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_formatRetryMessage(t *testing.T) {
	tests := []struct {
		name        string
		attempt     int
		maxAttempts int
		nextRetry   time.Duration
		err         error
		expected    string
	}{
		{
			name:        "Simple retry",
			attempt:     3,
			maxAttempts: 20,
			nextRetry:   30 * time.Second,
			err:         errors.New("SGROUP timeout"),
			expected:    "Retry 3/20 in 30s: SGROUP timeout",
		},
		{
			name:        "Retry with minutes",
			attempt:     5,
			maxAttempts: 20,
			nextRetry:   2 * time.Minute,
			err:         errors.New("connection refused"),
			expected:    "Retry 5/20 in 2m0s: connection refused",
		},
		{
			name:        "Long error message truncation",
			attempt:     1,
			maxAttempts: 10,
			nextRetry:   10 * time.Second,
			err:         errors.New("This is a very long error message that should be truncated because it exceeds the maximum length we allow for error messages in condition updates"),
			expected:    "Retry 1/10 in 10s: This is a very long error message that should be truncated because it exceeds the maximum length ...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatRetryMessage(tt.attempt, tt.maxAttempts, tt.nextRetry, tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_formatPermanentFailureMessage(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		err      error
		contains []string
	}{
		{
			name:     "Simple permanent failure",
			attempts: 20,
			err:      errors.New("validation error (400)"),
			contains: []string{"20 attempts", "validation error"},
		},
		{
			name:     "Long error truncation",
			attempts: 100,
			err:      errors.New("This is an extremely long error message that should be truncated because we don't want condition messages to be too verbose and consume too much space in the database or API responses"),
			contains: []string{"100 attempts", "..."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPermanentFailureMessage(tt.attempts, tt.err)
			for _, substr := range tt.contains {
				assert.Contains(t, result, substr)
			}
		})
	}
}

func Test_formatDependenciesMessage(t *testing.T) {
	tests := []struct {
		name      string
		resources []string
		expected  string
	}{
		{
			name:      "Single dependency",
			resources: []string{"Host/Host-1"},
			expected:  "Waiting for: Host/Host-1",
		},
		{
			name:      "Multiple dependencies",
			resources: []string{"Host/Host-1", "AddressGroup/AG-1"},
			expected:  "Waiting for: Host/Host-1, AddressGroup/AG-1",
		},
		{
			name:      "Many dependencies (truncated)",
			resources: []string{"Host/H-1", "Host/H-2", "Host/H-3", "Host/H-4", "Host/H-5", "Host/H-6", "Host/H-7"},
			expected:  "Waiting for: Host/H-1, Host/H-2, Host/H-3, Host/H-4, Host/H-5 (+2 more)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDependenciesMessage(tt.resources)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_formatSyncingMessage(t *testing.T) {
	tests := []struct {
		name     string
		attempt  int
		expected string
	}{
		{
			name:     "First attempt",
			attempt:  1,
			expected: "Syncing to SGROUP (attempt 1)",
		},
		{
			name:     "Third attempt",
			attempt:  3,
			expected: "Syncing to SGROUP (attempt 3)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSyncingMessage(tt.attempt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_formatEntityDependencyWaitMessage(t *testing.T) {
	tests := []struct {
		name        string
		depCount    int
		exampleType string
		exampleName string
		expected    string
	}{
		{
			name:        "Single dependency",
			depCount:    1,
			exampleType: "Host",
			exampleName: "Host-1",
			expected:    "Waiting for 1 dependencies to be Ready (e.g., Host/Host-1)",
		},
		{
			name:        "Multiple dependencies",
			depCount:    3,
			exampleType: "AddressGroup",
			exampleName: "AG-1",
			expected:    "Waiting for 3 dependencies to be Ready (e.g., AddressGroup/AG-1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatEntityDependencyWaitMessage(tt.depCount, tt.exampleType, tt.exampleName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_formatMultiplePendingMessage(t *testing.T) {
	tests := []struct {
		name     string
		details  []string
		expected string
	}{
		{
			name:     "Single pending",
			details:  []string{"Host-1 (3 attempts)"},
			expected: "1 resources pending: Host-1 (3 attempts)",
		},
		{
			name:     "Two pending",
			details:  []string{"Host-1 (3 attempts)", "AG-1 (0 attempts)"},
			expected: "2 resources pending: Host-1 (3 attempts), AG-1 (0 attempts)",
		},
		{
			name:     "Many pending (truncated)",
			details:  []string{"H-1 (1)", "H-2 (2)", "H-3 (3)", "H-4 (4)", "H-5 (5)"},
			expected: "5 resources pending: H-1 (1), H-2 (2), H-3 (3) (+2 more)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMultiplePendingMessage(tt.details)
			assert.Equal(t, tt.expected, result)
		})
	}
}
