package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultReverseSyncSystemConfig(t *testing.T) {
	config := DefaultReverseSyncSystemConfig()

	// Test Manager config
	assert.True(t, config.Manager.AutoStart)
	assert.Equal(t, 30*time.Second, config.Manager.ProcessingTimeout)
	assert.True(t, config.Manager.EnableStatistics)
	assert.Equal(t, 5, config.Manager.MaxConcurrentProcessors)
	assert.Equal(t, 60*time.Second, config.Manager.HealthCheckInterval)

	// Test SGROUP Detector config
	assert.Equal(t, 5*time.Second, config.SGROUPDetector.ReconnectInterval)
	assert.Equal(t, 10, config.SGROUPDetector.MaxRetries)
	assert.Equal(t, "sgroup", config.SGROUPDetector.ChangeEventSource)

	// Test Host Synchronizer config
	assert.Equal(t, 50, config.HostSynchronizer.BatchSize)
	assert.Equal(t, 5, config.HostSynchronizer.MaxConcurrency)
	assert.Equal(t, 30, config.HostSynchronizer.SyncTimeout)
	assert.Equal(t, 3, config.HostSynchronizer.RetryAttempts)
	assert.True(t, config.HostSynchronizer.EnableIPSetValidation)

	// Test Host Processor config
	assert.False(t, config.HostProcessor.EnableNamespaceFiltering)
	assert.Empty(t, config.HostProcessor.AllowedNamespaces)
	assert.False(t, config.HostProcessor.EnableFullSyncOnChange)
	assert.Equal(t, 3, config.HostProcessor.MaxRetryAttempts)

	// Test System config
	assert.Equal(t, "info", config.System.LogLevel)
	assert.True(t, config.System.EnableMetrics)
	assert.Equal(t, 8080, config.System.MetricsPort)
	assert.Equal(t, 30*time.Second, config.System.GracefulShutdownTimeout)
	assert.Equal(t, "development", config.System.Environment)
}

func TestDevelopmentConfig(t *testing.T) {
	config := DevelopmentConfig()

	// Test development-specific settings
	assert.Equal(t, "debug", config.System.LogLevel)
	assert.Equal(t, "development", config.System.Environment)

	// Test faster reconnection for development
	assert.Equal(t, 2*time.Second, config.SGROUPDetector.ReconnectInterval)
	assert.Equal(t, 5, config.SGROUPDetector.MaxRetries)

	// Test smaller batches for debugging
	assert.Equal(t, 10, config.HostSynchronizer.BatchSize)
	assert.Equal(t, 2, config.Manager.MaxConcurrentProcessors)

	// Test shorter timeouts for faster feedback
	assert.Equal(t, 10*time.Second, config.Manager.ProcessingTimeout)
	assert.Equal(t, 10*time.Second, config.System.GracefulShutdownTimeout)
}

func TestProductionConfig(t *testing.T) {
	config := ProductionConfig()

	// Test production-specific settings
	assert.Equal(t, "info", config.System.LogLevel)
	assert.Equal(t, "production", config.System.Environment)

	// Test longer reconnection intervals for stability
	assert.Equal(t, 10*time.Second, config.SGROUPDetector.ReconnectInterval)
	assert.Equal(t, 20, config.SGROUPDetector.MaxRetries)

	// Test larger batches for efficiency
	assert.Equal(t, 100, config.HostSynchronizer.BatchSize)
	assert.Equal(t, 10, config.Manager.MaxConcurrentProcessors)

	// Test longer timeouts for reliability
	assert.Equal(t, 60*time.Second, config.Manager.ProcessingTimeout)
	assert.Equal(t, 60*time.Second, config.System.GracefulShutdownTimeout)

	// Test more frequent health checks
	assert.Equal(t, 30*time.Second, config.Manager.HealthCheckInterval)
}

func TestTestConfig(t *testing.T) {
	config := TestConfig()

	// Test test-specific settings
	assert.Equal(t, "debug", config.System.LogLevel)
	assert.Equal(t, "test", config.System.Environment)

	// Test fast reconnection for tests
	assert.Equal(t, 100*time.Millisecond, config.SGROUPDetector.ReconnectInterval)
	assert.Equal(t, 2, config.SGROUPDetector.MaxRetries)

	// Test small batches for predictability
	assert.Equal(t, 2, config.HostSynchronizer.BatchSize)
	assert.Equal(t, 1, config.Manager.MaxConcurrentProcessors)

	// Test short timeouts for fast tests
	assert.Equal(t, 1*time.Second, config.Manager.ProcessingTimeout)
	assert.Equal(t, 1*time.Second, config.System.GracefulShutdownTimeout)

	// Test disabled health checks
	assert.Equal(t, time.Duration(0), config.Manager.HealthCheckInterval)

	// Test disabled metrics
	assert.False(t, config.System.EnableMetrics)
}

func TestReverseSyncSystemConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    ReverseSyncSystemConfig
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid default config",
			config:    DefaultReverseSyncSystemConfig(),
			wantError: false,
		},
		{
			name: "invalid manager processing timeout",
			config: func() ReverseSyncSystemConfig {
				c := DefaultReverseSyncSystemConfig()
				c.Manager.ProcessingTimeout = 0
				return c
			}(),
			wantError: true,
			errorMsg:  "manager processing timeout must be positive",
		},
		{
			name: "invalid manager max concurrent processors",
			config: func() ReverseSyncSystemConfig {
				c := DefaultReverseSyncSystemConfig()
				c.Manager.MaxConcurrentProcessors = 0
				return c
			}(),
			wantError: true,
			errorMsg:  "manager max concurrent processors must be positive",
		},
		{
			name: "invalid SGROUP detector reconnect interval",
			config: func() ReverseSyncSystemConfig {
				c := DefaultReverseSyncSystemConfig()
				c.SGROUPDetector.ReconnectInterval = 0
				return c
			}(),
			wantError: true,
			errorMsg:  "SGROUP detector reconnect interval must be positive",
		},
		{
			name: "invalid SGROUP detector max retries",
			config: func() ReverseSyncSystemConfig {
				c := DefaultReverseSyncSystemConfig()
				c.SGROUPDetector.MaxRetries = -1
				return c
			}(),
			wantError: true,
			errorMsg:  "SGROUP detector max retries must be non-negative",
		},
		{
			name: "invalid host synchronizer batch size",
			config: func() ReverseSyncSystemConfig {
				c := DefaultReverseSyncSystemConfig()
				c.HostSynchronizer.BatchSize = 0
				return c
			}(),
			wantError: true,
			errorMsg:  "host synchronizer batch size must be positive",
		},
		{
			name: "invalid host synchronizer sync timeout",
			config: func() ReverseSyncSystemConfig {
				c := DefaultReverseSyncSystemConfig()
				c.HostSynchronizer.SyncTimeout = 0
				return c
			}(),
			wantError: true,
			errorMsg:  "host synchronizer sync timeout must be positive",
		},
		{
			name: "empty log level",
			config: func() ReverseSyncSystemConfig {
				c := DefaultReverseSyncSystemConfig()
				c.System.LogLevel = ""
				return c
			}(),
			wantError: true,
			errorMsg:  "system log level cannot be empty",
		},
		{
			name: "invalid log level",
			config: func() ReverseSyncSystemConfig {
				c := DefaultReverseSyncSystemConfig()
				c.System.LogLevel = "invalid"
				return c
			}(),
			wantError: true,
			errorMsg:  "invalid log level: invalid",
		},
		{
			name: "invalid metrics port - too low",
			config: func() ReverseSyncSystemConfig {
				c := DefaultReverseSyncSystemConfig()
				c.System.EnableMetrics = true
				c.System.MetricsPort = 0
				return c
			}(),
			wantError: true,
			errorMsg:  "invalid metrics port: 0",
		},
		{
			name: "invalid metrics port - too high",
			config: func() ReverseSyncSystemConfig {
				c := DefaultReverseSyncSystemConfig()
				c.System.EnableMetrics = true
				c.System.MetricsPort = 70000
				return c
			}(),
			wantError: true,
			errorMsg:  "invalid metrics port: 70000",
		},
		{
			name: "metrics disabled - port validation skipped",
			config: func() ReverseSyncSystemConfig {
				c := DefaultReverseSyncSystemConfig()
				c.System.EnableMetrics = false
				c.System.MetricsPort = 0
				return c
			}(),
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSystemConfig(t *testing.T) {
	config := SystemConfig{
		LogLevel:                "debug",
		EnableMetrics:           true,
		MetricsPort:             9090,
		GracefulShutdownTimeout: 45 * time.Second,
		Environment:             "staging",
	}

	assert.Equal(t, "debug", config.LogLevel)
	assert.True(t, config.EnableMetrics)
	assert.Equal(t, 9090, config.MetricsPort)
	assert.Equal(t, 45*time.Second, config.GracefulShutdownTimeout)
	assert.Equal(t, "staging", config.Environment)
}

func TestValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		config := DefaultReverseSyncSystemConfig()
		config.System.LogLevel = level

		err := config.Validate()
		assert.NoError(t, err, "Log level '%s' should be valid", level)
	}
}

func TestConfigurationConsistency(t *testing.T) {
	// Test that all predefined configs are valid
	configs := map[string]ReverseSyncSystemConfig{
		"default":     DefaultReverseSyncSystemConfig(),
		"development": DevelopmentConfig(),
		"production":  ProductionConfig(),
		"test":        TestConfig(),
	}

	for name, config := range configs {
		t.Run(name, func(t *testing.T) {
			err := config.Validate()
			assert.NoError(t, err, "Config '%s' should be valid", name)
		})
	}
}

func TestConfigurationDifferences(t *testing.T) {
	dev := DevelopmentConfig()
	prod := ProductionConfig()
	test := TestConfig()

	// Development should have faster settings than production
	assert.Less(t, dev.SGROUPDetector.ReconnectInterval, prod.SGROUPDetector.ReconnectInterval)
	assert.Less(t, dev.SGROUPDetector.MaxRetries, prod.SGROUPDetector.MaxRetries)
	assert.Less(t, dev.HostSynchronizer.BatchSize, prod.HostSynchronizer.BatchSize)

	// Test should have the fastest settings
	assert.Less(t, test.SGROUPDetector.ReconnectInterval, dev.SGROUPDetector.ReconnectInterval)
	assert.Less(t, test.Manager.ProcessingTimeout, dev.Manager.ProcessingTimeout)
	assert.Equal(t, time.Duration(0), test.Manager.HealthCheckInterval)
}
