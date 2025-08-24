package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/ilyakaznacheev/cleanenv"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/k8s/admission"
	k8sclient "netguard-pg-backend/internal/k8s/client"
)

func main() {
	// Basic config; can be overridden via env (handled inside WebhookServerConfig)
	cfg := admission.WebhookServerConfig{
		BindAddress: "0.0.0.0",
		Port:        8443, // default for sidecar
		CertFile:    "/etc/certs/tls.crt",
		KeyFile:     "/etc/certs/tls.key",
		TLSEnabled:  true,
	}

	// Override from environment
	_ = cleanenv.ReadEnv(&cfg)

	if err := cfg.Validate(); err != nil {
		klog.Fatalf("config: %v", err)
	}

	// backend client config via env
	backendCfg, _ := k8sclient.LoadBackendClientConfig("")
	backend, err := k8sclient.NewBackendClient(backendCfg)
	if err != nil {
		klog.Fatalf("backend init: %v", err)
	}

	server, err := admission.NewWebhookServer(cfg, backend)
	if err != nil {
		klog.Fatalf("webhook server init: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if err := server.Start(ctx); err != nil {
		klog.Fatalf("webhook server run: %v", err)
	}
}
