package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSmoke_Infrastructure verifies basic integration test infrastructure works
//
// This test validates:
// 1. PostgreSQL container starts successfully
// 2. Database connection works
// 3. Migrations 025-030 are applied correctly
// 4. sync_outbox table exists with correct schema
// 5. Mock SGROUP server responds
//
// This is a smoke test - if this fails, all other integration tests will fail.
func TestSmoke_Infrastructure(t *testing.T) {
	// Setup embedded test environment
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	t.Run("PostgreSQL connection", func(t *testing.T) {
		var version string
		err := tc.DB.QueryRow("SELECT version()").Scan(&version)
		require.NoError(t, err, "should be able to query PostgreSQL version")
		assert.Contains(t, version, "PostgreSQL", "version should contain 'PostgreSQL'")
		t.Logf("  ℹ️  PostgreSQL version: %s", version)
	})

	t.Run("Migrations applied - sync_outbox table exists", func(t *testing.T) {
		var exists bool
		err := tc.DB.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_name = 'sync_outbox'
			)
		`).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "sync_outbox table should exist (Migration 025)")
	})

	t.Run("Migration 030 - resource_namespace column exists", func(t *testing.T) {
		var exists bool
		err := tc.DB.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.columns
				WHERE table_name = 'sync_outbox'
				AND column_name = 'resource_namespace'
			)
		`).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "resource_namespace column should exist (Migration 030)")
	})

	t.Run("Migration 030 - resource_name column exists", func(t *testing.T) {
		var exists bool
		err := tc.DB.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.columns
				WHERE table_name = 'sync_outbox'
				AND column_name = 'resource_name'
			)
		`).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "resource_name column should exist (Migration 030)")
	})

	t.Run("Migration 029 - auto-ready conditions trigger exists", func(t *testing.T) {
		// Verify that k8s_metadata has trigger for auto-ready on conditions change
		var exists bool
		err := tc.DB.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.triggers
				WHERE trigger_name = 'trg_sync_ready_from_conditions'
			)
		`).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "auto-ready trigger should exist on k8s_metadata (Migration 029)")
	})

	t.Run("sync_outbox table schema complete", func(t *testing.T) {
		// Query column names
		rows, err := tc.DB.Query(`
			SELECT column_name
			FROM information_schema.columns
			WHERE table_name = 'sync_outbox'
			ORDER BY ordinal_position
		`)
		require.NoError(t, err)
		defer rows.Close()

		var columns []string
		for rows.Next() {
			var col string
			require.NoError(t, rows.Scan(&col))
			columns = append(columns, col)
		}

		// Verify critical columns exist
		expectedColumns := []string{
			"id",
			"resource_type",
			"resource_id",
			"resource_namespace", // Migration 030
			"resource_name",      // Migration 030
			"operation",
			"target_system",
			"payload",
			"status",
			"attempts",
			"max_retries",
			"next_retry_at",
			"last_error",
			"error_category",
			"created_at",
			"updated_at",
			"processed_at",
		}

		for _, expected := range expectedColumns {
			assert.Contains(t, columns, expected, "sync_outbox should have column: %s", expected)
		}

		t.Logf("  ℹ️  sync_outbox has %d columns", len(columns))
	})

	t.Run("Mock SGROUP server responds", func(t *testing.T) {
		// Verify mock server URL is valid
		url := tc.MockSGROUP.URL()
		assert.NotEmpty(t, url, "mock SGROUP URL should not be empty")
		t.Logf("  ℹ️  Mock SGROUP URL: %s", url)

		// Verify initial mode is healthy
		mode := tc.MockSGROUP.GetMode()
		assert.Equal(t, ModeHealthy, mode, "initial mode should be Healthy")

		// Verify no requests yet
		count := tc.MockSGROUP.GetRequestCount()
		assert.Equal(t, 0, count, "should have no requests initially")
	})

	t.Run("Mock SGROUP mode switching works", func(t *testing.T) {
		// Test mode changes
		tc.MockSGROUP.SetMode(ModeServerError)
		assert.Equal(t, ModeServerError, tc.MockSGROUP.GetMode())

		tc.MockSGROUP.SetMode(ModeTimeout)
		assert.Equal(t, ModeTimeout, tc.MockSGROUP.GetMode())

		tc.MockSGROUP.SetMode(ModeConnectionRefused)
		assert.Equal(t, ModeConnectionRefused, tc.MockSGROUP.GetMode())

		// Reset to healthy
		tc.MockSGROUP.SetMode(ModeHealthy)
		assert.Equal(t, ModeHealthy, tc.MockSGROUP.GetMode())
	})

	t.Run("Helper functions work", func(t *testing.T) {
		// Test TCCountOutboxEntries
		count := TCCountOutboxEntries(t, tc.DB)
		assert.Equal(t, 0, count, "outbox should be empty initially")

		// Test TCCleanOutboxTable (should not fail on empty table)
		TCCleanOutboxTable(t, tc.DB)
		count = TCCountOutboxEntries(t, tc.DB)
		assert.Equal(t, 0, count, "outbox should still be empty after clean")

		// Test TCLogOutboxState (should not panic on empty table)
		TCLogOutboxState(t, tc.DB)
	})

	t.Run("Can insert and query outbox entry", func(t *testing.T) {
		// Insert a test outbox entry
		var entryID string
		err := tc.DB.QueryRow(`
			INSERT INTO sync_outbox (
				resource_type,
				resource_id,
				resource_namespace,
				resource_name,
				operation,
				target_system,
				payload,
				status,
				attempts,
				max_retries
			) VALUES (
				'Host',
				gen_random_uuid(),
				'default',
				'test-host',
				'CREATE',
				'SGROUP',
				'{"name": "test-host"}'::jsonb,
				'PENDING',
				0,
				5
			) RETURNING id
		`).Scan(&entryID)
		require.NoError(t, err, "should be able to insert outbox entry")
		t.Logf("  ℹ️  Inserted test outbox entry: %s", entryID)

		// Query it back
		entry, err := TCGetOutboxEntryByResourceName(t, tc.DB, "test-host")
		require.NoError(t, err, "should be able to query outbox entry")
		assert.NotNil(t, entry)
		assert.Equal(t, "test-host", entry.ResourceName)
		assert.Equal(t, "default", entry.ResourceNamespace)
		assert.Equal(t, "Host", entry.ResourceType)

		// Clean up
		TCCleanOutboxTable(t, tc.DB)
		count := TCCountOutboxEntries(t, tc.DB)
		assert.Equal(t, 0, count, "outbox should be empty after clean")
	})
}

// TestSmoke_CreateTestResources verifies helper functions for creating test resources
func TestSmoke_CreateTestResources(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	t.Run("TCCreateTestHostWithMetadata", func(t *testing.T) {
		conditions := `[{"type": "Ready", "status": "True"}]`

		hostID := TCCreateTestHostWithMetadata(
			t,
			tc.DB,
			"default",
			"test-host-1",
			"uuid-123",
			"host1.example.com",
			true,
			conditions,
		)

		assert.NotEqual(t, "", hostID, "host name should not be empty")

		// Verify host exists
		var exists bool
		err := tc.DB.QueryRow(`
			SELECT EXISTS (
				SELECT FROM hosts WHERE namespace = $1 AND name = $2
			)
		`, "default", hostID).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "host should exist in database")
	})

	t.Run("TCCreateTestNetworkWithMetadata", func(t *testing.T) {
		conditions := `[{"type": "Ready", "status": "True"}]`

		networkID := TCCreateTestNetworkWithMetadata(
			t,
			tc.DB,
			"default",
			"test-network-1",
			true,
			conditions,
		)

		assert.NotEqual(t, "", networkID, "network name should not be empty")

		// Verify network exists
		var exists bool
		err := tc.DB.QueryRow(`
			SELECT EXISTS (
				SELECT FROM networks WHERE namespace = $1 AND name = $2
			)
		`, "default", networkID).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "network should exist in database")
	})
}
