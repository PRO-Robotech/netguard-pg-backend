package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"netguard-pg-backend/internal/sync/manager"
)

// MetricsCollector collects and exposes metrics for the reverse sync system
type MetricsCollector struct {
	mu              sync.RWMutex
	manager         MetricsProvider
	server          *http.Server
	metricsSnapshot MetricsSnapshot
	config          MetricsConfig
}

// MetricsProvider defines interface for getting metrics
type MetricsProvider interface {
	GetStats() manager.ReverseSyncStats
	IsRunning() bool
}

// MetricsConfig holds configuration for metrics collection
type MetricsConfig struct {
	// Port for metrics HTTP server
	Port int

	// UpdateInterval for metrics collection
	UpdateInterval time.Duration

	// EnableJSONEndpoint enables /metrics/json endpoint
	EnableJSONEndpoint bool

	// EnablePrometheusEndpoint enables /metrics endpoint (Prometheus format)
	EnablePrometheusEndpoint bool

	// EnableHealthEndpoint enables /health endpoint
	EnableHealthEndpoint bool
}

// MetricsSnapshot represents a point-in-time view of system metrics
type MetricsSnapshot struct {
	Timestamp    time.Time                `json:"timestamp"`
	SystemStatus SystemStatus             `json:"system_status"`
	Events       EventMetrics             `json:"events"`
	Processing   ProcessingMetrics        `json:"processing"`
	Entities     map[string]EntityMetrics `json:"entities"`
	Health       HealthMetrics            `json:"health"`
}

// SystemStatus represents overall system status
type SystemStatus struct {
	IsRunning bool          `json:"is_running"`
	Uptime    time.Duration `json:"uptime_seconds"`
	StartTime time.Time     `json:"start_time"`
}

// EventMetrics represents event-related metrics
type EventMetrics struct {
	Total           int64     `json:"total"`
	Processed       int64     `json:"processed"`
	Failed          int64     `json:"failed"`
	SuccessRate     float64   `json:"success_rate_percent"`
	LastEventTime   time.Time `json:"last_event_time"`
	EventsPerMinute float64   `json:"events_per_minute"`
}

// ProcessingMetrics represents processing performance metrics
type ProcessingMetrics struct {
	AverageTime time.Duration `json:"average_time_ms"`
	TotalTime   time.Duration `json:"total_time_ms"`
	Throughput  float64       `json:"throughput_events_per_second"`
}

// EntityMetrics represents metrics for a specific entity type
type EntityMetrics struct {
	TotalRequests      int64     `json:"total_requests"`
	SuccessfulSyncs    int64     `json:"successful_syncs"`
	FailedSyncs        int64     `json:"failed_syncs"`
	AverageSuccessRate float64   `json:"average_success_rate_percent"`
	LastSyncTime       time.Time `json:"last_sync_time"`
}

// HealthMetrics represents system health metrics
type HealthMetrics struct {
	Status              string          `json:"status"`
	LastHealthCheck     time.Time       `json:"last_health_check"`
	ConsecutiveFailures int             `json:"consecutive_failures"`
	ComponentHealth     map[string]bool `json:"component_health"`
}

// DefaultMetricsConfig returns default metrics configuration
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Port:                     8080,
		UpdateInterval:           30 * time.Second,
		EnableJSONEndpoint:       true,
		EnablePrometheusEndpoint: true,
		EnableHealthEndpoint:     true,
	}
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(manager MetricsProvider, config MetricsConfig) *MetricsCollector {
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	collector := &MetricsCollector{
		manager: manager,
		server:  server,
		config:  config,
		metricsSnapshot: MetricsSnapshot{
			Entities: make(map[string]EntityMetrics),
			Health: HealthMetrics{
				Status:          "unknown",
				ComponentHealth: make(map[string]bool),
			},
		},
	}

	// Register HTTP endpoints
	if config.EnableJSONEndpoint {
		mux.HandleFunc("/metrics/json", collector.handleJSONMetrics)
	}

	if config.EnablePrometheusEndpoint {
		mux.HandleFunc("/metrics", collector.handlePrometheusMetrics)
	}

	if config.EnableHealthEndpoint {
		mux.HandleFunc("/health", collector.handleHealth)
	}

	// Debug endpoint
	mux.HandleFunc("/debug/stats", collector.handleDebugStats)

	return collector
}

// Start starts the metrics collection and HTTP server
func (c *MetricsCollector) Start(ctx context.Context) error {

	// Start metrics collection goroutine
	go c.collectMetrics(ctx)

	// Start HTTP server
	go func() {
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		}
	}()

	if c.config.EnableJSONEndpoint {
	}
	if c.config.EnablePrometheusEndpoint {
	}
	if c.config.EnableHealthEndpoint {
	}

	return nil
}

// Stop stops the metrics collection and HTTP server
func (c *MetricsCollector) Stop(ctx context.Context) error {

	// Shutdown HTTP server
	if err := c.server.Shutdown(ctx); err != nil {
		return err
	}

	return nil
}

// collectMetrics collects metrics at regular intervals
func (c *MetricsCollector) collectMetrics(ctx context.Context) {
	ticker := time.NewTicker(c.config.UpdateInterval)
	defer ticker.Stop()


	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.updateMetrics()
		}
	}
}

// updateMetrics updates the current metrics snapshot
func (c *MetricsCollector) updateMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()

	stats := c.manager.GetStats()
	isRunning := c.manager.IsRunning()

	now := time.Now()

	// Calculate uptime
	var uptime time.Duration
	if !stats.StartTime.IsZero() {
		uptime = now.Sub(stats.StartTime)
	}

	// Calculate events per minute
	var eventsPerMinute float64
	if stats.TotalEvents > 0 && !stats.StartTime.IsZero() {
		minutesRunning := uptime.Minutes()
		if minutesRunning > 0 {
			eventsPerMinute = float64(stats.TotalEvents) / minutesRunning
		}
	}

	// Calculate success rate
	var successRate float64
	if stats.TotalEvents > 0 {
		successRate = float64(stats.ProcessedEvents) / float64(stats.TotalEvents) * 100
	}

	// Calculate throughput
	var throughput float64
	if stats.ProcessedEvents > 0 && !stats.StartTime.IsZero() {
		secondsRunning := uptime.Seconds()
		if secondsRunning > 0 {
			throughput = float64(stats.ProcessedEvents) / secondsRunning
		}
	}

	// Update snapshot
	c.metricsSnapshot = MetricsSnapshot{
		Timestamp: now,
		SystemStatus: SystemStatus{
			IsRunning: isRunning,
			Uptime:    uptime,
			StartTime: stats.StartTime,
		},
		Events: EventMetrics{
			Total:           stats.TotalEvents,
			Processed:       stats.ProcessedEvents,
			Failed:          stats.FailedEvents,
			SuccessRate:     successRate,
			LastEventTime:   stats.LastEventTimestamp,
			EventsPerMinute: eventsPerMinute,
		},
		Processing: ProcessingMetrics{
			AverageTime: stats.AverageProcessingTime,
			TotalTime:   stats.TotalProcessingTime,
			Throughput:  throughput,
		},
		Entities: make(map[string]EntityMetrics),
		Health: HealthMetrics{
			Status:              c.calculateHealthStatus(stats, isRunning),
			LastHealthCheck:     now,
			ConsecutiveFailures: c.calculateConsecutiveFailures(stats),
			ComponentHealth: map[string]bool{
				"sync_manager":     isRunning,
				"event_processing": stats.FailedEvents == 0 || (stats.TotalEvents > 0 && successRate >= 80),
				"entity_sync":      c.calculateEntityHealthStatus(stats.EntityCounts),
			},
		},
	}

	// Update entity metrics
	for entityType, entityStats := range stats.EntityCounts {
		c.metricsSnapshot.Entities[entityType] = EntityMetrics{
			TotalRequests:      entityStats.TotalRequests,
			SuccessfulSyncs:    entityStats.SuccessfulSyncs,
			FailedSyncs:        entityStats.FailedSyncs,
			AverageSuccessRate: entityStats.AverageSuccessRate,
			LastSyncTime:       entityStats.LastSyncTime,
		}
	}
}

// calculateHealthStatus determines overall system health status
func (c *MetricsCollector) calculateHealthStatus(stats manager.ReverseSyncStats, isRunning bool) string {
	if !isRunning {
		return "down"
	}

	if stats.TotalEvents == 0 {
		return "starting"
	}

	successRate := float64(stats.ProcessedEvents) / float64(stats.TotalEvents) * 100

	if successRate >= 95 {
		return "healthy"
	} else if successRate >= 80 {
		return "degraded"
	} else {
		return "unhealthy"
	}
}

// calculateConsecutiveFailures calculates consecutive failures (simplified)
func (c *MetricsCollector) calculateConsecutiveFailures(stats manager.ReverseSyncStats) int {
	if stats.TotalEvents == 0 {
		return 0
	}

	// Simplified calculation - in real implementation, would track recent events
	if stats.FailedEvents > 0 && stats.ProcessedEvents == 0 {
		return int(stats.FailedEvents)
	}

	return 0
}

// calculateEntityHealthStatus determines if entities are healthy
func (c *MetricsCollector) calculateEntityHealthStatus(entityCounts map[string]manager.EntityStats) bool {
	for _, stats := range entityCounts {
		if stats.TotalRequests > 0 && stats.AverageSuccessRate < 80 {
			return false
		}
	}
	return true
}

// handleJSONMetrics handles JSON metrics endpoint
func (c *MetricsCollector) handleJSONMetrics(w http.ResponseWriter, r *http.Request) {
	c.mu.RLock()
	snapshot := c.metricsSnapshot
	c.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		http.Error(w, fmt.Sprintf("Error encoding metrics: %v", err), http.StatusInternalServerError)
		return
	}
}

// handlePrometheusMetrics handles Prometheus format metrics endpoint
func (c *MetricsCollector) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	c.mu.RLock()
	snapshot := c.metricsSnapshot
	c.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	// System metrics
	fmt.Fprintf(w, "# HELP reverse_sync_system_running Whether the reverse sync system is running\n")
	fmt.Fprintf(w, "# TYPE reverse_sync_system_running gauge\n")
	if snapshot.SystemStatus.IsRunning {
		fmt.Fprintf(w, "reverse_sync_system_running 1\n")
	} else {
		fmt.Fprintf(w, "reverse_sync_system_running 0\n")
	}

	fmt.Fprintf(w, "# HELP reverse_sync_uptime_seconds System uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE reverse_sync_uptime_seconds counter\n")
	fmt.Fprintf(w, "reverse_sync_uptime_seconds %.0f\n", snapshot.SystemStatus.Uptime.Seconds())

	// Event metrics
	fmt.Fprintf(w, "# HELP reverse_sync_events_total Total number of events processed\n")
	fmt.Fprintf(w, "# TYPE reverse_sync_events_total counter\n")
	fmt.Fprintf(w, "reverse_sync_events_total %d\n", snapshot.Events.Total)

	fmt.Fprintf(w, "# HELP reverse_sync_events_processed Number of successfully processed events\n")
	fmt.Fprintf(w, "# TYPE reverse_sync_events_processed counter\n")
	fmt.Fprintf(w, "reverse_sync_events_processed %d\n", snapshot.Events.Processed)

	fmt.Fprintf(w, "# HELP reverse_sync_events_failed Number of failed events\n")
	fmt.Fprintf(w, "# TYPE reverse_sync_events_failed counter\n")
	fmt.Fprintf(w, "reverse_sync_events_failed %d\n", snapshot.Events.Failed)

	fmt.Fprintf(w, "# HELP reverse_sync_success_rate Event processing success rate\n")
	fmt.Fprintf(w, "# TYPE reverse_sync_success_rate gauge\n")
	fmt.Fprintf(w, "reverse_sync_success_rate %.2f\n", snapshot.Events.SuccessRate)

	// Processing metrics
	fmt.Fprintf(w, "# HELP reverse_sync_processing_time_avg Average processing time in milliseconds\n")
	fmt.Fprintf(w, "# TYPE reverse_sync_processing_time_avg gauge\n")
	fmt.Fprintf(w, "reverse_sync_processing_time_avg %.2f\n", float64(snapshot.Processing.AverageTime.Nanoseconds())/1e6)

	fmt.Fprintf(w, "# HELP reverse_sync_throughput_events_per_second Events processed per second\n")
	fmt.Fprintf(w, "# TYPE reverse_sync_throughput_events_per_second gauge\n")
	fmt.Fprintf(w, "reverse_sync_throughput_events_per_second %.2f\n", snapshot.Processing.Throughput)

	// Entity metrics
	for entityType, entityMetrics := range snapshot.Entities {
		fmt.Fprintf(w, "# HELP reverse_sync_entity_requests_total Total requests for entity type\n")
		fmt.Fprintf(w, "# TYPE reverse_sync_entity_requests_total counter\n")
		fmt.Fprintf(w, "reverse_sync_entity_requests_total{entity_type=\"%s\"} %d\n", entityType, entityMetrics.TotalRequests)

		fmt.Fprintf(w, "# HELP reverse_sync_entity_success_total Successful syncs for entity type\n")
		fmt.Fprintf(w, "# TYPE reverse_sync_entity_success_total counter\n")
		fmt.Fprintf(w, "reverse_sync_entity_success_total{entity_type=\"%s\"} %d\n", entityType, entityMetrics.SuccessfulSyncs)

		fmt.Fprintf(w, "# HELP reverse_sync_entity_success_rate Success rate for entity type\n")
		fmt.Fprintf(w, "# TYPE reverse_sync_entity_success_rate gauge\n")
		fmt.Fprintf(w, "reverse_sync_entity_success_rate{entity_type=\"%s\"} %.2f\n", entityType, entityMetrics.AverageSuccessRate)
	}
}

// handleHealth handles health check endpoint
func (c *MetricsCollector) handleHealth(w http.ResponseWriter, r *http.Request) {
	c.mu.RLock()
	health := c.metricsSnapshot.Health
	c.mu.RUnlock()

	var statusCode int
	switch health.Status {
	case "healthy":
		statusCode = http.StatusOK
	case "degraded":
		statusCode = http.StatusOK
	case "starting":
		statusCode = http.StatusOK
	case "unhealthy":
		statusCode = http.StatusServiceUnavailable
	case "down":
		statusCode = http.StatusServiceUnavailable
	default:
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"status":               health.Status,
		"timestamp":            c.metricsSnapshot.Timestamp.Format(time.RFC3339),
		"consecutive_failures": health.ConsecutiveFailures,
		"components":           health.ComponentHealth,
	}

	json.NewEncoder(w).Encode(response)
}

// handleDebugStats handles debug statistics endpoint
func (c *MetricsCollector) handleDebugStats(w http.ResponseWriter, r *http.Request) {
	c.mu.RLock()
	snapshot := c.metricsSnapshot
	c.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")

	fmt.Fprintf(w, "=== REVERSE SYNC DEBUG STATISTICS ===\n")
	fmt.Fprintf(w, "Timestamp: %s\n", snapshot.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(w, "\nSystem Status:\n")
	fmt.Fprintf(w, "  Running: %v\n", snapshot.SystemStatus.IsRunning)
	fmt.Fprintf(w, "  Uptime: %v\n", snapshot.SystemStatus.Uptime)
	fmt.Fprintf(w, "  Start Time: %s\n", snapshot.SystemStatus.StartTime.Format(time.RFC3339))

	fmt.Fprintf(w, "\nEvents:\n")
	fmt.Fprintf(w, "  Total: %d\n", snapshot.Events.Total)
	fmt.Fprintf(w, "  Processed: %d\n", snapshot.Events.Processed)
	fmt.Fprintf(w, "  Failed: %d\n", snapshot.Events.Failed)
	fmt.Fprintf(w, "  Success Rate: %.1f%%\n", snapshot.Events.SuccessRate)
	fmt.Fprintf(w, "  Events/min: %.1f\n", snapshot.Events.EventsPerMinute)

	if !snapshot.Events.LastEventTime.IsZero() {
		fmt.Fprintf(w, "  Last Event: %s\n", snapshot.Events.LastEventTime.Format(time.RFC3339))
	}

	fmt.Fprintf(w, "\nProcessing:\n")
	fmt.Fprintf(w, "  Average Time: %v\n", snapshot.Processing.AverageTime)
	fmt.Fprintf(w, "  Total Time: %v\n", snapshot.Processing.TotalTime)
	fmt.Fprintf(w, "  Throughput: %.2f events/sec\n", snapshot.Processing.Throughput)

	fmt.Fprintf(w, "\nHealth:\n")
	fmt.Fprintf(w, "  Status: %s\n", snapshot.Health.Status)
	fmt.Fprintf(w, "  Last Check: %s\n", snapshot.Health.LastHealthCheck.Format(time.RFC3339))
	fmt.Fprintf(w, "  Consecutive Failures: %d\n", snapshot.Health.ConsecutiveFailures)

	fmt.Fprintf(w, "\nComponent Health:\n")
	for component, healthy := range snapshot.Health.ComponentHealth {
		fmt.Fprintf(w, "  %s: %v\n", component, healthy)
	}

	fmt.Fprintf(w, "\nEntity Statistics:\n")
	for entityType, metrics := range snapshot.Entities {
		fmt.Fprintf(w, "  %s:\n", entityType)
		fmt.Fprintf(w, "    Requests: %d\n", metrics.TotalRequests)
		fmt.Fprintf(w, "    Success: %d\n", metrics.SuccessfulSyncs)
		fmt.Fprintf(w, "    Failed: %d\n", metrics.FailedSyncs)
		fmt.Fprintf(w, "    Success Rate: %.1f%%\n", metrics.AverageSuccessRate)
		if !metrics.LastSyncTime.IsZero() {
			fmt.Fprintf(w, "    Last Sync: %s\n", metrics.LastSyncTime.Format(time.RFC3339))
		}
	}

	fmt.Fprintf(w, "\n=== END DEBUG STATISTICS ===\n")
}
