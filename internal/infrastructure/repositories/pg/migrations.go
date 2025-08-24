package pg

import (
	"github.com/pkg/errors"
)

// Migrations are now handled by separate Goose container (sgroups pattern)
// //go:embed migrations/*.sql
// var migrationsFS embed.FS

// RunMigrations - DEPRECATED: Now handled by separate Goose container (sgroups pattern)
func RunMigrations(connString string) error {
	// Migrations are now handled by separate Kubernetes Job with Goose
	// This function is no longer used and will be removed
	return errors.New("RunMigrations is deprecated - migrations are handled by separate Goose container")
}
