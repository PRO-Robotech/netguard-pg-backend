package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"netguard-pg-backend/internal/sync/monitor"
)

// HealthEndpointListener слушает connection events для /healthz/sync endpoint
type HealthEndpointListener struct {
	mu            sync.RWMutex
	isHealthy     bool
	lastHealthy   time.Time
	lastUnhealthy time.Time
	lastError     error
}

// NewHealthEndpointListener создаёт новый listener для health endpoint
func NewHealthEndpointListener() *HealthEndpointListener {
	return &HealthEndpointListener{
		isHealthy: false, // Initially unhealthy
	}
}

// OnConnectionEvent implements ConnectionListener interface
func (h *HealthEndpointListener) OnConnectionEvent(event monitor.ConnectionEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch event.Type {
	case monitor.EventConnected:
		h.isHealthy = true
		h.lastHealthy = event.OccurredAt

	case monitor.EventDisconnected:
		h.isHealthy = false
		h.lastUnhealthy = event.OccurredAt
		h.lastError = event.Error

	case monitor.EventTimestamp:
		// Timestamps не влияют на health status
		// Но можно обновить lastHealthy для heartbeat
		if h.isHealthy {
			h.lastHealthy = event.OccurredAt
		}
	}
}

// GetListenerName implements ConnectionListener interface
func (h *HealthEndpointListener) GetListenerName() string {
	return "HealthEndpoint"
}

// ServeHTTP implements http.Handler interface для /healthz/sync endpoint
func (h *HealthEndpointListener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if h.isHealthy {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":       "healthy",
			"last_healthy": h.lastHealthy.Format(time.RFC3339),
			"component":    "sgroup_sync",
		})
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)

		response := map[string]interface{}{
			"status":    "unhealthy",
			"component": "sgroup_sync",
		}

		if !h.lastUnhealthy.IsZero() {
			response["last_unhealthy"] = h.lastUnhealthy.Format(time.RFC3339)
		}

		if h.lastError != nil {
			response["error"] = h.lastError.Error()
		}

		json.NewEncoder(w).Encode(response)
	}
}

// IsHealthy возвращает текущий health status (для использования вне HTTP)
func (h *HealthEndpointListener) IsHealthy() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.isHealthy
}
