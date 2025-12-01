package helpers

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg"
	"netguard-pg-backend/internal/sync/monitor"
	"netguard-pg-backend/internal/sync/syncers"
	"netguard-pg-backend/internal/sync/worker"
)

// TestEnvironment holds all components needed for E2E testing
type TestEnvironment struct {
	DB         *sql.DB
	Pool       *pgxpool.Pool
	Worker     *worker.OutboxWorker
	MockSGroup *MockSGroupGateway
	Logger     *zap.Logger
	Registry   ports.Registry
	CancelFunc context.CancelFunc
	WorkerCtx  context.Context
}

// StartTestEnvironment initializes the full E2E test environment
func StartTestEnvironment(t *testing.T) *TestEnvironment {
	// Get connection string
	connStr := os.Getenv("TEST_DB_URL")
	if connStr == "" {
		connStr = "postgres://test:test@localhost:5433/netguard_test?sslmode=disable"
	}

	// Connect to test database (for helpers)
	db := connectToTestDB(t, connStr)

	// Clean database before test
	cleanDatabase(t, db)

	// Initialize logger
	zapLogger := zap.NewNop()
	logrLogger := zapr.NewLogger(zapLogger)

	// Initialize registry from URI
	ctx := context.Background()
	registry, err := pg.NewRegistryFromURI(ctx, connStr)
	require.NoError(t, err, "Failed to create registry")

	// Get pool from registry
	pool := registry.Pool()

	// Initialize mock SGROUP gateway
	mockSGroup := NewMockSGroupGateway()

	// Initialize syncers with logr.Logger
	hostSyncer := syncers.NewHostSyncer(mockSGroup, logrLogger)
	addressGroupSyncer := syncers.NewAddressGroupSyncer(mockSGroup, logrLogger)
	networkSyncer := syncers.NewNetworkSyncer(mockSGroup, logrLogger)
	serviceSyncer := syncers.NewServiceSyncer(mockSGroup, logrLogger)
	svcSvcRuleSyncer := syncers.NewSvcSvcRuleSyncer(mockSGroup, logrLogger)
	svcFqdnRuleSyncer := syncers.NewSvcFqdnRuleSyncer(mockSGroup, logrLogger)

	// Initialize OutboxWorker
	workerConfig := worker.DefaultConfig()
	workerConfig.PollInterval = 1000000000 // 1 second (in nanoseconds)
	workerConfig.BatchSize = 10
	workerConfig.MetricsEnabled = false

	connMonitor := monitor.NewSGroupConnectionMonitor(mockSGroup, monitor.DefaultConfig(), zapLogger)

	outboxWorker := worker.NewOutboxWorker(
		pool,
		registry,
		hostSyncer,
		addressGroupSyncer,
		networkSyncer,
		serviceSyncer,
		svcSvcRuleSyncer,
		svcFqdnRuleSyncer,
		nil, // conditionManager
		zapLogger,
		workerConfig,
		connMonitor,
		nil, // portMappingRegenerator
	)

	// Start worker in background
	workerCtx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := outboxWorker.Start(workerCtx); err != nil && err != context.Canceled {
			t.Logf("Worker stopped with error: %v", err)
		}
	}()

	return &TestEnvironment{
		DB:         db,
		Pool:       pool,
		Worker:     outboxWorker,
		MockSGroup: mockSGroup,
		Logger:     zapLogger,
		Registry:   registry,
		CancelFunc: cancel,
		WorkerCtx:  workerCtx,
	}
}

// Cleanup tears down the test environment
func (env *TestEnvironment) Cleanup() {
	// Stop worker
	env.CancelFunc()
	env.Worker.Stop()
	<-env.Worker.Done()

	// Close registry (this closes the pool)
	if env.Registry != nil {
		env.Registry.Close()
	}

	// Close database connection
	if env.DB != nil {
		env.DB.Close()
	}

	// Close mock SGROUP
	env.MockSGroup.Close()
}

// ProcessOnce manually triggers one batch processing cycle (for debugging)
func (env *TestEnvironment) ProcessOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := env.Worker.ProcessOnce(ctx)
	require.NoError(t, err, "Failed to process batch")
}

// Helper functions

func connectToTestDB(t *testing.T, connStr string) *sql.DB {
	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err, "Failed to connect to test database")

	err = db.Ping()
	require.NoError(t, err, "Failed to ping test database")

	return db
}

func cleanDatabase(t *testing.T, db *sql.DB) {
	// Order matters: delete child tables first
	tables := []string{
		"sync_outbox",
		"host_bindings",
		"network_bindings",
		"ieagag_rules",
		"service_specs",
		"hosts",
		"networks",
		"services",
		"address_groups",
		"k8s_metadata",
	}

	for _, table := range tables {
		query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
		_, err := db.Exec(query)
		// Ignore "relation does not exist" errors (table might not exist in test schema)
		if err != nil && !isTableNotExistError(err) {
			t.Logf("Warning: Failed to truncate table %s: %v", table, err)
		}
	}
}

func isTableNotExistError(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL error code 42P01 = relation does not exist
	errStr := err.Error()
	return contains(errStr, "does not exist")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && hasSubstring(s, substr))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
