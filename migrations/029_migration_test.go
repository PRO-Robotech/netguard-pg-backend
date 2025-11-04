package migrations

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestMigration029_SchemaCreation verifies Migration 029 creates all expected database objects
func TestMigration029_SchemaCreation(t *testing.T) {
	ctx := context.Background()

	// Setup isolated PostgreSQL container
	tc := setupTestContainer(t)
	defer tc.Cleanup()

	db := tc.DB

	t.Run("Tables", func(t *testing.T) {
		// Verify svc_svc_rules table exists
		var exists bool
		err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = 'svc_svc_rules'
			)
		`).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "svc_svc_rules table should exist")

		// Verify service_rule_refs table exists
		err = db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = 'service_rule_refs'
			)
		`).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "service_rule_refs table should exist")

		t.Log("✅ Both tables created successfully")
	})

	t.Run("Columns", func(t *testing.T) {
		// Verify svc_svc_rules columns (JSONB for service refs!)
		var dataType string
		err := db.QueryRowContext(ctx, `
			SELECT data_type FROM information_schema.columns
			WHERE table_name = 'svc_svc_rules' AND column_name = 'service_from_ref'
		`).Scan(&dataType)
		require.NoError(t, err)
		assert.Equal(t, "jsonb", dataType, "service_from_ref should be JSONB")

		err = db.QueryRowContext(ctx, `
			SELECT data_type FROM information_schema.columns
			WHERE table_name = 'svc_svc_rules' AND column_name = 'service_to_ref'
		`).Scan(&dataType)
		require.NoError(t, err)
		assert.Equal(t, "jsonb", dataType, "service_to_ref should be JSONB")

		// Verify services table has xSvcSvcRules columns
		var hasFrom, hasTo bool
		err = db.QueryRowContext(ctx, `
			SELECT
				EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'services' AND column_name = 'xsvcsvc_rules_as_from'),
				EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'services' AND column_name = 'xsvcsvc_rules_as_to')
		`).Scan(&hasFrom, &hasTo)
		require.NoError(t, err)
		assert.True(t, hasFrom, "services should have xsvcsvc_rules_as_from column")
		assert.True(t, hasTo, "services should have xsvcsvc_rules_as_to column")

		t.Log("✅ All columns created with correct types")
	})

	t.Run("Indexes", func(t *testing.T) {
		// Count indexes on svc_svc_rules
		var indexCount int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM pg_indexes
			WHERE tablename = 'svc_svc_rules'
		`).Scan(&indexCount)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, indexCount, 7, "should have at least 7 indexes on svc_svc_rules (1 PK + 1 UNIQUE + 5 JSONB)")

		// Verify UNIQUE index exists
		var uniqueExists bool
		err = db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes
				WHERE tablename = 'svc_svc_rules'
				AND indexname = 'idx_svc_svc_rules_unique_pair'
			)
		`).Scan(&uniqueExists)
		require.NoError(t, err)
		assert.True(t, uniqueExists, "UNIQUE index should exist")

		// Verify JSONB indexes exist
		jsonbIndexes := []string{
			"idx_svc_svc_rules_service_from_name",
			"idx_svc_svc_rules_service_from_namespace",
			"idx_svc_svc_rules_service_to_name",
			"idx_svc_svc_rules_service_to_namespace",
		}
		for _, indexName := range jsonbIndexes {
			var exists bool
			err = db.QueryRowContext(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM pg_indexes
					WHERE indexname = $1
				)
			`, indexName).Scan(&exists)
			require.NoError(t, err)
			assert.True(t, exists, "JSONB index %s should exist", indexName)
		}

		// Verify GIN indexes on services table
		var ginCount int
		err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM pg_indexes
			WHERE tablename = 'services'
			AND indexname LIKE '%xsvcsvc_rules%'
		`).Scan(&ginCount)
		require.NoError(t, err)
		assert.Equal(t, 2, ginCount, "should have 2 GIN indexes for xSvcSvcRules")

		t.Log("✅ All 12 indexes created successfully")
	})

	t.Run("Triggers", func(t *testing.T) {
		// Verify all 4 triggers exist
		triggers := []string{
			"trg_sync_service_rule_refs",
			"trg_svcsvc_rule_upsert_outbox",
			"trg_svcsvc_rule_delete_outbox",
			"trg_update_service_xsvcsvc_rules",
		}

		for _, triggerName := range triggers {
			var exists bool
			err := db.QueryRowContext(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM pg_trigger
					WHERE tgname = $1
				)
			`, triggerName).Scan(&exists)
			require.NoError(t, err)
			assert.True(t, exists, "trigger %s should exist", triggerName)
		}

		t.Log("✅ All 4 triggers created successfully")
	})

	t.Run("Functions", func(t *testing.T) {
		// Verify all 4 trigger functions exist
		functions := []string{
			"sync_service_rule_refs",
			"trigger_svcsvc_rule_upsert_outbox",
			"trigger_svcsvc_rule_delete_outbox",
			"update_service_xsvcsvc_rules",
		}

		for _, funcName := range functions {
			var exists bool
			err := db.QueryRowContext(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM pg_proc
					WHERE proname = $1
				)
			`, funcName).Scan(&exists)
			require.NoError(t, err)
			assert.True(t, exists, "function %s should exist", funcName)
		}

		t.Log("✅ All 4 functions created successfully")
	})
}

// TestMigration029_FunctionalTests verifies triggers and business logic work correctly
func TestMigration029_FunctionalTests(t *testing.T) {
	ctx := context.Background()

	// Setup isolated PostgreSQL container
	tc := setupTestContainer(t)
	defer tc.Cleanup()

	db := tc.DB

	// Prerequisite: Create 2 services for testing
	setupTestServices(t, db)

	t.Run("INSERT_Rule_Creates_Junction_Outbox_XSvcSvcRules", func(t *testing.T) {
		// Insert a SvcSvcRule
		ruleID := insertTestRule(t, db, "test-ns", "rule-001", "service-a", "service-b")

		// Verify junction table has 2 entries (SERVICE_FROM + SERVICE_TO)
		var junctionCount int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM service_rule_refs
			WHERE rule_id = $1
		`, ruleID).Scan(&junctionCount)
		require.NoError(t, err)
		assert.Equal(t, 2, junctionCount, "should have 2 junction entries")

		// Verify roles are correct
		var fromCount, toCount int
		err = db.QueryRowContext(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE role = 'SERVICE_FROM'),
				COUNT(*) FILTER (WHERE role = 'SERVICE_TO')
			FROM service_rule_refs
			WHERE rule_id = $1
		`, ruleID).Scan(&fromCount, &toCount)
		require.NoError(t, err)
		assert.Equal(t, 1, fromCount, "should have 1 SERVICE_FROM entry")
		assert.Equal(t, 1, toCount, "should have 1 SERVICE_TO entry")

		// Verify outbox entry created
		var outboxCount int
		err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sync_outbox
			WHERE resource_type = 'SvcSvcRule'
			AND resource_namespace = 'test-ns'
			AND resource_name = 'rule-001'
			AND operation = 'CREATE'
		`).Scan(&outboxCount)
		require.NoError(t, err)
		assert.Equal(t, 1, outboxCount, "should have 1 outbox entry for CREATE")

		// Verify xSvcSvcRules arrays updated in services table
		var fromRulesJSON, toRulesJSON string
		err = db.QueryRowContext(ctx, `
			SELECT xsvcsvc_rules_as_from::text, xsvcsvc_rules_as_to::text
			FROM services
			WHERE namespace = 'test-ns' AND name = 'service-a'
		`).Scan(&fromRulesJSON, &toRulesJSON)
		require.NoError(t, err)
		assert.Contains(t, fromRulesJSON, "rule-001", "service-a should have rule-001 in as_from")

		err = db.QueryRowContext(ctx, `
			SELECT xsvcsvc_rules_as_from::text, xsvcsvc_rules_as_to::text
			FROM services
			WHERE namespace = 'test-ns' AND name = 'service-b'
		`).Scan(&fromRulesJSON, &toRulesJSON)
		require.NoError(t, err)
		assert.Contains(t, toRulesJSON, "rule-001", "service-b should have rule-001 in as_to")

		t.Log("✅ INSERT trigger works: junction + outbox + xSvcSvcRules")
	})

	t.Run("UPDATE_Mutable_Fields_Creates_Outbox", func(t *testing.T) {
		// Update mutable fields (action, priority, logs, trace) - use different service pair
		ruleID := insertTestRule(t, db, "test-ns", "rule-002", "service-a", "service-c")

		_, err := db.ExecContext(ctx, `
			UPDATE svc_svc_rules
			SET action = 'DROP', priority = 100, logs = true, trace = true
			WHERE id = $1
		`, ruleID)
		require.NoError(t, err)

		// Verify UPDATE outbox entry created
		var outboxCount int
		err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sync_outbox
			WHERE resource_type = 'SvcSvcRule'
			AND resource_namespace = 'test-ns'
			AND resource_name = 'rule-002'
			AND operation = 'UPDATE'
		`).Scan(&outboxCount)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, outboxCount, 1, "should have at least 1 outbox entry for UPDATE")

		t.Log("✅ UPDATE trigger works: outbox entry created")
	})

	t.Run("UPDATE_Immutable_Fields_Fails", func(t *testing.T) {
		// Insert rule - use different service pair
		ruleID := insertTestRule(t, db, "test-ns", "rule-003", "service-b", "service-c")

		// Try to update immutable fields (service_from_ref, service_to_ref)
		_, err := db.ExecContext(ctx, `
			UPDATE svc_svc_rules
			SET service_from_ref = '{"name": "service-c", "namespace": "test-ns"}'::jsonb
			WHERE id = $1
		`, ruleID)

		// Should fail with immutability error
		assert.Error(t, err, "updating service_from_ref should fail")
		assert.Contains(t, err.Error(), "immutable", "error should mention immutability")

		t.Log("✅ Immutability enforcement works")
	})

	t.Run("DELETE_Rule_Cleans_Junction_Outbox_XSvcSvcRules", func(t *testing.T) {
		// Insert rule - use different service pair
		ruleID := insertTestRule(t, db, "test-ns", "rule-004", "service-c", "service-a")

		// Verify junction entries exist
		var junctionBefore int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM service_rule_refs WHERE rule_id = $1
		`, ruleID).Scan(&junctionBefore)
		require.NoError(t, err)
		assert.Equal(t, 2, junctionBefore)

		// Delete rule
		_, err = db.ExecContext(ctx, `DELETE FROM svc_svc_rules WHERE id = $1`, ruleID)
		require.NoError(t, err)

		// Verify junction entries removed (CASCADE)
		var junctionAfter int
		err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM service_rule_refs WHERE rule_id = $1
		`, ruleID).Scan(&junctionAfter)
		require.NoError(t, err)
		assert.Equal(t, 0, junctionAfter, "junction entries should be removed")

		// Verify DELETE outbox entry created
		var outboxCount int
		err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sync_outbox
			WHERE resource_type = 'SvcSvcRule'
			AND resource_namespace = 'test-ns'
			AND resource_name = 'rule-004'
			AND operation = 'DELETE'
		`).Scan(&outboxCount)
		require.NoError(t, err)
		assert.Equal(t, 1, outboxCount, "should have 1 DELETE outbox entry")

		// Verify xSvcSvcRules arrays cleaned in services table
		// (rule-004 should be removed from service-c as_from)
		var fromRulesJSON string
		err = db.QueryRowContext(ctx, `
			SELECT xsvcsvc_rules_as_from::text
			FROM services
			WHERE namespace = 'test-ns' AND name = 'service-c'
		`).Scan(&fromRulesJSON)
		require.NoError(t, err)
		assert.NotContains(t, fromRulesJSON, "rule-004", "rule-004 should be removed from service-c")

		t.Log("✅ DELETE trigger works: junction cleanup + outbox + xSvcSvcRules cleanup")
	})

	t.Run("UNIQUE_Constraint_Prevents_Duplicates", func(t *testing.T) {
		// First rule was already inserted in test 1: rule-001 from service-a to service-b
		// Try to insert duplicate (same namespace + service pair, different rule name)
		_, err := db.ExecContext(ctx, `
			INSERT INTO svc_svc_rules (namespace, name, service_from_ref, service_to_ref, action, priority)
			VALUES (
				'test-ns',
				'rule-006-duplicate',
				'{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "service-a", "namespace": "test-ns"}'::jsonb,
				'{"apiVersion": "netguard.sgroups.io/v1beta1", "kind": "Service", "name": "service-b", "namespace": "test-ns"}'::jsonb,
				'DROP',
				100
			)
		`)

		// Should fail with unique constraint violation
		assert.Error(t, err, "duplicate rule should fail")
		assert.Contains(t, err.Error(), "unique", "error should mention unique constraint")

		t.Log("✅ UNIQUE constraint prevents duplicates")
	})
}

// setupTestContainer creates isolated PostgreSQL container with all migrations applied
func setupTestContainer(t *testing.T) *TestContainer {
	t.Helper()
	ctx := context.Background()

	t.Log("🚀 Starting PostgreSQL testcontainer...")

	// Start PostgreSQL container
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
	require.NoError(t, err)

	// Get connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Connect to database
	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)

	err = db.PingContext(ctx)
	require.NoError(t, err)

	// Apply all migrations (001-029) using goose
	t.Log("📜 Applying all migrations (001-029)...")
	applyMigrations(t, db)

	cleanup := func() {
		t.Log("🧹 Cleaning up testcontainer...")
		db.Close()
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("⚠️  Failed to terminate container: %v", err)
		}
	}

	t.Log("✅ Testcontainer ready")

	return &TestContainer{
		DB:      db,
		Cleanup: cleanup,
	}
}

// applyMigrations applies all migrations using goose
func applyMigrations(t *testing.T, db *sql.DB) {
	t.Helper()

	// Find migrations directory
	migrationsDir := findMigrationsDir(t)

	// Set goose dialect
	err := goose.SetDialect("postgres")
	require.NoError(t, err)

	// Apply all migrations
	err = goose.Up(db, migrationsDir)
	require.NoError(t, err)

	t.Log("✅ All migrations applied")
}

// findMigrationsDir locates the migrations directory
func findMigrationsDir(t *testing.T) string {
	t.Helper()

	cwd, err := os.Getwd()
	require.NoError(t, err)

	// migrations/ is in the same directory as this test file
	return cwd
}

// setupTestServices creates 2 test services for rule testing
func setupTestServices(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()

	// Create k8s_metadata entries first
	var rv1, rv2 int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO k8s_metadata (uid, created_at)
		VALUES ('550e8400-e29b-41d4-a716-446655440001'::uuid, NOW())
		RETURNING resource_version
	`).Scan(&rv1)
	require.NoError(t, err)

	err = db.QueryRowContext(ctx, `
		INSERT INTO k8s_metadata (uid, created_at)
		VALUES ('550e8400-e29b-41d4-a716-446655440002'::uuid, NOW())
		RETURNING resource_version
	`).Scan(&rv2)
	require.NoError(t, err)

	// Create service-a with all JSONB columns
	_, err = db.ExecContext(ctx, `
		INSERT INTO services (
			namespace, name, description, ingress_ports, resource_version,
			address_groups, aggregated_address_groups,
			xsvcsvc_rules_as_from, xsvcsvc_rules_as_to
		)
		VALUES (
			'test-ns',
			'service-a',
			'Test service A',
			'[]'::jsonb,
			$1,
			'[]'::jsonb,
			'[]'::jsonb,
			'[]'::jsonb,
			'[]'::jsonb
		)
		ON CONFLICT (namespace, name) DO NOTHING
	`, rv1)
	require.NoError(t, err)

	// Create service-b with all JSONB columns
	_, err = db.ExecContext(ctx, `
		INSERT INTO services (
			namespace, name, description, ingress_ports, resource_version,
			address_groups, aggregated_address_groups,
			xsvcsvc_rules_as_from, xsvcsvc_rules_as_to
		)
		VALUES (
			'test-ns',
			'service-b',
			'Test service B',
			'[]'::jsonb,
			$1,
			'[]'::jsonb,
			'[]'::jsonb,
			'[]'::jsonb,
			'[]'::jsonb
		)
		ON CONFLICT (namespace, name) DO NOTHING
	`, rv2)
	require.NoError(t, err)

	// Create service-c with all JSONB columns (for additional test cases)
	var rv3 string
	err = db.QueryRowContext(ctx, `
		INSERT INTO k8s_metadata (uid, created_at)
		VALUES ('550e8400-e29b-41d4-a716-446655440003'::uuid, NOW())
		RETURNING resource_version
	`).Scan(&rv3)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO services (
			namespace, name, description, ingress_ports, resource_version,
			address_groups, aggregated_address_groups,
			xsvcsvc_rules_as_from, xsvcsvc_rules_as_to
		)
		VALUES (
			'test-ns',
			'service-c',
			'Test service C',
			'[]'::jsonb,
			$1,
			'[]'::jsonb,
			'[]'::jsonb,
			'[]'::jsonb,
			'[]'::jsonb
		)
		ON CONFLICT (namespace, name) DO NOTHING
	`, rv3)
	require.NoError(t, err)

	t.Log("✅ Test services created")
}

// insertTestRule inserts a test rule and returns its ID
func insertTestRule(t *testing.T, db *sql.DB, namespace, name, serviceFrom, serviceTo string) string {
	t.Helper()
	ctx := context.Background()

	// DIAGNOSTIC: Check column types before INSERT
	var serviceFromType, serviceToType string
	err := db.QueryRowContext(ctx, `
		SELECT
			(SELECT data_type FROM information_schema.columns WHERE table_name = 'svc_svc_rules' AND column_name = 'service_from_ref'),
			(SELECT data_type FROM information_schema.columns WHERE table_name = 'svc_svc_rules' AND column_name = 'service_to_ref')
	`).Scan(&serviceFromType, &serviceToType)
	if err == nil {
		t.Logf("DIAGNOSTIC: service_from_ref type = %s, service_to_ref type = %s", serviceFromType, serviceToType)
	}

	// Build JSONB values inline with literal strings to avoid parameter type issues
	query := `
		INSERT INTO svc_svc_rules (namespace, name, service_from_ref, service_to_ref, action, priority)
		VALUES (
			'` + namespace + `',
			'` + name + `',
			'{"apiVersion":"netguard.sgroups.io/v1beta1","kind":"Service","name":"` + serviceFrom + `","namespace":"` + namespace + `"}'::jsonb,
			'{"apiVersion":"netguard.sgroups.io/v1beta1","kind":"Service","name":"` + serviceTo + `","namespace":"` + namespace + `"}'::jsonb,
			'ACCEPT',
			0
		)
		RETURNING id
	`

	var ruleID string
	err = db.QueryRowContext(ctx, query).Scan(&ruleID)
	require.NoError(t, err)

	return ruleID
}

// TestContainer holds test database connection
type TestContainer struct {
	DB      *sql.DB
	Cleanup func()
}
