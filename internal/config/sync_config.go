package config

import (
	"fmt"
	"time"

	"netguard-pg-backend/internal/sync/clients"
	"netguard-pg-backend/internal/sync/interfaces"
)

// SyncConfig holds configuration for the synchronization system
type SyncConfig struct {
	// Enabled determines if synchronization is enabled
	Enabled bool `yaml:"enabled" env:"SYNC_ENABLED"`

	// SGroups holds sgroups service configuration
	SGroups clients.SGroupsConfig `yaml:"sgroups"`

	// Retry holds retry configuration
	Retry interfaces.RetryConfig `yaml:"retry"`

	// Debounce holds debouncing configuration
	Debounce DebounceConfig `yaml:"debounce"`

	// Cleanup holds cleanup configuration
	Cleanup CleanupConfig `yaml:"cleanup"`
}

// DebounceConfig holds debouncing configuration
type DebounceConfig struct {
	// Time is the debounce time duration
	Time time.Duration `yaml:"time" env:"SYNC_DEBOUNCE_TIME"`
}

// CleanupConfig holds cleanup configuration
type CleanupConfig struct {
	// Interval is the cleanup interval
	Interval time.Duration `yaml:"interval" env:"SYNC_CLEANUP_INTERVAL"`

	// MaxAge is the maximum age of entries before cleanup
	MaxAge time.Duration `yaml:"max_age" env:"SYNC_CLEANUP_MAX_AGE"`
}

// DefaultSyncConfig returns default synchronization configuration
func DefaultSyncConfig() SyncConfig {
	return SyncConfig{
		Enabled: true,
		SGroups: clients.DefaultSGroupsConfig(),
		Retry: interfaces.RetryConfig{
			MaxRetries:    3,
			InitialDelay:  100,  // 100ms
			MaxDelay:      5000, // 5s
			BackoffFactor: 2.0,
		},
		Debounce: DebounceConfig{
			Time: 5 * time.Second,
		},
		Cleanup: CleanupConfig{
			Interval: 10 * time.Minute,
			MaxAge:   1 * time.Hour,
		},
	}
}

// Validate validates the sync configuration
func (c *SyncConfig) Validate() error {
	if !c.Enabled {
		return nil // Skip validation if sync is disabled
	}

	if c.SGroups.GRPCAddress == "" {
		return fmt.Errorf("sgroups GRPC address is required when sync is enabled")
	}

	if c.Retry.MaxRetries < 0 {
		return fmt.Errorf("retry max_retries must be >= 0")
	}

	if c.Retry.InitialDelay <= 0 {
		return fmt.Errorf("retry initial_delay must be > 0")
	}

	if c.Retry.MaxDelay <= 0 {
		return fmt.Errorf("retry max_delay must be > 0")
	}

	if c.Retry.BackoffFactor <= 1.0 {
		return fmt.Errorf("retry backoff_factor must be > 1.0")
	}

	if c.Debounce.Time <= 0 {
		return fmt.Errorf("debounce time must be > 0")
	}

	if c.Cleanup.Interval <= 0 {
		return fmt.Errorf("cleanup interval must be > 0")
	}

	if c.Cleanup.MaxAge <= 0 {
		return fmt.Errorf("cleanup max_age must be > 0")
	}

	return nil
}
