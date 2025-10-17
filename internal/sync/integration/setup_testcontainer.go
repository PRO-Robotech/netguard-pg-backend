package integration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestContainer holds embedded PostgreSQL + mock SGROUP infrastructure
type TestContainer struct {
	PostgresContainer *postgres.PostgresContainer
	ConnectionString  string
	DB                *sql.DB
	MockSGROUP        *MockSGROUPServer
	Cleanup           func()
}

// SetupTestEnvironment creates PostgreSQL container + mock SGROUP for integration tests
//
// This provides:
// - Isolated PostgreSQL 16 container (embedded, no shared state)
// - All migrations applied automatically
// - Mock SGROUP HTTP server with failure mode simulation
// - Automatic cleanup on test completion
//
// Usage:
//
//	tc := SetupTestEnvironment(t)
//	defer tc.Cleanup()
//
//	// Use tc.DB for database operations
//	// Use tc.MockSGROUP for SGROUP client simulation
func SetupTestEnvironment(t *testing.T) *TestContainer {
	t.Helper()
	ctx := context.Background()

	t.Log("🚀 Starting test environment setup...")

	// 1. Start PostgreSQL container
	t.Log("  📦 Starting PostgreSQL container...")
	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		postgres.WithDatabase("netguard_test"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	require.NoError(t, err, "failed to start postgres container")
	t.Log("  ✅ PostgreSQL container started")

	// 2. Get connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "failed to get connection string")

	// 3. Connect to database
	t.Log("  🔌 Connecting to database...")
	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err, "failed to open database")

	// Verify connection
	err = db.PingContext(ctx)
	require.NoError(t, err, "failed to ping database")
	t.Log("  ✅ Database connection established")

	// 4. Apply migrations (minimal schema + outbox migrations only)
	t.Log("  📜 Applying migrations...")
	tcApplyMigrations(t, db)
	t.Log("  ✅ Migrations applied")

	// 5. Setup mock SGROUP server
	t.Log("  🎭 Starting mock SGROUP server...")
	mockSGROUP := NewMockSGROUPServer(t)
	mockSGROUP.Start()
	t.Logf("  ✅ Mock SGROUP server started at %s", mockSGROUP.URL())

	// 6. Cleanup function
	cleanup := func() {
		t.Log("🧹 Cleaning up test environment...")
		mockSGROUP.Stop()
		db.Close()
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("  ⚠️  Failed to terminate postgres container: %v", err)
		}
		t.Log("✅ Test environment cleanup complete")
	}

	t.Log("✅ Test environment setup complete")

	return &TestContainer{
		PostgresContainer: pgContainer,
		ConnectionString:  connStr,
		DB:                db,
		MockSGROUP:        mockSGROUP,
		Cleanup:           cleanup,
	}
}

// tcApplyMigrations applies ALL migrations (001-030) using goose
//
// NEW APPROACH (per user request):
// 1. Start embedded PostgreSQL via testcontainers
// 2. PostgreSQL creates database with default user (postgres)
// 3. Use goose.Up(db, "./migrations") to apply ALL migrations
//
// This is THE CORRECT WAY - goose handles StatementBegin/End properly!
func tcApplyMigrations(t *testing.T, db *sql.DB) {
	t.Helper()

	// Find migrations directory
	migrationsDir := tcFindMigrationsDir(t)
	t.Logf("    📂 Found migrations directory: %s", migrationsDir)

	// Set goose dialect
	if err := goose.SetDialect("postgres"); err != nil {
		require.NoError(t, err, "failed to set goose dialect")
	}

	// Apply ALL migrations using goose (handles StatementBegin/End correctly!)
	t.Log("    📜 Applying ALL migrations (001-030) using goose...")

	if err := goose.Up(db, migrationsDir); err != nil {
		require.NoError(t, err, "failed to apply migrations with goose")
	}

	t.Log("    ✅ All migrations applied successfully via goose")

	// Verify critical tables exist
	tcVerifySchema(t, db)
}

// tcFindMigrationsDir locates the migrations directory
func tcFindMigrationsDir(t *testing.T) string {
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

// tcVerifySchema validates critical tables were created correctly
func tcVerifySchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()

	// Verify sync_outbox table exists with correct schema
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'sync_outbox'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists, "sync_outbox table should exist")

	// Verify critical columns exist (from migration 030)
	var hasNamespace, hasName bool
	err = db.QueryRowContext(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_outbox' AND column_name = 'resource_namespace'),
			EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_outbox' AND column_name = 'resource_name')
	`).Scan(&hasNamespace, &hasName)
	require.NoError(t, err)
	require.True(t, hasNamespace, "sync_outbox should have resource_namespace column")
	require.True(t, hasName, "sync_outbox should have resource_name column")

	t.Log("    ✅ Schema validation passed")
}
