package worker

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// WorkerConfig configures the OutboxWorker behavior
type WorkerConfig struct {
	// Enabled controls if the worker should start
	Enabled bool

	// How often to poll sync_outbox table
	PollInterval time.Duration

	// Maximum entries to process per poll
	BatchSize int

	// Timeout for external sync operations (SGROUP)
	SGROUPTimeout time.Duration

	// Number of concurrent batches (future use)
	MaxConcurrentBatches int

	// Retry configuration
	MaxAttemptsValidation int
	MaxAttemptsTemporary  int
	MaxAttemptsNetwork    int

	// Health check
	HealthCheckInterval time.Duration

	// Metrics
	MetricsEnabled bool
}

// DefaultConfig returns sensible default configuration
func DefaultConfig() *WorkerConfig {
	return &WorkerConfig{
		Enabled:               true,
		PollInterval:          5 * time.Second,
		BatchSize:             10,
		SGROUPTimeout:         4 * time.Second,
		MaxConcurrentBatches:  1, // Sequential processing initially
		MaxAttemptsValidation: 3,
		MaxAttemptsTemporary:  20,
		MaxAttemptsNetwork:    100,
		HealthCheckInterval:   10 * time.Second,
		MetricsEnabled:        true,
	}
}

// LoadFromEnv loads worker configuration from environment variables
// Falls back to defaults if environment variables are not set
func LoadFromEnv() *WorkerConfig {
	cfg := DefaultConfig()

	// Enabled
	if v := os.Getenv("OUTBOX_WORKER_ENABLED"); v != "" {
		if enabled, err := strconv.ParseBool(v); err == nil {
			cfg.Enabled = enabled
		}
	}

	// PollInterval
	if v := os.Getenv("OUTBOX_WORKER_POLL_INTERVAL"); v != "" {
		if duration, err := time.ParseDuration(v); err == nil {
			cfg.PollInterval = duration
		}
	}

	// BatchSize
	if v := os.Getenv("OUTBOX_WORKER_BATCH_SIZE"); v != "" {
		if batchSize, err := strconv.Atoi(v); err == nil {
			cfg.BatchSize = batchSize
		}
	}

	// SGROUPTimeout
	if v := os.Getenv("OUTBOX_SGROUP_TIMEOUT"); v != "" {
		if timeout, err := time.ParseDuration(v); err == nil {
			cfg.SGROUPTimeout = timeout
		}
	}

	// MaxAttemptsValidation
	if v := os.Getenv("OUTBOX_MAX_ATTEMPTS_VALIDATION"); v != "" {
		if attempts, err := strconv.Atoi(v); err == nil {
			cfg.MaxAttemptsValidation = attempts
		}
	}

	// MaxAttemptsTemporary
	if v := os.Getenv("OUTBOX_MAX_ATTEMPTS_TEMPORARY"); v != "" {
		if attempts, err := strconv.Atoi(v); err == nil {
			cfg.MaxAttemptsTemporary = attempts
		}
	}

	// MaxAttemptsNetwork
	if v := os.Getenv("OUTBOX_MAX_ATTEMPTS_NETWORK"); v != "" {
		if attempts, err := strconv.Atoi(v); err == nil {
			cfg.MaxAttemptsNetwork = attempts
		}
	}

	// HealthCheckInterval
	if v := os.Getenv("OUTBOX_HEALTH_CHECK_INTERVAL"); v != "" {
		if interval, err := time.ParseDuration(v); err == nil {
			cfg.HealthCheckInterval = interval
		}
	}

	// MetricsEnabled
	if v := os.Getenv("OUTBOX_METRICS_ENABLED"); v != "" {
		if enabled, err := strconv.ParseBool(v); err == nil {
			cfg.MetricsEnabled = enabled
		}
	}

	return cfg
}

// Validate validates the configuration
func (c *WorkerConfig) Validate() error {
	if c.PollInterval <= 0 {
		return fmt.Errorf("poll_interval must be positive, got: %v", c.PollInterval)
	}
	if c.PollInterval < 1*time.Second {
		return fmt.Errorf("poll_interval too aggressive, minimum 1s, got: %v", c.PollInterval)
	}
	if c.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be positive, got: %d", c.BatchSize)
	}
	if c.BatchSize > 100 {
		return fmt.Errorf("batch_size too large, maximum 100, got: %d", c.BatchSize)
	}
	if c.SGROUPTimeout <= 0 {
		return fmt.Errorf("sgroup_timeout must be positive, got: %v", c.SGROUPTimeout)
	}
	if c.MaxConcurrentBatches <= 0 {
		return fmt.Errorf("max_concurrent_batches must be positive, got: %d", c.MaxConcurrentBatches)
	}
	if c.MaxAttemptsValidation <= 0 {
		return fmt.Errorf("max_attempts_validation must be positive, got: %d", c.MaxAttemptsValidation)
	}
	if c.MaxAttemptsTemporary <= 0 {
		return fmt.Errorf("max_attempts_temporary must be positive, got: %d", c.MaxAttemptsTemporary)
	}
	if c.MaxAttemptsNetwork <= 0 {
		return fmt.Errorf("max_attempts_network must be positive, got: %d", c.MaxAttemptsNetwork)
	}
	if c.HealthCheckInterval <= 0 {
		return fmt.Errorf("health_check_interval must be positive, got: %v", c.HealthCheckInterval)
	}
	return nil
}
