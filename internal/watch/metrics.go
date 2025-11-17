package watch

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	cacheEventsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "watch_cache_events_total",
			Help: "Total number of events processed per resource type",
		},
		[]string{"resource_type", "event_type"},
	)

	activeWatchers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "watch_active_watchers",
			Help: "Number of active watch clients per resource type",
		},
		[]string{"resource_type"},
	)

	pgNotifyEventsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "watch_pg_notify_events_total",
			Help: "Total PostgreSQL NOTIFY events received",
		},
		[]string{"resource_type", "operation"},
	)

	pgNotifyErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "watch_pg_notify_errors_total",
			Help: "Total PostgreSQL NOTIFY handling errors",
		},
	)
)

var watcherCounters sync.Map

func recordCacheEvent(resourceType, eventType string) {
	cacheEventsTotal.WithLabelValues(resourceType, eventType).Inc()
}

func incrementActiveWatchers(resourceType string) {
	activeWatchers.WithLabelValues(resourceType).Inc()
}

func decrementActiveWatchers(resourceType string) {
	activeWatchers.WithLabelValues(resourceType).Dec()
}

func recordPGNotifyEvent(resourceType, operation string) {
	pgNotifyEventsTotal.WithLabelValues(resourceType, operation).Inc()
}

func recordPGNotifyError() {
	pgNotifyErrorsTotal.Inc()
}

func IncrementActiveWatchers(resourceType string) {
	incrementActiveWatchers(resourceType)
}

func DecrementActiveWatchers(resourceType string) {
	decrementActiveWatchers(resourceType)
}

