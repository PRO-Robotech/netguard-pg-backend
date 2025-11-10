package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/sync/types"
)

// TestServiceUpdate_AddressGroupBinding validates that Service update
// is triggered when an AddressGroup is bound to Service
func TestServiceUpdate_AddressGroupBinding(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create AddressGroup
	agResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	agNamespace := "test-ns"
	agName := "test-ag"
	agKey := CreateTestAddressGroup(t, env.DB, agNamespace, agName, agResourceVersion)

	// Create Service
	serviceResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	serviceNamespace := "test-ns"
	serviceName := "test-service"

	var serviceID uuid.UUID
	err := env.DB.QueryRow(`
		INSERT INTO services (
			namespace, name, resource_version
		) VALUES ($1, $2, $3)
		RETURNING id
	`, serviceNamespace, serviceName, serviceResourceVersion).Scan(&serviceID)
	require.NoError(t, err)

	t.Logf("Created Service %s and AddressGroup %s/%s", serviceID, agKey.Namespace, agKey.Name)

	// ========================================
	// ACT
	// ========================================
	// Create service_spec binding (binds AddressGroup to Service)
	_, err = env.DB.Exec(`
		INSERT INTO service_specs (
			service_id, address_group_namespace, address_group_name, action
		) VALUES ($1, $2, $3, $4)
	`, serviceID, agNamespace, agName, "PERMIT")
	require.NoError(t, err)

	t.Logf("Created service_spec binding: service_id=%s → AG %s/%s", serviceID, agNamespace, agName)

	// ========================================
	// ASSERT
	// ========================================
	// Outbox entry should be created for Service UPDATE
	entry := WaitForOutboxEntry(t, env.DB, "Service", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry, "Outbox entry should be created for Service on AG binding")

	assert.Equal(t, "Service", entry.ResourceType)
	assert.Equal(t, domain.SyncOperationUpdate, entry.Operation)
	assert.Equal(t, domain.OutboxStatusPending, entry.Status)

	t.Logf("✅ Test PASSED: AddressGroup binding triggers Service outbox entry")
}

// TestServiceUpdate_MultipleAddressGroups validates that Service with multiple
// AddressGroup bindings creates appropriate outbox entries
func TestServiceUpdate_MultipleAddressGroups(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create Service
	serviceResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	var serviceID uuid.UUID
	err := env.DB.QueryRow(`
		INSERT INTO services (
			namespace, name, resource_version
		) VALUES ($1, $2, $3)
		RETURNING id
	`, "test-ns", "test-service-multi", serviceResourceVersion).Scan(&serviceID)
	require.NoError(t, err)

	// Create multiple AddressGroups
	agKeys := make([]AddressGroupKey, 3)
	for i := 0; i < 3; i++ {
		agResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
		agKey := CreateTestAddressGroup(t, env.DB, "test-ns",
			"ag-"+string(rune('a'+i)),
			agResourceVersion)
		agKeys[i] = agKey
	}

	t.Logf("Created Service %s and 3 AddressGroups", serviceID)

	// ========================================
	// ACT
	// ========================================
	// Bind all AddressGroups to Service
	for i, agKey := range agKeys {
		_, err = env.DB.Exec(`
			INSERT INTO service_specs (
				service_id, address_group_namespace, address_group_name, action
			) VALUES ($1, $2, $3, $4)
		`, serviceID, agKey.Namespace, agKey.Name, "PERMIT")
		require.NoError(t, err)

		t.Logf("Bound AG %d: %s/%s", i+1, agKey.Namespace, agKey.Name)
	}

	// ========================================
	// ASSERT
	// ========================================
	// Multiple outbox entries should be created (one per binding)
	time.Sleep(300 * time.Millisecond)
	count := GetOutboxEntryCount(t, env.DB, "Service", string(domain.SyncOperationUpdate))

	assert.GreaterOrEqual(t, count, 3, "At least 3 outbox entries should be created for 3 AG bindings")

	t.Logf("✅ Test PASSED: Multiple AG bindings create outbox entries (count=%d)", count)
}

// TestServiceUpdate_AddressGroupRemoval validates that removing an AddressGroup
// from a Service triggers outbox entry
func TestServiceUpdate_AddressGroupRemoval(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create AddressGroup
	agResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	agNamespace := "test-ns"
	agName := "test-ag-removal"
	agKey := CreateTestAddressGroup(t, env.DB, agNamespace, agName, agResourceVersion)

	// Create Service
	serviceResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	var serviceID uuid.UUID
	err := env.DB.QueryRow(`
		INSERT INTO services (
			namespace, name, resource_version
		) VALUES ($1, $2, $3)
		RETURNING id
	`, "test-ns", "test-service-removal", serviceResourceVersion).Scan(&serviceID)
	require.NoError(t, err)

	// Create service_spec binding
	var specID uuid.UUID
	err = env.DB.QueryRow(`
		INSERT INTO service_specs (
			service_id, address_group_namespace, address_group_name, action
		) VALUES ($1, $2, $3, $4)
		RETURNING id
	`, serviceID, agNamespace, agName, "PERMIT").Scan(&specID)
	require.NoError(t, err)

	// Wait for initial outbox entry
	WaitForOutboxEntry(t, env.DB, "Service", string(domain.SyncOperationUpdate), 5*time.Second)
	time.Sleep(200 * time.Millisecond)

	countBefore := GetOutboxEntryCount(t, env.DB, "Service", string(domain.SyncOperationUpdate))
	t.Logf("Initial state: AG bound to Service, outbox entries=%d, ag=%s/%s", countBefore, agKey.Namespace, agKey.Name)

	// ========================================
	// ACT
	// ========================================
	// Remove service_spec binding
	_, err = env.DB.Exec(`
		DELETE FROM service_specs
		WHERE id = $1
	`, specID)
	require.NoError(t, err)

	t.Logf("Deleted service_spec binding")

	// ========================================
	// ASSERT
	// ========================================
	// New outbox entry should be created for AG removal
	time.Sleep(300 * time.Millisecond)
	countAfter := GetOutboxEntryCount(t, env.DB, "Service", string(domain.SyncOperationUpdate))

	assert.Greater(t, countAfter, countBefore, "Outbox entry should be created on AG removal from Service")

	t.Logf("✅ Test PASSED: AG removal triggers Service outbox entry (before=%d, after=%d)", countBefore, countAfter)
}

// TestServiceUpdate_CreatesOutboxEntry validates that direct Service update
// triggers outbox entry creation
func TestServiceUpdate_CreatesOutboxEntry(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create Service
	serviceResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	var serviceID uuid.UUID
	err := env.DB.QueryRow(`
		INSERT INTO services (
			namespace, name, resource_version
		) VALUES ($1, $2, $3)
		RETURNING id
	`, "test-ns", "test-service-direct", serviceResourceVersion).Scan(&serviceID)
	require.NoError(t, err)

	t.Logf("Created Service %s", serviceID)

	// Wait for initial state
	time.Sleep(100 * time.Millisecond)

	// ========================================
	// ACT
	// ========================================
	// Update Service (change resource_version or other field)
	// Triggers should create outbox entry
	newResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	_, err = env.DB.Exec(`
		UPDATE services
		SET resource_version = $1
		WHERE id = $2
	`, newResourceVersion, serviceID)
	require.NoError(t, err)

	t.Logf("Updated Service resource_version")

	// ========================================
	// ASSERT
	// ========================================
	// Outbox entry should be created
	entry := WaitForOutboxEntry(t, env.DB, "Service", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	assert.Equal(t, "Service", entry.ResourceType)
	assert.Equal(t, domain.SyncOperationUpdate, entry.Operation)

	t.Logf("✅ Test PASSED: Direct Service update creates outbox entry")
}

// TestServiceUpdate_WaitsForAGsReady validates that Service sync waits for
// all referenced AddressGroups to be ready (dependency checking)
// NOTE: This is tested during worker processing, not during outbox creation
func TestServiceUpdate_WaitsForAGsReady(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create AddressGroup (not ready - no hosts)
	agResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	agNamespace := "test-ns"
	agName := "test-ag-not-ready"
	agKey := CreateTestAddressGroup(t, env.DB, agNamespace, agName, agResourceVersion)

	// Create Service
	serviceResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	var serviceID uuid.UUID
	err := env.DB.QueryRow(`
		INSERT INTO services (
			namespace, name, resource_version
		) VALUES ($1, $2, $3)
		RETURNING id
	`, "test-ns", "test-service-wait-ag", serviceResourceVersion).Scan(&serviceID)
	require.NoError(t, err)

	t.Logf("Created Service %s and AddressGroup %s/%s (not ready)", serviceID, agKey.Namespace, agKey.Name)

	// ========================================
	// ACT
	// ========================================
	// Bind AddressGroup to Service
	_, err = env.DB.Exec(`
		INSERT INTO service_specs (
			service_id, address_group_namespace, address_group_name, action
		) VALUES ($1, $2, $3, $4)
	`, serviceID, agNamespace, agName, "PERMIT")
	require.NoError(t, err)

	// ========================================
	// ASSERT
	// ========================================
	// Outbox entry should still be created (dependency check happens during worker processing)
	entry := WaitForOutboxEntry(t, env.DB, "Service", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	assert.Equal(t, "Service", entry.ResourceType)
	assert.Equal(t, domain.OutboxStatusPending, entry.Status)

	// The worker will check dependencies before syncing to SGROUP
	// If AG is not ready (has no hosts), worker should skip or wait

	t.Logf("✅ Test PASSED: Service outbox entry created (dependency check happens during worker processing)")
}

// TestServiceDelete_CascadesSvcSvcRule verifies that deleting a Service cascades to dependent SvcSvcRule resources
// ensuring database cleanup and SGROUP DELETE operations occur.
func TestServiceDelete_CascadesSvcSvcRule(t *testing.T) {
	const (
		namespace   = "test-ns"
		serviceFrom = "svc-cascade-from"
		serviceTo   = "svc-cascade-to"
		ruleName    = "svc-cascade-rule"
	)

	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	worker := TCCreateTestWorker(t, tc)

	// Create metadata for services and rule
	serviceFromRV := CreateK8sMetadata(t, tc.DB, `[]`)
	serviceToRV := CreateK8sMetadata(t, tc.DB, `[]`)
	ruleRV := CreateK8sMetadata(t, tc.DB, `[]`)

	// Insert services
	var serviceFromID, serviceToID uuid.UUID
	err := tc.DB.QueryRow(`
		INSERT INTO services (namespace, name, resource_version)
		VALUES ($1, $2, $3)
		RETURNING id
	`, namespace, serviceFrom, serviceFromRV).Scan(&serviceFromID)
	require.NoError(t, err)

	err = tc.DB.QueryRow(`
		INSERT INTO services (namespace, name, resource_version)
		VALUES ($1, $2, $3)
		RETURNING id
	`, namespace, serviceTo, serviceToRV).Scan(&serviceToID)
	require.NoError(t, err)

	serviceFromRef := fmt.Sprintf(`{"apiVersion":"netguard.sgroups.io/v1beta1","kind":"Service","name":"%s","namespace":"%s"}`, serviceFrom, namespace)
	serviceToRef := fmt.Sprintf(`{"apiVersion":"netguard.sgroups.io/v1beta1","kind":"Service","name":"%s","namespace":"%s"}`, serviceTo, namespace)

	_, err = tc.DB.Exec(`
		INSERT INTO svc_svc_rules (namespace, name, service_from_ref, service_to_ref, action, priority, logs, trace, resource_version)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, 'ACCEPT', 100, false, false, $5)
	`, namespace, ruleName, serviceFromRef, serviceToRef, ruleRV)
	require.NoError(t, err)

	// Soft-delete the source service to trigger cascade
	_, err = tc.DB.Exec(`DELETE FROM services WHERE namespace = $1 AND name = $2`, namespace, serviceFrom)
	require.NoError(t, err)

	// Process worker until outbox drains (handles SvcSvcRule then Service deletion)
	PollUntil(t, func() bool {
		if procErr := worker.ProcessOnce(context.Background()); procErr != nil {
			t.Logf("worker ProcessOnce error: %v", procErr)
		}
		var pending int
		err := tc.DB.QueryRow(`SELECT COUNT(*) FROM sync_outbox WHERE status != 'SUCCESS'`).Scan(&pending)
		require.NoError(t, err)
		return pending == 0
	}, 10*time.Second, 200*time.Millisecond, "sync_outbox drained after Service delete cascade")

	// One more pass ensures final cleanup
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Logf("worker ProcessOnce final pass error: %v", err)
	}

	// Assert SvcSvcRule removed from database
	var count int
	err = tc.DB.QueryRow(`SELECT COUNT(*) FROM svc_svc_rules WHERE namespace = $1 AND name = $2`, namespace, ruleName).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "SvcSvcRule should be deleted after Service cascade")

	// Assert junction table cleanup
	serviceFromKey := fmt.Sprintf("%s/%s", namespace, serviceFrom)
	err = tc.DB.QueryRow(`SELECT COUNT(*) FROM service_rule_refs WHERE service_ref = $1`, serviceFromKey).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "service_rule_refs should not retain entries for deleted Service")

	// Assert source Service fully removed
	err = tc.DB.QueryRow(`SELECT COUNT(*) FROM services WHERE namespace = $1 AND name = $2`, namespace, serviceFrom).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "Service should be physically removed after cascade")

	// Assert target Service still exists with cleared xSvcSvcRules arrays
	var fromJSON, toJSON string
	err = tc.DB.QueryRow(`
		SELECT xsvcsvc_rules_as_from::text, xsvcsvc_rules_as_to::text
		FROM services
		WHERE namespace = $1 AND name = $2
	`, namespace, serviceTo).Scan(&fromJSON, &toJSON)
	require.NoError(t, err)
	assert.Equal(t, "[]", fromJSON)
	assert.Equal(t, "[]", toJSON)

	// Verify SGROUP received DELETE operations for rule and service
	require.NoError(t, tc.MockSGROUP.AssertRequestReceived(types.SyncOperationDelete, types.SyncSubjectTypeSvcSvcRules))
	require.NoError(t, tc.MockSGROUP.AssertRequestReceived(types.SyncOperationDelete, types.SyncSubjectTypeServices))

	t.Logf("✅ Test PASSED: Service deletion cascades to SvcSvcRule and cleans up dependencies")
}
