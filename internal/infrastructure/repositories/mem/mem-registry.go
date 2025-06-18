package mem

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/patterns"
)

// Registry is an in-memory implementation of the Registry interface
type Registry struct {
	db     *MemDB
	mu     sync.RWMutex
	subj   patterns.Subject
	closed bool
}

// NewRegistry creates a new in-memory registry
func NewRegistry() *Registry {
	return &Registry{
		db:   NewMemDB(),
		subj: &subject{},
	}
}

// Subject returns the registry's subject
func (r *Registry) Subject() patterns.Subject {
	return r.subj
}

// Writer returns a new writer
func (r *Registry) Writer(ctx context.Context) (ports.Writer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, errors.New("registry is closed")
	}
	return &writer{
		registry: r,
		ctx:      ctx,
	}, nil
}

// Reader returns a new reader
func (r *Registry) Reader(ctx context.Context) (ports.Reader, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, errors.New("registry is closed")
	}
	return &reader{
		registry: r,
		ctx:      ctx,
	}, nil
}

// ReaderFromWriter returns a reader that can see changes made in the current transaction
func (r *Registry) ReaderFromWriter(ctx context.Context, w ports.Writer) (ports.Reader, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, errors.New("registry is closed")
	}

	memWriter, ok := w.(*writer)
	if !ok {
		return nil, errors.New("writer is not a memory writer")
	}

	return &reader{
		registry: r,
		ctx:      ctx,
		writer:   memWriter,
	}, nil
}

// Close closes the registry
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return nil
}

type subject struct {
	observers []interface{}
	mu        sync.RWMutex
}

func (s *subject) Subscribe(observer interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers = append(s.observers, observer)
	return nil
}

func (s *subject) Unsubscribe(observer interface{}) error {
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

func (s *subject) Notify(event interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, o := range s.observers {
		if handler, ok := o.(func(interface{})); ok {
			handler(event)
		}
	}
}
