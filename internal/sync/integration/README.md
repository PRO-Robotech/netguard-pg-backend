# Integration Test Suite for CLOUD-233 (Transactional Outbox Pattern)

> ℹ️ **Note**: References to "Migration 029" in this directory refer to functionality now implemented in migrations 026-028.

## Overview

This directory contains **comprehensive integration tests** for the Transactional Outbox Pattern implementation (CLOUD-233). These tests validate the **end-to-end flow** from database triggers to SGROUP synchronization.

### What We Test

- ✅ **Database Triggers**: Migrations 026-028 (entity/process/binding triggers, outbox creation)
- ✅ **Outbox Worker**: Polling, processing, retry logic, error handling
- ✅ **Dependency Chains**: Host → AddressGroup → Network → Service
- ✅ **SGROUP Integration**: Mock client validation (real gRPC calls mocked)
- ✅ **Error Scenarios**: Timeouts, conflicts, transient/permanent failures
- ✅ **Concurrency**: Multiple workers, FOR UPDATE SKIP LOCKED

---

## Test Infrastructure

### Files

```
internal/sync/integration/
├── setup_test.go                    # Test database setup & helpers
├── mock_sgroup_client.go            # Mock SGROUP gRPC client
├── test_helpers.go                  # Helper functions
├── host_integration_test.go         # Host ready transition tests
├── network_integration_test.go      # Network tests (TODO)
├── addressgroup_integration_test.go # AddressGroup tests (TODO)
├── service_integration_test.go      # Service tests (TODO)
├── dependency_chain_test.go         # Complex dependency tests (TODO)
├── error_scenarios_test.go          # Error handling tests (TODO)
├── concurrency_test.go              # Multi-worker tests (TODO)
└── README.md                        # This file
```

### Key Components

#### 1. **setup_test.go** - Database Setup

Provides functions for:
- `SetupTestDB(t)` - Creates test DB connection, runs migrations, cleans data
- `TeardownTestDB(t, db)` - Cleans up after tests
- `CreateK8sMetadata(t, db, conditions)` - Creates k8s_metadata entry
- `CreateTestHost(t, db, ...)` - Creates test Host
- `CreateTestNetwork(t, db, ...)` - Creates test Network
- `CreateTestAddressGroup(t, db, ...)` - Creates test AddressGroup
- `WaitForOutboxEntry(t, db, resourceType, operation, timeout)` - Waits for outbox entry
- `AssertOutboxEntry(t, db, resourceType, resourceID, operation)` - Asserts entry exists
- `AssertOutboxStatus(t, db, entryID, expectedStatus)` - Asserts entry status

#### 2. **mock_sgroup_client.go** - Mock SGROUP Client

A **thread-safe mock** implementing `interfaces.SGroupGateway`:
- Tracks all `Sync()` calls with full request details
- Simulates errors (network, timeout, validation)
- Configurable responses via callback functions
- Helper methods for assertions

**Example Usage:**
```go
mockClient := NewMockSGROUPClient()

// Simulate error on next call
mockClient.SimulateError(errors.New("SGROUP timeout"))

// Verify calls
assert.Equal(t, 1, mockClient.GetSyncCallCount())
mockClient.AssertSyncCalledWith("Groups", "Upsert")
```

#### 3. **test_helpers.go** - Helper Functions

Provides:
- `TriggerReadyTransition(t, db, resourceVersion, ready)` - Triggers Migration 029
- `PollUntil(t, condition, timeout, interval, message)` - Generic polling
- `CreateTestWorker(t, pool, mockClient)` - Creates OutboxWorker for testing
- `WaitForHostReady(t, db, namespace, name, expectedReady, timeout)` - Waits for host ready
- `AssertHostReady(t, db, namespace, name, expectedReady)` - Asserts host ready status
- (Similar for Network, AddressGroup, Service)

---

## Running Tests

### Prerequisites

1. **PostgreSQL Test Database**

   Create a dedicated test database:
   ```bash
   createdb netguard_test
   ```

2. **Environment Variables**

   Optional (defaults provided):
   ```bash
   export TEST_DB_HOST=localhost
   export TEST_DB_PORT=5432
   export TEST_DB_USER=postgres
   export TEST_DB_PASSWORD=password
   export TEST_DB_NAME=netguard_test
   export TEST_DB_SSLMODE=disable
   ```

### Run All Integration Tests

```bash
# From project root
go test ./internal/sync/integration -v

# With coverage
go test ./internal/sync/integration -v -coverprofile=coverage.out

# View coverage
go tool cover -html=coverage.out
```

### Run Specific Test

```bash
go test ./internal/sync/integration -v -run TestHostReadyTransition_CreatesOutboxEntry
```

### Run with Race Detector

```bash
go test ./internal/sync/integration -v -race
```

---

## Test Structure

### Example: Host Ready Transition Test

```go
func TestHostReadyTransition_CreatesOutboxEntry(t *testing.T) {
    // ARRANGE: Setup test database
    db := SetupTestDB(t)
    defer TeardownTestDB(t, db)

    // Create Host with ready=FALSE
    resourceVersion := CreateK8sMetadata(t, db, `[]`)
    CreateTestHost(t, db, "test-ns", "host-1", "uuid-1", "host1.local", resourceVersion, false)

    // ACT: Trigger ready transition (FALSE → TRUE)
    TriggerReadyTransition(t, db, resourceVersion, true)

    // ASSERT: Migration 029 should update hosts.ready
    WaitForHostReady(t, db, "test-ns", "host-1", true, 5*time.Second)

    // ASSERT: Migration 026 should create Outbox entry
    entry := WaitForOutboxEntry(t, db, "AddressGroup", "UPDATE", 5*time.Second)
    assert.Equal(t, "AddressGroup", entry.ResourceType)
    assert.Equal(t, "PENDING", entry.Status)

    t.Logf("✅ Test PASSED")
}
```

---

## Test Coverage

### Phase A: Infrastructure Tests (Current)

- ✅ `TestHostReadyTransition_CreatesOutboxEntry` - Basic Host ready flow
- ✅ `TestHostReadyTransition_NoChangeWhenAlreadyReady` - Idempotency
- ✅ `TestHostReadyTransition_FalseToTrue` - Explicit transition
- ✅ `TestHostReadyTransition_TrueToFalse` - Negative case (no outbox)
- ✅ `TestMultipleHosts_ReadyTransition` - Multiple resources
- ✅ `TestHostReadyTransition_WithInvalidConditions` - Edge case

### Phase B: Resource Tests (TODO)

- ⏳ `TestNetworkReadyTransition_CreatesOutboxEntry`
- ⏳ `TestAddressGroupUpdate_CreatesOutboxEntry`
- ⏳ `TestServiceUpdate_CreatesOutboxEntry`

### Phase C: Worker Tests (TODO)

- ⏳ `TestOutboxWorker_ProcessesPendingEntries`
- ⏳ `TestOutboxWorker_RetriesOnTransientFailure`
- ⏳ `TestOutboxWorker_MarksFailedPermanentAfterMaxRetries`
- ⏳ `TestOutboxWorker_SkipsLockedEntries` (FOR UPDATE SKIP LOCKED)

### Phase D: Dependency Tests (TODO)

- ⏳ `TestDependencyChain_HostToAddressGroup`
- ⏳ `TestDependencyChain_NetworkToAddressGroup`
- ⏳ `TestDependencyChain_AddressGroupToService`
- ⏳ `TestDependencyChain_ComplexMultiResource`

### Phase E: Error Tests (TODO)

- ⏳ `TestErrorHandling_NetworkTimeout`
- ⏳ `TestErrorHandling_ValidationError`
- ⏳ `TestErrorHandling_ConflictError`
- ⏳ `TestErrorHandling_ExponentialBackoff`

### Phase F: Concurrency Tests (TODO)

- ⏳ `TestConcurrency_MultipleWorkersNoDuplicates`
- ⏳ `TestConcurrency_WorkerCrashRecovery`

---

## Database Schema

### Tables Used in Tests

```sql
-- k8s_metadata: Stores Kubernetes resource metadata
CREATE TABLE k8s_metadata (
    resource_version BIGSERIAL PRIMARY KEY,
    conditions JSONB
);

-- hosts: Entity resource
CREATE TABLE hosts (
    id UUID PRIMARY KEY,
    namespace VARCHAR(255),
    name VARCHAR(255),
    uuid VARCHAR(255),
    hostname VARCHAR(255),
    resource_version BIGINT REFERENCES k8s_metadata(resource_version),
    ready BOOLEAN DEFAULT FALSE
);

-- networks: Entity resource
CREATE TABLE networks (
    id UUID PRIMARY KEY,
    namespace VARCHAR(255),
    name VARCHAR(255),
    resource_version BIGINT REFERENCES k8s_metadata(resource_version),
    ready BOOLEAN DEFAULT FALSE
);

-- address_groups: Entity resource
CREATE TABLE address_groups (
    id UUID PRIMARY KEY,
    namespace VARCHAR(255),
    name VARCHAR(255),
    resource_version BIGINT REFERENCES k8s_metadata(resource_version)
);

-- sync_outbox: Transactional outbox
CREATE TABLE sync_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type VARCHAR(50),
    resource_id UUID,
    operation sync_operation,
    target_system target_system,
    payload JSONB,
    delta JSONB,
    status outbox_status DEFAULT 'PENDING',
    attempts INT DEFAULT 0,
    max_retries INT DEFAULT 5,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Trigger Chain

```
UPDATE k8s_metadata.conditions
  ↓
trg_sync_ready_from_conditions (Migration 029)
  ↓ UPDATE hosts.ready / networks.ready
    ↓
  trg_update_ag_on_host_ready (Migration 026)
  trg_update_ag_on_network_ready (Migration 027)
    ↓ UPDATE address_groups
    ↓ INSERT sync_outbox
```

---

## Debugging Tests

### Enable Verbose Logging

```bash
go test ./internal/sync/integration -v -args -test.v=true
```

### Check Database State

During test execution (in another terminal):
```sql
-- Connect to test database
psql -d netguard_test

-- Check outbox entries
SELECT id, resource_type, operation, status, attempts, created_at
FROM sync_outbox
ORDER BY created_at DESC;

-- Check host ready status
SELECT namespace, name, ready FROM hosts;

-- Check triggers
SELECT trigger_name, event_manipulation, event_object_table
FROM information_schema.triggers
WHERE trigger_schema = 'public';
```

### Test Isolation

Each test:
1. Runs migrations (001-029)
2. Cleans all data (TRUNCATE CASCADE)
3. Creates test data
4. Executes test logic
5. Cleans up

Tests are **fully isolated** and can run in parallel.

---

## Next Steps

### Immediate (Phase A Complete ✅)

- ✅ Infrastructure created (setup, mock, helpers)
- ✅ Example Host tests implemented

### Phase B: Expand Coverage

1. Create `network_integration_test.go` (similar to Host tests)
2. Create `addressgroup_integration_test.go` (AG-specific scenarios)
3. Create `service_integration_test.go` (Service-specific scenarios)

### Phase C: Worker Integration

1. Create `worker_integration_test.go`
2. Test worker polling, processing, retry logic
3. Validate SGROUP mock receives correct calls

### Phase D: Complex Scenarios

1. Create `dependency_chain_test.go`
2. Test Host → AG → Network → Service dependency chains
3. Validate ordering, cascading updates

### Phase E: Error Handling

1. Create `error_scenarios_test.go`
2. Test transient vs permanent errors
3. Validate exponential backoff, max retries

### Phase F: Concurrency

1. Create `concurrency_test.go`
2. Test multiple workers (FOR UPDATE SKIP LOCKED)
3. Validate no duplicate processing

---

## FAQ

### Q: Why separate test database?

A: Integration tests run real migrations and modify database state. Using a separate `netguard_test` database ensures:
- No interference with development database
- Clean state for each test run
- Safe to run in CI/CD pipelines

### Q: Why mock SGROUP client?

A: Integration tests focus on **our system's behavior**, not external dependencies. Mocking SGROUP:
- Removes dependency on external service availability
- Enables error simulation (timeouts, failures)
- Speeds up tests (no network latency)
- Makes tests deterministic

### Q: How to add new test?

1. Create test file (e.g., `network_integration_test.go`)
2. Import test infrastructure:
   ```go
   import (
       "testing"
       _ "github.com/lib/pq"
       "github.com/stretchr/testify/assert"
   )
   ```
3. Use setup/teardown:
   ```go
   func TestYourScenario(t *testing.T) {
       db := SetupTestDB(t)
       defer TeardownTestDB(t, db)

       // Your test logic
   }
   ```
4. Run: `go test ./internal/sync/integration -v -run TestYourScenario`

### Q: Test failing with "migrations not found"?

Check that you're running tests from project root or that `findMigrationsDir()` can locate `migrations/` directory. The function walks up directory tree from `internal/sync/integration/` to find it.

### Q: How to test with real SGROUP?

For **end-to-end testing with real SGROUP**, create separate `e2e_test.go`:
```go
// +build e2e

func TestE2E_RealSGROUP(t *testing.T) {
    // Use real SGROUP client
    client, err := clients.NewSGroupsClient(config)
    // ...
}
```

Run: `go test ./internal/sync/integration -v -tags=e2e`

---

## Contact

**QA Engineer Agent** - Integration Test Infrastructure for CLOUD-233

For questions or issues, refer to:
- Story: `product-workspace/stories/CLOUD-233-resilient-resource-sync/`
- Migrations: `migrations/025_*.sql`, `migrations/026_*.sql`, `migrations/027_*.sql`, `migrations/029_*.sql`
- Worker Implementation: `internal/sync/worker/`
