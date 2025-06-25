/*
Copyright 2024 The Netguard Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apiserver

import (
	"fmt"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"

	"netguard-pg-backend/internal/k8s/client"
)

// Константы для типов аутентификации
const (
	AuthnTypeNone = "none"
	AuthnTypeTLS  = "tls"
)

// Константы для режимов проверки сертификатов
const (
	VerifyModeSkip          = "skip"
	VerifyModeCertsRequired = "certs-required"
	VerifyModeVerify        = "verify"
)

// APIServerConfig полная конфигурация API Server
type APIServerConfig struct {
	// Server настройки
	BindAddress  string `yaml:"bind_address" env:"APISERVER_BIND_ADDRESS" env-default:"0.0.0.0" env-description:"API server bind address"`
	SecurePort   int    `yaml:"secure_port" env:"APISERVER_SECURE_PORT" env-default:"8443" env-description:"API server secure port"`
	InsecurePort int    `yaml:"insecure_port" env:"APISERVER_INSECURE_PORT" env-default:"0" env-description:"API server insecure port (0 = disabled)"`

	// Logging
	LogLevel  string `yaml:"log_level" env:"LOG_LEVEL" env-default:"info" env-description:"Log level (debug, info, warn, error)"`
	LogFormat string `yaml:"log_format" env:"LOG_FORMAT" env-default:"json" env-description:"Log format (json, text)"`

	// Health Checks
	HealthCheckTimeout time.Duration `yaml:"health_check_timeout" env:"HEALTH_CHECK_TIMEOUT" env-default:"5s" env-description:"Health check timeout"`

	// Graceful Shutdown
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT" env-default:"30s" env-description:"Graceful shutdown timeout"`

	// Backend Client
	BackendClient client.BackendClientConfig `yaml:"backend_client"`

	// Watch Configuration
	Watch WatchConfig `yaml:"watch"`

	// Authn - аутентификация (твой стиль)
	Authn Authn `yaml:"authn"`
}

// Authn содержит конфигурацию аутентификации
type Authn struct {
	Type string   `yaml:"type" env:"AUTHN_TYPE" env-default:"tls" env-description:"Authentication type: none, tls"`
	TLS  TLSAuthn `yaml:"tls"`
}

// TLSAuthn содержит конфигурацию TLS аутентификации
type TLSAuthn struct {
	KeyFile  string    `yaml:"key-file" env:"TLS_KEY_FILE" env-default:"" env-description:"TLS private key file"`
	CertFile string    `yaml:"cert-file" env:"TLS_CERT_FILE" env-default:"" env-description:"TLS certificate file"`
	Client   TLSClient `yaml:"client"`
}

// TLSClient содержит клиентскую конфигурацию TLS
type TLSClient struct {
	Verify  string   `yaml:"verify" env:"TLS_CLIENT_VERIFY" env-default:"skip" env-description:"Client cert verification: skip, certs-required, verify"`
	CAFiles []string `yaml:"ca-files" env:"TLS_CLIENT_CA_FILES" env-description:"CA certificate files"`
}

// WatchConfig конфигурация Watch functionality
type WatchConfig struct {
	Mode                 WatchMode     `yaml:"mode" env:"WATCH_MODE" env-default:"polling" env-description:"Watch mode (polling, streaming, auto)"`
	StreamingEnabled     bool          `yaml:"streaming_enabled" env:"WATCH_STREAMING_ENABLED" env-default:"false" env-description:"Enable streaming if supported"`
	PollingInterval      time.Duration `yaml:"polling_interval" env:"WATCH_POLLING_INTERVAL" env-default:"5s" env-description:"Polling interval"`
	StreamReconnectDelay time.Duration `yaml:"stream_reconnect_delay" env:"WATCH_STREAM_RECONNECT_DELAY" env-default:"1s" env-description:"Stream reconnect delay"`
}

type WatchMode string

const (
	WatchModePolling   WatchMode = "polling"
	WatchModeStreaming WatchMode = "streaming"
	WatchModeAuto      WatchMode = "auto"
)

// IsTLSEnabled проверяет включен ли TLS
func (c *APIServerConfig) IsTLSEnabled() bool {
	return c.Authn.Type == AuthnTypeTLS || c.Authn.Type == ""
}

// LoadAPIServerConfig загружает полную конфигурацию API Server
func LoadAPIServerConfig(configPath string) (APIServerConfig, error) {
	var config APIServerConfig

	if configPath != "" {
		err := cleanenv.ReadConfig(configPath, &config)
		if err != nil {
			return config, fmt.Errorf("failed to read config from %s: %w", configPath, err)
		}
	} else {
		err := cleanenv.ReadEnv(&config)
		if err != nil {
			return config, fmt.Errorf("failed to read config from environment: %w", err)
		}
	}

	return config, nil
}

// Validate проверяет корректность конфигурации
func (c *APIServerConfig) Validate() error {
	// Проверка портов
	if c.SecurePort <= 0 || c.SecurePort > 65535 {
		return fmt.Errorf("secure_port must be between 1 and 65535")
	}

	if c.InsecurePort < 0 || c.InsecurePort > 65535 {
		return fmt.Errorf("insecure_port must be between 0 and 65535")
	}

	// Должен быть включен хотя бы один порт
	if !c.IsTLSEnabled() && c.InsecurePort == 0 {
		return fmt.Errorf("either TLS must be enabled or insecure_port must be set")
	}

	// TLS валидация
	if c.IsTLSEnabled() {
		if c.Authn.TLS.CertFile == "" {
			return fmt.Errorf("authn.tls.cert-file is required when TLS is enabled")
		}

		if c.Authn.TLS.KeyFile == "" {
			return fmt.Errorf("authn.tls.key-file is required when TLS is enabled")
		}

		// Проверить существование файлов сертификатов
		if _, err := os.Stat(c.Authn.TLS.CertFile); os.IsNotExist(err) {
			return fmt.Errorf("authn.tls.cert-file does not exist: %s", c.Authn.TLS.CertFile)
		}

		if _, err := os.Stat(c.Authn.TLS.KeyFile); os.IsNotExist(err) {
			return fmt.Errorf("authn.tls.key-file does not exist: %s", c.Authn.TLS.KeyFile)
		}

		// Проверяем CA файлы если указаны
		for _, caFile := range c.Authn.TLS.Client.CAFiles {
			if _, err := os.Stat(caFile); os.IsNotExist(err) {
				return fmt.Errorf("authn.tls.client.ca-file does not exist: %s", caFile)
			}
		}
	}

	if c.HealthCheckTimeout <= 0 {
		return fmt.Errorf("health_check_timeout must be positive")
	}

	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown_timeout must be positive")
	}

	// Валидация backend client конфигурации
	if err := c.BackendClient.Validate(); err != nil {
		return fmt.Errorf("backend_client config invalid: %w", err)
	}

	// Валидация watch конфигурации
	if err := c.Watch.Validate(); err != nil {
		return fmt.Errorf("watch config invalid: %w", err)
	}

	return nil
}

// Validate для WatchConfig
func (w *WatchConfig) Validate() error {
	switch w.Mode {
	case WatchModePolling, WatchModeStreaming, WatchModeAuto:
		// OK
	default:
		return fmt.Errorf("invalid watch mode: %s", w.Mode)
	}

	if w.PollingInterval <= 0 {
		return fmt.Errorf("polling_interval must be positive")
	}

	if w.StreamReconnectDelay <= 0 {
		return fmt.Errorf("stream_reconnect_delay must be positive")
	}

	return nil
}

// GetConfigUsage возвращает описание всех параметров конфигурации
func GetAPIServerConfigUsage() string {
	var config APIServerConfig
	usage, _ := cleanenv.GetDescription(&config, nil)
	return usage
}
