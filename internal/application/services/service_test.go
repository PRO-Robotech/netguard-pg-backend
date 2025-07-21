package services_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/patterns"

	"github.com/pkg/errors"
)

// MockSubject реализует интерфейс patterns.Subject для тестирования
type MockSubject struct{}

func (m *MockSubject) Subscribe(observer interface{}) error {
	return nil
}

func (m *MockSubject) Unsubscribe(observer interface{}) error {
	return nil
}

func (m *MockSubject) Notify(event interface{}) {
	// Ничего не делаем в тесте
}

// MockRegistry для тестирования NetguardService
type MockRegistry struct {
	reader  *MockReader
	writer  *MockWriter
	subject *MockSubject
}

func NewMockRegistry() *MockRegistry {
	return &MockRegistry{
		reader:  NewMockReader(),
		writer:  NewMockWriter(),
		subject: &MockSubject{},
	}
}

func (m *MockRegistry) Reader(ctx context.Context) (ports.Reader, error) {
	return m.reader, nil
}

func (m *MockRegistry) Writer(ctx context.Context) (ports.Writer, error) {
	return m.writer, nil
}

func (m *MockRegistry) Subject() patterns.Subject {
	return m.subject
}

func (m *MockRegistry) ReaderFromWriter(ctx context.Context, writer ports.Writer) (ports.Reader, error) {
	return m.reader, nil
}

func (m *MockRegistry) Close() error {
	return nil
}

// MockReader для тестирования NetguardService
type MockReader struct {
	services                 map[string]models.Service
	addressGroups            map[string]models.AddressGroup
	addressGroupPortMappings map[string]models.AddressGroupPortMapping
	ieAgAgRules              map[string]models.IEAgAgRule
}

func NewMockReader() *MockReader {
	return &MockReader{
		services:                 make(map[string]models.Service),
		addressGroups:            make(map[string]models.AddressGroup),
		addressGroupPortMappings: make(map[string]models.AddressGroupPortMapping),
		ieAgAgRules:              make(map[string]models.IEAgAgRule),
	}
}

func (m *MockReader) Close() error {
	return nil
}

func (m *MockReader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	for _, service := range m.services {
		if err := consume(service); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockReader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	for _, ag := range m.addressGroups {
		if err := consume(ag); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockReader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	service, ok := m.services[id.Key()]
	if !ok {
		return nil, errors.New("service not found")
	}
	return &service, nil
}

func (m *MockReader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	ag, ok := m.addressGroups[id.Key()]
	if !ok {
		return nil, errors.New("address group not found")
	}
	return &ag, nil
}

func (m *MockReader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	agpm, ok := m.addressGroupPortMappings[id.Key()]
	if !ok {
		return nil, errors.New("address group port mapping not found")
	}
	return &agpm, nil
}

// Остальные методы интерфейса Reader, которые не используются в тестах
func (m *MockReader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, nil
}

func (m *MockReader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, nil
}

func (m *MockReader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, nil
}

func (m *MockReader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	for _, rule := range m.ieAgAgRules {
		if err := consume(rule); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockReader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	rule, ok := m.ieAgAgRules[id.Key()]
	if !ok {
		return nil, errors.New("IEAgAgRule not found")
	}
	return &rule, nil
}

// Вспомогательные методы для настройки мока
func (m *MockReader) AddService(service models.Service) {
	m.services[service.Key()] = service
}

func (m *MockReader) AddAddressGroup(ag models.AddressGroup) {
	m.addressGroups[ag.Key()] = ag
}

func (m *MockReader) AddAddressGroupPortMapping(agpm models.AddressGroupPortMapping) {
	m.addressGroupPortMappings[agpm.Key()] = agpm
}

// MockWriter для тестирования NetguardService
type MockWriter struct {
	syncServicesError error
	commitError       error
	abortCalled       bool
}

func (m *MockWriter) SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope ports.Scope, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return nil
}

func NewMockWriter() *MockWriter {
	return &MockWriter{}
}

func (m *MockWriter) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
	return m.syncServicesError
}

func (m *MockWriter) Commit() error {
	return m.commitError
}

func (m *MockWriter) Abort() {
	m.abortCalled = true
}

// Остальные методы интерфейса Writer, которые не используются в тестах
func (m *MockWriter) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope, opts ...ports.Option) error {
	return nil
}

func (m *MockWriter) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return nil
}

// TestCreateService тестирует метод CreateService
func TestCreateService(t *testing.T) {
	ctx := context.Background()
	mockRegistry := NewMockRegistry()

	// Создаем AddressGroup
	ag := models.AddressGroup{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-ag")),
	}
	mockRegistry.reader.AddAddressGroup(ag)

	// Создаем AddressGroupPortMapping
	agpm := models.AddressGroupPortMapping{
		SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("test-ag")),
		AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
	}
	mockRegistry.reader.AddAddressGroupPortMapping(agpm)

	// Создаем сервис для тестирования
	service := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: models.NewResourceIdentifier("test-ag")},
		},
	}

	netguardService := services.NewNetguardService(mockRegistry)

	// Тест успешного создания сервиса
	_, err := netguardService.CreateService(ctx, service)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Тест ошибки при создании сервиса (ошибка в SyncServices)
	mockRegistry.writer.syncServicesError = errors.New("sync services error")
	_, err = netguardService.CreateService(ctx, service)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Тест ошибки при создании сервиса (ошибка в Commit)
	mockRegistry.writer.syncServicesError = nil
	mockRegistry.writer.commitError = errors.New("commit error")
	_, err = netguardService.CreateService(ctx, service)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Тест ошибки валидации (перекрытие портов)
	mockRegistry.writer.commitError = nil

	// Добавляем существующий сервис с тем же портом
	existingService := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("existing-service")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
	}
	mockRegistry.reader.AddService(existingService)

	// Добавляем порты существующего сервиса в AddressGroupPortMapping
	servicePorts := models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 80, End: 80}},
		},
	}
	agpm.AccessPorts[models.NewServiceRef("existing-service")] = servicePorts
	mockRegistry.reader.AddAddressGroupPortMapping(agpm)

	// Должна быть ошибка валидации из-за перекрытия портов
	_, err = netguardService.CreateService(ctx, service)
	if err == nil {
		t.Error("Expected validation error, got nil")
	}
}

// TestGetServiceByID тестирует метод GetServiceByID
func TestGetServiceByID(t *testing.T) {
	ctx := context.Background()
	mockRegistry := NewMockRegistry()

	// Создаем сервис для тестирования
	service := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
	}
	mockRegistry.reader.AddService(service)

	netguardService := services.NewNetguardService(mockRegistry)

	// Тест успешного получения
	retrievedService, err := netguardService.GetServiceByID(ctx, models.NewResourceIdentifier("test-service"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if retrievedService == nil {
		t.Error("Expected service, got nil")
	} else if retrievedService.Key() != service.Key() {
		t.Errorf("Expected service with key %s, got %s", service.Key(), retrievedService.Key())
	}

	// Тест сервис не найден
	retrievedService, err = netguardService.GetServiceByID(ctx, models.NewResourceIdentifier("non-existent-service"))
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if retrievedService != nil {
		t.Errorf("Expected nil service, got %v", retrievedService)
	}
}

// TestGetServices тестирует метод GetServices
func TestGetServices(t *testing.T) {
	ctx := context.Background()
	mockRegistry := NewMockRegistry()

	// Создаем сервисы для тестирования
	service1 := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service-1")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
	}
	service2 := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service-2")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "443"},
		},
	}
	mockRegistry.reader.AddService(service1)
	mockRegistry.reader.AddService(service2)

	netguardService := services.NewNetguardService(mockRegistry)

	// Тест успешного получения
	svc, err := netguardService.GetServices(ctx, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(svc) != 2 {
		t.Errorf("Expected 2 services, got %d", len(svc))
	}

	// Тест с пустым результатом
	emptyRegistry := NewMockRegistry() // Сбрасываем реестр
	emptyService := services.NewNetguardService(emptyRegistry)
	emptyResults, err := emptyService.GetServices(ctx, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(emptyResults) != 0 {
		t.Errorf("Expected 0 services, got %d", len(emptyResults))
	}
}

// TestUpdateService тестирует метод UpdateService
func TestUpdateService(t *testing.T) {
	ctx := context.Background()
	mockRegistry := NewMockRegistry()

	// Создаем AddressGroup
	ag := models.AddressGroup{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-ag")),
	}
	mockRegistry.reader.AddAddressGroup(ag)

	// Создаем AddressGroupPortMapping
	agpm := models.AddressGroupPortMapping{
		SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("test-ag")),
		AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
	}

	// Добавляем существующий сервис с портом 80 TCP
	existingService := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("existing-service")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
	}
	mockRegistry.reader.AddService(existingService)

	// Добавляем сервис, который будем обновлять
	oldService := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "443"},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: models.NewResourceIdentifier("test-ag")},
		},
	}
	mockRegistry.reader.AddService(oldService)

	// Добавляем порты сервисов в AddressGroupPortMapping
	existingServicePorts := models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 80, End: 80}},
		},
	}
	agpm.AccessPorts[models.NewServiceRef("existing-service")] = existingServicePorts

	oldServicePorts := models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 443, End: 443}},
		},
	}
	agpm.AccessPorts[models.NewServiceRef("test-service")] = oldServicePorts

	mockRegistry.reader.AddAddressGroupPortMapping(agpm)

	netguardService := services.NewNetguardService(mockRegistry)

	// Создаем новую версию сервиса для обновления
	newService := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "8080"},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: models.NewResourceIdentifier("test-ag")},
		},
	}

	// Тест успешного обновления сервиса
	_, err := netguardService.UpdateService(ctx, newService)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Тест ошибки при обновлении сервиса (ошибка в SyncServices)
	mockRegistry.writer.syncServicesError = errors.New("sync services error")
	_, err = netguardService.UpdateService(ctx, newService)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Тест ошибки при обновлении сервиса (ошибка в Commit)
	mockRegistry.writer.syncServicesError = nil
	mockRegistry.writer.commitError = errors.New("commit error")
	_, err = netguardService.UpdateService(ctx, newService)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Тест ошибки валидации (перекрытие портов)
	mockRegistry.writer.commitError = nil

	// Создаем новую версию сервиса с портом, который перекрывается с существующим
	newServiceWithOverlap := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: models.NewResourceIdentifier("test-ag")},
		},
	}

	// Должна быть ошибка валидации из-за перекрытия портов
	_, err = netguardService.UpdateService(ctx, newServiceWithOverlap)
	if err == nil {
		t.Error("Expected validation error, got nil")
	}
}

func (m *MockReader) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, nil
}

// AddIEAgAgRule adds an IEAgAgRule to the mock reader
func (m *MockReader) AddIEAgAgRule(rule models.IEAgAgRule) {
	m.ieAgAgRules[rule.Key()] = rule
}
