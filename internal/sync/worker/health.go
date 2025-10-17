package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// HealthStatus represents worker health status
type HealthStatus struct {
	Status     string       `json:"status"` // "healthy", "degraded", "unhealthy"
	Worker     WorkerStatus `json:"worker"`
	QueueDepth int          `json:"queue_depth"`
	LastPoll   string       `json:"last_poll"` // ISO 8601 timestamp
	Uptime     string       `json:"uptime"`    // Human-readable duration
}

// WorkerStatus represents worker internal status
type WorkerStatus struct {
	Running   bool   `json:"running"`
	LastError string `json:"last_error,omitempty"`
	Processed int64  `json:"processed_total"`
	Failed    int64  `json:"failed_total"`
}

// HealthHandler handles health check requests
// GET /healthz/worker
// Returns 200 OK if healthy, 503 Service Unavailable if unhealthy
func (w *OutboxWorker) HealthHandler(rw http.ResponseWriter, r *http.Request) {
	status := w.getHealthStatus()

	httpCode := http.StatusOK
	if status.Status == "unhealthy" {
		httpCode = http.StatusServiceUnavailable
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(httpCode)

	if err := json.NewEncoder(rw).Encode(status); err != nil {
		w.logger.Error("failed to encode health status response", zap.Error(err))
	}
}

// getHealthStatus computes current health status
func (w *OutboxWorker) getHealthStatus() HealthStatus {
	now := time.Now()

	w.mu.RLock()
	lastPoll := w.lastPoll
	lastError := w.lastError
	startedAt := w.startedAt
	w.mu.RUnlock()

	processed := atomic.LoadInt64(&w.processed)
	failed := atomic.LoadInt64(&w.failed)

	// Calculate uptime
	var uptime time.Duration
	if !startedAt.IsZero() {
		uptime = now.Sub(startedAt)
	}

	// Get queue depth from repository
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	queueDepth, err := w.outboxRepo.CountPending(ctx)
	if err != nil {
		w.logger.Error("failed to get queue depth for health check", zap.Error(err))
		queueDepth = -1 // Indicate error
	}

	// Determine status
	status := w.computeHealthStatus(lastPoll, queueDepth, failed, processed)

	return HealthStatus{
		Status: status,
		Worker: WorkerStatus{
			Running:   true, // If we're responding, we're running
			LastError: lastError,
			Processed: processed,
			Failed:    failed,
		},
		QueueDepth: queueDepth,
		LastPoll:   formatTimestamp(lastPoll),
		Uptime:     formatDuration(uptime),
	}
}

// computeHealthStatus determines health status based on metrics
func (w *OutboxWorker) computeHealthStatus(lastPoll time.Time, queueDepth int, failed, processed int64) string {
	now := time.Now()

	// Unhealthy: Not polling
	if !lastPoll.IsZero() && now.Sub(lastPoll) > 30*time.Second {
		return "unhealthy"
	}

	// Degraded: Queue too deep
	if queueDepth > 1000 {
		return "degraded"
	}

	// Degraded: Error rate > 10%
	if processed > 0 {
		errorRate := float64(failed) / float64(processed+failed)
		if errorRate > 0.10 {
			return "degraded"
		}
	}

	return "healthy"
}

// formatTimestamp formats time to ISO 8601 string
func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format(time.RFC3339)
}

// formatDuration formats duration to human-readable string
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
