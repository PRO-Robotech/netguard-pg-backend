package watch

import (
	"net/http"

	"k8s.io/apimachinery/pkg/util/json"
)

type HealthStatus struct {
	Healthy bool              `json:"healthy"`
	Stats   PGNotifierStats   `json:"stats"`
	Error   string            `json:"error,omitempty"`
	Caches  map[string]CacheSummary `json:"caches"`
}

type CacheSummary struct {
	EventCount            int   `json:"eventCount"`
	CurrentObjectCount    int   `json:"currentObjectCount"`
	LatestResourceVersion int64 `json:"latestResourceVersion"`
	MaxEvents             int   `json:"maxEvents"`
}

type HealthHandler struct {
	manager *Manager
}

func NewHealthHandler(manager *Manager) *HealthHandler {
	return &HealthHandler{manager: manager}
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status := HealthStatus{
		Healthy: h.manager.notifier.IsHealthy(),
		Stats:   h.manager.NotifierStats(),
		Caches:  make(map[string]CacheSummary),
	}

	if !status.Healthy {
		status.Error = "pg notifier unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	for resourceType, cache := range h.manager.caches {
		stats := cache.GetStats()
		status.Caches[resourceType] = CacheSummary{
			EventCount:            stats.EventCount,
			CurrentObjectCount:    stats.CurrentObjectCount,
			LatestResourceVersion: stats.LatestResourceVersion,
			MaxEvents:             stats.MaxEvents,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

