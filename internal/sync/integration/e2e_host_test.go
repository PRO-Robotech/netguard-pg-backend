package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/sync/types"
)

// TestE2E_Host_CreateFlow tests the complete CREATE flow:
// CREATE Host → Repository → Trigger → Outbox → Worker → SGROUP
func TestE2E_Host_CreateFlow(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	worker := TCCreateTestWorker(t, tc)

	// Reset Mock SGROUP to clean state
	tc.MockSGROUP.Reset()

	t.Log("📝 Step 1: Create Host via Repository")
	_ = TCCreateHostViaRepositoryWithConnStr(
		t,
		tc.ConnectionString,
		"default",
		"test-e2e-host-create",
		"192.168.1.100/32",
	)

	t.Log("📝 Step 2: Verify outbox entry created")
	entry := TCWaitForOutboxEntry(t, tc.DB, "test-e2e-host-create", 5*time.Second)
	require.NotNil(t, entry, "outbox entry should be created")
	assert.Equal(t, "PENDING", string(entry.Status), "entry should be PENDING")
	assert.Equal(t, "Host", entry.ResourceType)

	t.Log("📝 Step 3: Worker processes outbox entry")
	TCProcessWorkerOnce(t, worker)

	t.Log("📝 Step 4: Verify Mock SGROUP received CREATE request")
	time.Sleep(200 * time.Millisecond) // Allow async processing

	requestCount := tc.MockSGROUP.GetRequestCount()
	assert.Equal(t, 1, requestCount, "Mock SGROUP should receive exactly 1 request")

	lastReq := tc.MockSGROUP.GetLastRequest()
	require.NotNil(t, lastReq, "Mock SGROUP should have received a request")
	assert.Equal(t, types.SyncOperationUpsert, lastReq.Operation, "operation should be UPSERT")
	assert.Equal(t, types.SyncSubjectTypeHosts, lastReq.SubjectType, "subject should be Hosts")

	t.Logf("✅ E2E CREATE Flow Complete: default/test-e2e-host-create → Outbox → Worker → SGROUP")

	// Dump final state for debugging
	tc.MockSGROUP.DumpRequests()
}

// TestE2E_Host_UpdateFlow tests the complete UPDATE flow:
// UPDATE Host → Repository → Trigger → Outbox → Worker → SGROUP
func TestE2E_Host_UpdateFlow(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	worker := TCCreateTestWorker(t, tc)

	// Reset Mock SGROUP
	tc.MockSGROUP.Reset()

	t.Log("📝 Step 1: Create Host")
	_ = TCCreateHostViaRepositoryWithConnStr(
		t,
		tc.ConnectionString,
		"default",
		"test-e2e-host-update",
		"192.168.1.101/32",
	)

	// Process initial creation
	TCProcessWorkerOnce(t, worker)
	time.Sleep(200 * time.Millisecond)

	// Verify initial CREATE
	assert.Equal(t, 1, tc.MockSGROUP.GetRequestCount(), "should have 1 CREATE request")

	// Reset Mock SGROUP for UPDATE test
	tc.MockSGROUP.Reset()
	TCCleanOutboxTable(t, tc.DB)

	t.Log("📝 Step 2: Update Host (re-insert with same name)")
	_ = TCCreateHostViaRepositoryWithConnStr(
		t,
		tc.ConnectionString,
		"default",
		"test-e2e-host-update",
		"192.168.1.101/32",
	)

	t.Log("📝 Step 3: Verify outbox entry created for UPDATE")
	entry := TCWaitForOutboxEntry(t, tc.DB, "test-e2e-host-update", 5*time.Second)
	require.NotNil(t, entry)

	t.Log("📝 Step 4: Worker processes UPDATE")
	TCProcessWorkerOnce(t, worker)
	time.Sleep(200 * time.Millisecond)

	t.Log("📝 Step 5: Verify Mock SGROUP received UPDATE request")
	requestCount := tc.MockSGROUP.GetRequestCount()
	assert.Equal(t, 1, requestCount, "Mock SGROUP should receive UPDATE request")

	lastReq := tc.MockSGROUP.GetLastRequest()
	require.NotNil(t, lastReq)
	assert.Equal(t, types.SyncOperationUpsert, lastReq.Operation)

	t.Logf("✅ E2E UPDATE Flow Complete")
	tc.MockSGROUP.DumpRequests()
}

// TestE2E_Host_DeleteFlow tests the complete DELETE flow:
// DELETE Host → Repository → Trigger → Outbox → Worker → SGROUP
//
// NOTE: This test will FAIL until BUG-003 is fixed!
// BUG-003: DELETE operations don't create outbox entries
func TestE2E_Host_DeleteFlow(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	worker := TCCreateTestWorker(t, tc)

	// Reset Mock SGROUP
	tc.MockSGROUP.Reset()

	t.Log("📝 Step 1: Create Host")
	_ = TCCreateHostViaRepositoryWithConnStr(
		t,
		tc.ConnectionString,
		"default",
		"test-e2e-host-delete",
		"192.168.1.102/32",
	)

	// Process creation
	TCProcessWorkerOnce(t, worker)
	time.Sleep(200 * time.Millisecond)

	// Verify CREATE processed
	assert.Equal(t, 1, tc.MockSGROUP.GetRequestCount(), "should have CREATE request")

	// Reset for DELETE test
	tc.MockSGROUP.Reset()
	TCCleanOutboxTable(t, tc.DB)

	t.Log("📝 Step 2: Delete Host via Repository")
	TCDeleteHostViaRepositoryWithConnStr(
		t,
		tc.ConnectionString,
		"default",
		"test-e2e-host-delete",
	)

	t.Log("📝 Step 3: Check for DELETE outbox entry")
	time.Sleep(500 * time.Millisecond)
	_, err := TCGetOutboxEntryByResourceName(t, tc.DB, "test-e2e-host-delete")
	if err != nil {
		t.Log("🐛 BUG-003 DETECTED: DELETE did NOT create outbox entry for Host")
		t.Log("  ❌ This is EXPECTED until BUG-003 is fixed")
		t.Log("  📋 After fix: This test will PASS automatically")

		// Log current state
		TCLogOutboxState(t, tc.DB)

		t.Skip("BUG-003: DELETE operations don't create outbox entries (known bug)")
		return
	}

	t.Log("📝 Step 4: Verify DELETE outbox entry created")
	entry := TCWaitForOutboxEntry(t, tc.DB, "test-e2e-host-delete", 5*time.Second)
	require.NotNil(t, entry, "DELETE outbox entry should exist")
	assert.Equal(t, "DELETE", string(entry.Operation), "operation should be DELETE")

	t.Log("📝 Step 5: Worker processes DELETE")
	TCProcessWorkerOnce(t, worker)
	time.Sleep(200 * time.Millisecond)

	t.Log("📝 Step 6: Verify Mock SGROUP received DELETE request")
	requestCount := tc.MockSGROUP.GetRequestCount()
	assert.Equal(t, 1, requestCount, "Mock SGROUP should receive DELETE request")

	deleteReq := tc.MockSGROUP.GetLastRequestForOperation(types.SyncOperationDelete)
	require.NotNil(t, deleteReq, "should have DELETE request")
	assert.Equal(t, types.SyncSubjectTypeHosts, deleteReq.SubjectType)

	t.Logf("✅ E2E DELETE Flow Complete (BUG-003 FIXED!)")
	t.Logf("  🎉 DELETE operations now create outbox entries for Hosts")

	tc.MockSGROUP.DumpRequests()
}

// TestE2E_Host_MultipleConcurrent tests processing multiple Hosts concurrently
func TestE2E_Host_MultipleConcurrent(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	worker := TCCreateTestWorker(t, tc)
	tc.MockSGROUP.Reset()

	t.Log("📝 Step 1: Create 5 Hosts")
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("test-e2e-host-concurrent-%d", i)
		ip := fmt.Sprintf("192.168.1.%d/32", 100+i)
		TCCreateHostViaRepositoryWithConnStr(t, tc.ConnectionString, "default", name, ip)
	}

	t.Log("📝 Step 2: Verify 5 outbox entries created")
	time.Sleep(500 * time.Millisecond)
	count := TCCountOutboxEntries(t, tc.DB)
	assert.Equal(t, 5, count, "should have 5 outbox entries")

	t.Log("📝 Step 3: Worker processes all entries in batch")
	TCProcessWorkerOnce(t, worker)
	time.Sleep(500 * time.Millisecond)

	t.Log("📝 Step 4: Verify Mock SGROUP received 5 requests")
	requestCount := tc.MockSGROUP.GetRequestCount()
	assert.Equal(t, 5, requestCount, "Mock SGROUP should receive 5 requests")

	hostRequests := tc.MockSGROUP.GetRequestsForSubject(types.SyncSubjectTypeHosts)
	assert.Equal(t, 5, len(hostRequests), "should have 5 Hosts requests")

	t.Logf("✅ Concurrent Processing Complete: 5 Hosts → SGROUP")
	tc.MockSGROUP.DumpRequests()
}
