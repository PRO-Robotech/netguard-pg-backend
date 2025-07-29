package admission

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/k8s/client"
	clientscheme "netguard-pg-backend/pkg/k8s/clientset/versioned/scheme"

	"k8s.io/apimachinery/pkg/types"
)

// WebhookServer handles admission webhook HTTP requests
type WebhookServer struct {
	server            *http.Server
	validationWebhook *ValidationWebhook
	mutationWebhook   *MutationWebhook
	decoder           runtime.Decoder
}

// WebhookServerConfig configuration for webhook server
type WebhookServerConfig struct {
	BindAddress string `yaml:"bind_address" env:"WEBHOOK_BIND_ADDRESS" env-default:"0.0.0.0"`
	Port        int    `yaml:"port" env:"WEBHOOK_PORT" env-default:"8443"`
	CertFile    string `yaml:"cert_file" env:"WEBHOOK_CERT_FILE" env-default:"/etc/certs/tls.crt"`
	KeyFile     string `yaml:"key_file" env:"WEBHOOK_KEY_FILE" env-default:"/etc/certs/tls.key"`
	TLSEnabled  bool   `yaml:"tls_enabled" env:"WEBHOOK_TLS_ENABLED" env-default:"true"`

	// Timeouts
	ReadTimeout  time.Duration `yaml:"read_timeout" env:"WEBHOOK_READ_TIMEOUT" env-default:"10s"`
	WriteTimeout time.Duration `yaml:"write_timeout" env:"WEBHOOK_WRITE_TIMEOUT" env-default:"10s"`
	IdleTimeout  time.Duration `yaml:"idle_timeout" env:"WEBHOOK_IDLE_TIMEOUT" env-default:"60s"`
}

// NewWebhookServer creates a new webhook server
func NewWebhookServer(config WebhookServerConfig, backendClient client.BackendClient) (*WebhookServer, error) {
	// Create validation webhook
	validationWebhook := NewValidationWebhook(backendClient)

	// Create mutation webhook
	mutationWebhook, err := NewMutationWebhook()
	if err != nil {
		return nil, fmt.Errorf("failed to create mutation webhook: %w", err)
	}

	// Create decoder
	scheme := runtime.NewScheme()
	if err := clientscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add to scheme: %w", err)
	}
	if err := admissionv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add admission to scheme: %w", err)
	}
	codecs := serializer.NewCodecFactory(scheme)
	decoder := codecs.UniversalDeserializer()

	// Create HTTP server
	mux := http.NewServeMux()

	server := &WebhookServer{
		validationWebhook: validationWebhook,
		mutationWebhook:   mutationWebhook,
		decoder:           decoder,
	}

	// Register handlers
	mux.HandleFunc("/validate", server.handleValidation)
	mux.HandleFunc("/mutate", server.handleMutation)
	mux.HandleFunc("/healthz", server.handleHealth)
	mux.HandleFunc("/readyz", server.handleReady)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.BindAddress, config.Port),
		Handler:      mux,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	// Configure TLS if enabled
	if config.TLSEnabled {
		cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
		}

		httpServer.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	server.server = httpServer
	return server, nil
}

// Start starts the webhook server
func (s *WebhookServer) Start(ctx context.Context) error {
	klog.Infof("Starting webhook server on %s", s.server.Addr)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if s.server.TLSConfig != nil {
			errChan <- s.server.ListenAndServeTLS("", "")
		} else {
			klog.Warning("Webhook server running without TLS - NOT RECOMMENDED FOR PRODUCTION")
			errChan <- s.server.ListenAndServe()
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		klog.Info("Webhook server context cancelled, shutting down...")
		return s.Shutdown(context.Background())
	case err := <-errChan:
		if err != http.ErrServerClosed {
			return fmt.Errorf("webhook server failed: %w", err)
		}
		return nil
	}
}

// Shutdown gracefully shuts down the webhook server
func (s *WebhookServer) Shutdown(ctx context.Context) error {
	klog.Info("Shutting down webhook server...")
	return s.server.Shutdown(ctx)
}

// handleValidation handles validation admission requests
func (s *WebhookServer) handleValidation(w http.ResponseWriter, r *http.Request) {
	s.handleAdmission(w, r, "validation", func(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
		return s.validationWebhook.ValidateAdmissionReview(ctx, req)
	})
}

// handleMutation handles mutation admission requests
func (s *WebhookServer) handleMutation(w http.ResponseWriter, r *http.Request) {
	s.handleAdmission(w, r, "mutation", func(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
		return s.mutationWebhook.Handle(ctx, req)
	})
}

// handleAdmission handles admission requests (common logic)
func (s *WebhookServer) handleAdmission(w http.ResponseWriter, r *http.Request, webhookType string, handler func(context.Context, *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse) {
	start := time.Now()
	defer func() {
		klog.V(2).Infof("%s webhook request processed in %v", webhookType, time.Since(start))
	}()

	// Declare body variable early so it's available in defer
	var body []byte

	// Add panic recovery to prevent EOF errors
	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("Panic in %s webhook: %v", webhookType, r)

			// Try to get UID from the request if available
			var requestUID types.UID
			if body != nil {
				var tempReview admissionv1.AdmissionReview
				if err := json.Unmarshal(body, &tempReview); err == nil && tempReview.Request != nil {
					requestUID = tempReview.Request.UID
				}
			}

			// Create error response for panic
			errorResponse := &admissionv1.AdmissionResponse{
				UID:     requestUID,
				Allowed: false,
				Result: &metav1.Status{
					Code:    500,
					Message: fmt.Sprintf("Internal webhook error: %v", r),
				},
			}

			responseReview := &admissionv1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					APIVersion: admissionv1.SchemeGroupVersion.String(),
					Kind:       "AdmissionReview",
				},
				Response: errorResponse,
			}

			responseBytes, err := json.Marshal(responseReview)
			if err != nil {
				klog.Errorf("Failed to encode panic response: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK) // K8s expects 200 even for validation failures
			w.Write(responseBytes)
		}
	}()

	// Only allow POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	// Read request body
	var err error
	body, err = io.ReadAll(r.Body)
	if err != nil {
		klog.Errorf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Decode admission review
	var admissionReview admissionv1.AdmissionReview
	if err := json.Unmarshal(body, &admissionReview); err != nil {
		klog.Errorf("Failed to decode admission review: %v", err)
		http.Error(w, "Failed to decode admission review", http.StatusBadRequest)
		return
	}

	// Validate admission review
	if admissionReview.Request == nil {
		klog.Error("Admission review request is nil")
		http.Error(w, "Admission review request is nil", http.StatusBadRequest)
		return
	}

	// Process the request
	response := handler(r.Context(), admissionReview.Request)

	// Ensure UID is set from request
	if response.UID == "" && admissionReview.Request != nil {
		response.UID = admissionReview.Request.UID
	}

	// Create response admission review
	responseReview := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: admissionv1.SchemeGroupVersion.String(),
			Kind:       "AdmissionReview",
		},
		Response: response,
	}

	// Encode response
	responseBytes, err := json.Marshal(responseReview)
	if err != nil {
		klog.Errorf("Failed to encode admission review response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(responseBytes); err != nil {
		klog.Errorf("Failed to write response: %v", err)
	}

	// Log the operation
	allowed := "denied"
	if response.Allowed {
		allowed = "allowed"
	}
	klog.V(1).Infof("%s webhook %s %s %s/%s in namespace %s",
		webhookType, allowed, admissionReview.Request.Operation,
		admissionReview.Request.Kind.Kind, admissionReview.Request.Name,
		admissionReview.Request.Namespace)
}

// handleHealth handles health check requests
func (s *WebhookServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "ok"}`))
}

// handleReady handles readiness check requests
func (s *WebhookServer) handleReady(w http.ResponseWriter, r *http.Request) {
	// Check backend connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := s.validationWebhook.backendClient.Ping(ctx); err != nil {
		klog.Errorf("Backend ping failed in readiness check: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(fmt.Sprintf(`{"status": "not ready", "error": "%v"}`, err)))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "ready"}`))
}

// Validate validates the webhook server configuration
func (c *WebhookServerConfig) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	if c.TLSEnabled {
		if c.CertFile == "" {
			return fmt.Errorf("cert_file is required when TLS is enabled")
		}
		if c.KeyFile == "" {
			return fmt.Errorf("key_file is required when TLS is enabled")
		}
	}

	if c.ReadTimeout <= 0 {
		return fmt.Errorf("read_timeout must be positive")
	}
	if c.WriteTimeout <= 0 {
		return fmt.Errorf("write_timeout must be positive")
	}
	if c.IdleTimeout <= 0 {
		return fmt.Errorf("idle_timeout must be positive")
	}

	return nil
}
