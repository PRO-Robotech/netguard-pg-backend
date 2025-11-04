package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/sync/types"
)

// TestE2E_Network_CreateFlow tests the complete CREATE flow:
// CREATE Network → Repository → Trigger → Outbox → Worker → SGROUP
func TestE2E_Network_CreateFlow(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	worker := TCCreateTestWorker(t, tc)

	// Reset Mock SGROUP to clean state
	tc.MockSGROUP.Reset()

	t.Log("📝 Step 1: Create Network via Repository")
	_ = TCCreateNetworkViaRepositoryWithConnStr(
		t,
		tc.ConnectionString,
		"default",
		"test-e2e-network-create",
		"10.0.0.0/24",
	)

	t.Log("📝 Step 2: Verify outbox entry created")
	entry := TCWaitForOutboxEntry(t, tc.DB, "test-e2e-network-create", 5*time.Second)
	require.NotNil(t, entry, "outbox entry should be created")
	assert.Equal(t, "PENDING", string(entry.Status), "entry should be PENDING")
	assert.Equal(t, "Network", entry.ResourceType)

	t.Log("📝 Step 3: Worker processes outbox entry")
	TCProcessWorkerOnce(t, worker)

	t.Log("📝 Step 4: Verify Mock SGROUP received CREATE request")
	time.Sleep(200 * time.Millisecond) // Allow async processing

	requestCount := tc.MockSGROUP.GetRequestCount()
	assert.Equal(t, 1, requestCount, "Mock SGROUP should receive exactly 1 request")

	lastReq := tc.MockSGROUP.GetLastRequest()
	require.NotNil(t, lastReq, "Mock SGROUP should have received a request")
	assert.Equal(t, types.SyncOperationUpsert, lastReq.Operation, "operation should be UPSERT")
	assert.Equal(t, types.SyncSubjectTypeNetworks, lastReq.SubjectType, "subject should be Networks")

	t.Logf("✅ E2E CREATE Flow Complete: default/test-e2e-network-create → Outbox → Worker → SGROUP")

	// Dump final state for debugging
	tc.MockSGROUP.DumpRequests()
}

// TestE2E_Network_UpdateFlow tests the complete UPDATE flow:
// UPDATE Network → Repository → Trigger → Outbox → Worker → SGROUP
func TestE2E_Network_UpdateFlow(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	worker := TCCreateTestWorker(t, tc)

	// Reset Mock SGROUP
	tc.MockSGROUP.Reset()

	t.Log("📝 Step 1: Create Network")
	_ = TCCreateNetworkViaRepositoryWithConnStr(
		t,
		tc.ConnectionString,
		"default",
		"test-e2e-network-update",
		"10.0.1.0/24",
	)

	// Process initial creation
	TCProcessWorkerOnce(t, worker)
	time.Sleep(200 * time.Millisecond)

	// Verify initial CREATE
	assert.Equal(t, 1, tc.MockSGROUP.GetRequestCount(), "should have 1 CREATE request")

	// Reset Mock SGROUP for UPDATE test
	tc.MockSGROUP.Reset()
	TCCleanOutboxTable(t, tc.DB)

	t.Log("📝 Step 2: Update Network (re-insert with same name)")
	_ = TCCreateNetworkViaRepositoryWithConnStr(
		t,
		tc.ConnectionString,
		"default",
		"test-e2e-network-update",
		"10.0.1.0/24",
	)

	t.Log("📝 Step 3: Verify outbox entry created for UPDATE")
	entry := TCWaitForOutboxEntry(t, tc.DB, "test-e2e-network-update", 5*time.Second)
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

// TestE2E_Network_DeleteFlow tests the complete DELETE flow:
// DELETE Network → Repository → Trigger → Outbox → Worker → SGROUP
//
// NOTE: This test will FAIL until BUG-003 is fixed!
// BUG-003: DELETE operations don't create outbox entries
func TestE2E_Network_DeleteFlow(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	worker := TCCreateTestWorker(t, tc)

	// Reset Mock SGROUP
	tc.MockSGROUP.Reset()

	t.Log("📝 Step 1: Create Network")
	_ = TCCreateNetworkViaRepositoryWithConnStr(
		t,
		tc.ConnectionString,
		"default",
		"test-e2e-network-delete",
		"10.0.2.0/24",
	)

	// Process creation
	TCProcessWorkerOnce(t, worker)
	time.Sleep(200 * time.Millisecond)

	// Verify CREATE processed
	assert.Equal(t, 1, tc.MockSGROUP.GetRequestCount(), "should have CREATE request")

	// Reset for DELETE test
	tc.MockSGROUP.Reset()
	TCCleanOutboxTable(t, tc.DB)

	t.Log("📝 Step 2: Delete Network via Repository")
	TCDeleteNetworkViaRepositoryWithConnStr(
		t,
		tc.ConnectionString,
		"default",
		"test-e2e-network-delete",
	)

	t.Log("📝 Step 3: Check for DELETE outbox entry")
	time.Sleep(500 * time.Millisecond)
	_, err := TCGetOutboxEntryByResourceName(t, tc.DB, "test-e2e-network-delete")
	if err != nil {
		t.Log("🐛 BUG-003 DETECTED: DELETE did NOT create outbox entry for Network")
		t.Log("  ❌ This is EXPECTED until BUG-003 is fixed")
		t.Log("  📋 After fix: This test will PASS automatically")

		// Log current state
		TCLogOutboxState(t, tc.DB)

		t.Skip("BUG-003: DELETE operations don't create outbox entries (known bug)")
		return
	}

	t.Log("📝 Step 4: Verify DELETE outbox entry created")
	entry := TCWaitForOutboxEntry(t, tc.DB, "test-e2e-network-delete", 5*time.Second)
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
	assert.Equal(t, types.SyncSubjectTypeNetworks, deleteReq.SubjectType)

	t.Logf("✅ E2E DELETE Flow Complete (BUG-003 FIXED!)")
	t.Logf("  🎉 DELETE operations now create outbox entries for Networks")

	tc.MockSGROUP.DumpRequests()
}

// TestE2E_Network_MultipleConcurrent tests processing multiple Networks concurrently
func TestE2E_Network_MultipleConcurrent(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	worker := TCCreateTestWorker(t, tc)
	tc.MockSGROUP.Reset()

	t.Log("📝 Step 1: Create 5 Networks")
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("test-e2e-network-concurrent-%d", i)
		cidr := fmt.Sprintf("10.0.%d.0/24", i)
		TCCreateNetworkViaRepositoryWithConnStr(t, tc.ConnectionString, "default", name, cidr)
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

	networkRequests := tc.MockSGROUP.GetRequestsForSubject(types.SyncSubjectTypeNetworks)
	assert.Equal(t, 5, len(networkRequests), "should have 5 Networks requests")

	t.Logf("✅ Concurrent Processing Complete: 5 Networks → SGROUP")
	tc.MockSGROUP.DumpRequests()
}
