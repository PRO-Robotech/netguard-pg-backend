package client

import (
	"fmt"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

// BackendClientConfig конфигурация клиента с cleanenv тегами
type BackendClientConfig struct {
	// gRPC настройки
	Endpoint       string        `yaml:"endpoint" env:"BACKEND_ENDPOINT" env-default:"localhost:8080" env-description:"Backend gRPC endpoint"`
	MaxRetries     int           `yaml:"max_retries" env:"BACKEND_MAX_RETRIES" env-default:"3" env-description:"Maximum number of retries"`
	ConnectTimeout time.Duration `yaml:"connect_timeout" env:"BACKEND_CONNECT_TIMEOUT" env-default:"10s" env-description:"Connection timeout"`
	RequestTimeout time.Duration `yaml:"request_timeout" env:"BACKEND_REQUEST_TIMEOUT" env-default:"30s" env-description:"Request timeout"`

	// Rate Limiting
	RateLimit float64 `yaml:"rate_limit" env:"BACKEND_RATE_LIMIT" env-default:"100.0" env-description:"Rate limit (requests per second)"`
	RateBurst int     `yaml:"rate_burst" env:"BACKEND_RATE_BURST" env-default:"200" env-description:"Rate burst size"`

	// Circuit Breaker
	CBMaxRequests      uint32        `yaml:"cb_max_requests" env:"BACKEND_CB_MAX_REQUESTS" env-default:"5" env-description:"Circuit breaker max requests in half-open state"`
	CBInterval         time.Duration `yaml:"cb_interval" env:"BACKEND_CB_INTERVAL" env-default:"60s" env-description:"Circuit breaker interval"`
	CBTimeout          time.Duration `yaml:"cb_timeout" env:"BACKEND_CB_TIMEOUT" env-default:"60s" env-description:"Circuit breaker timeout"`
	CBFailureThreshold uint32        `yaml:"cb_failure_threshold" env:"BACKEND_CB_FAILURE_THRESHOLD" env-default:"5" env-description:"Circuit breaker failure threshold"`

	// Cache
	CacheDefaultTTL      time.Duration `yaml:"cache_default_ttl" env:"BACKEND_CACHE_DEFAULT_TTL" env-default:"5m" env-description:"Cache default TTL"`
	CacheCleanupInterval time.Duration `yaml:"cache_cleanup_interval" env:"BACKEND_CACHE_CLEANUP_INTERVAL" env-default:"10m" env-description:"Cache cleanup interval"`
}

// LoadBackendClientConfig загружает конфигурацию с помощью cleanenv
func LoadBackendClientConfig(configPath string) (BackendClientConfig, error) {
	var config BackendClientConfig

	if configPath != "" {
		// Загрузка из YAML файла с автоматическим применением env переменных
		err := cleanenv.ReadConfig(configPath, &config)
		if err != nil {
			return config, fmt.Errorf("failed to read config from %s: %w", configPath, err)
		}
	} else {
		// Загрузка только из env переменных с defaults
		err := cleanenv.ReadEnv(&config)
		if err != nil {
			return config, fmt.Errorf("failed to read config from environment: %w", err)
		}
	}

	return config, nil
}

// GetConfigUsage возвращает описание всех конфигурационных параметров
func GetConfigUsage() string {
	var config BackendClientConfig
	usage, _ := cleanenv.GetDescription(&config, nil)
	return usage
}

// Validate проверяет корректность конфигурации
func (c *BackendClientConfig) Validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("endpoint cannot be empty")
	}

	if c.MaxRetries < 0 {
		return fmt.Errorf("max_retries cannot be negative")
	}

	if c.ConnectTimeout <= 0 {
		return fmt.Errorf("connect_timeout must be positive")
	}

	if c.RequestTimeout <= 0 {
		return fmt.Errorf("request_timeout must be positive")
	}

	if c.RateLimit <= 0 {
		return fmt.Errorf("rate_limit must be positive")
	}

	if c.RateBurst <= 0 {
		return fmt.Errorf("rate_burst must be positive")
	}

	if c.CBMaxRequests == 0 {
		return fmt.Errorf("cb_max_requests must be positive")
	}

	if c.CBInterval <= 0 {
		return fmt.Errorf("cb_interval must be positive")
	}

	if c.CBTimeout <= 0 {
		return fmt.Errorf("cb_timeout must be positive")
	}

	if c.CBFailureThreshold == 0 {
		return fmt.Errorf("cb_failure_threshold must be positive")
	}

	if c.CacheDefaultTTL <= 0 {
		return fmt.Errorf("cache_default_ttl must be positive")
	}

	if c.CacheCleanupInterval <= 0 {
		return fmt.Errorf("cache_cleanup_interval must be positive")
	}

	return nil
}
