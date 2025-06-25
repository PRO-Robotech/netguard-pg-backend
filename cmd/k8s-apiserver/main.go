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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/k8s/apiserver"
	"netguard-pg-backend/internal/k8s/client"
)

var (
	configPath = flag.String("config", "", "Path to configuration file")
	showUsage  = flag.Bool("help", false, "Show configuration usage")
	version    = flag.Bool("version", false, "Show version information")
)

func main() {
	flag.Parse()

	// Show version
	if *version {
		fmt.Printf("netguard-k8s-apiserver version: %s\n", getVersion())
		os.Exit(0)
	}

	// Show configuration usage
	if *showUsage {
		fmt.Println("Configuration parameters:")
		fmt.Println(apiserver.GetAPIServerConfigUsage())
		os.Exit(0)
	}

	// Load configuration
	config, err := apiserver.LoadAPIServerConfig(*configPath)
	if err != nil {
		klog.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		klog.Fatalf("Configuration validation failed: %v", err)
	}

	// Setup logging
	setupLogging(config.LogLevel, config.LogFormat)

	// Log startup information
	klog.Infof("Starting netguard-k8s-apiserver with config: %+v", sanitizeConfig(config))

	if config.IsTLSEnabled() {
		klog.Infof("TLS enabled - serving on secure port %d", config.SecurePort)
	} else {
		klog.Warningf("TLS disabled - serving on insecure port %d (NOT RECOMMENDED FOR PRODUCTION)", config.InsecurePort)
	}

	// Create backend client
	backendClient, err := client.NewBackendClient(config.BackendClient)
	if err != nil {
		klog.Fatalf("Failed to create backend client: %v", err)
	}
	defer backendClient.Close()

	// Test backend connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := backendClient.HealthCheck(ctx); err != nil {
		klog.Fatalf("Backend health check failed: %v", err)
	}
	klog.Info("Backend connectivity verified")

	// Create simple API server (без segfault!)
	server, err := apiserver.NewSimpleAPIServer(config, backendClient)
	if err != nil {
		klog.Fatalf("Failed to create API server: %v", err)
	}

	// Setup graceful shutdown
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Обработка сигналов
	go func() {
		sig := <-sigChan
		klog.Infof("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Запускаем простой сервер
	klog.Info("Starting simple API server...")
	if err := server.Start(ctx); err != nil {
		klog.Errorf("API server error: %v", err)
	}

	klog.Info("API server stopped")
}

func setupLogging(level, format string) {
	// Setup klog
	klog.InitFlags(nil)

	// Set log level
	var verbosity int
	switch level {
	case "debug":
		verbosity = 4
	case "info":
		verbosity = 2
	case "warn":
		verbosity = 1
	case "error":
		verbosity = 0
	default:
		verbosity = 2
	}

	flag.Set("v", fmt.Sprintf("%d", verbosity))
	flag.Set("logtostderr", "true")

	if format == "json" {
		flag.Set("log_file_max_size", "0") // Disable file rotation for JSON logs
	}
}

func sanitizeConfig(config apiserver.APIServerConfig) apiserver.APIServerConfig {
	// Hide sensitive information in logs
	sanitized := config
	sanitized.Authn.TLS.CertFile = "***"
	sanitized.Authn.TLS.KeyFile = "***"
	return sanitized
}

func getVersion() string {
	// This would be set during build
	return "v1.0.0-dev"
}
