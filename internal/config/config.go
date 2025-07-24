package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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

type (
	// Config - основная конфигурация приложения
	Config struct {
		App      `yaml:"app"`
		Settings `yaml:"settings"`
		Log      `yaml:"logger"`
		Authn    `yaml:"authn"`
		Sync     SyncConfig `yaml:"sync"`
	}

	// App - конфигурация приложения
	App struct {
		Name    string `yaml:"name" env:"APP_NAME"`
		Version string `yaml:"version" env:"APP_VERSION"`
	}

	// Log - конфигурация логирования
	Log struct {
		Level string `yaml:"log-level" env:"LOG_LEVEL"`
	}

	// Settings - основные настройки
	Settings struct {
		SGroupGRPCAddress string `yaml:"sgroup-grpc-address" env:"SGROUP_GRPC_ADDRESS"`
		HTTPAddr          string `yaml:"http-addr" env:"HTTP_ADDR"`
		GRPCAddr          string `yaml:"grpc-addr" env:"GRPC_ADDR"`
	}

	// Authn - конфигурация аутентификации
	Authn struct {
		Type string   `yaml:"type" env:"AUTHN_TYPE"`
		TLS  TLSAuthn `yaml:"tls"`
	}

	// TLSAuthn - конфигурация TLS аутентификации
	TLSAuthn struct {
		KeyFile  string    `yaml:"key-file" env:"TLS_KEY_FILE"`
		CertFile string    `yaml:"cert-file" env:"TLS_CERT_FILE"`
		Client   TLSClient `yaml:"client"`
	}

	// TLSClient - клиентская конфигурация TLS
	TLSClient struct {
		Verify  string   `yaml:"verify" env:"TLS_CLIENT_VERIFY"`
		CAFiles []string `yaml:"ca-files" env:"TLS_CLIENT_CA_FILES"`
	}
)

// NewConfig создает новую конфигурацию
func NewConfig(path string) (*Config, error) {
	cfg := &Config{}

	// Установка значений по умолчанию
	cfg.App.Name = "netguard-pg-backend"
	cfg.App.Version = "v1.0.0"
	cfg.Log.Level = "info"
	cfg.Settings.HTTPAddr = ":8080"
	cfg.Settings.GRPCAddr = ":9090"
	cfg.Settings.SGroupGRPCAddress = "localhost:9007"
	cfg.Authn.Type = AuthnTypeTLS
	cfg.Authn.TLS.Client.Verify = VerifyModeSkip
	cfg.Sync = DefaultSyncConfig()

	// Загрузка из файла конфигурации
	if path != "" {
		err := cleanenv.ReadConfig(path, cfg)
		if err != nil {
			return nil, fmt.Errorf("config error: %w", err)
		}
	}

	// Загрузка из переменных окружения
	err := cleanenv.ReadEnv(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// GetTransportCredentials возвращает gRPC TransportCredentials на основе конфигурации
func (c *Config) GetTransportCredentials() (credentials.TransportCredentials, error) {
	authType := c.Authn.Type
	if authType == "" {
		authType = AuthnTypeTLS
	}

	switch authType {
	case AuthnTypeNone:
		return insecure.NewCredentials(), nil

	case AuthnTypeTLS:
		tlsConfig := &tls.Config{}

		// Загружаем клиентский сертификат, если указан
		if c.Authn.TLS.CertFile != "" && c.Authn.TLS.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(c.Authn.TLS.CertFile, c.Authn.TLS.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to load client certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		verifyMode := c.Authn.TLS.Client.Verify
		if verifyMode == "" {
			verifyMode = VerifyModeSkip
		}

		switch verifyMode {
		case VerifyModeSkip:
			tlsConfig.InsecureSkipVerify = true

		case VerifyModeCertsRequired:
			tlsConfig.InsecureSkipVerify = true

			if len(c.Authn.TLS.Client.CAFiles) > 0 {
				caCertPool := x509.NewCertPool()
				for _, caFile := range c.Authn.TLS.Client.CAFiles {
					caCert, err := os.ReadFile(caFile)
					if err != nil {
						return nil, fmt.Errorf("failed to read CA certificate %s: %w", caFile, err)
					}
					if !caCertPool.AppendCertsFromPEM(caCert) {
						return nil, fmt.Errorf("failed to add CA certificate %s to pool", caFile)
					}
				}
				tlsConfig.RootCAs = caCertPool
			}

		case VerifyModeVerify:
			tlsConfig.InsecureSkipVerify = false

			if len(c.Authn.TLS.Client.CAFiles) == 0 {
				return nil, fmt.Errorf("CA certificates are required for verify mode")
			}

			caCertPool := x509.NewCertPool()
			for _, caFile := range c.Authn.TLS.Client.CAFiles {
				caCert, err := os.ReadFile(caFile)
				if err != nil {
					return nil, fmt.Errorf("failed to read CA certificate %s: %w", caFile, err)
				}
				if !caCertPool.AppendCertsFromPEM(caCert) {
					return nil, fmt.Errorf("failed to add CA certificate %s to pool", caFile)
				}
			}
			tlsConfig.RootCAs = caCertPool

		default:
			return nil, fmt.Errorf("unknown client verify mode: %s", verifyMode)
		}

		return credentials.NewTLS(tlsConfig), nil

	default:
		return nil, fmt.Errorf("unknown authentication type: %s", authType)
	}
}

// Validate валидирует конфигурацию
func (c *Config) Validate() error {
	if c.Settings.SGroupGRPCAddress == "" {
		return fmt.Errorf("sgroup GRPC address is required")
	}

	if c.Sync.Enabled {
		if err := c.Sync.Validate(); err != nil {
			return fmt.Errorf("sync config validation failed: %w", err)
		}
	}

	return nil
}
