package config

import (
	"fmt"
	"time"

	"netguard-pg-backend/internal/sync/detector"
	"netguard-pg-backend/internal/sync/manager"
	"netguard-pg-backend/internal/sync/processors"
	"netguard-pg-backend/internal/sync/synchronizer"
)

// ReverseSyncSystemConfig holds complete configuration for the reverse sync system
type ReverseSyncSystemConfig struct {
	// Manager configuration
	Manager manager.ReverseSyncConfig `json:"manager" yaml:"manager"`

	// Change detector configuration
	SGROUPDetector detector.SGROUPDetectorConfig `json:"sgroup_detector" yaml:"sgroup_detector"`

	// Host synchronization configuration
	HostSynchronizer synchronizer.HostSyncConfig `json:"host_synchronizer" yaml:"host_synchronizer"`

	// Host processor configuration
	HostProcessor processors.HostProcessorConfig `json:"host_processor" yaml:"host_processor"`

	// System-wide settings
	System SystemConfig `json:"system" yaml:"system"`
}

// SystemConfig holds system-wide configuration
type SystemConfig struct {
	// LogLevel defines the logging level (debug, info, warn, error)
	LogLevel string `json:"log_level" yaml:"log_level"`

	// EnableMetrics enables metrics collection
	EnableMetrics bool `json:"enable_metrics" yaml:"enable_metrics"`

	// MetricsPort is the port for metrics endpoint
	MetricsPort int `json:"metrics_port" yaml:"metrics_port"`

	// GracefulShutdownTimeout is timeout for graceful shutdown
	GracefulShutdownTimeout time.Duration `json:"graceful_shutdown_timeout" yaml:"graceful_shutdown_timeout"`

	// Environment specifies the environment (development, staging, production)
	Environment string `json:"environment" yaml:"environment"`
}

// DefaultReverseSyncSystemConfig returns default configuration for the entire system
func DefaultReverseSyncSystemConfig() ReverseSyncSystemConfig {
	return ReverseSyncSystemConfig{
		Manager: manager.ReverseSyncConfig{
			AutoStart:               true,
			ProcessingTimeout:       30 * time.Second,
			EnableStatistics:        true,
			MaxConcurrentProcessors: 5,
			HealthCheckInterval:     60 * time.Second,
		},
		SGROUPDetector: detector.SGROUPDetectorConfig{
			ReconnectInterval: 5 * time.Second,
			MaxRetries:        10,
			ChangeEventSource: "sgroup",
		},
		HostSynchronizer: synchronizer.DefaultHostSyncConfig(),
		HostProcessor:    processors.DefaultHostProcessorConfig(),
		System: SystemConfig{
			LogLevel:                "info",
			EnableMetrics:           true,
			MetricsPort:             8080,
			GracefulShutdownTimeout: 30 * time.Second,
			Environment:             "development",
		},
	}
}

// DevelopmentConfig returns configuration optimized for development
func DevelopmentConfig() ReverseSyncSystemConfig {
	config := DefaultReverseSyncSystemConfig()

	// More verbose logging for development
	config.System.LogLevel = "debug"
	config.System.Environment = "development"

	// Faster reconnection for development
	config.SGROUPDetector.ReconnectInterval = 2 * time.Second
	config.SGROUPDetector.MaxRetries = 5

	// Smaller batches for easier debugging
	config.HostSynchronizer.BatchSize = 10
	config.Manager.MaxConcurrentProcessors = 2

	// Shorter timeouts for faster feedback
	config.Manager.ProcessingTimeout = 10 * time.Second
	config.System.GracefulShutdownTimeout = 10 * time.Second

	return config
}

// ProductionConfig returns configuration optimized for production
func ProductionConfig() ReverseSyncSystemConfig {
	config := DefaultReverseSyncSystemConfig()

	// Less verbose logging for production
	config.System.LogLevel = "info"
	config.System.Environment = "production"

	// Longer reconnection intervals for stability
	config.SGROUPDetector.ReconnectInterval = 10 * time.Second
	config.SGROUPDetector.MaxRetries = 20

	// Larger batches for efficiency
	config.HostSynchronizer.BatchSize = 100
	config.Manager.MaxConcurrentProcessors = 10

	// Longer timeouts for reliability
	config.Manager.ProcessingTimeout = 60 * time.Second
	config.System.GracefulShutdownTimeout = 60 * time.Second

	// More health checks
	config.Manager.HealthCheckInterval = 30 * time.Second

	return config
}

// TestConfig returns configuration optimized for testing
func TestConfig() ReverseSyncSystemConfig {
	config := DefaultReverseSyncSystemConfig()

	// Debug logging for tests
	config.System.LogLevel = "debug"
	config.System.Environment = "test"

	// Fast reconnection for tests
	config.SGROUPDetector.ReconnectInterval = 100 * time.Millisecond
	config.SGROUPDetector.MaxRetries = 2

	// Small batches for test predictability
	config.HostSynchronizer.BatchSize = 2
	config.Manager.MaxConcurrentProcessors = 1

	// Short timeouts for fast tests
	config.Manager.ProcessingTimeout = 1 * time.Second
	config.System.GracefulShutdownTimeout = 1 * time.Second

	// Disable health checks for tests
	config.Manager.HealthCheckInterval = 0

	// Disable metrics for tests
	config.System.EnableMetrics = false

	return config
}

// Validate validates the configuration
func (c *ReverseSyncSystemConfig) Validate() error {
	// Validate manager config
	if c.Manager.ProcessingTimeout <= 0 {
		return fmt.Errorf("manager processing timeout must be positive")
	}

	if c.Manager.MaxConcurrentProcessors <= 0 {
		return fmt.Errorf("manager max concurrent processors must be positive")
	}

	// Validate SGROUP detector config
	if c.SGROUPDetector.ReconnectInterval <= 0 {
		return fmt.Errorf("SGROUP detector reconnect interval must be positive")
	}

	if c.SGROUPDetector.MaxRetries < 0 {
		return fmt.Errorf("SGROUP detector max retries must be non-negative")
	}

	// Validate host synchronizer config
	if c.HostSynchronizer.BatchSize <= 0 {
		return fmt.Errorf("host synchronizer batch size must be positive")
	}

	if c.HostSynchronizer.SyncTimeout <= 0 {
		return fmt.Errorf("host synchronizer sync timeout must be positive")
	}

	// Validate system config
	if c.System.LogLevel == "" {
		return fmt.Errorf("system log level cannot be empty")
	}

	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLogLevels[c.System.LogLevel] {
		return fmt.Errorf("invalid log level: %s", c.System.LogLevel)
	}

	if c.System.EnableMetrics && (c.System.MetricsPort <= 0 || c.System.MetricsPort > 65535) {
		return fmt.Errorf("invalid metrics port: %d", c.System.MetricsPort)
	}

	return nil
}
