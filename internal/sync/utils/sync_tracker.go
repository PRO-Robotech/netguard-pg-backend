package utils

import (
	"sync"
	"time"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// syncTracker implements SyncTracker interface
type syncTracker struct {
	mu           sync.RWMutex
	stats        map[types.SyncSubjectType]*syncStats
	debounceMap  map[string]*debounceEntry
	debounceTime time.Duration
}

// syncStats holds statistics for a subject type
type syncStats struct {
	totalRequests   int64
	successfulSyncs int64
	failedSyncs     int64
	lastSyncTime    int64
	totalLatency    int64 // for calculating average
}

// debounceEntry holds debouncing information for an entity
type debounceEntry struct {
	lastSyncTime time.Time
	operation    types.SyncOperation
}

// NewSyncTracker creates a new sync tracker with specified debounce time
func NewSyncTracker(debounceTime time.Duration) interfaces.SyncTracker {
	return &syncTracker{
		stats:        make(map[types.SyncSubjectType]*syncStats),
		debounceMap:  make(map[string]*debounceEntry),
		debounceTime: debounceTime,
	}
}

// Track records a sync operation
func (st *syncTracker) Track(subjectType types.SyncSubjectType, operation types.SyncOperation, success bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	stats, exists := st.stats[subjectType]
	if !exists {
		stats = &syncStats{}
		st.stats[subjectType] = stats
	}

	stats.totalRequests++
	stats.lastSyncTime = time.Now().Unix()

	if success {
		stats.successfulSyncs++
	} else {
		stats.failedSyncs++
	}
}

// GetStats returns synchronization statistics
func (st *syncTracker) GetStats() map[types.SyncSubjectType]interfaces.SyncStats {
	st.mu.RLock()
	defer st.mu.RUnlock()

	result := make(map[types.SyncSubjectType]interfaces.SyncStats)

	for subjectType, stats := range st.stats {
		avgLatency := int64(0)
		if stats.totalRequests > 0 {
			avgLatency = stats.totalLatency / stats.totalRequests
		}

		result[subjectType] = interfaces.SyncStats{
			TotalRequests:   stats.totalRequests,
			SuccessfulSyncs: stats.successfulSyncs,
			FailedSyncs:     stats.failedSyncs,
			LastSyncTime:    stats.lastSyncTime,
			AverageLatency:  avgLatency,
		}
	}

	return result
}

// ShouldSync determines if an entity should be synchronized (debouncing)
func (st *syncTracker) ShouldSync(key string, operation types.SyncOperation) bool {
	return st.shouldSyncInternal(key, operation, false)
}

// ShouldSyncForced forces sync regardless of debouncing
func (st *syncTracker) ShouldSyncForced(key string, operation types.SyncOperation) bool {
	return st.shouldSyncInternal(key, operation, true)
}

// shouldSyncInternal implements the core logic with optional forced sync
func (st *syncTracker) shouldSyncInternal(key string, operation types.SyncOperation, forced bool) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	entry, exists := st.debounceMap[key]
	now := time.Now()

	// If forced sync, always allow
	if forced {
		st.debounceMap[key] = &debounceEntry{
			lastSyncTime: now,
			operation:    operation,
		}
		return true
	}

	// If no previous sync or debounce time has passed, allow sync
	if !exists || now.Sub(entry.lastSyncTime) >= st.debounceTime {
		st.debounceMap[key] = &debounceEntry{
			lastSyncTime: now,
			operation:    operation,
		}
		return true
	}

	// If it's a delete operation, always allow (higher priority)
	if operation == types.SyncOperationDelete {
		st.debounceMap[key] = &debounceEntry{
			lastSyncTime: now,
			operation:    operation,
		}
		return true
	}

	return false
}

// CleanupOldEntries removes old debounce entries to prevent memory leaks
func (st *syncTracker) CleanupOldEntries(maxAge time.Duration) {
	st.mu.Lock()
	defer st.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)

	for key, entry := range st.debounceMap {
		if entry.lastSyncTime.Before(cutoff) {
			delete(st.debounceMap, key)
		}
	}
}
