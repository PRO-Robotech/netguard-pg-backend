package manager

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
	"netguard-pg-backend/internal/sync/utils"
)

// syncManager implements SyncManager interface
type syncManager struct {
	gateway     interfaces.SGroupGateway
	syncers     map[types.SyncSubjectType]interface{}
	syncTracker interfaces.SyncTracker
	retryConfig interfaces.RetryConfig
	logger      logr.Logger

	// Background processing
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	// Configuration
	cleanupInterval time.Duration
	maxEntryAge     time.Duration
}

// NewSyncManager creates a new sync manager
func NewSyncManager(gateway interfaces.SGroupGateway, logger logr.Logger) interfaces.SyncManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &syncManager{
		gateway:         gateway,
		syncers:         make(map[types.SyncSubjectType]interface{}),
		syncTracker:     utils.NewSyncTracker(1 * time.Second), // 1 second debounce
		retryConfig:     utils.DefaultRetryConfig(),
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
		cleanupInterval: 10 * time.Minute,
		maxEntryAge:     1 * time.Hour,
	}
}

// RegisterSyncer registers a syncer for a specific subject type
func (sm *syncManager) RegisterSyncer(subjectType types.SyncSubjectType, syncer interface{}) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Validate that syncer implements the correct interface using reflection
	if err := sm.validateSyncer(syncer); err != nil {
		return fmt.Errorf("invalid syncer for subject type %s: %w", subjectType, err)
	}

	sm.syncers[subjectType] = syncer
	sm.logger.Info("Registered syncer", "subjectType", subjectType)

	return nil
}

// SyncEntity synchronizes a single entity using the appropriate syncer
func (sm *syncManager) SyncEntity(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	return sm.syncEntityInternal(ctx, entity, operation, false)
}

// SyncEntityForced synchronizes a single entity bypassing debouncing
func (sm *syncManager) SyncEntityForced(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	return sm.syncEntityInternal(ctx, entity, operation, true)
}

// syncEntityInternal implements the core sync logic with optional forced sync
func (sm *syncManager) syncEntityInternal(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation, forced bool) error {
	if entity == nil {
		return fmt.Errorf("entity cannot be nil")
	}

	subjectType := entity.GetSyncSubjectType()
	syncKey := entity.GetSyncKey()


	// Check if we should sync (debouncing or forced)
	var shouldSync bool
	if forced {
		shouldSync = sm.syncTracker.ShouldSyncForced(syncKey, operation)
	} else {
		shouldSync = sm.syncTracker.ShouldSync(syncKey, operation)
	}

	if !shouldSync {
		sm.logger.V(1).Info("Skipping sync due to debouncing", "key", syncKey, "operation", operation)
		return nil
	}

	// Get the appropriate syncer
	sm.mu.RLock()
	syncer, exists := sm.syncers[subjectType]
	sm.mu.RUnlock()


	if !exists {
		err := fmt.Errorf("no syncer registered for subject type: %s", subjectType)
		sm.syncTracker.Track(subjectType, operation, false)
		return err
	}

	// Execute sync with retry
	startTime := time.Now()
	err := utils.ExecuteWithRetry(ctx, sm.retryConfig, func() error {
		return sm.executeSyncWithReflection(ctx, syncer, entity, operation)
	})

	// Track the result
	success := err == nil
	sm.syncTracker.Track(subjectType, operation, success)

	if success {
		sm.logger.Info("Successfully synced entity",
			"key", syncKey,
			"subjectType", subjectType,
			"operation", operation,
			"duration", time.Since(startTime))
	} else {
		sm.logger.Error(err, "Failed to sync entity",
			"key", syncKey,
			"subjectType", subjectType,
			"operation", operation,
			"duration", time.Since(startTime))
	}

	return err
}

// SyncBatch synchronizes multiple entities in a batch
func (sm *syncManager) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	if len(entities) == 0 {
		return nil
	}

	// Group entities by subject type
	entityGroups := make(map[types.SyncSubjectType][]interfaces.SyncableEntity)
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		subjectType := entity.GetSyncSubjectType()
		entityGroups[subjectType] = append(entityGroups[subjectType], entity)
	}

	// Sync each group
	var lastErr error
	for subjectType, groupEntities := range entityGroups {
		sm.mu.RLock()
		syncer, exists := sm.syncers[subjectType]
		sm.mu.RUnlock()

		if !exists {
			err := fmt.Errorf("no syncer registered for subject type: %s", subjectType)
			sm.syncTracker.Track(subjectType, operation, false)
			lastErr = err
			continue
		}

		// Execute batch sync with retry
		startTime := time.Now()
		err := utils.ExecuteWithRetry(ctx, sm.retryConfig, func() error {
			return sm.executeBatchSyncWithReflection(ctx, syncer, groupEntities, operation)
		})

		// Track the result
		success := err == nil
		sm.syncTracker.Track(subjectType, operation, success)

		if success {
			sm.logger.Info("Successfully synced batch",
				"subjectType", subjectType,
				"operation", operation,
				"count", len(groupEntities),
				"duration", time.Since(startTime))
		} else {
			sm.logger.Error(err, "Failed to sync batch",
				"subjectType", subjectType,
				"operation", operation,
				"count", len(groupEntities),
				"duration", time.Since(startTime))
			lastErr = err
		}
	}

	return lastErr
}

// Start starts the sync manager background processes
func (sm *syncManager) Start(ctx context.Context) error {
	sm.logger.Info("Starting sync manager")

	// Start cleanup routine
	sm.wg.Add(1)
	go sm.cleanupRoutine()

	return nil
}

// Stop stops the sync manager
func (sm *syncManager) Stop() error {
	sm.logger.Info("Stopping sync manager")

	sm.cancel()
	sm.wg.Wait()

	return nil
}

// validateSyncer validates that syncer implements EntitySyncer interface using reflection
func (sm *syncManager) validateSyncer(syncer interface{}) error {
	syncerType := reflect.TypeOf(syncer)
	if syncerType == nil {
		return fmt.Errorf("syncer cannot be nil")
	}

	// Check if syncer has required methods
	requiredMethods := []string{"Sync", "SyncBatch", "GetSupportedSubjectType"}
	for _, methodName := range requiredMethods {
		if _, found := syncerType.MethodByName(methodName); !found {
			return fmt.Errorf("syncer must implement method: %s", methodName)
		}
	}

	return nil
}

// executeSyncWithReflection executes sync using reflection
func (sm *syncManager) executeSyncWithReflection(ctx context.Context, syncer interface{}, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	syncerValue := reflect.ValueOf(syncer)
	method := syncerValue.MethodByName("Sync")

	if !method.IsValid() {
		return fmt.Errorf("syncer does not have Sync method")
	}

	// Call Sync(ctx, entity, operation)
	args := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(entity),
		reflect.ValueOf(operation),
	}

	results := method.Call(args)
	if len(results) != 1 {
		return fmt.Errorf("Sync method should return exactly one value (error)")
	}

	// Check if error was returned
	if !results[0].IsNil() {
		return results[0].Interface().(error)
	}

	return nil
}

// executeBatchSyncWithReflection executes batch sync using reflection
func (sm *syncManager) executeBatchSyncWithReflection(ctx context.Context, syncer interface{}, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	syncerValue := reflect.ValueOf(syncer)
	method := syncerValue.MethodByName("SyncBatch")

	if !method.IsValid() {
		return fmt.Errorf("syncer does not have SyncBatch method")
	}

	// Call SyncBatch(ctx, entities, operation)
	args := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(entities),
		reflect.ValueOf(operation),
	}

	results := method.Call(args)
	if len(results) != 1 {
		return fmt.Errorf("SyncBatch method should return exactly one value (error)")
	}

	// Check if error was returned
	if !results[0].IsNil() {
		return results[0].Interface().(error)
	}

	return nil
}

// cleanupRoutine runs periodic cleanup of old entries
func (sm *syncManager) cleanupRoutine() {
	defer sm.wg.Done()

	ticker := time.NewTicker(sm.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			// Periodic cleanup - for now just log
			// TODO: Add cleanup method to SyncTracker interface if needed
			sm.logger.V(1).Info("Cleanup routine executed")
		}
	}
}
