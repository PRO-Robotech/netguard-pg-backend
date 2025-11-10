package helpers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func CreateTestHost(t *testing.T, db *sql.DB, name string, ipList []string, ready bool) uuid.UUID {
	ipListJSON, _ := json.Marshal(ipList)
	hostID := uuid.New()
	query := `
		INSERT INTO hosts (id, namespace, name, ip_list, is_bound, address_group_ref, ready, created_at)
		VALUES ($1, 'default', $2, $3, false, '', $4, NOW())
	`
	_, err := db.Exec(query, hostID, name, ipListJSON, ready)
	require.NoError(t, err, "Failed to create test host")
	createK8sMetadata(t, db, "Host", "default", name, ready)
	return hostID
}
func CreateTestAddressGroup(t *testing.T, db *sql.DB, name string, ready bool) uuid.UUID {
	agID := uuid.New()
	query := `
		INSERT INTO address_groups (id, namespace, name, aggregated_hosts, aggregated_networks, ready, created_at)
		VALUES ($1, 'default', $2, '[]'::jsonb, '[]'::jsonb, $3, NOW())
	`
	_, err := db.Exec(query, agID, name, ready)
	require.NoError(t, err, "Failed to create test address group")
	createK8sMetadata(t, db, "AddressGroup", "default", name, ready)
	return agID
}
func CreateTestNetwork(t *testing.T, db *sql.DB, name string, cidrs []string, ready bool) uuid.UUID {
	cidrsJSON, _ := json.Marshal(cidrs)
	networkID := uuid.New()
	query := `
		INSERT INTO networks (id, namespace, name, cidrs, is_bound, address_group_ref, ready, created_at)
		VALUES ($1, 'default', $2, $3, false, '', $4, NOW())
	`
	_, err := db.Exec(query, networkID, name, cidrsJSON, ready)
	require.NoError(t, err, "Failed to create test network")
	createK8sMetadata(t, db, "Network", "default", name, ready)
	return networkID
}
func CreateTestHostBinding(t *testing.T, db *sql.DB, name, hostRef, agRef string) uuid.UUID {
	bindingID := uuid.New()
	query := `
		INSERT INTO host_bindings (id, namespace, name, host_ref, address_group_ref, ready, created_at)
		VALUES ($1, 'default', $2, $3, $4, false, NOW())
	`
	_, err := db.Exec(query, bindingID, name, hostRef, agRef)
	require.NoError(t, err, "Failed to create test host binding")
	createK8sMetadataWithPendingSync(t, db, "HostBinding", "default", name)
	return bindingID
}
func CreateTestNetworkBinding(t *testing.T, db *sql.DB, name, networkRef, agRef string) uuid.UUID {
	bindingID := uuid.New()
	query := `
		INSERT INTO network_bindings (id, namespace, name, network_ref, address_group_ref, ready, created_at)
		VALUES ($1, 'default', $2, $3, $4, false, NOW())
	`
	_, err := db.Exec(query, bindingID, name, networkRef, agRef)
	require.NoError(t, err, "Failed to create test network binding")
	createK8sMetadataWithPendingSync(t, db, "NetworkBinding", "default", name)
	return bindingID
}
func WaitForCondition(t *testing.T, db *sql.DB, resourceType, resourceName, conditionType, expectedStatus string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var conditionsJSON []byte
		query := `
			SELECT conditions FROM k8s_metadata
			WHERE resource_type = $1 AND resource_namespace = 'default' AND resource_name = $2
		`
		err := db.QueryRow(query, resourceType, resourceName).Scan(&conditionsJSON)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var conditions []map[string]interface{}
		if err := json.Unmarshal(conditionsJSON, &conditions); err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, cond := range conditions {
			if cond["type"] == conditionType && cond["status"] == expectedStatus {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("Timeout waiting for %s.%s condition %s=%s", resourceType, resourceName, conditionType, expectedStatus)
}
func WaitForResourceReady(t *testing.T, db *sql.DB, resourceType, resourceName string, timeout time.Duration) {
	WaitForCondition(t, db, resourceType, resourceName, "Ready", "True", timeout)
}
func VerifyOutboxEmpty(t *testing.T, db *sql.DB) {
	var count int
	query := `SELECT COUNT(*) FROM sync_outbox WHERE status IN ('PENDING', 'PROCESSING')`
	err := db.QueryRow(query).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "Outbox should be empty (no PENDING or PROCESSING entries)")
}
func VerifyOutboxHasEntry(t *testing.T, db *sql.DB, resourceType, resourceName string) {
	var count int
	query := `
		SELECT COUNT(*) FROM sync_outbox
		WHERE resource_type = $1 AND payload->>'name' = $2 AND status = 'PENDING'
	`
	err := db.QueryRow(query, resourceType, resourceName).Scan(&count)
	require.NoError(t, err)
	require.Greater(t, count, 0, "Outbox should have entry for %s/%s", resourceType, resourceName)
}
func GetOutboxEntry(t *testing.T, db *sql.DB, resourceType, resourceName string) map[string]interface{} {
	query := `
		SELECT id, resource_type, operation, status, attempts, last_error, target_system
		FROM sync_outbox
		WHERE resource_type = $1 AND payload->>'name' = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	var id, resType, operation, status, targetSystem string
	var attempts int
	var lastError sql.NullString
	err := db.QueryRow(query, resourceType, resourceName).Scan(&id, &resType, &operation, &status, &attempts, &lastError, &targetSystem)
	require.NoError(t, err, "Failed to get outbox entry")
	return map[string]interface{}{
		"id":            id,
		"resource_type": resType,
		"operation":     operation,
		"status":        status,
		"attempts":      attempts,
		"last_error":    lastError.String,
		"target_system": targetSystem,
	}
}
func VerifyHostBound(t *testing.T, db *sql.DB, hostName, expectedAG string) {
	var isBound bool
	var agRef string
	query := `
		SELECT is_bound, address_group_ref FROM hosts
		WHERE namespace='default' AND name=$1
	`
	err := db.QueryRow(query, hostName).Scan(&isBound, &agRef)
	require.NoError(t, err)
	require.True(t, isBound, "Host %s should be bound", hostName)
	require.Equal(t, expectedAG, agRef, "Host should reference AG %s", expectedAG)
}
func VerifyNetworkBound(t *testing.T, db *sql.DB, networkName, expectedAG string) {
	var isBound bool
	var agRef string
	query := `
		SELECT is_bound, address_group_ref FROM networks
		WHERE namespace='default' AND name=$1
	`
	err := db.QueryRow(query, networkName).Scan(&isBound, &agRef)
	require.NoError(t, err)
	require.True(t, isBound, "Network %s should be bound", networkName)
	require.Equal(t, expectedAG, agRef, "Network should reference AG %s", expectedAG)
}
func VerifyAGContainsHost(t *testing.T, db *sql.DB, agName, hostName string) {
	var aggregatedHostsJSON []byte
	query := `
		SELECT aggregated_hosts FROM address_groups
		WHERE namespace='default' AND name=$1
	`
	err := db.QueryRow(query, agName).Scan(&aggregatedHostsJSON)
	require.NoError(t, err)
	var aggregatedHosts []map[string]interface{}
	err = json.Unmarshal(aggregatedHostsJSON, &aggregatedHosts)
	require.NoError(t, err)
	found := false
	for _, host := range aggregatedHosts {
		if hostRef, ok := host["hostRef"].(string); ok && hostRef == hostName {
			found = true
			break
		}
	}
	require.True(t, found, "AddressGroup %s should contain Host %s in aggregated_hosts", agName, hostName)
}
func VerifyAGContainsNetwork(t *testing.T, db *sql.DB, agName, networkName string) {
	var aggregatedNetworksJSON []byte
	query := `
		SELECT aggregated_networks FROM address_groups
		WHERE namespace='default' AND name=$1
	`
	err := db.QueryRow(query, agName).Scan(&aggregatedNetworksJSON)
	require.NoError(t, err)
	var aggregatedNetworks []map[string]interface{}
	err = json.Unmarshal(aggregatedNetworksJSON, &aggregatedNetworks)
	require.NoError(t, err)
	found := false
	for _, network := range aggregatedNetworks {
		if networkRef, ok := network["networkRef"].(string); ok && networkRef == networkName {
			found = true
			break
		}
	}
	require.True(t, found, "AddressGroup %s should contain Network %s in aggregated_networks", agName, networkName)
}
func createK8sMetadata(t *testing.T, db *sql.DB, resourceType, namespace, name string, ready bool) {
	status := "True"
	reason := "Synced"
	if !ready {
		status = "False"
		reason = "Pending"
	}
	conditions := json.RawMessage(fmt.Sprintf(`[{"type":"Ready","status":"%s","reason":"%s","lastTransitionTime":"%s"}]`,
		status, reason, time.Now().Format(time.RFC3339)))
	query := `
		INSERT INTO k8s_metadata (resource_type, resource_namespace, resource_name, conditions, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (resource_type, resource_namespace, resource_name)
		DO UPDATE SET conditions = $4, updated_at = NOW()
	`
	_, err := db.Exec(query, resourceType, namespace, name, conditions)
	require.NoError(t, err)
}
func createK8sMetadataWithPendingSync(t *testing.T, db *sql.DB, resourceType, namespace, name string) {
	conditions := json.RawMessage(fmt.Sprintf(`[
		{"type":"Ready","status":"False","reason":"Pending","lastTransitionTime":"%s"},
		{"type":"PendingSync","status":"True","reason":"InitialSync","lastTransitionTime":"%s"}
	]`, time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339)))
	query := `
		INSERT INTO k8s_metadata (resource_type, resource_namespace, resource_name, conditions, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (resource_type, resource_namespace, resource_name)
		DO UPDATE SET conditions = $4, updated_at = NOW()
	`
	_, err := db.Exec(query, resourceType, namespace, name, conditions)
	require.NoError(t, err)
}
