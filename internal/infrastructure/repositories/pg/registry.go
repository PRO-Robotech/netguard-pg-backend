package pg

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pkg/errors"
	"net/url"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/readers"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/writers"
	"netguard-pg-backend/internal/patterns"
	"sync"
	"time"
)

type readCommittedReader struct {
	*readers.Reader
	tx  pgx.Tx
	ctx context.Context
}

func (r *readCommittedReader) Close() error {
	if r.tx != nil {
		return r.tx.Rollback(r.ctx)
	}
	return nil
}

var _ ports.Registry = (*Registry)(nil)

type PostgreSQLWriter interface {
	ports.Writer
	GetTx() pgx.Tx
}
type Registry struct {
	subject patterns.Subject
	pool    *pgxpool.Pool
	mu      sync.RWMutex
}

func NewRegistryFromPG(ctx context.Context, dbURL url.URL) (ports.Registry, error) {
	conf, err := pgxpool.ParseConfig(dbURL.String())
	if err != nil {
		return nil, errors.WithMessage(err, "NewRegistryFromPG parse config")
	}
	conf.MaxConns = 50
	conf.MinConns = 5
	conf.MaxConnLifetime = 2 * time.Hour
	conf.MaxConnIdleTime = 15 * time.Minute
	conf.HealthCheckPeriod = 30 * time.Second
	conf.ConnConfig.ConnectTimeout = 5 * time.Second
	conf.ConnConfig.RuntimeParams = map[string]string{
		"statement_timeout":                   "60000",
		"idle_in_transaction_session_timeout": "120000",
		"lock_timeout":                        "30000",
	}
	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		return nil, errors.WithMessage(err, "NewRegistryFromPG create pool")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.WithMessage(err, "NewRegistryFromPG ping")
	}
	ret := &Registry{
		subject: &simpleSubject{
			observers: make([]interface{}, 0),
		},
		pool: pool,
	}
	return ret, nil
}
func NewRegistryFromURI(ctx context.Context, uri string) (*Registry, error) {
	dbURL, err := url.Parse(uri)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse PostgreSQL URI")
	}
	registry, err := NewRegistryFromPG(ctx, *dbURL)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create PostgreSQL registry")
	}
	if pgRegistry, ok := registry.(*Registry); ok {
		return pgRegistry, nil
	}
	return nil, errors.New("unexpected registry type returned")
}
func (r *Registry) Subject() patterns.Subject {
	return r.subject
}
func (r *Registry) Pool() *pgxpool.Pool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pool
}
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
	modularWriter := writers.NewWriter(r, tx, ctx)
	return &simpleWriter{
		tx:            tx,
		ctx:           ctx,
		modularWriter: modularWriter,
	}, nil
}
func (r *Registry) WriterForConditions(ctx context.Context) (ports.Writer, error) {
	r.mu.RLock()
	pool := r.pool
	r.mu.RUnlock()
	if pool == nil {
		return nil, errors.New("registry pool is nil")
	}
	txOpts := pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	}
	tx, err := pool.BeginTx(ctx, txOpts)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to begin transaction for conditions")
	}
	modularWriter := writers.NewWriter(r, tx, ctx)
	return &simpleWriter{
		tx:            tx,
		ctx:           ctx,
		modularWriter: modularWriter,
	}, nil
}
func (r *Registry) WriterForDeletes(ctx context.Context) (ports.Writer, error) {
	r.mu.RLock()
	pool := r.pool
	r.mu.RUnlock()
	if pool == nil {
		return nil, errors.New("registry pool is nil")
	}
	txOpts := pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	}
	tx, err := pool.BeginTx(ctx, txOpts)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to begin transaction for deletes")
	}
	modularWriter := writers.NewWriter(r, tx, ctx)
	return &simpleWriter{
		tx:            tx,
		ctx:           ctx,
		modularWriter: modularWriter,
	}, nil
}
func (r *Registry) Reader(ctx context.Context) (ports.Reader, error) {
	r.mu.RLock()
	pool := r.pool
	r.mu.RUnlock()
	if pool == nil {
		return nil, errors.New("registry pool is nil")
	}
	reader := readers.NewReader(r, pool, nil, ctx)
	return reader, nil
}
func (r *Registry) ReaderWithReadCommitted(ctx context.Context) (ports.Reader, error) {
	r.mu.RLock()
	pool := r.pool
	r.mu.RUnlock()
	if pool == nil {
		return nil, errors.New("registry pool is nil")
	}
	txOpts := pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadOnly,
	}
	tx, err := pool.BeginTx(ctx, txOpts)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to begin ReadCommitted transaction for reader")
	}
	baseReader := readers.NewReader(r, pool, tx, ctx)
	return &readCommittedReader{
		Reader: baseReader,
		tx:     tx,
		ctx:    ctx,
	}, nil
}
func (r *Registry) ReaderFromWriter(ctx context.Context, w ports.Writer) (ports.Reader, error) {
	r.mu.RLock()
	pool := r.pool
	r.mu.RUnlock()
	if pool == nil {
		return nil, errors.New("registry pool is nil")
	}
	var tx pgx.Tx
	if pgWriter, ok := w.(PostgreSQLWriter); ok {
		tx = pgWriter.GetTx()
	}
	reader := readers.NewReader(r, pool, tx, ctx)
	return reader, nil
}
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pool != nil {
		r.pool.Close()
		r.pool = nil
	}
	return nil
}

type simpleWriter struct {
	tx            pgx.Tx
	ctx           context.Context
	modularWriter *writers.Writer
}

func (w *simpleWriter) GetTx() pgx.Tx {
	return w.tx
}
func (w *simpleWriter) Commit() error {
	return w.tx.Commit(w.ctx)
}
func (w *simpleWriter) Abort() {
	w.tx.Rollback(w.ctx)
}
func (w *simpleWriter) Close() error {
	return nil
}
func (w *simpleWriter) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncServices(ctx, services, scope, opts...)
}
func (w *simpleWriter) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteServicesByIDs(ctx, ids, opts...)
}
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
	return w.modularWriter.DeleteRuleS2SByIDs(ctx, ids)
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
	return w.modularWriter.DeleteIEAgAgRulesByIDs(ctx, ids)
}
func (w *simpleWriter) SyncNetworks(ctx context.Context, networks []models.Network, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncNetworks(ctx, networks, scope, opts...)
}
func (w *simpleWriter) DeleteNetworksByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteNetworksByIDs(ctx, ids)
}
func (w *simpleWriter) SyncNetworkBindings(ctx context.Context, bindings []models.NetworkBinding, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncNetworkBindings(ctx, bindings, scope, opts...)
}
func (w *simpleWriter) DeleteNetworkBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteNetworkBindingsByIDs(ctx, ids)
}
func (w *simpleWriter) SyncHosts(ctx context.Context, hosts []models.Host, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncHosts(ctx, hosts, scope, opts...)
}
func (w *simpleWriter) DeleteHostsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteHostsByIDs(ctx, ids)
}
func (w *simpleWriter) SyncHostBindings(ctx context.Context, hostBindings []models.HostBinding, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncHostBindings(ctx, hostBindings, scope, opts...)
}
func (w *simpleWriter) DeleteHostBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteHostBindingsByIDs(ctx, ids)
}
func (w *simpleWriter) SyncSvcSvcRules(ctx context.Context, rules []models.SvcSvcRule, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncSvcSvcRules(ctx, rules, scope, opts...)
}
func (w *simpleWriter) DeleteSvcSvcRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteSvcSvcRulesByIDs(ctx, ids, opts...)
}
func (w *simpleWriter) SyncSvcFqdnRules(ctx context.Context, rules []models.SvcFqdnRule, scope ports.Scope, opts ...ports.Option) error {
	return w.modularWriter.SyncSvcFqdnRules(ctx, rules, scope, opts...)
}
func (w *simpleWriter) DeleteSvcFqdnRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.modularWriter.DeleteSvcFqdnRulesByIDs(ctx, ids, opts...)
}
func (w *simpleWriter) UpdateSyncStatus(ctx context.Context) error {
	return nil
}
func (w *simpleWriter) MarkForDeletionWithStatus(namespace, name, kind string) error {
	return w.modularWriter.MarkForDeletionWithStatus(namespace, name, kind)
}

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
