package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNetwork_AutoDelete reproduces the critical bug where Networks are automatically
// deleted after creation without any explicit DELETE operation.
//
// Bug Report (from user):
// "Сущности Host и Network после создания удаляются из БД! при отсутствии связи с СГРУП
// АГ все равно переходит в статус Ready TRUE!"
//
// This test creates ONLY a Network (no NetworkBinding, no AddressGroup references)
// and checks if it gets auto-deleted after 15 seconds.
func TestNetwork_AutoDelete(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	namespace := "default"
	name := "test-network-standalone"
	cidr := "192.168.100.0/24"

	t.Logf("📝 Step 1: Creating standalone Network: %s/%s (CIDR: %s) - NO bindings, NO references",
		namespace, name, cidr)

	// Create Network using Repository Writer (goes through Writers → outbox creation)
	_ = TCCreateNetworkViaRepositoryWithConnStr(t, tc.ConnectionString, namespace, name, cidr)

	t.Logf("✅ Network created successfully: %s/%s", namespace, name)

	// Step 2: Verify Network exists immediately after creation
	var existsCount int
	query := `SELECT COUNT(*) FROM networks WHERE namespace = $1 AND name = $2`
	err := tc.DB.QueryRow(query, namespace, name).Scan(&existsCount)
	require.NoError(t, err)
	require.Equal(t, 1, existsCount, "Network should exist immediately after creation")
	t.Logf("✅ Verified: Network exists in DB immediately after creation")

	// Step 3: Check initial outbox entries (should be CREATE, not DELETE)
	var createEntryCount, deleteEntryCount int
	query = `SELECT COUNT(*) FROM sync_outbox WHERE resource_type = 'Network' AND resource_name = $1 AND operation = 'CREATE'`
	err = tc.DB.QueryRow(query, name).Scan(&createEntryCount)
	require.NoError(t, err)
	t.Logf("📦 Initial outbox state: %d CREATE entries", createEntryCount)

	query = `SELECT COUNT(*) FROM sync_outbox WHERE resource_type = 'Network' AND resource_name = $1 AND operation = 'DELETE'`
	err = tc.DB.QueryRow(query, name).Scan(&deleteEntryCount)
	require.NoError(t, err)

	if deleteEntryCount > 0 {
		t.Errorf("❌ UNEXPECTED: DELETE outbox entry exists immediately after creation!")
	}

	// Step 4: Wait 15 seconds to observe if auto-deletion occurs
	t.Log("⏳ Waiting 15 seconds to observe potential auto-deletion...")
	time.Sleep(15 * time.Second)

	// Step 5: Check if Network still exists in DB
	query = `SELECT COUNT(*) FROM networks WHERE namespace = $1 AND name = $2`
	err = tc.DB.QueryRow(query, namespace, name).Scan(&existsCount)
	require.NoError(t, err)

	// THIS IS THE BUG CHECK
	if existsCount == 0 {
		t.Errorf("❌ BUG REPRODUCED: Network was AUTO-DELETED after creation!")
		t.Logf("Network %s/%s no longer exists in database after 15 seconds", namespace, name)

		// Check outbox for DELETE entry
		var finalDeleteCount int
		query = `SELECT COUNT(*) FROM sync_outbox WHERE resource_type = 'Network' AND resource_name = $1 AND operation = 'DELETE'`
		err = tc.DB.QueryRow(query, name).Scan(&finalDeleteCount)
		require.NoError(t, err)

		if finalDeleteCount > 0 {
			t.Errorf("❌ FOUND DELETE OUTBOX ENTRY: %d DELETE entries created", finalDeleteCount)

			// Get DELETE entry details
			var payload string
			var createdAt time.Time
			query2 := `SELECT payload, created_at FROM sync_outbox WHERE resource_type = 'Network' AND resource_name = $1 AND operation = 'DELETE' LIMIT 1`
			err = tc.DB.QueryRow(query2, name).Scan(&payload, &createdAt)
			if err == nil {
				t.Logf("DELETE outbox created at: %s", createdAt)
				t.Logf("DELETE outbox payload: %s", payload)
			}
		}

		t.FailNow()
	} else {
		t.Logf("✅ SUCCESS: Network still exists after 15 seconds: %s/%s", namespace, name)
	}

	// Step 6: Final outbox check
	query = `SELECT COUNT(*) FROM sync_outbox WHERE resource_type = 'Network' AND resource_name = $1 AND operation = 'DELETE'`
	err = tc.DB.QueryRow(query, name).Scan(&deleteEntryCount)
	require.NoError(t, err)

	if deleteEntryCount > 0 {
		t.Errorf("❌ UNEXPECTED: DELETE outbox entries found: %d", deleteEntryCount)
	} else {
		t.Logf("✅ No DELETE outbox entries found")
	}
}
