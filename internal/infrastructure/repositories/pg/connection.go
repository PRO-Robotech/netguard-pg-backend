package pg

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pkg/errors"
)

// ConnectionConfig holds PostgreSQL connection configuration
type ConnectionConfig struct {
	URI             string        `yaml:"uri"`
	MaxConns        int32         `yaml:"maxConns"`
	MinConns        int32         `yaml:"minConns"`
	MaxConnLifetime time.Duration `yaml:"maxConnLifetime"`
	MaxConnIdleTime time.Duration `yaml:"maxConnIdleTime"`
	HealthTimeout   time.Duration `yaml:"healthTimeout"`
}

// DefaultConnectionConfig returns production-ready defaults
func DefaultConnectionConfig() ConnectionConfig {
	return ConnectionConfig{
		MaxConns:        30,
		MinConns:        3,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 30 * time.Minute,
		HealthTimeout:   30 * time.Second,
	}
}

// ConnectionManager manages PostgreSQL connections with health monitoring
type ConnectionManager struct {
	config ConnectionConfig
	pool   atomic.Pointer[pgxpool.Pool]

	// Health monitoring
	healthTicker *time.Ticker
	stopHealth   chan struct{}
	isHealthy    atomic.Bool
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(config ConnectionConfig) *ConnectionManager {
	cm := &ConnectionManager{
		config:     config,
		stopHealth: make(chan struct{}),
	}
	cm.isHealthy.Store(false)
	return cm
}

// Connect establishes the database connection with health monitoring
func (cm *ConnectionManager) Connect(ctx context.Context) error {
	poolConfig, err := pgxpool.ParseConfig(cm.config.URI)
	if err != nil {
		return errors.Wrap(err, "failed to parse connection URI")
	}

	// Configure connection pool following sgroups patterns
	poolConfig.MaxConns = cm.config.MaxConns
	poolConfig.MinConns = cm.config.MinConns
	poolConfig.MaxConnLifetime = cm.config.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cm.config.MaxConnIdleTime

	// Health check configuration
	poolConfig.HealthCheckPeriod = 30 * time.Second

	// Connection callback for custom type registration
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		// Register custom PostgreSQL types if needed
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return errors.Wrap(err, "failed to create connection pool")
	}

	// Test the connection
	conn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		return errors.Wrap(err, "failed to acquire connection")
	}
	defer conn.Release()

	if err := conn.Ping(ctx); err != nil {
		pool.Close()
		return errors.Wrap(err, "failed to ping database")
	}

	cm.pool.Store(pool)
	cm.isHealthy.Store(true)

	// Start health monitoring
	cm.startHealthMonitoring()

	return nil
}

// Close closes the connection pool and stops health monitoring
func (cm *ConnectionManager) Close() error {
	// Stop health monitoring
	if cm.healthTicker != nil {
		cm.healthTicker.Stop()
		close(cm.stopHealth)
	}

	// Close connection pool
	if pool := cm.pool.Load(); pool != nil {
		pool.Close()
		cm.pool.Store(nil)
	}

	cm.isHealthy.Store(false)
	return nil
}

// Pool returns the current connection pool
func (cm *ConnectionManager) Pool() *pgxpool.Pool {
	return cm.pool.Load()
}

// IsHealthy returns the current health status
func (cm *ConnectionManager) IsHealthy() bool {
	return cm.isHealthy.Load()
}

// RunMigrations executes database migrations using goose
func (cm *ConnectionManager) RunMigrations(ctx context.Context, migrationsDir string) error {
	pool := cm.Pool()
	if pool == nil {
		return errors.New("connection pool not initialized")
	}

	// Use pool.Config().ConnString to get database connection string for goose
	// Since goose expects a *sql.DB, we need to create one from the connection string
	return errors.New("RunMigrations implementation needs to be updated - use goose with database/sql instead of pgx")
}

// BeginTx starts a new transaction with proper context handling
func (cm *ConnectionManager) BeginTx(ctx context.Context) (pgx.Tx, error) {
	pool := cm.Pool()
	if pool == nil {
		return nil, errors.New("connection pool not initialized")
	}

	return pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	})
}

// WithTx executes a function within a transaction
func (cm *ConnectionManager) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := cm.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetPool returns the current connection pool for direct access
func (cm *ConnectionManager) GetPool() *pgxpool.Pool {
	return cm.pool.Load()
}

// startHealthMonitoring starts the health check routine
func (cm *ConnectionManager) startHealthMonitoring() {
	cm.healthTicker = time.NewTicker(30 * time.Second)

	go func() {
		for {
			select {
			case <-cm.healthTicker.C:
				cm.performHealthCheck()
			case <-cm.stopHealth:
				return
			}
		}
	}()
}

// performHealthCheck checks database connectivity
func (cm *ConnectionManager) performHealthCheck() {
	pool := cm.Pool()
	if pool == nil {
		cm.isHealthy.Store(false)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), cm.config.HealthTimeout)
	defer cancel()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		cm.isHealthy.Store(false)
		return
	}
	defer conn.Release()

	if err := conn.Ping(ctx); err != nil {
		cm.isHealthy.Store(false)
		return
	}

	cm.isHealthy.Store(true)
}

// HealthStatus returns detailed health information
func (cm *ConnectionManager) HealthStatus() HealthStatus {
	pool := cm.Pool()
	if pool == nil {
		return HealthStatus{
			IsHealthy: false,
			Error:     "connection pool not initialized",
			CheckedAt: time.Now(),
		}
	}

	stat := pool.Stat()
	return HealthStatus{
		IsHealthy:     cm.IsHealthy(),
		TotalConns:    stat.TotalConns(),
		IdleConns:     stat.IdleConns(),
		AcquiredConns: stat.AcquiredConns(),
		CheckedAt:     time.Now(),
	}
}

// HealthStatus provides detailed connection pool health information
type HealthStatus struct {
	IsHealthy     bool      `json:"isHealthy"`
	TotalConns    int32     `json:"totalConns"`
	IdleConns     int32     `json:"idleConns"`
	AcquiredConns int32     `json:"acquiredConns"`
	Error         string    `json:"error,omitempty"`
	CheckedAt     time.Time `json:"checkedAt"`
}

// String returns a human-readable health status
func (hs HealthStatus) String() string {
	status := "HEALTHY"
	if !hs.IsHealthy {
		status = "UNHEALTHY"
	}

	return fmt.Sprintf("PostgreSQL: %s (total:%d, idle:%d, acquired:%d) at %s",
		status, hs.TotalConns, hs.IdleConns, hs.AcquiredConns,
		hs.CheckedAt.Format(time.RFC3339))
}
