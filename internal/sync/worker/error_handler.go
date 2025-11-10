package worker

import (
	"context"
	"errors"
	"net"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorCategory represents the category of an error for retry strategy
type ErrorCategory string

const (
	// ErrorCategoryValidation represents 4xx validation errors (fail fast)
	ErrorCategoryValidation ErrorCategory = "validation"
	// ErrorCategoryTemporary represents 5xx temporary errors and timeouts (retry with backoff)
	ErrorCategoryTemporary ErrorCategory = "temporary"
	// ErrorCategoryNetwork represents network connection errors (long retry)
	ErrorCategoryNetwork ErrorCategory = "network"
	// ErrorCategoryConflict represents 409 conflict errors (moderate retry)
	ErrorCategoryConflict ErrorCategory = "conflict"
)

// categorizeError inspects an error and categorizes it for appropriate retry strategy
// Default to ErrorCategoryTemporary (safe to retry) if unable to determine
func (w *OutboxWorker) categorizeError(err error) ErrorCategory {
	if err == nil {
		return ErrorCategoryTemporary // Should not happen, but safe default
	}

	// Check for permanent resource deletion error first
	// This error means the resource was deleted before sync could complete
	// and should be marked as FAILED_PERMANENT after validation attempts
	if errors.Is(err, ErrResourceDeleted) {
		return ErrorCategoryValidation // Fail fast - resource no longer exists
	}

	// Check context timeout/deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorCategoryTemporary
	}
	if errors.Is(err, context.Canceled) {
		return ErrorCategoryTemporary
	}

	// Check gRPC status codes
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.InvalidArgument:
			// Invalid request parameters
			return ErrorCategoryValidation

		case codes.FailedPrecondition:
			// Precondition not met (e.g., resource not found)
			return ErrorCategoryValidation

		case codes.AlreadyExists:
			// Resource already exists (409 conflict)
			return ErrorCategoryConflict

		case codes.NotFound:
			// Resource not found (could be transient)
			return ErrorCategoryValidation

		case codes.Unavailable:
			// Service unavailable (503)
			return ErrorCategoryTemporary

		case codes.Unknown:
			// Unknown error (500)
			return ErrorCategoryTemporary

		case codes.Internal:
			// Internal server error (500)
			return ErrorCategoryTemporary

		case codes.DeadlineExceeded:
			// Request timeout
			return ErrorCategoryTemporary

		case codes.ResourceExhausted:
			// Rate limit or quota exceeded (429)
			return ErrorCategoryTemporary

		case codes.Aborted:
			// Operation aborted (conflict or transient)
			return ErrorCategoryTemporary

		default:
			// Unknown gRPC code, default to temporary
			return ErrorCategoryTemporary
		}
	}

	// Check for "Binding not found" errors
	// These occur when UPDATE/DELETE operations reference non-existent bindings
	// Should fail fast (3 attempts) instead of retrying 20 times
	errMsg := err.Error()
	if strings.Contains(errMsg, "Binding not found") ||
		strings.Contains(errMsg, "HostBinding not found") ||
		strings.Contains(errMsg, "NetworkBinding not found") ||
		strings.Contains(errMsg, "AddressGroupBinding not found") {
		return ErrorCategoryValidation // Fail fast - binding was already deleted
	}

	// Check network errors
	if isNetworkError(err) {
		return ErrorCategoryNetwork
	}

	// Default to temporary (safe to retry)
	return ErrorCategoryTemporary
}

// isNetworkError checks if the error is a network-related error
// Network errors indicate infrastructure issues (connection refused, DNS, TLS)
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for net.Error interface
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Check for common network error types
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// Check error message for common network issues
	errMsg := strings.ToLower(err.Error())
	networkKeywords := []string{
		"connection refused",
		"connection reset",
		"connection reset by peer",
		"no such host",
		"network unreachable",
		"network is unreachable",
		"tls handshake",
		"tls handshake timeout",
		"i/o timeout",
		"dial tcp",
		"dial: connection refused",
		"eof", // Unexpected EOF can indicate network issues
	}

	for _, keyword := range networkKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// getMaxAttempts returns the maximum retry attempts for a given error category
func getMaxAttempts(category ErrorCategory) int {
	switch category {
	case ErrorCategoryValidation:
		return 3 // Fail fast for validation errors (not likely to resolve)

	case ErrorCategoryTemporary:
		return 20 // Standard retry for temporary errors (5xx, timeout)

	case ErrorCategoryNetwork:
		return 100 // Long retry for network issues (SGROUP may be down for extended periods)

	case ErrorCategoryConflict:
		return 10 // Moderate retry for conflicts (may resolve)

	default:
		return 10 // Safe default
	}
}
