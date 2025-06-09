package mem

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"netguard-pg-backend/internal/domain/models"
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

type reader struct {
	registry *Registry
	ctx      context.Context
}

func (r *reader) Close() error {
	return nil
}

func (r *reader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	services := r.registry.db.GetServices()
	if scope != nil && !scope.IsEmpty() {
		if ns, ok := scope.(ports.NameScope); ok {
			for _, name := range ns.Names {
				if service, ok := services[name]; ok {
					if err := consume(service); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, service := range services {
		if err := consume(service); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	addressGroups := r.registry.db.GetAddressGroups()
	if scope != nil && !scope.IsEmpty() {
		if ns, ok := scope.(ports.NameScope); ok {
			for _, name := range ns.Names {
				if addressGroup, ok := addressGroups[name]; ok {
					if err := consume(addressGroup); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, addressGroup := range addressGroups {
		if err := consume(addressGroup); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	bindings := r.registry.db.GetAddressGroupBindings()
	if scope != nil && !scope.IsEmpty() {
		if ns, ok := scope.(ports.NameScope); ok {
			for _, name := range ns.Names {
				if binding, ok := bindings[name]; ok {
					if err := consume(binding); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, binding := range bindings {
		if err := consume(binding); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	mappings := r.registry.db.GetAddressGroupPortMappings()
	if scope != nil && !scope.IsEmpty() {
		if ns, ok := scope.(ports.NameScope); ok {
			for _, name := range ns.Names {
				if mapping, ok := mappings[name]; ok {
					if err := consume(mapping); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, mapping := range mappings {
		if err := consume(mapping); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	rules := r.registry.db.GetRuleS2S()
	if scope != nil && !scope.IsEmpty() {
		if ns, ok := scope.(ports.NameScope); ok {
			for _, name := range ns.Names {
				if rule, ok := rules[name]; ok {
					if err := consume(rule); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, rule := range rules {
		if err := consume(rule); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	status := r.registry.db.GetSyncStatus()
	return &status, nil
}

type writer struct {
	registry                 *Registry
	ctx                      context.Context
	services                 map[string]models.Service
	addressGroups            map[string]models.AddressGroup
	addressGroupBindings     map[string]models.AddressGroupBinding
	addressGroupPortMappings map[string]models.AddressGroupPortMapping
	ruleS2S                  map[string]models.RuleS2S
}

func (w *writer) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
	if w.services == nil {
		w.services = make(map[string]models.Service)
	}
	for _, service := range services {
		key := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
		w.services[key] = service
	}
	return nil
}

func (w *writer) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope, opts ...ports.Option) error {
	if w.addressGroups == nil {
		w.addressGroups = make(map[string]models.AddressGroup)
	}
	for _, addressGroup := range addressGroups {
		key := fmt.Sprintf("%s/%s", addressGroup.Namespace, addressGroup.Name)
		w.addressGroups[key] = addressGroup
	}
	return nil
}

func (w *writer) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, opts ...ports.Option) error {
	if w.addressGroupBindings == nil {
		w.addressGroupBindings = make(map[string]models.AddressGroupBinding)
	}
	for _, binding := range bindings {
		key := fmt.Sprintf("%s/%s", binding.Namespace, binding.Name)
		w.addressGroupBindings[key] = binding
	}
	return nil
}

func (w *writer) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, opts ...ports.Option) error {
	if w.addressGroupPortMappings == nil {
		w.addressGroupPortMappings = make(map[string]models.AddressGroupPortMapping)
	}
	for _, mapping := range mappings {
		key := fmt.Sprintf("%s/%s", mapping.Namespace, mapping.Name)
		w.addressGroupPortMappings[key] = mapping
	}
	return nil
}

func (w *writer) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, opts ...ports.Option) error {
	if w.ruleS2S == nil {
		w.ruleS2S = make(map[string]models.RuleS2S)
	}
	for _, rule := range rules {
		key := fmt.Sprintf("%s/%s", rule.Namespace, rule.Name)
		w.ruleS2S[key] = rule
	}
	return nil
}

func (w *writer) Commit() error {
	if w.services != nil {
		w.registry.db.SetServices(w.services)
	}
	if w.addressGroups != nil {
		w.registry.db.SetAddressGroups(w.addressGroups)
	}
	if w.addressGroupBindings != nil {
		w.registry.db.SetAddressGroupBindings(w.addressGroupBindings)
	}
	if w.addressGroupPortMappings != nil {
		w.registry.db.SetAddressGroupPortMappings(w.addressGroupPortMappings)
	}
	if w.ruleS2S != nil {
		w.registry.db.SetRuleS2S(w.ruleS2S)
	}
	w.registry.db.SetSyncStatus(models.SyncStatus{
		UpdatedAt: time.Now(),
	})
	return nil
}

func (w *writer) Abort() {
	w.services = nil
	w.addressGroups = nil
	w.addressGroupBindings = nil
	w.addressGroupPortMappings = nil
	w.ruleS2S = nil
}
