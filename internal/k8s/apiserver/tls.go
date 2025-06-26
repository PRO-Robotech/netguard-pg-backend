package apiserver

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// GetTLSConfig возвращает TLS конфигурацию на основе настроек
func (c *APIServerConfig) GetTLSConfig() (*tls.Config, error) {
	// По умолчанию используем TLS, если не указано иное
	authType := c.Authn.Type
	if authType == "" {
		authType = AuthnTypeTLS
	}

	switch authType {
	case AuthnTypeNone:
		return nil, nil

	case AuthnTypeTLS:
		tlsConfig := &tls.Config{}

		// Загружаем серверный сертификат
		if c.Authn.TLS.CertFile != "" && c.Authn.TLS.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(c.Authn.TLS.CertFile, c.Authn.TLS.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to load server certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		verifyMode := c.Authn.TLS.Client.Verify
		if verifyMode == "" {
			verifyMode = VerifyModeSkip // По умолчанию пропускаем проверку
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
				tlsConfig.ClientCAs = caCertPool
			}

		case VerifyModeVerify:
			tlsConfig.InsecureSkipVerify = false

			// Для полной проверки требуются CA сертификаты
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
			tlsConfig.ClientCAs = caCertPool

		default:
			return nil, fmt.Errorf("unknown client verify mode: %s", verifyMode)
		}

		return tlsConfig, nil

	default:
		return nil, fmt.Errorf("unknown authentication type: %s", authType)
	}
}
