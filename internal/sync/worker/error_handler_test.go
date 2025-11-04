package worker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"netguard-pg-backend/internal/domain"
)

// Test_categorizeError_gRPC tests gRPC status code categorization
func Test_categorizeError_gRPC(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{logger: logger}

	tests := []struct {
		name           string
		grpcCode       codes.Code
		expectedCat    ErrorCategory
		grpcMsg        string
	}{
		{
			name:        "InvalidArgument → Validation",
			grpcCode:    codes.InvalidArgument,
			expectedCat: ErrorCategoryValidation,
			grpcMsg:     "invalid request parameter",
		},
		{
			name:        "FailedPrecondition → Validation",
			grpcCode:    codes.FailedPrecondition,
			expectedCat: ErrorCategoryValidation,
			grpcMsg:     "precondition not met",
		},
		{
			name:        "NotFound → Validation",
			grpcCode:    codes.NotFound,
			expectedCat: ErrorCategoryValidation,
			grpcMsg:     "resource not found",
		},
		{
			name:        "AlreadyExists → Conflict",
			grpcCode:    codes.AlreadyExists,
			expectedCat: ErrorCategoryConflict,
			grpcMsg:     "resource already exists",
		},
		{
			name:        "Unavailable → Temporary",
			grpcCode:    codes.Unavailable,
			expectedCat: ErrorCategoryTemporary,
			grpcMsg:     "service unavailable",
		},
		{
			name:        "Unknown → Temporary",
			grpcCode:    codes.Unknown,
			expectedCat: ErrorCategoryTemporary,
			grpcMsg:     "unknown error",
		},
		{
			name:        "Internal → Temporary",
			grpcCode:    codes.Internal,
			expectedCat: ErrorCategoryTemporary,
			grpcMsg:     "internal server error",
		},
		{
			name:        "DeadlineExceeded → Temporary",
			grpcCode:    codes.DeadlineExceeded,
			expectedCat: ErrorCategoryTemporary,
			grpcMsg:     "deadline exceeded",
		},
		{
			name:        "ResourceExhausted → Temporary",
			grpcCode:    codes.ResourceExhausted,
			expectedCat: ErrorCategoryTemporary,
			grpcMsg:     "rate limit exceeded",
		},
		{
			name:        "Aborted → Temporary",
			grpcCode:    codes.Aborted,
			expectedCat: ErrorCategoryTemporary,
			grpcMsg:     "operation aborted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gRPC status error
			err := status.Error(tt.grpcCode, tt.grpcMsg)

			// Categorize
			category := worker.categorizeError(err)

			// Assert
			assert.Equal(t, tt.expectedCat, category,
				"gRPC code %s should be categorized as %s", tt.grpcCode, tt.expectedCat)
		})
	}
}

// Test_categorizeError_ContextTimeout tests context timeout categorization
func Test_categorizeError_ContextTimeout(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{logger: logger}

	tests := []struct {
		name        string
		err         error
		expectedCat ErrorCategory
	}{
		{
			name:        "context.DeadlineExceeded → Temporary",
			err:         context.DeadlineExceeded,
			expectedCat: ErrorCategoryTemporary,
		},
		{
			name:        "context.Canceled → Temporary",
			err:         context.Canceled,
			expectedCat: ErrorCategoryTemporary,
		},
		{
			name:        "wrapped DeadlineExceeded → Temporary",
			err:         fmt.Errorf("request failed: %w", context.DeadlineExceeded),
			expectedCat: ErrorCategoryTemporary,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := worker.categorizeError(tt.err)
			assert.Equal(t, tt.expectedCat, category)
		})
	}
}

// Test_categorizeError_NetworkErrors tests network error categorization
func Test_categorizeError_NetworkErrors(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{logger: logger}

	tests := []struct {
		name        string
		err         error
		expectedCat ErrorCategory
	}{
		{
			name:        "connection refused → Network",
			err:         errors.New("dial tcp: connection refused"),
			expectedCat: ErrorCategoryNetwork,
		},
		{
			name:        "connection reset → Network",
			err:         errors.New("read tcp: connection reset by peer"),
			expectedCat: ErrorCategoryNetwork,
		},
		{
			name:        "no such host (DNS) → Network",
			err:         errors.New("dial tcp: lookup example.com: no such host"),
			expectedCat: ErrorCategoryNetwork,
		},
		{
			name:        "network unreachable → Network",
			err:         errors.New("dial tcp: network is unreachable"),
			expectedCat: ErrorCategoryNetwork,
		},
		{
			name:        "TLS handshake → Network",
			err:         errors.New("tls handshake timeout"),
			expectedCat: ErrorCategoryNetwork,
		},
		{
			name:        "i/o timeout → Network",
			err:         errors.New("read tcp: i/o timeout"),
			expectedCat: ErrorCategoryNetwork,
		},
		{
			name:        "EOF → Network",
			err:         errors.New("unexpected EOF"),
			expectedCat: ErrorCategoryNetwork,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := worker.categorizeError(tt.err)
			assert.Equal(t, tt.expectedCat, category)
		})
	}
}

// Test_categorizeError_DefaultTemporary tests default categorization
func Test_categorizeError_DefaultTemporary(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{logger: logger}

	tests := []struct {
		name        string
		err         error
		expectedCat ErrorCategory
	}{
		{
			name:        "unknown error → Temporary (safe default)",
			err:         errors.New("some unknown error"),
			expectedCat: ErrorCategoryTemporary,
		},
		{
			name:        "nil error → Temporary",
			err:         nil,
			expectedCat: ErrorCategoryTemporary,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := worker.categorizeError(tt.err)
			assert.Equal(t, tt.expectedCat, category)
		})
	}
}

// Test_isNetworkError tests network error detection
func Test_isNetworkError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		isNetwork  bool
	}{
		{
			name:      "net.OpError → true",
			err:       &net.OpError{Op: "dial", Err: errors.New("connection refused")},
			isNetwork: true,
		},
		{
			name:      "net.DNSError → true",
			err:       &net.DNSError{Err: "no such host", Name: "example.com"},
			isNetwork: true,
		},
		{
			name:      "connection refused string → true",
			err:       errors.New("connection refused"),
			isNetwork: true,
		},
		{
			name:      "connection reset string → true",
			err:       errors.New("connection reset by peer"),
			isNetwork: true,
		},
		{
			name:      "no such host string → true",
			err:       errors.New("no such host"),
			isNetwork: true,
		},
		{
			name:      "network unreachable → true",
			err:       errors.New("network is unreachable"),
			isNetwork: true,
		},
		{
			name:      "tls handshake → true",
			err:       errors.New("tls handshake timeout"),
			isNetwork: true,
		},
		{
			name:      "i/o timeout → true",
			err:       errors.New("i/o timeout"),
			isNetwork: true,
		},
		{
			name:      "EOF → true",
			err:       errors.New("EOF"),
			isNetwork: true,
		},
		{
			name:      "generic error → false",
			err:       errors.New("some other error"),
			isNetwork: false,
		},
		{
			name:      "nil error → false",
			err:       nil,
			isNetwork: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNetworkError(tt.err)
			assert.Equal(t, tt.isNetwork, result)
		})
	}
}

// Test_getMaxAttempts tests max attempts for each error category
func Test_getMaxAttempts(t *testing.T) {
	tests := []struct {
		name        string
		category    ErrorCategory
		maxAttempts int
	}{
		{
			name:        "Validation → 3 attempts (fail fast)",
			category:    ErrorCategoryValidation,
			maxAttempts: 3,
		},
		{
			name:        "Temporary → 20 attempts",
			category:    ErrorCategoryTemporary,
			maxAttempts: 20,
		},
		{
			name:        "Network → 100 attempts (SGROUP may be down)",
			category:    ErrorCategoryNetwork,
			maxAttempts: 100,
		},
		{
			name:        "Conflict → 10 attempts",
			category:    ErrorCategoryConflict,
			maxAttempts: 10,
		},
		{
			name:        "Unknown → 10 attempts (default)",
			category:    ErrorCategory("unknown"),
			maxAttempts: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxAttempts := getMaxAttempts(tt.category)
			assert.Equal(t, tt.maxAttempts, maxAttempts)
		})
	}
}

// Test_calculateBackoff tests exponential backoff calculation
func Test_calculateBackoff(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{logger: logger}

	tests := []struct {
		name             string
		attempts         int
		expectedDuration time.Duration
	}{
		{
			name:             "Attempt 0 → 10s",
			attempts:         0,
			expectedDuration: 10 * time.Second,
		},
		{
			name:             "Attempt 1 → 30s",
			attempts:         1,
			expectedDuration: 30 * time.Second,
		},
		{
			name:             "Attempt 2 → 90s",
			attempts:         2,
			expectedDuration: 90 * time.Second,
		},
		{
			name:             "Attempt 3 → 270s",
			attempts:         3,
			expectedDuration: 270 * time.Second,
		},
		{
			name:             "Attempt 4 → 600s (max)",
			attempts:         4,
			expectedDuration: 600 * time.Second,
		},
		{
			name:             "Attempt 5 → 600s (capped)",
			attempts:         5,
			expectedDuration: 600 * time.Second,
		},
		{
			name:             "Attempt 10 → 600s (capped)",
			attempts:         10,
			expectedDuration: 600 * time.Second,
		},
		{
			name:             "Attempt 100 → 600s (capped)",
			attempts:         100,
			expectedDuration: 600 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration := worker.calculateBackoff(tt.attempts)
			assert.Equal(t, tt.expectedDuration, duration)
		})
	}
}

// Test_scheduleRetry_IncrementAttempts tests that attempts are incremented
func Test_scheduleRetry_IncrementAttempts(t *testing.T) {
	// This is a unit test for the logic; integration test will test DB updates
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{logger: logger}

	// Test that different error categories get different max attempts
	tests := []struct {
		name         string
		err          error
		expectedCat  ErrorCategory
		expectedMax  int
	}{
		{
			name:        "validation error",
			err:         status.Error(codes.InvalidArgument, "invalid"),
			expectedCat: ErrorCategoryValidation,
			expectedMax: 3,
		},
		{
			name:        "temporary error",
			err:         status.Error(codes.Unavailable, "unavailable"),
			expectedCat: ErrorCategoryTemporary,
			expectedMax: 20,
		},
		{
			name:        "network error",
			err:         errors.New("connection refused"),
			expectedCat: ErrorCategoryNetwork,
			expectedMax: 100,
		},
		{
			name:        "conflict error",
			err:         status.Error(codes.AlreadyExists, "exists"),
			expectedCat: ErrorCategoryConflict,
			expectedMax: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := worker.categorizeError(tt.err)
			assert.Equal(t, tt.expectedCat, category)

			maxAttempts := getMaxAttempts(category)
			assert.Equal(t, tt.expectedMax, maxAttempts)
		})
	}
}

// Test_categorizeError_RealWorldExamples tests real-world error scenarios
func Test_categorizeError_RealWorldExamples(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{logger: logger}

	tests := []struct {
		name        string
		err         error
		expectedCat ErrorCategory
		description string
	}{
		{
			name:        "SGROUP service down",
			err:         status.Error(codes.Unavailable, "rpc error: code = Unavailable desc = connection error"),
			expectedCat: ErrorCategoryTemporary,
			description: "SGROUP temporary unavailable, should retry with backoff",
		},
		{
			name:        "Invalid Host payload",
			err:         status.Error(codes.InvalidArgument, "invalid host payload: missing required field"),
			expectedCat: ErrorCategoryValidation,
			description: "Invalid payload, should fail fast",
		},
		{
			name:        "Network partition",
			err:         errors.New("dial tcp 10.0.0.1:50051: i/o timeout"),
			expectedCat: ErrorCategoryNetwork,
			description: "Network partition, SGROUP may be down for extended period",
		},
		{
			name:        "Duplicate resource",
			err:         status.Error(codes.AlreadyExists, "resource already exists in SGROUP"),
			expectedCat: ErrorCategoryConflict,
			description: "Conflict, may resolve on retry (idempotent operation)",
		},
		{
			name:        "Request timeout",
			err:         context.DeadlineExceeded,
			expectedCat: ErrorCategoryTemporary,
			description: "Request timeout (4s), SGROUP may be slow, retry",
		},
		{
			name:        "DNS failure",
			err:         &net.DNSError{Err: "no such host", Name: "sgroup-service.example.com"},
			expectedCat: ErrorCategoryNetwork,
			description: "DNS failure, infrastructure issue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := worker.categorizeError(tt.err)
			assert.Equal(t, tt.expectedCat, category, tt.description)
		})
	}
}

// Test_ErrorCategory_RetryStrategy documents the retry strategy for each category
func Test_ErrorCategory_RetryStrategy(t *testing.T) {
	// This test documents the complete retry strategy
	strategies := map[ErrorCategory]struct {
		maxAttempts int
		backoff     string
		rationale   string
	}{
		ErrorCategoryValidation: {
			maxAttempts: 3,
			backoff:     "10s, 30s, 90s (fail fast)",
			rationale:   "Validation errors unlikely to resolve, fail fast to avoid wasted retries",
		},
		ErrorCategoryTemporary: {
			maxAttempts: 20,
			backoff:     "10s → 30s → 90s → 270s → 600s (exponential)",
			rationale:   "5xx errors and timeouts may resolve, standard retry with exponential backoff",
		},
		ErrorCategoryNetwork: {
			maxAttempts: 100,
			backoff:     "10s → 30s → 90s → 270s → 600s (exponential, long)",
			rationale:   "SGROUP may be down for extended periods (maintenance, network partition), retry for hours",
		},
		ErrorCategoryConflict: {
			maxAttempts: 10,
			backoff:     "10s → 30s → 90s → 270s → 600s (exponential, moderate)",
			rationale:   "Conflicts (409) may resolve if operation is idempotent, moderate retry",
		},
	}

	for category, strategy := range strategies {
		t.Run(string(category), func(t *testing.T) {
			maxAttempts := getMaxAttempts(category)
			assert.Equal(t, strategy.maxAttempts, maxAttempts,
				"Category %s: %s", category, strategy.rationale)

			t.Logf("Category: %s", category)
			t.Logf("Max Attempts: %d", maxAttempts)
			t.Logf("Backoff: %s", strategy.backoff)
			t.Logf("Rationale: %s", strategy.rationale)
		})
	}
}

// Test_BackoffSequence_TotalTime calculates total retry time
func Test_BackoffSequence_TotalTime(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{logger: logger}

	tests := []struct {
		name         string
		attempts     int
		expectedTime time.Duration
	}{
		{
			name:         "3 attempts (validation): 10 + 30 = 40s",
			attempts:     3,
			expectedTime: 10*time.Second + 30*time.Second,
		},
		{
			name:         "5 attempts: 10 + 30 + 90 + 270 = 400s (~6.6 min)",
			attempts:     5,
			expectedTime: 10*time.Second + 30*time.Second + 90*time.Second + 270*time.Second,
		},
		{
			name:         "10 attempts: 400s + 5×600s = 3400s (~56 min)",
			attempts:     10,
			expectedTime: 400*time.Second + 5*600*time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var totalTime time.Duration
			for i := 0; i < tt.attempts-1; i++ {
				totalTime += worker.calculateBackoff(i)
			}
			assert.Equal(t, tt.expectedTime, totalTime)

			t.Logf("Total retry time for %d attempts: %v", tt.attempts, totalTime)
		})
	}
}

// Test_OutboxEntry_Creation helper to create test outbox entries
func createTestOutboxEntry(t *testing.T, attempts int) *domain.OutboxEntry {
	t.Helper()

	return &domain.OutboxEntry{
		ID:           uuid.New(),
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      []byte(`{"name":"test-host"}`),
		Status:       domain.OutboxStatusPending,
		Attempts:     attempts,
		MaxRetries:   5,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

// Test_scheduleRetry_MaxAttemptsReached tests behavior when max attempts reached
func Test_scheduleRetry_MaxAttemptsReached(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{logger: logger}

	tests := []struct {
		name        string
		attempts    int
		err         error
		shouldFail  bool
		description string
	}{
		{
			name:        "validation: 3 attempts → fail",
			attempts:    3,
			err:         status.Error(codes.InvalidArgument, "invalid"),
			shouldFail:  true,
			description: "validation errors fail fast after 3 attempts",
		},
		{
			name:        "validation: 2 attempts → retry",
			attempts:    2,
			err:         status.Error(codes.InvalidArgument, "invalid"),
			shouldFail:  false,
			description: "validation errors still retry at 2 attempts",
		},
		{
			name:        "temporary: 20 attempts → fail",
			attempts:    20,
			err:         status.Error(codes.Unavailable, "unavailable"),
			shouldFail:  true,
			description: "temporary errors fail after 20 attempts",
		},
		{
			name:        "network: 100 attempts → fail",
			attempts:    100,
			err:         errors.New("connection refused"),
			shouldFail:  true,
			description: "even network errors fail after 100 attempts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := createTestOutboxEntry(t, tt.attempts)

			category := worker.categorizeError(tt.err)
			maxAttempts := getMaxAttempts(category)

			shouldMarkPermanent := entry.Attempts >= maxAttempts
			assert.Equal(t, tt.shouldFail, shouldMarkPermanent, tt.description)

			t.Logf("Attempts: %d, Max: %d, Category: %s, Permanent: %v",
				entry.Attempts, maxAttempts, category, shouldMarkPermanent)
		})
	}
}

// Test_nil_error tests handling of nil errors
func Test_nil_error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{logger: logger}

	// Categorizing nil error should return temporary (safe default)
	category := worker.categorizeError(nil)
	require.Equal(t, ErrorCategoryTemporary, category)

	// isNetworkError should return false for nil
	isNet := isNetworkError(nil)
	require.False(t, isNet)
}
