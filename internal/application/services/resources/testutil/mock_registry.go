package testutil

import (
	"context"
	"fmt"
	"sync"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/patterns"
)

// MockRegistry implements a test-friendly in-memory registry
type MockRegistry struct {
	mu         sync.RWMutex
	readers    map[string]*MockReader
	writers    map[string]*MockWriter
	closed     bool
	sharedData map[string]interface{} // Shared data store
}

// NewMockRegistry creates a new mock registry for testing
func NewMockRegistry() *MockRegistry {
	return &MockRegistry{
		readers:    make(map[string]*MockReader),
		writers:    make(map[string]*MockWriter),
		sharedData: make(map[string]interface{}),
	}
}

func (m *MockRegistry) Reader(ctx context.Context) (ports.Reader, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("registry is closed")
	}

	readerID := fmt.Sprintf("reader_%p", ctx)
	reader := &MockReader{
		registry: m,
		id:       readerID,
		data:     m.sharedData, // Use shared data
	}
	m.readers[readerID] = reader
	return reader, nil
}

func (m *MockRegistry) Writer(ctx context.Context) (ports.Writer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("registry is closed")
	}

	writerID := fmt.Sprintf("writer_%p", ctx)
	// Each writer gets its own copy of data to avoid concurrent map writes
	writerData := copyData(m.sharedData)
	writer := &MockWriter{
		registry:    m,
		id:          writerID,
		data:        writerData,            // Each writer gets its own data copy
		deletedKeys: make(map[string]bool), // Track deletions
		committed:   false,
	}
	m.writers[writerID] = writer
	return writer, nil
}

func (m *MockRegistry) ReaderFromWriter(ctx context.Context, writer ports.Writer) (ports.Reader, error) {
	mockWriter, ok := writer.(*MockWriter)
	if !ok {
		return nil, fmt.Errorf("writer is not a MockWriter")
	}

	// Lock to prevent concurrent map access
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a combined view of shared data + writer's uncommitted data
	combinedData := make(map[string]interface{})

	// First copy shared data
	for k, v := range m.sharedData {
		combinedData[k] = v
	}

	// Then overlay writer's data (uncommitted changes)
	for k, v := range mockWriter.data {
		combinedData[k] = v
	}

	reader := &MockReader{
		registry: m,
		id:       fmt.Sprintf("reader_from_writer_%s", mockWriter.id),
		data:     combinedData, // Combined view
	}
	return reader, nil
}

// Subject returns a mock subject for the registry
func (m *MockRegistry) Subject() patterns.Subject {
	// Return a simple mock subject
	return &MockSubject{}
}

func (m *MockRegistry) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	m.readers = nil
	m.writers = nil
	return nil
}

// Helper function to deep copy data
func copyData(original map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{})
	for k, v := range original {
		copy[k] = v
	}
	return copy
}

// SetupTestData allows tests to pre-populate the registry with data
func (m *MockRegistry) SetupTestData(data map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Apply data to shared data store
	for k, v := range data {
		m.sharedData[k] = v
	}
}

// MockReader implements ports.Reader for testing
type MockReader struct {
	registry *MockRegistry
	id       string
	data     map[string]interface{}
}

// Service operations
func (r *MockReader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	key := fmt.Sprintf("service_%s", id.Key())
	if svc, exists := r.data[key]; exists {
		if service, ok := svc.(*models.Service); ok {
			return service, nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *MockReader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	for key, value := range r.data {
		if service, ok := value.(*models.Service); ok && key[:8] == "service_" {
			if err := consume(*service); err != nil {
				return err
			}
		}
	}
	return nil
}

// AddressGroup operations
func (r *MockReader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	key := fmt.Sprintf("addressgroup_%s", id.Key())
	if ag, exists := r.data[key]; exists {
		if addressGroup, ok := ag.(*models.AddressGroup); ok {
			return addressGroup, nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *MockReader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	for key, value := range r.data {
		if ag, ok := value.(*models.AddressGroup); ok && key[:13] == "addressgroup_" {
			if err := consume(*ag); err != nil {
				return err
			}
		}
	}
	return nil
}

// Network operations
func (r *MockReader) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	key := fmt.Sprintf("network_%s", id.Key())
	if net, exists := r.data[key]; exists {
		if network, ok := net.(*models.Network); ok {
			return network, nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *MockReader) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	for key, value := range r.data {
		if net, ok := value.(*models.Network); ok && key[:8] == "network_" {
			if err := consume(*net); err != nil {
				return err
			}
		}
	}
	return nil
}

// NetworkBinding operations
func (r *MockReader) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	key := fmt.Sprintf("networkbinding_%s", id.Key())
	if nb, exists := r.data[key]; exists {
		if binding, ok := nb.(*models.NetworkBinding); ok {
			return binding, nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *MockReader) ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope ports.Scope) error {
	for key, value := range r.data {
		if nb, ok := value.(*models.NetworkBinding); ok && key[:15] == "networkbinding_" {
			if err := consume(*nb); err != nil {
				return err
			}
		}
	}
	return nil
}

// Implement other required reader methods with basic implementations
func (r *MockReader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil // Basic implementation for testing
}

func (r *MockReader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, ports.ErrNotFound
}

func (r *MockReader) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (r *MockReader) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, ports.ErrNotFound
}

func (r *MockReader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (r *MockReader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, ports.ErrNotFound
}

func (r *MockReader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	for key, value := range r.data {
		if rule, ok := value.(*models.RuleS2S); ok && key[:8] == "rules2s_" {
			if err := consume(*rule); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *MockReader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	key := fmt.Sprintf("rules2s_%s", id.Key())
	if rule, exists := r.data[key]; exists {
		if ruleS2S, ok := rule.(*models.RuleS2S); ok {
			return ruleS2S, nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *MockReader) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	for key, value := range r.data {
		if len(key) >= 13 && key[:13] == "servicealias_" {
			if alias, ok := value.(*models.ServiceAlias); ok {
				if err := consume(*alias); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *MockReader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	key := fmt.Sprintf("servicealias_%s", id.Key())
	if alias, exists := r.data[key]; exists {
		if serviceAlias, ok := alias.(*models.ServiceAlias); ok {
			return serviceAlias, nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *MockReader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	for key, value := range r.data {
		if len(key) >= 11 && key[:11] == "ieagagrule_" {
			if rule, ok := value.(*models.IEAgAgRule); ok {
				if err := consume(*rule); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *MockReader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	key := fmt.Sprintf("ieagagrule_%s", id.Key())
	if rule, exists := r.data[key]; exists {
		if ieAgAgRule, ok := rule.(*models.IEAgAgRule); ok {
			return ieAgAgRule, nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *MockReader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return &models.SyncStatus{}, nil
}

func (r *MockReader) Close() error {
	return nil
}

// MockWriter implements ports.Writer for testing
type MockWriter struct {
	registry    *MockRegistry
	id          string
	data        map[string]interface{}
	deletedKeys map[string]bool // Track deleted keys for proper commit handling
	committed   bool
}

func (w *MockWriter) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
	for _, service := range services {
		key := fmt.Sprintf("service_%s", service.SelfRef.ResourceIdentifier.Key())
		w.data[key] = &service
	}
	return nil
}

func (w *MockWriter) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	for _, id := range ids {
		key := fmt.Sprintf("service_%s", id.Key())
		delete(w.data, key)
		w.deletedKeys[key] = true // Track deletion
	}
	return nil
}

func (w *MockWriter) SyncAddressGroups(ctx context.Context, groups []models.AddressGroup, scope ports.Scope, opts ...ports.Option) error {
	for _, group := range groups {
		key := fmt.Sprintf("addressgroup_%s", group.SelfRef.ResourceIdentifier.Key())
		w.data[key] = &group
	}
	return nil
}

func (w *MockWriter) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	for _, id := range ids {
		key := fmt.Sprintf("addressgroup_%s", id.Key())
		delete(w.data, key)
		w.deletedKeys[key] = true // Track deletion
	}
	return nil
}

func (w *MockWriter) SyncNetworks(ctx context.Context, networks []models.Network, scope ports.Scope, opts ...ports.Option) error {
	for _, network := range networks {
		key := fmt.Sprintf("network_%s", network.SelfRef.ResourceIdentifier.Key())
		w.data[key] = &network
	}
	return nil
}

func (w *MockWriter) DeleteNetworksByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	for _, id := range ids {
		key := fmt.Sprintf("network_%s", id.Key())
		delete(w.data, key)
		w.deletedKeys[key] = true // Track deletion
	}
	return nil
}

func (w *MockWriter) SyncNetworkBindings(ctx context.Context, bindings []models.NetworkBinding, scope ports.Scope, opts ...ports.Option) error {
	for _, binding := range bindings {
		key := fmt.Sprintf("networkbinding_%s", binding.SelfRef.ResourceIdentifier.Key())
		w.data[key] = &binding
	}
	return nil
}

func (w *MockWriter) DeleteNetworkBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	for _, id := range ids {
		key := fmt.Sprintf("networkbinding_%s", id.Key())
		delete(w.data, key)
		w.deletedKeys[key] = true // Track deletion
	}
	return nil
}

// Implement other required writer methods with basic implementations
func (w *MockWriter) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, opts ...ports.Option) error {
	return nil
}

func (w *MockWriter) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return nil
}

func (w *MockWriter) SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope ports.Scope, opts ...ports.Option) error {
	return nil
}

func (w *MockWriter) DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return nil
}

func (w *MockWriter) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, opts ...ports.Option) error {
	return nil
}

func (w *MockWriter) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return nil
}

func (w *MockWriter) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, opts ...ports.Option) error {
	for _, rule := range rules {
		key := fmt.Sprintf("rules2s_%s", rule.SelfRef.ResourceIdentifier.Key())
		w.data[key] = &rule
	}
	return nil
}

func (w *MockWriter) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	for _, id := range ids {
		key := fmt.Sprintf("rules2s_%s", id.Key())
		delete(w.data, key)
		w.deletedKeys[key] = true // Track deletion
	}
	return nil
}

func (w *MockWriter) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope, opts ...ports.Option) error {
	for _, alias := range aliases {
		key := fmt.Sprintf("servicealias_%s", alias.SelfRef.ResourceIdentifier.Key())
		w.data[key] = &alias
	}
	return nil
}

func (w *MockWriter) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	for _, id := range ids {
		key := fmt.Sprintf("servicealias_%s", id.Key())
		delete(w.data, key)
		w.deletedKeys[key] = true // Track deletion
	}
	return nil
}

func (w *MockWriter) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope, opts ...ports.Option) error {
	for _, rule := range rules {
		key := fmt.Sprintf("ieagagrule_%s", rule.SelfRef.ResourceIdentifier.Key())
		w.data[key] = &rule
	}
	return nil
}

func (w *MockWriter) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	for _, id := range ids {
		key := fmt.Sprintf("ieagagrule_%s", id.Key())
		delete(w.data, key)
		w.deletedKeys[key] = true // Track deletion
	}
	return nil
}

func (w *MockWriter) Commit() error {
	w.committed = true

	// Create a copy of data to commit to minimize lock time
	dataToCommit := make(map[string]interface{})
	for k, v := range w.data {
		dataToCommit[k] = v
	}

	// Create a copy of deleted keys
	keysToDelete := make(map[string]bool)
	for k := range w.deletedKeys {
		keysToDelete[k] = true
	}

	// Persist writer's changes to the shared registry data with minimal lock time
	w.registry.mu.Lock()
	// Apply updates
	for k, v := range dataToCommit {
		w.registry.sharedData[k] = v
	}
	// Apply deletions
	for k := range keysToDelete {
		delete(w.registry.sharedData, k)
	}
	w.registry.mu.Unlock()

	return nil
}

func (w *MockWriter) Abort() {
	w.data = make(map[string]interface{}) // Clear uncommitted changes
	w.deletedKeys = make(map[string]bool) // Clear tracked deletions
}

// MockSubject implements patterns.Subject for testing
type MockSubject struct{}

func (s *MockSubject) String() string {
	return "mock-subject"
}

func (s *MockSubject) Subscribe(observer interface{}) error {
	// Mock implementation - do nothing
	return nil
}

func (s *MockSubject) Unsubscribe(observer interface{}) error {
	// Mock implementation - do nothing
	return nil
}

func (s *MockSubject) Notify(event interface{}) {
	// Mock implementation - do nothing
}
