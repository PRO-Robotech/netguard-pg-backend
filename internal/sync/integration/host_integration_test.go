package integration

import (
	"testing"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain"
)

// TestHostReadyTransition_CreatesOutboxEntry validates that Host ready transition
// triggers Migration 029 → Migration 026 → sync_outbox entry creation
func TestHostReadyTransition_CreatesOutboxEntry(t *testing.T) {
	// ARRANGE: Setup test database
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create k8s_metadata with conditions empty (ready=false)
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)

	// Create Host with ready=FALSE
	namespace := "test-ns"
	name := "host-1"
	hostUUID := "550e8400-e29b-41d4-a716-446655440001"

	hostID := CreateTestHost(t, env.DB, namespace, name, hostUUID, resourceVersion, false)

	t.Logf("Test setup complete: Host %s/%s (id=%s, ready=false)", namespace, name, hostID)

	// ACT: Trigger ready transition (FALSE → TRUE)
	// This updates k8s_metadata.conditions → triggers Migration 029
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	// ASSERT Step 1: Migration 029 should update hosts.ready = TRUE
	WaitForHostReady(t, env.DB, namespace, name, true, 5*time.Second)
	AssertHostReady(t, env.DB, namespace, name, true)

	// ASSERT Step 2: Migration 026 should create Outbox entry for AddressGroup
	// When host becomes ready, it triggers AG aggregation update
	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)

	require.NotNil(t, entry, "Outbox entry should be created for AddressGroup")
	assert.Equal(t, "AddressGroup", entry.ResourceType)
	assert.Equal(t, domain.SyncOperationUpdate, entry.Operation)
	assert.Equal(t, domain.TargetSystemSGROUP, entry.TargetSystem)
	assert.Equal(t, domain.OutboxStatusPending, entry.Status)
	assert.Equal(t, 0, entry.Attempts)

	t.Logf("✅ Test PASSED: Host ready transition → Outbox entry created")
}

// TestHostReadyTransition_NoChangeWhenAlreadyReady validates that
// updating conditions when host is already ready does NOT create duplicate entries
func TestHostReadyTransition_NoChangeWhenAlreadyReady(t *testing.T) {
	// ARRANGE: Setup test database
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create k8s_metadata with Ready=True
	resourceVersion := CreateK8sMetadata(t, env.DB, `[{"type":"Ready","status":"True"}]`)

	// Create Host with ready=TRUE (already ready)
	namespace := "test-ns"
	name := "host-2"
	hostUUID := "550e8400-e29b-41d4-a716-446655440002"

	hostID := CreateTestHost(t, env.DB, namespace, name, hostUUID, resourceVersion, true)

	t.Logf("Test setup complete: Host %s/%s (id=%s, ready=true)", namespace, name, hostID)

	// Wait a bit to ensure initial state is stable
	time.Sleep(100 * time.Millisecond)

	// Count outbox entries before
	countBefore := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))
	t.Logf("Outbox entries before: %d", countBefore)

	// ACT: Update conditions again (ready=true → ready=true, no change)
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	// Wait a bit for any potential triggers
	time.Sleep(200 * time.Millisecond)

	// ASSERT: No new outbox entries should be created
	countAfter := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))
	t.Logf("Outbox entries after: %d", countAfter)

	assert.Equal(t, countBefore, countAfter, "No new outbox entries should be created when ready status unchanged")

	t.Logf("✅ Test PASSED: No duplicate entries when already ready")
}

// TestHostReadyTransition_FalseToTrue validates explicit FALSE → TRUE transition
func TestHostReadyTransition_FalseToTrue(t *testing.T) {
	// ARRANGE
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create k8s_metadata with conditions empty (ready=false)
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)

	// Create Host with ready=FALSE
	namespace := "test-ns"
	name := "host-3"
	hostUUID := "550e8400-e29b-41d4-a716-446655440003"

	CreateTestHost(t, env.DB, namespace, name, hostUUID, resourceVersion, false)

	// Verify initial state
	AssertHostReady(t, env.DB, namespace, name, false)

	// ACT: Transition FALSE → TRUE
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	// ASSERT: Host ready should be TRUE
	WaitForHostReady(t, env.DB, namespace, name, true, 5*time.Second)

	// ASSERT: Outbox entry should be created
	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	t.Logf("✅ Test PASSED: FALSE → TRUE transition creates outbox entry")
}

// TestHostReadyTransition_TrueToFalse validates TRUE → FALSE transition
// (Negative case: should update ready but not create outbox, as trigger only fires on FALSE→TRUE)
func TestHostReadyTransition_TrueToFalse(t *testing.T) {
	// ARRANGE
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create k8s_metadata with Ready=True
	resourceVersion := CreateK8sMetadata(t, env.DB, `[{"type":"Ready","status":"True"}]`)

	// Create Host with ready=TRUE
	namespace := "test-ns"
	name := "host-4"
	hostUUID := "550e8400-e29b-41d4-a716-446655440004"

	CreateTestHost(t, env.DB, namespace, name, hostUUID, resourceVersion, true)

	// Verify initial state
	AssertHostReady(t, env.DB, namespace, name, true)

	// Count outbox entries before
	countBefore := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))

	// ACT: Transition TRUE → FALSE
	TriggerReadyTransition(t, env.DB, resourceVersion, false)

	// ASSERT: Host ready should be FALSE
	WaitForHostReady(t, env.DB, namespace, name, false, 5*time.Second)

	// ASSERT: No new outbox entry (Migration 026 trigger only fires on FALSE→TRUE)
	time.Sleep(200 * time.Millisecond)
	countAfter := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))

	assert.Equal(t, countBefore, countAfter, "No outbox entry on TRUE→FALSE (trigger only fires on FALSE→TRUE)")

	t.Logf("✅ Test PASSED: TRUE → FALSE does not create outbox entry")
}

// TestMultipleHosts_ReadyTransition validates that multiple hosts
// can transition ready status independently
func TestMultipleHosts_ReadyTransition(t *testing.T) {
	// ARRANGE
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create 3 hosts with different ready states
	hosts := []struct {
		namespace string
		name      string
		uuid      string
		hostname  string
		ready     bool
	}{
		{"ns1", "host-a", "550e8400-e29b-41d4-a716-446655440010", "hosta.com", false},
		{"ns1", "host-b", "550e8400-e29b-41d4-a716-446655440011", "hostb.com", false},
		{"ns2", "host-c", "550e8400-e29b-41d4-a716-446655440012", "hostc.com", false},
	}

	resourceVersions := make([]int64, len(hosts))

	for i, h := range hosts {
		rv := CreateK8sMetadata(t, env.DB, `[]`)
		resourceVersions[i] = rv
		CreateTestHost(t, env.DB, h.namespace, h.name, h.uuid, rv, h.ready)
	}

	t.Logf("Created %d test hosts", len(hosts))

	// ACT: Trigger ready transition for host-a and host-c (skip host-b)
	TriggerReadyTransition(t, env.DB, resourceVersions[0], true) // host-a
	TriggerReadyTransition(t, env.DB, resourceVersions[2], true) // host-c

	// ASSERT: host-a should be ready
	WaitForHostReady(t, env.DB, hosts[0].namespace, hosts[0].name, true, 5*time.Second)

	// ASSERT: host-b should still be not ready
	AssertHostReady(t, env.DB, hosts[1].namespace, hosts[1].name, false)

	// ASSERT: host-c should be ready
	WaitForHostReady(t, env.DB, hosts[2].namespace, hosts[2].name, true, 5*time.Second)

	t.Logf("✅ Test PASSED: Multiple hosts transition independently")
}

// TestHostReadyTransition_WithInvalidConditions validates behavior
// with malformed conditions JSON
func TestHostReadyTransition_WithInvalidConditions(t *testing.T) {
	// ARRANGE
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create k8s_metadata with conditions that don't contain Ready=True
	resourceVersion := CreateK8sMetadata(t, env.DB, `[{"type":"OtherCondition","status":"True"}]`)

	// Create Host with ready=FALSE
	namespace := "test-ns"
	name := "host-5"
	hostUUID := "550e8400-e29b-41d4-a716-446655440005"

	CreateTestHost(t, env.DB, namespace, name, hostUUID, resourceVersion, false)

	// Verify initial state
	AssertHostReady(t, env.DB, namespace, name, false)

	// Count outbox entries before
	countBefore := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))

	// ACT: Update conditions (still no Ready=True)
	// This should NOT change ready status (should remain FALSE)
	_, err := env.DB.Exec(`
		UPDATE k8s_metadata
		SET conditions = $1::jsonb
		WHERE resource_version = $2
	`, `[{"type":"AnotherCondition","status":"True"}]`, resourceVersion)
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// ASSERT: Host ready should still be FALSE
	AssertHostReady(t, env.DB, namespace, name, false)

	// ASSERT: No outbox entry created
	countAfter := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))
	assert.Equal(t, countBefore, countAfter, "No outbox entry when Ready=True not present")

	t.Logf("✅ Test PASSED: Invalid conditions do not trigger ready transition")
}
