package integration

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/monitor"
	"netguard-pg-backend/internal/sync/syncers"
	"netguard-pg-backend/internal/sync/worker"
)

// TriggerReadyTransition updates k8s_metadata.conditions to trigger ready transition
// This simulates the backend updating conditions and triggers Migration 029's auto-sync
func TriggerReadyTransition(t *testing.T, db *sql.DB, resourceVersion int64, ready bool) {
	t.Helper()

	var conditionsJSON string
	if ready {
		conditionsJSON = `[{"type":"Ready","status":"True"}]`
	} else {
		conditionsJSON = `[]`
	}

	_, err := db.Exec(`
		UPDATE k8s_metadata
		SET conditions = $1::jsonb
		WHERE resource_version = $2
	`, conditionsJSON, resourceVersion)

	require.NoError(t, err, "failed to trigger ready transition")

	t.Logf("✅ Triggered ready transition: resourceVersion=%d, ready=%v", resourceVersion, ready)
}

// PollUntil polls until condition is met or timeout expires
func PollUntil(
	t *testing.T,
	condition func() bool,
	timeout time.Duration,
	interval time.Duration,
	message string,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if condition() {
			t.Logf("✅ Condition met: %s", message)
			return
		}
		time.Sleep(interval)
	}

	t.Fatalf("Timeout waiting for condition: %s (timeout=%v)", message, timeout)
}

// CreateTestWorker creates an OutboxWorker instance for testing
func CreateTestWorker(
	t *testing.T,
	pool *pgxpool.Pool,
	mockClient *MockSGROUPClient,
) *worker.OutboxWorker {
	t.Helper()

	// Create zap logger
	zapLogger, err := zap.NewDevelopment()
	require.NoError(t, err, "failed to create logger")

	// Convert zap logger to logr.Logger (required by syncers)
	logger := zapr.NewLogger(zapLogger)

	// Create registry from pool
	registry, err := createTestRegistry(t, pool)
	require.NoError(t, err, "failed to create registry")

	// Create syncers with mock client
	hostSyncer := syncers.NewHostSyncer(mockClient, logger)
	addressGroupSyncer := syncers.NewAddressGroupSyncer(mockClient, logger)
	networkSyncer := syncers.NewNetworkSyncer(mockClient, logger)
	serviceSyncer := syncers.NewServiceSyncer(mockClient, logger)
	svcSvcRuleSyncer := syncers.NewSvcSvcRuleSyncer(mockClient, logger)
	svcFqdnRuleSyncer := syncers.NewSvcFqdnRuleSyncer(mockClient, logger)
	connMonitor := monitor.NewSGroupConnectionMonitor(mockClient, monitor.DefaultConfig(), zapLogger)

	// Create worker config
	// 🔧 FIX: Added all required fields
	config := &worker.WorkerConfig{
		PollInterval:          1 * time.Second, // Changed from 100ms to 1s (minimum)
		BatchSize:             10,
		SGROUPTimeout:         5 * time.Second,
		MaxConcurrentBatches:  1, // Sequential processing for tests
		MaxAttemptsValidation: 3,
		MaxAttemptsTemporary:  20,
		MaxAttemptsNetwork:    100,
		HealthCheckInterval:   10 * time.Second,
		MetricsEnabled:        false, // Disabled for tests
	}

	// Create worker
	w := worker.NewOutboxWorker(
		pool,
		registry,
		hostSyncer,
		addressGroupSyncer,
		networkSyncer,
		serviceSyncer,
		svcSvcRuleSyncer,
		svcFqdnRuleSyncer,
		nil,
		zapLogger,
		config,
		connMonitor,
	)

	t.Logf("✅ Created test worker (PollInterval=%v, BatchSize=%d)", config.PollInterval, config.BatchSize)

	return w
}

// createTestRegistry creates a minimal test registry
func createTestRegistry(t *testing.T, pool *pgxpool.Pool) (ports.Registry, error) {
	t.Helper()

	// For integration tests, we can use a simplified registry wrapper
	// or pass nil if worker doesn't actually need full registry functionality

	// Option 1: Use real PostgreSQL registry (requires pool to implement registry interface)
	// For now, return nil as worker may not need full registry for outbox processing
	return nil, nil
}

// CreatePgxPool creates a pgxpool.Pool for worker testing
func CreatePgxPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx := context.Background()

	dsn := getTestDSN()
	poolConfig, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err, "failed to parse pool config")

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	require.NoError(t, err, "failed to create pgx pool")

	// Verify connection
	err = pool.Ping(ctx)
	require.NoError(t, err, "failed to ping pool")

	t.Logf("✅ Created pgx pool")

	return pool
}

// getTestDSN returns test database DSN from environment
func getTestDSN() string {
	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5432")
	user := getEnvOrDefault("TEST_DB_USER", "postgres")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "password")
	dbname := getEnvOrDefault("TEST_DB_NAME", "netguard_test")
	sslmode := getEnvOrDefault("TEST_DB_SSLMODE", "disable")

	return "host=" + host + " port=" + port + " user=" + user +
		" password=" + password + " dbname=" + dbname + " sslmode=" + sslmode
}

// getEnvOrDefault returns environment variable or default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ClosePgxPool closes pgxpool.Pool
func ClosePgxPool(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	if pool != nil {
		pool.Close()
		t.Logf("✅ Closed pgx pool")
	}
}

// WaitForHostReady waits for host.ready to become the expected value
func WaitForHostReady(
	t *testing.T,
	db *sql.DB,
	namespace, name string,
	expectedReady bool,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var ready bool
		err := db.QueryRow(`
			SELECT ready
			FROM hosts
			WHERE namespace = $1 AND name = $2
		`, namespace, name).Scan(&ready)

		if err == nil && ready == expectedReady {
			t.Logf("✅ Host ready status: %s/%s ready=%v", namespace, name, ready)
			return
		}

		if err != nil && err != sql.ErrNoRows {
			require.NoError(t, err, "unexpected error querying host ready status")
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("Timeout waiting for host ready status: %s/%s expected=%v", namespace, name, expectedReady)
}

// WaitForNetworkReady waits for network.ready to become the expected value
func WaitForNetworkReady(
	t *testing.T,
	db *sql.DB,
	namespace, name string,
	expectedReady bool,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var ready bool
		err := db.QueryRow(`
			SELECT ready
			FROM networks
			WHERE namespace = $1 AND name = $2
		`, namespace, name).Scan(&ready)

		if err == nil && ready == expectedReady {
			t.Logf("✅ Network ready status: %s/%s ready=%v", namespace, name, ready)
			return
		}

		if err != nil && err != sql.ErrNoRows {
			require.NoError(t, err, "unexpected error querying network ready status")
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("Timeout waiting for network ready status: %s/%s expected=%v", namespace, name, expectedReady)
}

// GetOutboxEntryCount returns the number of outbox entries matching criteria
func GetOutboxEntryCount(t *testing.T, db *sql.DB, resourceType, operation string) int {
	t.Helper()

	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM sync_outbox
		WHERE resource_type = $1 AND operation = $2
	`, resourceType, operation).Scan(&count)

	require.NoError(t, err, "failed to count outbox entries")

	return count
}

// UpdateHostReady manually updates host.ready (for testing without trigger)
func UpdateHostReady(t *testing.T, db *sql.DB, namespace, name string, ready bool) {
	t.Helper()

	_, err := db.Exec(`
		UPDATE hosts
		SET ready = $1
		WHERE namespace = $2 AND name = $3
	`, ready, namespace, name)

	require.NoError(t, err, "failed to update host ready")

	t.Logf("✅ Updated host ready: %s/%s ready=%v", namespace, name, ready)
}

// UpdateNetworkReady manually updates network.ready (for testing without trigger)
func UpdateNetworkReady(t *testing.T, db *sql.DB, namespace, name string, ready bool) {
	t.Helper()

	_, err := db.Exec(`
		UPDATE networks
		SET ready = $1
		WHERE namespace = $2 AND name = $3
	`, ready, namespace, name)

	require.NoError(t, err, "failed to update network ready")

	t.Logf("✅ Updated network ready: %s/%s ready=%v", namespace, name, ready)
}

// AssertHostReady asserts that host.ready has expected value
func AssertHostReady(t *testing.T, db *sql.DB, namespace, name string, expectedReady bool) {
	t.Helper()

	var ready bool
	err := db.QueryRow(`
		SELECT ready
		FROM hosts
		WHERE namespace = $1 AND name = $2
	`, namespace, name).Scan(&ready)

	require.NoError(t, err, "failed to query host ready")
	require.Equal(t, expectedReady, ready, "host ready mismatch: %s/%s", namespace, name)

	t.Logf("✅ Host ready verified: %s/%s ready=%v", namespace, name, ready)
}

// AssertNetworkReady asserts that network.ready has expected value
func AssertNetworkReady(t *testing.T, db *sql.DB, namespace, name string, expectedReady bool) {
	t.Helper()

	var ready bool
	err := db.QueryRow(`
		SELECT ready
		FROM networks
		WHERE namespace = $1 AND name = $2
	`, namespace, name).Scan(&ready)

	require.NoError(t, err, "failed to query network ready")
	require.Equal(t, expectedReady, ready, "network ready mismatch: %s/%s", namespace, name)

	t.Logf("✅ Network ready verified: %s/%s ready=%v", namespace, name, ready)
}

// CreateTestRegistryFromPool creates a test registry from pgxpool (if needed for worker)
func CreateTestRegistryFromPool(t *testing.T, pool *pgxpool.Pool) ports.Registry {
	t.Helper()

	// For now, return nil as worker may not need full registry
	return nil
}
