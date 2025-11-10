package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain"
)

// OutboxExpectation defines expected state of an outbox entry
type OutboxExpectation struct {
	ShouldExist bool
	Status      domain.OutboxStatus
	Attempts    int
	LastError   string // Substring to match in error message
}

// TCAssertOutboxEntryState verifies outbox entry state (TestContainer version)
//
// Usage:
//
//	TCAssertOutboxEntryState(t, tc.DB, "host-1", OutboxExpectation{
//	    ShouldExist: true,
//	    Status:      domain.OutboxStatusPending,
//	    Attempts:    0,
//	})
func TCAssertOutboxEntryState(t *testing.T, db *sql.DB, resourceName string, expected OutboxExpectation) {
	t.Helper()

	var entry struct {
		ID                uuid.UUID
		ResourceType      string
		ResourceNamespace string
		ResourceName      string
		Operation         domain.SyncOperation
		Status            domain.OutboxStatus
		Attempts          int
		LastError         sql.NullString
		NextRetryAt       sql.NullTime
	}

	query := `
		SELECT id, resource_type, resource_namespace, resource_name,
		       operation, status, attempts, last_error, next_retry_at
		FROM sync_outbox
		WHERE resource_name = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	err := db.QueryRow(query, resourceName).Scan(
		&entry.ID, &entry.ResourceType, &entry.ResourceNamespace, &entry.ResourceName,
		&entry.Operation, &entry.Status, &entry.Attempts, &entry.LastError, &entry.NextRetryAt,
	)

	if expected.ShouldExist {
		require.NoError(t, err, "outbox entry should exist for resource: %s", resourceName)
		assert.Equal(t, expected.Status, entry.Status, "outbox status mismatch")
		assert.Equal(t, expected.Attempts, entry.Attempts, "outbox attempts mismatch")

		if expected.LastError != "" {
			require.True(t, entry.LastError.Valid, "last_error should not be NULL")
			assert.Contains(t, entry.LastError.String, expected.LastError, "last_error should contain expected substring")
		}

		t.Logf("✅ Outbox entry verified: %s (status=%s, attempts=%d)", resourceName, entry.Status, entry.Attempts)
	} else {
		assert.Equal(t, sql.ErrNoRows, err, "outbox entry should not exist for resource: %s", resourceName)
		t.Logf("✅ Confirmed no outbox entry for: %s", resourceName)
	}
}

// TCWaitForOutboxProcessing waits for outbox entry to be processed (TestContainer version)
//
// Usage:
//
//	TCWaitForOutboxProcessing(t, tc.DB, "host-1", 5*time.Second)
func TCWaitForOutboxProcessing(t *testing.T, db *sql.DB, resourceName string, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("❌ Timeout waiting for outbox processing of '%s' (waited %v)", resourceName, time.Since(startTime))

		case <-ticker.C:
			var status string
			err := db.QueryRow(`
				SELECT status FROM sync_outbox WHERE resource_name = $1
			`, resourceName).Scan(&status)

			if err == sql.ErrNoRows {
				// Entry deleted (processed successfully)
				t.Logf("✅ Outbox entry processed and deleted: %s (took %v)", resourceName, time.Since(startTime))
				return
			}

			require.NoError(t, err, "unexpected error querying outbox")

			// Check if in terminal state
			if status == string(domain.OutboxStatusSuccess) ||
				status == string(domain.OutboxStatusFailedPermanent) {
				t.Logf("✅ Outbox entry reached terminal state: %s (status=%s, took %v)",
					resourceName, status, time.Since(startTime))
				return
			}
		}
	}
}

// TCWaitForOutboxEntry waits for an outbox entry to appear (TestContainer version)
//
// Usage:
//
//	entry := TCWaitForOutboxEntry(t, tc.DB, "host-1", 5*time.Second)
func TCWaitForOutboxEntry(t *testing.T, db *sql.DB, resourceName string, timeout time.Duration) *domain.OutboxEntry {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("❌ Timeout waiting for outbox entry: %s (waited %v)", resourceName, time.Since(startTime))

		case <-ticker.C:
			var entry domain.OutboxEntry
			var lastError, errorCategory sql.NullString
			var nextRetryAt, processedAt sql.NullTime

			err := db.QueryRow(`
				SELECT id, resource_type, resource_id, resource_namespace, resource_name,
				       operation, target_system, payload, status, attempts, max_retries,
				       last_error, error_category, next_retry_at,
				       created_at, updated_at, processed_at
				FROM sync_outbox
				WHERE resource_name = $1
				ORDER BY created_at DESC
				LIMIT 1
			`, resourceName).Scan(
				&entry.ID, &entry.ResourceType, &entry.ResourceID,
				&entry.ResourceNamespace, &entry.ResourceName,
				&entry.Operation, &entry.TargetSystem, &entry.Payload,
				&entry.Status, &entry.Attempts, &entry.MaxRetries,
				&lastError, &errorCategory, &nextRetryAt,
				&entry.CreatedAt, &entry.UpdatedAt, &processedAt,
			)

			if err == nil {
				// Convert nullable fields
				if lastError.Valid {
					entry.LastError = &lastError.String
				}
				if errorCategory.Valid {
					entry.ErrorCategory = &errorCategory.String
				}
				if nextRetryAt.Valid {
					entry.NextRetryAt = &nextRetryAt.Time
				}
				if processedAt.Valid {
					entry.ProcessedAt = &processedAt.Time
				}

				t.Logf("✅ Found outbox entry: %s (status=%s, took %v)",
					resourceName, entry.Status, time.Since(startTime))
				return &entry
			}

			if err != sql.ErrNoRows {
				require.NoError(t, err, "unexpected error querying outbox")
			}
		}
	}
}

// TCCreateTestHostWithMetadata creates a test Host with k8s_metadata (TestContainer version)
// Note: hostname column removed in migration 011, ready column doesn't exist in current schema
func TCCreateTestHostWithMetadata(
	t *testing.T,
	db *sql.DB,
	namespace, name, hostUUID, hostname string,
	ready bool,
	conditions string,
) string {
	t.Helper()

	// Create k8s_metadata
	var resourceVersion int64
	err := db.QueryRow(`
		INSERT INTO k8s_metadata (conditions)
		VALUES ($1::jsonb)
		RETURNING resource_version
	`, conditions).Scan(&resourceVersion)
	require.NoError(t, err, "failed to create k8s_metadata")

	// Create host (hostname and ready columns don't exist in current schema)
	// Current schema after migration 011: namespace, name, uuid, host_name_sync, address_group_name, is_bound, etc.
	_, err = db.Exec(`
		INSERT INTO hosts (
			namespace, name, uuid, resource_version
		) VALUES ($1, $2, $3, $4)
	`, namespace, name, hostUUID, resourceVersion)
	require.NoError(t, err, "failed to create test host")

	t.Logf("Created test Host: %s/%s (uuid=%s)",
		namespace, name, hostUUID)

	return name // Return name for consistency with other helpers
}

// TCCreateTestNetworkWithMetadata creates a test Network with k8s_metadata (TestContainer version)
// Note: ready column doesn't exist in current schema, id column doesn't exist
func TCCreateTestNetworkWithMetadata(
	t *testing.T,
	db *sql.DB,
	namespace, name string,
	ready bool,
	conditions string,
) string {
	t.Helper()

	// Create k8s_metadata
	var resourceVersion int64
	err := db.QueryRow(`
		INSERT INTO k8s_metadata (conditions)
		VALUES ($1::jsonb)
		RETURNING resource_version
	`, conditions).Scan(&resourceVersion)
	require.NoError(t, err, "failed to create k8s_metadata")

	// Create network (ready and id columns don't exist in current schema)
	// Current schema: namespace, name, network_items, cidr (added in migration 009, NOT NULL), is_bound, binding_ref_*, address_group_ref_*, resource_version
	_, err = db.Exec(`
		INSERT INTO networks (
			namespace, name, cidr, resource_version
		) VALUES ($1, $2, $3, $4)
	`, namespace, name, "10.0.0.0/24", resourceVersion)
	require.NoError(t, err, "failed to create test network")

	t.Logf("Created test Network: %s/%s",
		namespace, name)

	return name // Return name for consistency with other helpers
}

// TCGetOutboxEntryByResourceName retrieves outbox entry by resource name (TestContainer version)
func TCGetOutboxEntryByResourceName(t *testing.T, db *sql.DB, resourceName string) (*domain.OutboxEntry, error) {
	t.Helper()

	var entry domain.OutboxEntry
	var lastError, errorCategory sql.NullString
	var nextRetryAt, processedAt sql.NullTime

	err := db.QueryRow(`
		SELECT id, resource_type, resource_id, resource_namespace, resource_name,
		       operation, target_system, payload, status, attempts, max_retries,
		       last_error, error_category, next_retry_at,
		       created_at, updated_at, processed_at
		FROM sync_outbox
		WHERE resource_name = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, resourceName).Scan(
		&entry.ID, &entry.ResourceType, &entry.ResourceID,
		&entry.ResourceNamespace, &entry.ResourceName,
		&entry.Operation, &entry.TargetSystem, &entry.Payload,
		&entry.Status, &entry.Attempts, &entry.MaxRetries,
		&lastError, &errorCategory, &nextRetryAt,
		&entry.CreatedAt, &entry.UpdatedAt, &processedAt,
	)

	if err != nil {
		return nil, err
	}

	// Convert nullable fields
	if lastError.Valid {
		entry.LastError = &lastError.String
	}
	if errorCategory.Valid {
		entry.ErrorCategory = &errorCategory.String
	}
	if nextRetryAt.Valid {
		entry.NextRetryAt = &nextRetryAt.Time
	}
	if processedAt.Valid {
		entry.ProcessedAt = &processedAt.Time
	}

	return &entry, nil
}

// TCCleanOutboxTable truncates sync_outbox table for clean test state (TestContainer version)
func TCCleanOutboxTable(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec("TRUNCATE TABLE sync_outbox CASCADE")
	require.NoError(t, err, "failed to clean outbox table")

	t.Log("Outbox table cleaned")
}

// TCCountOutboxEntries counts total outbox entries (TestContainer version)
func TCCountOutboxEntries(t *testing.T, db *sql.DB) int {
	t.Helper()

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sync_outbox").Scan(&count)
	require.NoError(t, err, "failed to count outbox entries")

	return count
}

// TCCountOutboxEntriesByStatus counts outbox entries by status (TestContainer version)
func TCCountOutboxEntriesByStatus(t *testing.T, db *sql.DB, status domain.OutboxStatus) int {
	t.Helper()

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sync_outbox WHERE status = $1", status).Scan(&count)
	require.NoError(t, err, "failed to count outbox entries by status")

	return count
}

// TCGetAllOutboxEntries retrieves all outbox entries (TestContainer version)
func TCGetAllOutboxEntries(t *testing.T, db *sql.DB) []domain.OutboxEntry {
	t.Helper()

	rows, err := db.Query(`
		SELECT id, resource_type, resource_id, resource_namespace, resource_name,
		       operation, target_system, payload, status, attempts, max_retries,
		       last_error, error_category, next_retry_at,
		       created_at, updated_at, processed_at
		FROM sync_outbox
		ORDER BY created_at ASC
	`)
	require.NoError(t, err, "failed to query outbox entries")
	defer rows.Close()

	var entries []domain.OutboxEntry
	for rows.Next() {
		var entry domain.OutboxEntry
		var lastError, errorCategory sql.NullString
		var nextRetryAt, processedAt sql.NullTime

		err := rows.Scan(
			&entry.ID, &entry.ResourceType, &entry.ResourceID,
			&entry.ResourceNamespace, &entry.ResourceName,
			&entry.Operation, &entry.TargetSystem, &entry.Payload,
			&entry.Status, &entry.Attempts, &entry.MaxRetries,
			&lastError, &errorCategory, &nextRetryAt,
			&entry.CreatedAt, &entry.UpdatedAt, &processedAt,
		)
		require.NoError(t, err, "failed to scan outbox entry")

		// Convert nullable fields
		if lastError.Valid {
			entry.LastError = &lastError.String
		}
		if errorCategory.Valid {
			entry.ErrorCategory = &errorCategory.String
		}
		if nextRetryAt.Valid {
			entry.NextRetryAt = &nextRetryAt.Time
		}
		if processedAt.Valid {
			entry.ProcessedAt = &processedAt.Time
		}

		entries = append(entries, entry)
	}

	require.NoError(t, rows.Err(), "error iterating outbox entries")

	return entries
}

// TCLogOutboxState logs current outbox state for debugging (TestContainer version)
func TCLogOutboxState(t *testing.T, db *sql.DB) {
	t.Helper()

	entries := TCGetAllOutboxEntries(t, db)

	t.Logf("Outbox State: %d entries", len(entries))
	for i, entry := range entries {
		t.Logf("  [%d] %s/%s | %s %s | status=%s attempts=%d",
			i+1,
			entry.ResourceNamespace,
			entry.ResourceName,
			entry.Operation,
			entry.ResourceType,
			entry.Status,
			entry.Attempts,
		)
	}
}

// TCCreateTestAddressGroup creates a test AddressGroup (TestContainer version)
func TCCreateTestAddressGroup(t *testing.T, db *sql.DB, namespace, name string) string {
	t.Helper()

	// Create k8s_metadata
	var resourceVersion int64
	err := db.QueryRow(`
		INSERT INTO k8s_metadata (conditions)
		VALUES ('[]'::jsonb)
		RETURNING resource_version
	`).Scan(&resourceVersion)
	require.NoError(t, err, "failed to create k8s_metadata")

	// Create address_group
	_, err = db.Exec(`
		INSERT INTO address_groups (
			namespace, name, default_action, logs, trace, resource_version
		) VALUES ($1, $2, 'DROP', false, false, $3)
	`, namespace, name, resourceVersion)
	require.NoError(t, err, "failed to create test address_group")

	t.Logf("Created test AddressGroup: %s/%s", namespace, name)

	return name // Return name for easy query
}
