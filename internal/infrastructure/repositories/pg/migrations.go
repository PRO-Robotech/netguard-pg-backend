package pg

//
//import (
//	"context"
//	"embed"
//	"fmt"
//	"io/fs"
//	"path"
//	"sort"
//	"strings"
//
//	"github.com/jackc/pgx/v5"
//	"github.com/pkg/errors"
//)
//
////go:embed migrations/*.sql
//var migrationsFS embed.FS
//
//// RunMigrations runs the database migrations
//func RunMigrations(connString string) error {
//	ctx := context.Background()
//
//	// Connect to the database
//	conn, err := pgx.Connect(ctx, connString)
//	if err != nil {
//		return errors.Wrap(err, "failed to connect to database")
//	}
//	defer conn.Close(ctx)
//
//	// Create migrations table if it doesn't exist
//	_, err = conn.Exec(ctx, `
//		CREATE TABLE IF NOT EXISTS netguard.migrations (
//			id SERIAL PRIMARY KEY,
//			name TEXT NOT NULL UNIQUE,
//			applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
//		)
//	`)
//	if err != nil {
//		// If schema doesn't exist, create it
//		if strings.Contains(err.Error(), "schema") && strings.Contains(err.Error(), "does not exist") {
//			_, err = conn.Exec(ctx, `CREATE SCHEMA netguard`)
//			if err != nil {
//				return errors.Wrap(err, "failed to create schema")
//			}
//
//			// Try creating migrations table again
//			_, err = conn.Exec(ctx, `
//				CREATE TABLE IF NOT EXISTS netguard.migrations (
//					id SERIAL PRIMARY KEY,
//					name TEXT NOT NULL UNIQUE,
//					applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
//				)
//			`)
//			if err != nil {
//				return errors.Wrap(err, "failed to create migrations table")
//			}
//		} else {
//			return errors.Wrap(err, "failed to create migrations table")
//		}
//	}
//
//	// Get list of applied migrations
//	rows, err := conn.Query(ctx, `SELECT name FROM netguard.migrations ORDER BY id`)
//	if err != nil {
//		return errors.Wrap(err, "failed to query migrations")
//	}
//
//	appliedMigrations := make(map[string]bool)
//	for rows.Next() {
//		var name string
//		if err := rows.Scan(&name); err != nil {
//			rows.Close()
//			return errors.Wrap(err, "failed to scan migration name")
//		}
//		appliedMigrations[name] = true
//	}
//	rows.Close()
//
//	if err := rows.Err(); err != nil {
//		return errors.Wrap(err, "error iterating migrations")
//	}
//
//	// Get list of migration files
//	entries, err := fs.ReadDir(migrationsFS, "migrations")
//	if err != nil {
//		return errors.Wrap(err, "failed to read migrations directory")
//	}
//
//	// Sort migrations by name
//	sort.Slice(entries, func(i, j int) bool {
//		return entries[i].Name() < entries[j].Name()
//	})
//
//	// Apply migrations
//	for _, entry := range entries {
//		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
//			continue
//		}
//
//		// Skip already applied migrations
//		if appliedMigrations[entry.Name()] {
//			fmt.Printf("Migration %s already applied\n", entry.Name())
//			continue
//		}
//
//		// Read migration file
//		migrationPath := path.Join("migrations", entry.Name())
//		migrationSQL, err := fs.ReadFile(migrationsFS, migrationPath)
//		if err != nil {
//			return errors.Wrapf(err, "failed to read migration %s", entry.Name())
//		}
//
//		// Begin transaction
//		tx, err := conn.Begin(ctx)
//		if err != nil {
//			return errors.Wrapf(err, "failed to begin transaction for migration %s", entry.Name())
//		}
//
//		// Apply migration
//		_, err = tx.Exec(ctx, string(migrationSQL))
//		if err != nil {
//			tx.Rollback(ctx)
//			return errors.Wrapf(err, "failed to apply migration %s", entry.Name())
//		}
//
//		// Record migration
//		_, err = tx.Exec(ctx, `INSERT INTO netguard.migrations (name) VALUES ($1)`, entry.Name())
//		if err != nil {
//			tx.Rollback(ctx)
//			return errors.Wrapf(err, "failed to record migration %s", entry.Name())
//		}
//
//		// Commit transaction
//		if err := tx.Commit(ctx); err != nil {
//			return errors.Wrapf(err, "failed to commit transaction for migration %s", entry.Name())
//		}
//
//		fmt.Printf("Applied migration %s\n", entry.Name())
//	}
//
//	return nil
//}
