package utils

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"netguard-pg-backend/internal/sync/interfaces"
)

// retryExecutor implements retry logic with exponential backoff
type retryExecutor struct {
	config interfaces.RetryConfig
}

// NewRetryExecutor creates a new retry executor with the given configuration
func NewRetryExecutor(config interfaces.RetryConfig) *retryExecutor {
	return &retryExecutor{
		config: config,
	}
}

// Execute executes a function with retry logic and exponential backoff
func (re *retryExecutor) Execute(ctx context.Context, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= re.config.MaxRetries; attempt++ {
		// Execute the operation
		err := operation()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		if !isRetryableError(err) {
			return err
		}

		// If this was the last attempt, don't wait
		if attempt == re.config.MaxRetries {
			break
		}

		// Calculate delay with exponential backoff
		delay := re.calculateDelay(attempt)

		// Wait for the calculated delay or until context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled by context: %w", ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", re.config.MaxRetries+1, lastErr)
}

// calculateDelay calculates the delay for the given attempt using exponential backoff
func (re *retryExecutor) calculateDelay(attempt int) time.Duration {
	// Calculate exponential backoff: initialDelay * (backoffFactor ^ attempt)
	delay := float64(re.config.InitialDelay) * math.Pow(re.config.BackoffFactor, float64(attempt))

	// Cap the delay at maxDelay
	if delay > float64(re.config.MaxDelay) {
		delay = float64(re.config.MaxDelay)
	}

	return time.Duration(delay) * time.Millisecond
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() interfaces.RetryConfig {
	return interfaces.RetryConfig{
		MaxRetries:    3,
		InitialDelay:  100,  // 100ms
		MaxDelay:      5000, // 5s
		BackoffFactor: 2.0,  // Double the delay each time
	}
}

// AggressiveRetryConfig returns a more aggressive retry configuration for critical operations
func AggressiveRetryConfig() interfaces.RetryConfig {
	return interfaces.RetryConfig{
		MaxRetries:    5,
		InitialDelay:  50,    // 50ms
		MaxDelay:      10000, // 10s
		BackoffFactor: 1.5,   // 1.5x backoff
	}
}

// ConservativeRetryConfig returns a conservative retry configuration for non-critical operations
func ConservativeRetryConfig() interfaces.RetryConfig {
	return interfaces.RetryConfig{
		MaxRetries:    2,
		InitialDelay:  200,  // 200ms
		MaxDelay:      2000, // 2s
		BackoffFactor: 3.0,  // Triple the delay each time
	}
}

// ExecuteWithRetry is a convenience function that creates a retry executor and executes an operation
func ExecuteWithRetry(ctx context.Context, config interfaces.RetryConfig, operation func() error) error {
	executor := NewRetryExecutor(config)
	return executor.Execute(ctx, operation)
}

// ExecuteWithDefaultRetry is a convenience function that uses default retry configuration
func ExecuteWithDefaultRetry(ctx context.Context, operation func() error) error {
	return ExecuteWithRetry(ctx, DefaultRetryConfig(), operation)
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	// Check for permanent database errors (no retry)
	if strings.Contains(errMsg, "sqlstate 23") || // Integrity constraint violations (23xxx)
		strings.Contains(errMsg, "constraint") && strings.Contains(errMsg, "violat") ||
		strings.Contains(errMsg, "duplicate key") ||
		strings.Contains(errMsg, "unique constraint") ||
		strings.Contains(errMsg, "foreign key constraint") ||
		strings.Contains(errMsg, "check constraint") ||
		strings.Contains(errMsg, "exclusion constraint") {
		return false
	}

	// Check for temporary errors (retry)
	return strings.Contains(errMsg, "temporary") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "connection") ||
		strings.Contains(errMsg, "network") ||
		strings.Contains(errMsg, "unavailable") ||
		strings.Contains(errMsg, "deadline exceeded")
}
