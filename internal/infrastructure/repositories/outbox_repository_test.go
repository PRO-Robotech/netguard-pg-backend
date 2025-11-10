package repositories

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain"
)

// setupTestDB creates a test database connection pool
func setupTestDB(t *testing.T) *pgxpool.Pool {
	// Use DATABASE_URL from environment or default test database
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/netguard_test?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err, "Failed to connect to test database")

	// Verify connection
	err = pool.Ping(ctx)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}

	return pool
}

// cleanupTestDB truncates the sync_outbox table
func cleanupTestDB(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	_, err := pool.Exec(ctx, "TRUNCATE TABLE sync_outbox")
	require.NoError(t, err)
}

func TestOutboxRepository_Create(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	entry := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{"name": "test-host"}`),
		Status:       domain.OutboxStatusPending,
		MaxRetries:   5,
	}

	err := repo.Create(ctx, entry)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, entry.ID, "ID should be generated")
	assert.False(t, entry.CreatedAt.IsZero(), "CreatedAt should be set")
	assert.False(t, entry.UpdatedAt.IsZero(), "UpdatedAt should be set")
}

func TestOutboxRepository_Create_Upsert(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	resourceID := uuid.New()

	// First insert
	entry1 := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   resourceID,
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{"version": 1}`),
		Status:       domain.OutboxStatusPending,
		MaxRetries:   5,
	}
	err := repo.Create(ctx, entry1)
	require.NoError(t, err)

	firstID := entry1.ID

	// Second insert with same unique key (should UPDATE)
	entry2 := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   resourceID,
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{"version": 2}`),
		Status:       domain.OutboxStatusPending,
		MaxRetries:   5,
	}
	err = repo.Create(ctx, entry2)
	require.NoError(t, err)

	// Verify it's the SAME entry (upserted)
	assert.Equal(t, firstID, entry2.ID, "UPSERT should return same ID")

	// Verify only one entry exists with updated payload
	entries, err := repo.FindByResource(ctx, "Host", resourceID)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "Should have only one entry after UPSERT")
	assert.Contains(t, string(entries[0].Payload), `"version": 2`, "Payload should be updated")
}

func TestOutboxRepository_FindByID(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	entry := &domain.OutboxEntry{
		ResourceType: "Network",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationUpdate,
		TargetSystem: domain.TargetSystemInternal,
		Payload:      json.RawMessage(`{"cidr": "10.0.0.0/24"}`),
		Status:       domain.OutboxStatusPending,
		MaxRetries:   5,
	}
	err := repo.Create(ctx, entry)
	require.NoError(t, err)

	// Find by ID
	found, err := repo.FindByID(ctx, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, entry.ID, found.ID)
	assert.Equal(t, entry.ResourceType, found.ResourceType)
	assert.Equal(t, entry.ResourceID, found.ResourceID)
	assert.Equal(t, entry.Operation, found.Operation)
	assert.Equal(t, entry.TargetSystem, found.TargetSystem)
	assert.JSONEq(t, string(entry.Payload), string(found.Payload))
}

func TestOutboxRepository_FindByID_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	_, err := repo.FindByID(ctx, uuid.New())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestOutboxRepository_FindByResource(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	resourceID := uuid.New()

	// Create multiple entries for same resource
	for i := 0; i < 3; i++ {
		entry := &domain.OutboxEntry{
			ResourceType: "Host",
			ResourceID:   resourceID,
			Operation:    domain.SyncOperationUpdate,
			TargetSystem: domain.TargetSystemSGROUP,
			Payload:      json.RawMessage(`{"version": ` + string(rune('0'+i)) + `}`),
			Status:       domain.OutboxStatusPending,
			MaxRetries:   5,
		}
		err := repo.Create(ctx, entry)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Ensure different created_at
	}

	// Create entry for different resource
	otherEntry := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{"other": true}`),
		Status:       domain.OutboxStatusPending,
		MaxRetries:   5,
	}
	err := repo.Create(ctx, otherEntry)
	require.NoError(t, err)

	// Find by resource
	entries, err := repo.FindByResource(ctx, "Host", resourceID)
	require.NoError(t, err)
	assert.Len(t, entries, 3, "Should find 3 entries for the resource")

	// Verify ordering (newest first)
	assert.True(t, entries[0].CreatedAt.After(entries[1].CreatedAt) ||
		entries[0].CreatedAt.Equal(entries[1].CreatedAt),
		"Entries should be ordered by created_at DESC")
}

func TestOutboxRepository_FindPending(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	// Create 2 PENDING entries
	for i := 0; i < 2; i++ {
		entry := &domain.OutboxEntry{
			ResourceType: "Host",
			ResourceID:   uuid.New(),
			Operation:    domain.SyncOperationCreate,
			TargetSystem: domain.TargetSystemSGROUP,
			Payload:      json.RawMessage(`{}`),
			Status:       domain.OutboxStatusPending,
			MaxRetries:   5,
		}
		err := repo.Create(ctx, entry)
		require.NoError(t, err)
	}

	// Create 1 FAILED_RETRYABLE entry (ready to retry)
	retryableEntry := &domain.OutboxEntry{
		ResourceType: "Network",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationUpdate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{}`),
		Status:       domain.OutboxStatusFailedRetryable,
		Attempts:     1,
		MaxRetries:   5,
		NextRetryAt:  nil, // Ready immediately
	}
	err := repo.Create(ctx, retryableEntry)
	require.NoError(t, err)

	// Create 1 SUCCESS entry (should NOT be returned)
	successEntry := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationDelete,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{}`),
		Status:       domain.OutboxStatusSuccess,
		MaxRetries:   5,
	}
	err = repo.Create(ctx, successEntry)
	require.NoError(t, err)

	// Create 1 FAILED_RETRYABLE with future next_retry_at (should NOT be returned)
	futureRetry := time.Now().Add(1 * time.Hour)
	futureEntry := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{}`),
		Status:       domain.OutboxStatusFailedRetryable,
		Attempts:     2,
		MaxRetries:   5,
		NextRetryAt:  &futureRetry,
	}
	err = repo.Create(ctx, futureEntry)
	require.NoError(t, err)

	// Find pending
	pending, err := repo.FindPending(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, pending, 3, "Should return 2 PENDING + 1 FAILED_RETRYABLE (ready)")

	// Verify statuses
	for _, entry := range pending {
		assert.True(t, entry.Status == domain.OutboxStatusPending ||
			entry.Status == domain.OutboxStatusFailedRetryable,
			"Should only return PENDING or FAILED_RETRYABLE")
	}
}

func TestOutboxRepository_FindPending_SkipLocked(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	// Create entry
	entry := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{}`),
		Status:       domain.OutboxStatusPending,
		MaxRetries:   5,
	}
	err := repo.Create(ctx, entry)
	require.NoError(t, err)

	// Start transaction 1 and lock the entry
	tx1, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx1.Rollback(ctx)

	repo1 := NewOutboxRepository(&pgxPoolWrapper{tx: tx1})
	locked, err := repo1.FindPending(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, locked, 1, "Transaction 1 should lock 1 entry")

	// Transaction 2 should NOT see the locked entry (SKIP LOCKED)
	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx2.Rollback(ctx)

	repo2 := NewOutboxRepository(&pgxPoolWrapper{tx: tx2})
	available, err := repo2.FindPending(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, available, 0, "SKIP LOCKED should skip locked entries")
}

func TestOutboxRepository_UpdateStatus(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	entry := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{}`),
		Status:       domain.OutboxStatusPending,
		MaxRetries:   5,
	}
	err := repo.Create(ctx, entry)
	require.NoError(t, err)

	// Update to SUCCESS
	now := time.Now()
	err = repo.UpdateStatus(ctx, entry.ID, domain.OutboxStatusSuccess, &now, nil, nil)
	require.NoError(t, err)

	// Verify update
	updated, err := repo.FindByID(ctx, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.OutboxStatusSuccess, updated.Status)
	assert.NotNil(t, updated.ProcessedAt)
	assert.True(t, updated.UpdatedAt.After(entry.UpdatedAt) ||
		updated.UpdatedAt.Equal(entry.UpdatedAt))
}

func TestOutboxRepository_UpdateStatus_WithError(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	entry := &domain.OutboxEntry{
		ResourceType: "Network",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationUpdate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{}`),
		Status:       domain.OutboxStatusProcessing,
		MaxRetries:   5,
	}
	err := repo.Create(ctx, entry)
	require.NoError(t, err)

	// Update to FAILED_RETRYABLE with error
	errorMsg := "network timeout"
	errorCat := "NETWORK"
	err = repo.UpdateStatus(ctx, entry.ID, domain.OutboxStatusFailedRetryable,
		nil, &errorMsg, &errorCat)
	require.NoError(t, err)

	// Verify error tracking
	updated, err := repo.FindByID(ctx, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.OutboxStatusFailedRetryable, updated.Status)
	require.NotNil(t, updated.LastError)
	assert.Equal(t, "network timeout", *updated.LastError)
	require.NotNil(t, updated.ErrorCategory)
	assert.Equal(t, "NETWORK", *updated.ErrorCategory)
}

func TestOutboxRepository_IncrementAttempts(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	entry := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{}`),
		Status:       domain.OutboxStatusFailedRetryable,
		Attempts:     1,
		MaxRetries:   5,
	}
	err := repo.Create(ctx, entry)
	require.NoError(t, err)

	// Increment attempts
	nextRetry := time.Now().Add(10 * time.Second)
	err = repo.IncrementAttempts(ctx, entry.ID, &nextRetry)
	require.NoError(t, err)

	// Verify increment
	updated, err := repo.FindByID(ctx, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Attempts, "Attempts should be incremented")
	require.NotNil(t, updated.NextRetryAt)
	assert.True(t, updated.NextRetryAt.After(time.Now().Add(9*time.Second)),
		"NextRetryAt should be set correctly")
}

func TestOutboxRepository_Delete(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	entry := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{}`),
		Status:       domain.OutboxStatusSuccess,
		MaxRetries:   5,
	}
	err := repo.Create(ctx, entry)
	require.NoError(t, err)

	// Delete
	err = repo.Delete(ctx, entry.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = repo.FindByID(ctx, entry.ID)
	assert.Error(t, err, "Entry should not exist after deletion")
	assert.Contains(t, err.Error(), "not found")
}

func TestOutboxRepository_DeleteOldSuccessful(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	// Create old SUCCESS entry
	oldTime := time.Now().Add(-48 * time.Hour)
	oldEntry := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{}`),
		Status:       domain.OutboxStatusSuccess,
		MaxRetries:   5,
		ProcessedAt:  &oldTime,
	}
	err := repo.Create(ctx, oldEntry)
	require.NoError(t, err)

	// Update processed_at to old time (manual override)
	_, err = pool.Exec(ctx,
		"UPDATE sync_outbox SET processed_at = $1 WHERE id = $2",
		oldTime, oldEntry.ID)
	require.NoError(t, err)

	// Create recent SUCCESS entry
	recentTime := time.Now().Add(-12 * time.Hour)
	recentEntry := &domain.OutboxEntry{
		ResourceType: "Network",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationUpdate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{}`),
		Status:       domain.OutboxStatusSuccess,
		MaxRetries:   5,
		ProcessedAt:  &recentTime,
	}
	err = repo.Create(ctx, recentEntry)
	require.NoError(t, err)

	// Delete entries older than 24h
	deleted, err := repo.DeleteOldSuccessful(ctx, 24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted, "Should delete 1 old entry")

	// Verify old entry deleted
	_, err = repo.FindByID(ctx, oldEntry.ID)
	assert.Error(t, err)

	// Verify recent entry still exists
	found, err := repo.FindByID(ctx, recentEntry.ID)
	require.NoError(t, err)
	assert.Equal(t, recentEntry.ID, found.ID)
}

func TestOutboxRepository_CompleteLifecycle(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupTestDB(t, pool)

	repo := NewOutboxRepository(pool)
	ctx := context.Background()

	// Step 1: Create PENDING entry
	entry := &domain.OutboxEntry{
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      json.RawMessage(`{"name": "lifecycle-test"}`),
		Status:       domain.OutboxStatusPending,
		Attempts:     0,
		MaxRetries:   3,
	}
	err := repo.Create(ctx, entry)
	require.NoError(t, err)

	// Step 2: Worker picks it up (FindPending with FOR UPDATE SKIP LOCKED)
	pending, err := repo.FindPending(ctx, 1)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	picked := pending[0]

	// Step 3: Worker starts processing (UpdateStatus to PROCESSING)
	err = repo.UpdateStatus(ctx, picked.ID, domain.OutboxStatusProcessing, nil, nil, nil)
	require.NoError(t, err)

	// Step 4: Processing fails (IncrementAttempts + UpdateStatus to FAILED_RETRYABLE)
	nextRetry := time.Now().Add(2 * time.Second)
	err = repo.IncrementAttempts(ctx, picked.ID, &nextRetry)
	require.NoError(t, err)

	errorMsg := "temporary network error"
	errorCat := "NETWORK"
	err = repo.UpdateStatus(ctx, picked.ID, domain.OutboxStatusFailedRetryable,
		nil, &errorMsg, &errorCat)
	require.NoError(t, err)

	// Verify state after first failure
	afterFail, err := repo.FindByID(ctx, picked.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.OutboxStatusFailedRetryable, afterFail.Status)
	assert.Equal(t, 1, afterFail.Attempts)

	// Step 5: Wait for retry time (simulate)
	time.Sleep(3 * time.Second)

	// Step 6: Worker picks it up again
	pending2, err := repo.FindPending(ctx, 1)
	require.NoError(t, err)
	require.Len(t, pending2, 1)

	// Step 7: Second attempt succeeds
	err = repo.UpdateStatus(ctx, picked.ID, domain.OutboxStatusProcessing, nil, nil, nil)
	require.NoError(t, err)

	now := time.Now()
	err = repo.UpdateStatus(ctx, picked.ID, domain.OutboxStatusSuccess, &now, nil, nil)
	require.NoError(t, err)

	// Verify final state
	final, err := repo.FindByID(ctx, picked.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.OutboxStatusSuccess, final.Status)
	assert.NotNil(t, final.ProcessedAt)
	assert.Nil(t, final.LastError, "Error should be cleared on success")
}

// pgxPoolWrapper adapts pgx.Tx to look like a pool for testing transactions
type pgxPoolWrapper struct {
	tx pgx.Tx
}

func (w *pgxPoolWrapper) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return w.tx.Query(ctx, sql, args...)
}

func (w *pgxPoolWrapper) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return w.tx.QueryRow(ctx, sql, args...)
}

func (w *pgxPoolWrapper) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return w.tx.Exec(ctx, sql, args...)
}
