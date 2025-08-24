package pg

import (
	"context"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/readers"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/writers"
	"netguard-pg-backend/internal/patterns"
)

// readCommittedReader wraps a readers.Reader and properly manages ReadCommitted transaction lifecycle
type readCommittedReader struct {
	*readers.Reader // Embed the reader to inherit all interface methods
	tx              pgx.Tx
	ctx             context.Context
}

// Close properly closes the ReadCommitted transaction
func (r *readCommittedReader) Close() error {
	if r.tx != nil {
		// For read-only transactions, we should rollback (which is effectively a commit for read-only)
		return r.tx.Rollback(r.ctx)
	}
	return nil
}

// Compile-time check that Registry implements ports.Registry
var _ ports.Registry = (*Registry)(nil)

// PostgreSQLWriter interface for accessing transaction from writer
type PostgreSQLWriter interface {
	ports.Writer
	GetTx() pgx.Tx
}

// Registry implements the PostgreSQL-based registry pattern
// Simplified approach with standard Go patterns
type Registry struct {
	subject patterns.Subject
	pool    *pgxpool.Pool // Simple pool reference instead of atomic
	mu      sync.RWMutex  // Protect pool access
}

// NewRegistryFromPG creates registry from Postgres (simplified approach)
func NewRegistryFromPG(ctx context.Context, dbURL url.URL) (ports.Registry, error) {
	log.Printf("Creating PostgreSQL registry from URL: %s", dbURL.Host)

	conf, err := pgxpool.ParseConfig(dbURL.String())
	if err != nil {
		log.Printf("Failed to parse PostgreSQL config: %v", err)
		return nil, errors.WithMessage(err, "NewRegistryFromPG parse config")
	}

	// 🎯 TIMEOUT_FIX: Optimize connection pool for high concurrency and condition operations
	// Increase connection limits for heavy reactive flows and concurrent condition processing
	conf.MaxConns = 50                        // Increased from default ~4 to handle concurrent condition operations
	conf.MinConns = 5                         // Keep minimum connections warm
	conf.MaxConnLifetime = 2 * time.Hour      // Longer lifetime to reduce connection churn
	conf.MaxConnIdleTime = 15 * time.Minute   // Reasonable idle time
	conf.HealthCheckPeriod = 30 * time.Second // Regular health checks

	// 🔧 OPTIMIZED_FIX: Aggressive timeout settings for better concurrent performance
	conf.ConnConfig.ConnectTimeout = 5 * time.Second // Faster connection timeout

	// 🎯 BUSINESS_FLOW_FIX: Adjusted PostgreSQL timeouts for complex business flows
	// Previous: 10s statement, 15s idle transaction - TOO AGGRESSIVE for RuleS2S complex flows
	// New: Balanced approach - prevent hung connections while allowing complex business logic
	conf.ConnConfig.RuntimeParams = map[string]string{
		"statement_timeout":                   "60000",  // 60 second statement timeout (complex queries)
		"idle_in_transaction_session_timeout": "120000", // 2 minute idle transaction timeout (business flows)
		"lock_timeout":                        "30000",  // 30 second lock timeout (reduced contention)
	}

	log.Printf("🎯 TIMEOUT_FIX: Optimized PostgreSQL pool config - MaxConns: %d, MinConns: %d, ConnectTimeout: %v",
		conf.MaxConns, conf.MinConns, conf.ConnConfig.ConnectTimeout)

	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		log.Printf("Failed to create PostgreSQL pool: %v", err)
		return nil, errors.WithMessage(err, "NewRegistryFromPG create pool")
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		log.Printf("Failed to ping PostgreSQL: %v", err)
		pool.Close()
		return nil, errors.WithMessage(err, "NewRegistryFromPG ping")
	}

	log.Printf("PostgreSQL connection established successfully")

	ret := &Registry{
		subject: &simpleSubject{
			observers: make([]interface{}, 0),
		},
		pool: pool, // Simple assignment instead of atomic store
	}

	log.Printf("PostgreSQL registry created successfully")
	return ret, nil
}

// NewRegistryFromURI creates a PostgreSQL registry from a connection URI
// Wrapper for sgroups-style function (migrations handled separately via Job)
func NewRegistryFromURI(ctx context.Context, uri string) (*Registry, error) {
	// Parse URI and delegate to NewRegistryFromPG
	dbURL, err := url.Parse(uri)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse PostgreSQL URI")
	}

	registry, err := NewRegistryFromPG(ctx, *dbURL)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create PostgreSQL registry")
	}

	// Cast to concrete type for consistency
	if pgRegistry, ok := registry.(*Registry); ok {
		return pgRegistry, nil
	}

	return nil, errors.New("unexpected registry type returned")
}

// Subject returns the registry's subject for observer pattern
func (r *Registry) Subject() patterns.Subject {
	return r.subject
}

// Writer creates a new PostgreSQL writer (simplified approach)
func (r *Registry) Writer(ctx context.Context) (ports.Writer, error) {
	r.mu.RLock()
	pool := r.pool
	r.mu.RUnlock()

	if pool == nil {
		return nil, errors.New("registry pool is nil")
	}

	txOpts := pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadWrite,
	}

	tx, err := pool.BeginTx(ctx, txOpts)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to begin transaction")
	}

	// Use only the modular writer - eliminate complex writer wrapper
	modularWriter := writers.NewWriter(r, tx, ctx)

	return &simpleWriter{
		tx:            tx,
		ctx:           ctx,
		modularWriter: modularWriter,
	}, nil
}

// 🔧 PRODUCTION FIX: WriterForConditions creates a writer with ReadCommitted isolation
// This allows condition sync operations to see data committed by other transactions
// avoiding UID conflicts where ConditionManager can't find the service that was just created
func (r *Registry) WriterForConditions(ctx context.Context) (ports.Writer, error) {
	r.mu.RLock()
	pool := r.pool
	r.mu.RUnlock()

	if pool == nil {
		return nil, errors.New("registry pool is nil")
	}

	// 🚨 CRITICAL: Use ReadCommitted instead of RepeatableRead
	// This allows seeing committed data from other transactions
	txOpts := pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted, // Can see committed data from other transactions
		AccessMode: pgx.ReadWrite,
	}

	tx, err := pool.BeginTx(ctx, txOpts)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to begin transaction for conditions")
	}

	// Use only the modular writer - eliminate complex writer wrapper
	modularWriter := writers.NewWriter(r, tx, ctx)

	return &simpleWriter{
		tx:            tx,
		ctx:           ctx,
		modularWriter: modularWriter,
	}, nil
}

// 🔧 SERIALIZATION_FIX: WriterForDeletes creates a writer with ReadCommitted isolation for delete operations
// This reduces serialization conflict sensitivity during concurrent DELETE operations
func (r *Registry) WriterForDeletes(ctx context.Context) (ports.Writer, error) {
	r.mu.RLock()
	pool := r.pool
	r.mu.RUnlock()

	if pool == nil {
		return nil, errors.New("registry pool is nil")
	}

	// 🚨 CRITICAL: Use ReadCommitted instead of RepeatableRead for delete operations
	// This allows seeing committed data from other transactions and reduces serialization conflicts
	txOpts := pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted, // Less sensitive to concurrent access
		AccessMode: pgx.ReadWrite,
	}

	tx, err := pool.BeginTx(ctx, txOpts)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to begin transaction for deletes")
	}

	// Use only the modular writer - eliminate complex writer wrapper
	modularWriter := writers.NewWriter(r, tx, ctx)

	return &simpleWriter{
		tx:            tx,
		ctx:           ctx,
		modularWriter: modularWriter,
	}, nil
}

// Reader creates a new PostgreSQL reader using the dedicated readers module
func (r *Registry) Reader(ctx context.Context) (ports.Reader, error) {
	r.mu.RLock()
	pool := r.pool
	r.mu.RUnlock()

	if pool == nil {
		return nil, errors.New("registry pool is nil")
	}

	// Use the proper readers.Reader instead of duplicating code
	reader := readers.NewReader(r, pool, nil, ctx)
	return reader, nil
}

// 🔧 CROSS-RULES2S FIX: ReaderWithReadCommitted creates a reader with ReadCommitted isolation
// This allows Cross-RuleS2S aggregation to see data committed by other transactions immediately,
// fixing the timing bug where deleted AddressGroupBindings were still visible in new readers
func (r *Registry) ReaderWithReadCommitted(ctx context.Context) (ports.Reader, error) {
	r.mu.RLock()
	pool := r.pool
	r.mu.RUnlock()

	if pool == nil {
		return nil, errors.New("registry pool is nil")
	}

	// 🚨 CRITICAL: Use ReadCommitted isolation to see recently committed data
	// This fixes the Cross-RuleS2S aggregation bug where populateServiceAddressGroups
	// couldn't see deleted AddressGroupBindings due to connection pool timing issues
	txOpts := pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted, // Can see committed data from other transactions immediately
		AccessMode: pgx.ReadOnly,      // Read-only for performance
	}

	tx, err := pool.BeginTx(ctx, txOpts)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to begin ReadCommitted transaction for reader")
	}

	// Create a transaction-aware reader that properly manages the transaction lifecycle
	baseReader := readers.NewReader(r, pool, tx, ctx)
	return &readCommittedReader{
		Reader: baseReader,
		tx:     tx,
		ctx:    ctx,
	}, nil
}

// ReaderFromWriter creates a reader that uses the same transaction as the writer
func (r *Registry) ReaderFromWriter(ctx context.Context, w ports.Writer) (ports.Reader, error) {
	r.mu.RLock()
	pool := r.pool
	r.mu.RUnlock()

	if pool == nil {
		return nil, errors.New("registry pool is nil")
	}

	// Try to get the transaction from the writer if it supports it
	var tx pgx.Tx
	if pgWriter, ok := w.(PostgreSQLWriter); ok {
		tx = pgWriter.GetTx()
	}

	// Use the proper readers.Reader with the writer's transaction for consistency
	reader := readers.NewReader(r, pool, tx, ctx)
	return reader, nil
}

// Close closes the registry and its connections (simplified pattern)
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.pool != nil {
		r.pool.Close()
		r.pool = nil
	}

	return nil
}

// simpleWriter implements a simplified PostgreSQL writer
type simpleWriter struct {
	tx            pgx.Tx
	ctx           context.Context
	modularWriter *writers.Writer
}

// Implement required Writer interface methods
func (w *simpleWriter) Commit() error {
	return w.tx.Commit(w.ctx)
}

func (w *simpleWriter) Abort() {
	w.tx.Rollback(w.ctx) // Ignore error for simplified approach
}

func (w *simpleWriter) Close() error {
	return nil // Transaction lifecycle managed by Commit/Abort
}

// Delegate all resource methods to modular writer
func (w *simpleWriter) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncServices(ctx, services, scope, opts...)
}

func (w *simpleWriter) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteServicesByIDs(ctx, ids, opts...)
}

// Delegate all resource methods to modular writer (implementing all required methods)
func (w *simpleWriter) SyncAddressGroups(ctx context.Context, groups []models.AddressGroup, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncAddressGroups(ctx, groups, scope, opts...)
}

func (w *simpleWriter) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteAddressGroupsByIDs(ctx, ids, opts...)
}

func (w *simpleWriter) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncAddressGroupBindings(ctx, bindings, scope, opts...)
}

func (w *simpleWriter) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteAddressGroupBindingsByIDs(ctx, ids, opts...)
}

func (w *simpleWriter) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncAddressGroupPortMappings(ctx, mappings, scope, opts...)
}

func (w *simpleWriter) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteAddressGroupPortMappingsByIDs(ctx, ids, opts...)
}

func (w *simpleWriter) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncRuleS2S(ctx, rules, scope, opts...)
}

func (w *simpleWriter) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteRuleS2SByIDs(ctx, ids) // modularWriter doesn't accept opts
}

func (w *simpleWriter) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncServiceAliases(ctx, aliases, scope, opts...)
}

func (w *simpleWriter) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteServiceAliasesByIDs(ctx, ids, opts...)
}

func (w *simpleWriter) SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncAddressGroupBindingPolicies(ctx, policies, scope, opts...)
}

func (w *simpleWriter) DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteAddressGroupBindingPoliciesByIDs(ctx, ids, opts...)
}

func (w *simpleWriter) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncIEAgAgRules(ctx, rules, scope, opts...)
}

func (w *simpleWriter) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteIEAgAgRulesByIDs(ctx, ids) // modularWriter doesn't accept opts
}

func (w *simpleWriter) SyncNetworks(ctx context.Context, networks []models.Network, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncNetworks(ctx, networks, scope, opts...)
}

func (w *simpleWriter) DeleteNetworksByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteNetworksByIDs(ctx, ids) // modularWriter doesn't accept opts
}

func (w *simpleWriter) SyncNetworkBindings(ctx context.Context, bindings []models.NetworkBinding, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncNetworkBindings(ctx, bindings, scope, opts...)
}

func (w *simpleWriter) DeleteNetworkBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteNetworkBindingsByIDs(ctx, ids) // modularWriter doesn't accept opts
}

func (w *simpleWriter) UpdateSyncStatus(ctx context.Context) error {
	// For simplified approach, just return success
	return nil
}

// simpleSubject implements the patterns.Subject interface with basic functionality
type simpleSubject struct {
	observers []interface{}
	mu        sync.RWMutex
}

func (s *simpleSubject) Subscribe(observer interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers = append(s.observers, observer)
	return nil
}

func (s *simpleSubject) Unsubscribe(observer interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, o := range s.observers {
		if o == observer {
			s.observers = append(s.observers[:i], s.observers[i+1:]...)
			return nil
		}
	}
	return errors.New("observer not found")
}

func (s *simpleSubject) Notify(event interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, o := range s.observers {
		if handler, ok := o.(func(interface{})); ok {
			handler(event)
		}
	}
}
