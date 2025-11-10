package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain"
)

// Composite key types for resources (matching actual schema from migration 010+)
type HostKey struct {
	Namespace string
	Name      string
}

type AddressGroupKey struct {
	Namespace string
	Name      string
}

type NetworkKey struct {
	Namespace string
	Name      string
}

type ServiceKey struct {
	Namespace string
	Name      string
}

// TestDBConfig holds test database configuration
type TestDBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// DefaultTestDBConfig returns default test database configuration
func DefaultTestDBConfig() *TestDBConfig {
	return &TestDBConfig{
		Host:     getEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:     getEnvOrDefault("TEST_DB_PORT", "5432"),
		User:     getEnvOrDefault("TEST_DB_USER", "postgres"),
		Password: getEnvOrDefault("TEST_DB_PASSWORD", "password"),
		DBName:   getEnvOrDefault("TEST_DB_NAME", "netguard_test"),
		SSLMode:  getEnvOrDefault("TEST_DB_SSLMODE", "disable"),
	}
}

// DSN returns PostgreSQL connection string
func (c *TestDBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

// SetupTestDB creates and configures test database with migrations
func SetupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	config := DefaultTestDBConfig()

	// Connect to test database
	db, err := sql.Open("postgres", config.DSN())
	require.NoError(t, err, "failed to connect to test database")

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	require.NoError(t, err, "failed to ping test database")

	// Run migrations
	runMigrations(t, db)

	// Clean existing test data
	cleanTestData(t, db)

	t.Logf("✅ Test database setup complete: %s", config.DBName)

	return db
}

// TeardownTestDB cleans up and closes test database connection
func TeardownTestDB(t *testing.T, db *sql.DB) {
	t.Helper()

	if db != nil {
		// Clean test data before closing
		cleanTestData(t, db)

		err := db.Close()
		require.NoError(t, err, "failed to close database connection")

		t.Logf("✅ Test database teardown complete")
	}
}

// runMigrations executes all migrations in order
func runMigrations(t *testing.T, db *sql.DB) {
	t.Helper()

	// Find migrations directory (go up from internal/sync/integration to project root)
	migrationsDir := findMigrationsDir(t)

	// Read all migration files
	files, err := os.ReadDir(migrationsDir)
	require.NoError(t, err, "failed to read migrations directory")

	// Filter and sort .sql files
	var migrationFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".sql") {
			// Only include up migrations (exclude down migrations)
			if !strings.Contains(file.Name(), "down") {
				migrationFiles = append(migrationFiles, file.Name())
			}
		}
	}

	sort.Strings(migrationFiles)

	t.Logf("Found %d migrations to run", len(migrationFiles))

	// Execute each migration
	for _, filename := range migrationFiles {
		migrationPath := filepath.Join(migrationsDir, filename)
		content, err := os.ReadFile(migrationPath)
		require.NoError(t, err, "failed to read migration file: %s", filename)

		// Execute migration SQL
		_, err = db.Exec(string(content))
		require.NoError(t, err, "failed to execute migration: %s", filename)

		t.Logf("  ✅ Applied migration: %s", filename)
	}

	t.Logf("✅ All migrations applied successfully")
}

// findMigrationsDir locates the migrations directory
func findMigrationsDir(t *testing.T) string {
	t.Helper()

	// Start from current directory and go up to find migrations/
	cwd, err := os.Getwd()
	require.NoError(t, err, "failed to get current working directory")

	dir := cwd
	for {
		migrationsPath := filepath.Join(dir, "migrations")
		if info, err := os.Stat(migrationsPath); err == nil && info.IsDir() {
			return migrationsPath
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root, not found
			t.Fatalf("migrations directory not found from %s", cwd)
		}
		dir = parent
	}
}

// cleanTestData removes all test data from tables
func cleanTestData(t *testing.T, db *sql.DB) {
	t.Helper()

	// Disable foreign key checks temporarily
	_, err := db.Exec("SET CONSTRAINTS ALL DEFERRED")
	require.NoError(t, err, "failed to defer constraints")

	// Clean tables in dependency order (children first)
	tables := []string{
		"sync_outbox",
		"ieagrule_port_ranges",
		"ieagrules",
		"service_icmp_specs",
		"service_specs",
		"services",
		"network_bindings",
		"networks",
		"host_bindings",
		"hosts",
		"address_group_namespaced_names",
		"address_groups",
		"k8s_metadata",
	}

	for _, table := range tables {
		_, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			// Ignore errors for tables that might not exist
			t.Logf("  ⚠️  Failed to truncate %s: %v (might not exist)", table, err)
		}
	}

	t.Logf("✅ Test data cleaned")
}

// CreateK8sMetadata creates a k8s_metadata entry for testing
func CreateK8sMetadata(t *testing.T, db *sql.DB, conditions string) int64 {
	t.Helper()

	var resourceVersion int64
	err := db.QueryRow(`
		INSERT INTO k8s_metadata (conditions)
		VALUES ($1::jsonb)
		RETURNING resource_version
	`, conditions).Scan(&resourceVersion)

	require.NoError(t, err, "failed to create k8s_metadata")

	return resourceVersion
}

// CreateTestHost creates a test Host entry
// Returns composite key (namespace, name) matching actual schema (migration 010+)
// Note: hosts table uses (namespace, name) as PRIMARY KEY, NO id column
func CreateTestHost(
	t *testing.T,
	db *sql.DB,
	namespace, name, hostUUID string,
	resourceVersion int64,
	ready bool,
) HostKey {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO hosts (
			namespace, name, uuid, resource_version, ready
		) VALUES ($1, $2, $3, $4, $5)
	`, namespace, name, hostUUID, resourceVersion, ready)

	require.NoError(t, err, "failed to create test host")

	t.Logf("Created test Host: %s/%s (uuid=%s, ready=%v)", namespace, name, hostUUID, ready)

	return HostKey{Namespace: namespace, Name: name}
}

// CreateTestNetwork creates a test Network entry
// Returns composite key (namespace, name) matching actual schema (migration 010+)
func CreateTestNetwork(
	t *testing.T,
	db *sql.DB,
	namespace, name string,
	resourceVersion int64,
	ready bool,
) NetworkKey {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO networks (
			namespace, name, resource_version, ready
		) VALUES ($1, $2, $3, $4)
	`, namespace, name, resourceVersion, ready)

	require.NoError(t, err, "failed to create test network")

	t.Logf("Created test Network: %s/%s (ready=%v)", namespace, name, ready)

	return NetworkKey{Namespace: namespace, Name: name}
}

// CreateTestAddressGroup creates a test AddressGroup entry
// Returns composite key (namespace, name) matching actual schema (migration 010+)
func CreateTestAddressGroup(
	t *testing.T,
	db *sql.DB,
	namespace, name string,
	resourceVersion int64,
) AddressGroupKey {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO address_groups (
			namespace, name, resource_version
		) VALUES ($1, $2, $3)
	`, namespace, name, resourceVersion)

	require.NoError(t, err, "failed to create test address group")

	t.Logf("Created test AddressGroup: %s/%s", namespace, name)

	return AddressGroupKey{Namespace: namespace, Name: name}
}

// CreateTestAddressGroupReady creates a test AddressGroup entry with Ready condition
// This is a convenience helper for tests that need AG to be ready for binding creation
// Returns composite key (namespace, name) matching actual schema (migration 010+)
func CreateTestAddressGroupReady(
	t *testing.T,
	db *sql.DB,
	namespace, name string,
) AddressGroupKey {
	t.Helper()

	// Create k8s_metadata with Ready condition
	resourceVersion := CreateK8sMetadata(t, db, `[{"type":"Ready","status":"True"}]`)

	// Create AddressGroup with this resource_version
	_, err := db.Exec(`
		INSERT INTO address_groups (
			namespace, name, resource_version, aggregated_hosts
		) VALUES ($1, $2, $3, '[]'::jsonb)
	`, namespace, name, resourceVersion)

	require.NoError(t, err, "failed to create test address group (ready)")

	t.Logf("Created test AddressGroup (READY): %s/%s", namespace, name)

	return AddressGroupKey{Namespace: namespace, Name: name}
}

// WaitForOutboxEntry waits for an outbox entry to appear
func WaitForOutboxEntry(
	t *testing.T,
	db *sql.DB,
	resourceType string,
	operation string,
	timeout time.Duration,
) *domain.OutboxEntry {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var entry domain.OutboxEntry
		err := db.QueryRow(`
			SELECT
				id, resource_type, resource_id, operation, target_system,
				payload, COALESCE(delta, 'null'::jsonb), status, attempts, max_retries,
				created_at, updated_at
			FROM sync_outbox
			WHERE resource_type = $1 AND operation = $2
			ORDER BY created_at DESC
			LIMIT 1
		`, resourceType, operation).Scan(
			&entry.ID,
			&entry.ResourceType,
			&entry.ResourceID,
			&entry.Operation,
			&entry.TargetSystem,
			&entry.Payload,
			&entry.Delta,
			&entry.Status,
			&entry.Attempts,
			&entry.MaxRetries,
			&entry.CreatedAt,
			&entry.UpdatedAt,
		)

		if err == nil {
			t.Logf("✅ Found outbox entry: type=%s, operation=%s, status=%s", resourceType, operation, entry.Status)
			return &entry
		}

		if err != sql.ErrNoRows {
			require.NoError(t, err, "unexpected error querying outbox")
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("Timeout waiting for outbox entry: type=%s, operation=%s, timeout=%v", resourceType, operation, timeout)
	return nil
}

// AssertOutboxEntry asserts that an outbox entry exists with specific fields
func AssertOutboxEntry(
	t *testing.T,
	db *sql.DB,
	resourceType, resourceID, operation string,
) {
	t.Helper()

	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM sync_outbox
		WHERE resource_type = $1
		  AND resource_id::text = $2
		  AND operation = $3
	`, resourceType, resourceID, operation).Scan(&count)

	require.NoError(t, err, "failed to query outbox")
	require.Equal(t, 1, count, "expected exactly 1 outbox entry, found %d", count)

	t.Logf("✅ Outbox entry exists: type=%s, id=%s, operation=%s", resourceType, resourceID, operation)
}

// AssertOutboxStatus asserts that an outbox entry has a specific status
func AssertOutboxStatus(
	t *testing.T,
	db *sql.DB,
	entryID uuid.UUID,
	expectedStatus domain.OutboxStatus,
) {
	t.Helper()

	var status domain.OutboxStatus
	err := db.QueryRow(`
		SELECT status
		FROM sync_outbox
		WHERE id = $1
	`, entryID).Scan(&status)

	require.NoError(t, err, "failed to query outbox status")
	require.Equal(t, expectedStatus, status, "outbox status mismatch")

	t.Logf("✅ Outbox entry %s has status: %s", entryID, status)
}

// AssertNoOutboxEntry asserts that NO outbox entry exists for resource
func AssertNoOutboxEntry(
	t *testing.T,
	db *sql.DB,
	resourceType, operation string,
) {
	t.Helper()

	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM sync_outbox
		WHERE resource_type = $1
		  AND operation = $2
	`, resourceType, operation).Scan(&count)

	require.NoError(t, err, "failed to query outbox")
	require.Equal(t, 0, count, "expected no outbox entries, found %d", count)

	t.Logf("✅ No outbox entry exists: type=%s, operation=%s", resourceType, operation)
}

// CreateTestHostBinding creates a host_binding entry using composite keys
// Matches actual schema from migration 010+
func CreateTestHostBinding(
	t *testing.T,
	db *sql.DB,
	namespace, name string,
	hostKey HostKey,
	agKey AddressGroupKey,
	resourceVersion int64,
) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO host_bindings (
			namespace, name,
			host_namespace, host_name,
			address_group_namespace, address_group_name,
			resource_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, namespace, name,
		hostKey.Namespace, hostKey.Name,
		agKey.Namespace, agKey.Name,
		resourceVersion)

	require.NoError(t, err, "failed to create test host binding")

	t.Logf("Created host_binding: %s/%s → Host(%s/%s) → AG(%s/%s)",
		namespace, name,
		hostKey.Namespace, hostKey.Name,
		agKey.Namespace, agKey.Name)
}

// CreateTestNetworkBinding creates a network_binding entry using composite keys
// Matches actual schema from migration 010+
func CreateTestNetworkBinding(
	t *testing.T,
	db *sql.DB,
	namespace, name string,
	networkKey NetworkKey,
	agKey AddressGroupKey,
	resourceVersion int64,
) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO network_bindings (
			namespace, name,
			network_namespace, network_name,
			address_group_namespace, address_group_name,
			resource_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, namespace, name,
		networkKey.Namespace, networkKey.Name,
		agKey.Namespace, agKey.Name,
		resourceVersion)

	require.NoError(t, err, "failed to create test network binding")

	t.Logf("Created network_binding: %s/%s → Network(%s/%s) → AG(%s/%s)",
		namespace, name,
		networkKey.Namespace, networkKey.Name,
		agKey.Namespace, agKey.Name)
}
