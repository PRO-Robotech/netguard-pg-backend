package pg

//
//import (
//	"context"
//	"sync"
//
//	"github.com/jackc/pgx/v5/pgxpool"
//	"github.com/pkg/errors"
//
//	"netguard-pg-backend/internal/domain/ports"
//	"netguard-pg-backend/internal/patterns"
//)
//
//// Registry implements the ports.Registry interface for PostgreSQL
//type Registry struct {
//	pool   *pgxpool.Pool
//	subj   patterns.Subject
//	mu     sync.RWMutex
//	closed bool
//}
//
//// NewRegistry creates a new PostgreSQL registry
//func NewRegistry(ctx context.Context, connString string) (*Registry, error) {
//	config, err := pgxpool.ParseConfig(connString)
//	if err != nil {
//		return nil, errors.Wrap(err, "failed to parse connection string")
//	}
//
//	pool, err := pgxpool.NewWithConfig(ctx, config)
//	if err != nil {
//		return nil, errors.Wrap(err, "failed to create connection pool")
//	}
//
//	// Test connection
//	if err := pool.Ping(ctx); err != nil {
//		pool.Close()
//		return nil, errors.Wrap(err, "failed to ping database")
//	}
//
//	// Register custom types
//	conn, err := pool.Acquire(ctx)
//	if err != nil {
//		pool.Close()
//		return nil, errors.Wrap(err, "failed to acquire connection")
//	}
//	defer conn.Release()
//
//	if err := RegisterNetguardTypesOntoPGX(ctx, conn.Conn()); err != nil {
//		pool.Close()
//		return nil, errors.Wrap(err, "failed to register types")
//	}
//
//	return &Registry{
//		pool: pool,
//		subj: &subject{},
//	}, nil
//}
//
//// Subject returns the registry's subject
//func (r *Registry) Subject() patterns.Subject {
//	return r.subj
//}
//
//// Writer returns a new writer
//func (r *Registry) Writer(ctx context.Context) (ports.Writer, error) {
//	r.mu.RLock()
//	defer r.mu.RUnlock()
//
//	if r.closed {
//		return nil, errors.New("registry is closed")
//	}
//
//	conn, err := r.pool.Acquire(ctx)
//	if err != nil {
//		return nil, errors.Wrap(err, "failed to acquire connection")
//	}
//
//	// Begin transaction
//	tx, err := conn.Begin(ctx)
//	if err != nil {
//		conn.Release()
//		return nil, errors.Wrap(err, "failed to begin transaction")
//	}
//
//	return &writer{
//		registry: r,
//		conn:     conn,
//		tx:       tx,
//		ctx:      ctx,
//	}, nil
//}
//
//// Reader returns a new reader
//func (r *Registry) Reader(ctx context.Context) (ports.Reader, error) {
//	r.mu.RLock()
//	defer r.mu.RUnlock()
//
//	if r.closed {
//		return nil, errors.New("registry is closed")
//	}
//
//	conn, err := r.pool.Acquire(ctx)
//	if err != nil {
//		return nil, errors.Wrap(err, "failed to acquire connection")
//	}
//
//	return &reader{
//		registry: r,
//		conn:     conn,
//		ctx:      ctx,
//	}, nil
//}
//
//// Close closes the registry
//func (r *Registry) Close() error {
//	r.mu.Lock()
//	defer r.mu.Unlock()
//
//	if r.closed {
//		return nil
//	}
//
//	r.closed = true
//	r.pool.Close()
//
//	return nil
//}
//
//// subject implements the patterns.Subject interface
//type subject struct {
//	observers []interface{}
//	mu        sync.RWMutex
//}
//
//func (s *subject) Subscribe(observer interface{}) error {
//	s.mu.Lock()
//	defer s.mu.Unlock()
//	s.observers = append(s.observers, observer)
//	return nil
//}
//
//func (s *subject) Unsubscribe(observer interface{}) error {
//	s.mu.Lock()
//	defer s.mu.Unlock()
//	for i, o := range s.observers {
//		if o == observer {
//			s.observers = append(s.observers[:i], s.observers[i+1:]...)
//			return nil
//		}
//	}
//	return errors.New("observer not found")
//}
//
//func (s *subject) Notify(event interface{}) {
//	s.mu.RLock()
//	defer s.mu.RUnlock()
//	for _, o := range s.observers {
//		if handler, ok := o.(func(interface{})); ok {
//			handler(event)
//		}
//	}
//}
