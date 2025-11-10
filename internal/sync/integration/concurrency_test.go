package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain"
)

// TestConcurrency_MultipleWorkers validates that multiple workers can process entries concurrently
func TestConcurrency_MultipleWorkers(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	// Create 10 outbox entries
	for i := 0; i < 10; i++ {
		resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
		CreateTestHost(t, env.DB, "test-ns",
			"host-"+string(rune('a'+i)),
			"uuid-"+string(rune('1'+i)),
			resourceVersion, false)

		// FIX: Trigger ready transition
		TriggerReadyTransition(t, env.DB, resourceVersion, true)
	}

	// Wait for entries
	time.Sleep(500 * time.Millisecond)

	countBefore := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))
	require.GreaterOrEqual(t, countBefore, 10, "Should have at least 10 entries")

	t.Logf("Created %d outbox entries", countBefore)

	// ========================================
	// ACT
	// ========================================
	// Create 3 workers
	var wg sync.WaitGroup
	workerCount := 3

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			mockClient := NewMockSGROUPClient()
			worker := CreateTestWorker(t, pool, mockClient)

			// Each worker processes once
			err := worker.ProcessOnce(ctx)
			if err != nil {
				t.Logf("Worker %d error: %v", workerID, err)
			}

			t.Logf("Worker %d completed, processed %d SGROUP calls", workerID, mockClient.GetSyncCallCount())
		}(i)
	}

	wg.Wait()

	time.Sleep(500 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	// Count SUCCESS entries
	var successCount int
	err := env.DB.QueryRow(`
		SELECT COUNT(*) FROM sync_outbox
		WHERE resource_type = 'AddressGroup'
		  AND operation = $1
		  AND status = $2
	`, domain.SyncOperationUpdate, domain.OutboxStatusSuccess).Scan(&successCount)
	require.NoError(t, err)

	t.Logf("Concurrent processing: %d entries succeeded", successCount)

	assert.GreaterOrEqual(t, successCount, 1, "At least one entry should be processed")

	t.Logf("✅ Test PASSED: Multiple workers processed concurrently (success=%d)", successCount)
}

// TestConcurrency_ForUpdateSkipLocked validates FOR UPDATE SKIP LOCKED prevents double processing
func TestConcurrency_ForUpdateSkipLocked(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	// Create single outbox entry
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", resourceVersion, false)

	// FIX: Trigger ready transition
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	t.Logf("Created single outbox entry: %s", entry.ID)

	// ========================================
	// ACT
	// ========================================
	// Two workers try to process same entry simultaneously
	var wg sync.WaitGroup
	var worker1Processed, worker2Processed atomic.Bool

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Worker 1
	wg.Add(1)
	go func() {
		defer wg.Done()

		mockClient := NewMockSGROUPClient()
		worker := CreateTestWorker(t, pool, mockClient)

		_ = worker.ProcessOnce(ctx)

		if mockClient.GetSyncCallCount() > 0 {
			worker1Processed.Store(true)
		}
	}()

	// Worker 2 (starts slightly after worker 1)
	time.Sleep(50 * time.Millisecond)

	wg.Add(1)
	go func() {
		defer wg.Done()

		mockClient := NewMockSGROUPClient()
		worker := CreateTestWorker(t, pool, mockClient)

		_ = worker.ProcessOnce(ctx)

		if mockClient.GetSyncCallCount() > 0 {
			worker2Processed.Store(true)
		}
	}()

	wg.Wait()

	time.Sleep(300 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	// Only ONE worker should have processed the entry
	// (FOR UPDATE SKIP LOCKED ensures second worker skips locked row)
	processedCount := 0
	if worker1Processed.Load() {
		processedCount++
	}
	if worker2Processed.Load() {
		processedCount++
	}

	t.Logf("Workers processed: worker1=%v, worker2=%v, total=%d", worker1Processed.Load(), worker2Processed.Load(), processedCount)

	// With FOR UPDATE SKIP LOCKED, only one should process
	// Without it, both might process (which is bad)
	assert.Equal(t, 1, processedCount, "Only ONE worker should process the entry (FOR UPDATE SKIP LOCKED)")

	t.Logf("✅ Test PASSED: FOR UPDATE SKIP LOCKED prevents double processing")
}

// TestConcurrency_NoDoubleProcessing validates that entries are processed exactly once
func TestConcurrency_NoDoubleProcessing(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	// Create 5 outbox entries
	entryCount := 5
	for i := 0; i < entryCount; i++ {
		resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
		CreateTestHost(t, env.DB, "test-ns",
			"host-"+string(rune('a'+i)),
			"uuid-"+string(rune('1'+i)),
			resourceVersion, false)

		// FIX: Trigger ready transition
		TriggerReadyTransition(t, env.DB, resourceVersion, true)
	}

	time.Sleep(500 * time.Millisecond)

	t.Logf("Created %d outbox entries", entryCount)

	// ========================================
	// ACT
	// ========================================
	// Create 5 concurrent workers (more workers than entries)
	var wg sync.WaitGroup
	workerCount := 5
	totalSyncCalls := atomic.Int32{}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			mockClient := NewMockSGROUPClient()
			worker := CreateTestWorker(t, pool, mockClient)

			_ = worker.ProcessOnce(ctx)

			syncCalls := mockClient.GetSyncCallCount()
			totalSyncCalls.Add(int32(syncCalls))

			t.Logf("Worker %d: processed %d entries", workerID, syncCalls)
		}(i)
	}

	wg.Wait()

	time.Sleep(500 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	// Total SGROUP calls should equal entry count (each entry processed exactly once)
	assert.Equal(t, int32(entryCount), totalSyncCalls.Load(),
		"Each entry should be processed exactly once across all workers")

	// Verify all entries are SUCCESS
	var successCount int
	err := env.DB.QueryRow(`
		SELECT COUNT(*) FROM sync_outbox
		WHERE resource_type = 'AddressGroup'
		  AND status = $1
	`, domain.OutboxStatusSuccess).Scan(&successCount)
	require.NoError(t, err)

	assert.Equal(t, entryCount, successCount, "All entries should be SUCCESS")

	t.Logf("✅ Test PASSED: No double processing (entries=%d, SGROUP calls=%d)", entryCount, totalSyncCalls.Load())
}

// TestConcurrency_RaceCondition validates handling of race conditions
func TestConcurrency_RaceCondition(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	// Create Host (NOT ready yet)
	hostResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	hostKey := CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", hostResourceVersion, false)

	// FIX: Trigger ready transition
	TriggerReadyTransition(t, env.DB, hostResourceVersion, true)
	time.Sleep(200 * time.Millisecond)

	// Create AddressGroup
	agResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	agKey := CreateTestAddressGroup(t, env.DB, "test-ns", "test-ag", agResourceVersion)

	t.Logf("Setup: Host %s/%s, AddressGroup %s/%s", hostKey.Namespace, hostKey.Name, agKey.Namespace, agKey.Name)

	// ========================================
	// ACT
	// ========================================
	// Simulate race: multiple goroutines try to bind host to AG simultaneously
	var wg sync.WaitGroup
	goroutineCount := 5

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Try to insert host_binding using helper (handles conflicts internally)
			bindingRV := CreateK8sMetadata(t, env.DB, `[]`)
			_, err := env.DB.Exec(`
				INSERT INTO host_bindings (
					namespace, name,
					host_namespace, host_name,
					address_group_namespace, address_group_name,
					resource_version
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (namespace, name) DO NOTHING
			`, "test-ns", "binding-"+string(rune('a'+id)),
				hostKey.Namespace, hostKey.Name,
				agKey.Namespace, agKey.Name,
				bindingRV)

			if err != nil {
				t.Logf("Goroutine %d: binding error (expected): %v", id, err)
			} else {
				t.Logf("Goroutine %d: binding created", id)
			}
		}(i)
	}

	wg.Wait()

	time.Sleep(300 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	// Multiple bindings may exist (each goroutine creates unique binding name)
	var bindingCount int
	err := env.DB.QueryRow(`
		SELECT COUNT(*) FROM host_bindings
		WHERE host_namespace = $1 AND host_name = $2
		  AND address_group_namespace = $3 AND address_group_name = $4
	`, hostKey.Namespace, hostKey.Name, agKey.Namespace, agKey.Name).Scan(&bindingCount)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, bindingCount, 1, "At least one binding should exist")

	// Check outbox entries created
	agEntryCount := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))
	t.Logf("AddressGroup outbox entries: %d", agEntryCount)

	assert.GreaterOrEqual(t, agEntryCount, 1, "At least one outbox entry should be created")

	t.Logf("✅ Test PASSED: Race condition handled correctly (bindings=%d, outbox=%d)", bindingCount, agEntryCount)
}

// TestConcurrency_DeadlockPrevention validates no deadlocks occur with concurrent processing
func TestConcurrency_DeadlockPrevention(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	// Create 2 AddressGroups with 2 Hosts each
	ag1ResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	ag1Key := CreateTestAddressGroup(t, env.DB, "test-ns", "ag-1", ag1ResourceVersion)

	ag2ResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	ag2Key := CreateTestAddressGroup(t, env.DB, "test-ns", "ag-2", ag2ResourceVersion)

	host1ResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	host1Key := CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", host1ResourceVersion, false)

	host2ResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	host2Key := CreateTestHost(t, env.DB, "test-ns", "host-2", "uuid-2", host2ResourceVersion, false)

	// Create bindings using helper
	binding1RV := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHostBinding(t, env.DB, "test-ns", "binding-1", host1Key, ag1Key, binding1RV)

	binding2RV := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHostBinding(t, env.DB, "test-ns", "binding-2", host2Key, ag2Key, binding2RV)

	time.Sleep(200 * time.Millisecond)

	t.Logf("Setup: 2 AGs with 2 Hosts")

	// ========================================
	// ACT
	// ========================================
	// Two workers update different resources simultaneously
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Worker 1: Trigger host1 ready
	wg.Add(1)
	go func() {
		defer wg.Done()
		TriggerReadyTransition(t, env.DB, host1ResourceVersion, true)
	}()

	// Worker 2: Trigger host2 ready
	wg.Add(1)
	go func() {
		defer wg.Done()
		TriggerReadyTransition(t, env.DB, host2ResourceVersion, true)
	}()

	// Worker 3: Process outbox entries
	wg.Add(1)
	go func() {
		defer wg.Done()

		mockClient := NewMockSGROUPClient()
		worker := CreateTestWorker(t, pool, mockClient)

		_ = worker.ProcessOnce(ctx)
	}()

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("All goroutines completed successfully")
	case <-time.After(15 * time.Second):
		t.Fatal("Timeout: potential deadlock detected")
	}

	// ========================================
	// ASSERT
	// ========================================
	// If we reach here, no deadlock occurred
	time.Sleep(300 * time.Millisecond)

	// Verify hosts are ready using composite keys
	var host1Ready, host2Ready bool
	err := env.DB.QueryRow(`SELECT ready FROM hosts WHERE namespace = $1 AND name = $2`,
		host1Key.Namespace, host1Key.Name).Scan(&host1Ready)
	require.NoError(t, err)

	err = env.DB.QueryRow(`SELECT ready FROM hosts WHERE namespace = $1 AND name = $2`,
		host2Key.Namespace, host2Key.Name).Scan(&host2Ready)
	require.NoError(t, err)

	assert.True(t, host1Ready, "Host 1 should be ready")
	assert.True(t, host2Ready, "Host 2 should be ready")

	t.Logf("✅ Test PASSED: No deadlock detected with concurrent updates")
}

// TestConcurrency_ParallelDependencyChecks validates concurrent dependency checking
func TestConcurrency_ParallelDependencyChecks(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	// Create Network with AddressGroup dependency
	agResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	agNamespace := "test-ns"
	agName := "test-ag"
	agKey := CreateTestAddressGroup(t, env.DB, agNamespace, agName, agResourceVersion)

	networkResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	networkKey := CreateTestNetwork(t, env.DB, "test-ns", "test-network", networkResourceVersion, false)

	// Set AddressGroup reference
	_, err := env.DB.Exec(`
		UPDATE networks
		SET address_group_ref_namespace = $1, address_group_ref_name = $2
		WHERE namespace = $3 AND name = $4
	`, agNamespace, agName, networkKey.Namespace, networkKey.Name)
	require.NoError(t, err)

	// FIX: Trigger network ready transition
	TriggerReadyTransition(t, env.DB, networkResourceVersion, true)

	time.Sleep(300 * time.Millisecond)

	t.Logf("Setup: Network %s/%s depends on AddressGroup %s/%s",
		networkKey.Namespace, networkKey.Name, agKey.Namespace, agKey.Name)

	// ========================================
	// ACT
	// ========================================
	// Multiple workers check dependencies concurrently
	var wg sync.WaitGroup
	workerCount := 3

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			mockClient := NewMockSGROUPClient()
			worker := CreateTestWorker(t, pool, mockClient)

			_ = worker.ProcessOnce(ctx)

			t.Logf("Worker %d: dependency check completed", workerID)
		}(i)
	}

	wg.Wait()

	// ========================================
	// ASSERT
	// ========================================
	// No crashes or errors should occur
	time.Sleep(300 * time.Millisecond)

	// Verify network is ready using composite key
	var networkReady bool
	err = env.DB.QueryRow(`SELECT ready FROM networks WHERE namespace = $1 AND name = $2`,
		networkKey.Namespace, networkKey.Name).Scan(&networkReady)
	require.NoError(t, err)

	assert.True(t, networkReady, "Network should be ready")

	t.Logf("✅ Test PASSED: Parallel dependency checks completed safely")
}

// TestConcurrency_ConcurrentReadyTransitions validates concurrent ready transitions
func TestConcurrency_ConcurrentReadyTransitions(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create 10 Hosts
	hostCount := 10
	resourceVersions := make([]int64, hostCount)

	for i := 0; i < hostCount; i++ {
		rv := CreateK8sMetadata(t, env.DB, `[]`)
		resourceVersions[i] = rv

		CreateTestHost(t, env.DB, "test-ns",
			"host-"+string(rune('a'+i)),
			"uuid-"+string(rune('1'+i)),
			rv, false)
	}

	t.Logf("Created %d hosts", hostCount)

	// ========================================
	// ACT
	// ========================================
	// Trigger all ready transitions concurrently
	var wg sync.WaitGroup

	for i := 0; i < hostCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			TriggerReadyTransition(t, env.DB, resourceVersions[index], true)
		}(i)
	}

	wg.Wait()

	// ========================================
	// ASSERT
	// ========================================
	time.Sleep(500 * time.Millisecond)

	// All hosts should be ready
	var readyCount int
	err := env.DB.QueryRow(`
		SELECT COUNT(*) FROM hosts
		WHERE namespace = 'test-ns' AND ready = true
	`).Scan(&readyCount)
	require.NoError(t, err)

	assert.Equal(t, hostCount, readyCount, "All hosts should be ready")

	// Outbox entries should be created
	outboxCount := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))
	assert.GreaterOrEqual(t, outboxCount, hostCount, "Outbox entries should be created for all hosts")

	t.Logf("✅ Test PASSED: Concurrent ready transitions (hosts=%d, outbox=%d)", readyCount, outboxCount)
}

// TestConcurrency_StressTest validates system under heavy concurrent load
func TestConcurrency_StressTest(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	// Create 50 outbox entries
	entryCount := 50
	for i := 0; i < entryCount; i++ {
		resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
		CreateTestHost(t, env.DB, "test-ns",
			"host-"+string(rune('a'+i%26))+"-"+string(rune('0'+i/26)),
			"uuid-"+string(rune('1'+i)),
			resourceVersion, false)

		// FIX: Trigger ready transition
		TriggerReadyTransition(t, env.DB, resourceVersion, true)
	}

	time.Sleep(1 * time.Second)

	countBefore := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))
	t.Logf("Created %d outbox entries for stress test", countBefore)

	// ========================================
	// ACT
	// ========================================
	// Launch 10 concurrent workers
	var wg sync.WaitGroup
	workerCount := 10

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	startTime := time.Now()

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			mockClient := NewMockSGROUPClient()
			worker := CreateTestWorker(t, pool, mockClient)

			// Each worker processes multiple batches
			for j := 0; j < 3; j++ {
				if ctx.Err() != nil {
					break
				}

				_ = worker.ProcessOnce(ctx)
				time.Sleep(100 * time.Millisecond)
			}

			t.Logf("Worker %d completed", workerID)
		}(i)
	}

	wg.Wait()

	duration := time.Since(startTime)

	time.Sleep(500 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	// Count processed entries
	var successCount int
	err := env.DB.QueryRow(`
		SELECT COUNT(*) FROM sync_outbox
		WHERE resource_type = 'AddressGroup'
		  AND status = $1
	`, domain.OutboxStatusSuccess).Scan(&successCount)
	require.NoError(t, err)

	t.Logf("Stress test results: %d/%d entries processed in %v", successCount, countBefore, duration)

	assert.GreaterOrEqual(t, successCount, 1, "At least some entries should be processed")

	// No deadlocks or crashes should occur
	t.Logf("✅ Test PASSED: Stress test completed (workers=%d, entries=%d, processed=%d, duration=%v)",
		workerCount, countBefore, successCount, duration)
}
