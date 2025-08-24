package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
	// "netguard-pg-backend/internal/infrastructure/repositories/pg" // TEMPORARILY COMMENTED FOR DEBUGGING
)

// RepositoryType represents the type of repository backend
type RepositoryType string

const (
	RepositoryTypeMemory     RepositoryType = "memory"
	RepositoryTypePostgreSQL RepositoryType = "postgresql"
)

// Config holds configuration for repository factory
type Config struct {
	Type RepositoryType `yaml:"type"`

	// PostgreSQL configuration - COMMENTED OUT FOR DEBUGGING
	// PostgreSQL *pg.ConnectionConfig `yaml:"postgresql,omitempty"`

	// Memory configuration (if any specific settings needed)
	Memory *MemoryConfig `yaml:"memory,omitempty"`
}

// MemoryConfig holds memory repository configuration
type MemoryConfig struct {
	// Future memory-specific settings
}

// Factory creates repository instances based on configuration
type Factory struct {
	config Config
}

// NewFactory creates a new repository factory
func NewFactory(config Config) *Factory {
	return &Factory{
		config: config,
	}
}

// CreateRegistry creates a registry based on the configured type
func (f *Factory) CreateRegistry(ctx context.Context) (ports.Registry, error) {
	switch f.config.Type {
	case RepositoryTypeMemory:
		return f.createMemoryRegistry()
	case RepositoryTypePostgreSQL:
		// return f.createPostgreSQLRegistry(ctx) // COMMENTED OUT FOR DEBUGGING
		return nil, errors.New("PostgreSQL temporarily disabled for debugging")
	default:
		return nil, fmt.Errorf("unsupported repository type: %s", f.config.Type)
	}
}

// createMemoryRegistry creates an in-memory registry
func (f *Factory) createMemoryRegistry() (ports.Registry, error) {
	return mem.NewRegistry(), nil
}

// createPostgreSQLRegistry creates a PostgreSQL registry - COMMENTED OUT FOR DEBUGGING
/*
func (f *Factory) createPostgreSQLRegistry(ctx context.Context) (ports.Registry, error) {
	if f.config.PostgreSQL == nil {
		return nil, errors.New("PostgreSQL configuration is required")
	}

	// Apply defaults if not set
	config := *f.config.PostgreSQL
	if config.MaxConns == 0 {
		config.MaxConns = 30
	}
	if config.MinConns == 0 {
		config.MinConns = 3
	}
	if config.MaxConnLifetime == 0 {
		config.MaxConnLifetime = time.Hour
	}
	if config.MaxConnIdleTime == 0 {
		config.MaxConnIdleTime = 30 * time.Minute
	}
	if config.HealthTimeout == 0 {
		config.HealthTimeout = 30 * time.Second
	}

	// Create connection manager
	connManager := pg.NewConnectionManager(config)

	// Connect to database
	if err := connManager.Connect(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to connect to PostgreSQL")
	}

	// Create registry
	registry := pg.NewRegistry(connManager)

	return registry, nil
}
*/

// DefaultConfig returns default configuration for the repository factory
func DefaultConfig() Config {
	return Config{
		Type: RepositoryTypeMemory, // Default to memory for backward compatibility
		// PostgreSQL: &pg.ConnectionConfig{ // COMMENTED OUT FOR DEBUGGING
		//	URI:             "postgres://postgres:postgres@localhost:5432/netguard?sslmode=disable",
		//	MaxConns:        30,
		//	MinConns:        3,
		//	MaxConnLifetime: time.Hour,
		//	MaxConnIdleTime: 30 * time.Minute,
		//	HealthTimeout:   30 * time.Second,
		// },
		Memory: &MemoryConfig{},
	}
}

// PostgreSQLConfig creates a configuration for PostgreSQL backend - COMMENTED OUT FOR DEBUGGING
/*
func PostgreSQLConfig(uri string) Config {
	return Config{
		Type: RepositoryTypePostgreSQL,
		PostgreSQL: &pg.ConnectionConfig{
			URI:             uri,
			MaxConns:        30,
			MinConns:        3,
			MaxConnLifetime: time.Hour,
			MaxConnIdleTime: 30 * time.Minute,
			HealthTimeout:   30 * time.Second,
		},
	}
}
*/

// NewMemoryConfig creates a configuration for memory backend
func NewMemoryConfig() Config {
	return Config{
		Type:   RepositoryTypeMemory,
		Memory: &MemoryConfig{},
	}
}

// WithMigrations runs database migrations if using PostgreSQL - COMMENTED OUT FOR DEBUGGING
/*
func (f *Factory) WithMigrations(ctx context.Context, migrationsDir string) error {
	if f.config.Type != RepositoryTypePostgreSQL {
		return nil // No migrations needed for memory backend
	}

	if f.config.PostgreSQL == nil {
		return errors.New("PostgreSQL configuration is required for migrations")
	}

	// Create temporary connection manager for migrations
	connManager := pg.NewConnectionManager(*f.config.PostgreSQL)

	if err := connManager.Connect(ctx); err != nil {
		return errors.Wrap(err, "failed to connect for migrations")
	}
	defer connManager.Close()

	return connManager.RunMigrations(ctx, migrationsDir)
}
*/
