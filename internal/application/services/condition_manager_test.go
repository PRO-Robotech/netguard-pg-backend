package services

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/patterns"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MockReader для тестирования
type MockReader struct {
	mock.Mock
	services       map[string]models.Service
	addressGroups  map[string]models.AddressGroup
	serviceAliases map[string]models.ServiceAlias
	ieAgAgRules    map[string]models.IEAgAgRule
	portMappings   map[string]models.AddressGroupPortMapping
}

func NewMockReader() *MockReader {
	return &MockReader{
		services:       make(map[string]models.Service),
		addressGroups:  make(map[string]models.AddressGroup),
		serviceAliases: make(map[string]models.ServiceAlias),
		ieAgAgRules:    make(map[string]models.IEAgAgRule),
		portMappings:   make(map[string]models.AddressGroupPortMapping),
	}
}

func (m *MockReader) AddService(service models.Service) {
	m.services[service.Key()] = service
}

func (m *MockReader) AddAddressGroup(ag models.AddressGroup) {
	m.addressGroups[ag.Key()] = ag
}

func (m *MockReader) AddServiceAlias(alias models.ServiceAlias) {
	m.serviceAliases[alias.Key()] = alias
}

func (m *MockReader) AddIEAgAgRule(rule models.IEAgAgRule) {
	m.ieAgAgRules[rule.Key()] = rule
}

func (m *MockReader) AddPortMapping(mapping models.AddressGroupPortMapping) {
	m.portMappings[mapping.Key()] = mapping
}

func (m *MockReader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	if service, exists := m.services[id.Key()]; exists {
		return &service, nil
	}
	return nil, ports.ErrNotFound
}

func (m *MockReader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	if ag, exists := m.addressGroups[id.Key()]; exists {
		return &ag, nil
	}
	return nil, ports.ErrNotFound
}

func (m *MockReader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	if alias, exists := m.serviceAliases[id.Key()]; exists {
		return &alias, nil
	}
	return nil, ports.ErrNotFound
}

func (m *MockReader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	if rule, exists := m.ieAgAgRules[id.Key()]; exists {
		return &rule, nil
	}
	return nil, ports.ErrNotFound
}

func (m *MockReader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	if mapping, exists := m.portMappings[id.Key()]; exists {
		return &mapping, nil
	}
	return nil, ports.ErrNotFound
}

func (m *MockReader) Close() error {
	return nil
}

// Stub implementations для остальных методов Reader интерфейса
func (m *MockReader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	for _, service := range m.services {
		if err := consume(service); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockReader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	// Если scope предоставлен, фильтруем по нему
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				if ag, exists := m.addressGroups[id.Key()]; exists {
					if err := consume(ag); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}

	// Если scope не предоставлен, возвращаем все
	for _, ag := range m.addressGroups {
		if err := consume(ag); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockReader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	for _, mapping := range m.portMappings {
		if err := consume(mapping); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockReader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	for _, alias := range m.serviceAliases {
		if err := consume(alias); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockReader) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	for _, rule := range m.ieAgAgRules {
		if err := consume(rule); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockReader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) { return nil, nil }
func (m *MockReader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, ports.ErrNotFound
}
func (m *MockReader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, ports.ErrNotFound
}
func (m *MockReader) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, ports.ErrNotFound
}

// MockRegistry для тестирования
type MockRegistry struct {
	mock.Mock
	reader *MockReader
}

func NewMockRegistry() *MockRegistry {
	return &MockRegistry{
		reader: NewMockReader(),
	}
}

func (m *MockRegistry) Reader(ctx context.Context) (ports.Reader, error) {
	return m.reader, nil
}

func (m *MockRegistry) Writer(ctx context.Context) (ports.Writer, error) {
	// Простая реализация для тестов
	return nil, nil
}

func (m *MockRegistry) ReaderFromWriter(ctx context.Context, writer ports.Writer) (ports.Reader, error) {
	return m.reader, nil
}

func (m *MockRegistry) Subject() patterns.Subject {
	return nil
}

func (m *MockRegistry) Close() error {
	return nil
}

func TestConditionManager_ProcessServiceConditions_Success(t *testing.T) {
	ctx := context.Background()
	registry := NewMockRegistry()
	netguardService := NewNetguardService(registry)
	conditionManager := NewConditionManager(registry, netguardService)

	// Создаем сервис с AddressGroup
	service := &models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service", models.WithNamespace("default"))),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default"))},
		},
	}

	// Добавляем AddressGroup в mock
	addressGroup := models.AddressGroup{
		SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("test-ag", models.WithNamespace("default"))),
		DefaultAction: models.ActionAccept,
		Logs:          true,
		Trace:         false,
	}
	registry.reader.AddAddressGroup(addressGroup)

	// Execute
	err := conditionManager.ProcessServiceConditions(ctx, service)

	// Verify
	assert.NoError(t, err)
	assert.True(t, service.Meta.IsReady())
	assert.True(t, service.Meta.IsSynced())
	assert.True(t, service.Meta.IsValidated())
	assert.False(t, service.Meta.HasError())

	// Проверяем что ResourceVersion обновлен
	assert.Equal(t, "v1", service.Meta.ResourceVersion)
}

func TestConditionManager_ProcessServiceConditions_MissingAddressGroup(t *testing.T) {
	ctx := context.Background()
	registry := NewMockRegistry()
	netguardService := NewNetguardService(registry)
	conditionManager := NewConditionManager(registry, netguardService)

	// Создаем сервис с несуществующим AddressGroup
	service := &models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service", models.WithNamespace("default"))),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: models.NewResourceIdentifier("missing-ag", models.WithNamespace("default"))},
		},
	}

	// Execute
	err := conditionManager.ProcessServiceConditions(ctx, service)

	// Verify
	assert.NoError(t, err)
	assert.False(t, service.Meta.IsReady())
	assert.True(t, service.Meta.IsSynced())
	assert.True(t, service.Meta.IsValidated())
	assert.True(t, service.Meta.HasError())

	// Проверяем условие ошибки
	errorCondition := service.Meta.GetCondition(models.ConditionError)
	assert.NotNil(t, errorCondition)
	assert.Equal(t, metav1.ConditionTrue, errorCondition.Status)
	assert.Contains(t, errorCondition.Message, "Missing AddressGroups")
}

func TestConditionManager_ProcessServiceConditions_NoIngressPorts(t *testing.T) {
	ctx := context.Background()
	registry := NewMockRegistry()
	netguardService := NewNetguardService(registry)
	conditionManager := NewConditionManager(registry, netguardService)

	// Создаем сервис без портов
	service := &models.Service{
		SelfRef:      models.NewSelfRef(models.NewResourceIdentifier("test-service", models.WithNamespace("default"))),
		IngressPorts: []models.IngressPort{}, // Пустой список портов
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default"))},
		},
	}

	// Добавляем AddressGroup в mock
	addressGroup := models.AddressGroup{
		SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("test-ag", models.WithNamespace("default"))),
		DefaultAction: models.ActionAccept,
		Logs:          true,
		Trace:         false,
	}
	registry.reader.AddAddressGroup(addressGroup)

	// Execute
	err := conditionManager.ProcessServiceConditions(ctx, service)

	// Verify
	assert.NoError(t, err)
	assert.False(t, service.Meta.IsReady())
	assert.True(t, service.Meta.IsSynced())
	assert.True(t, service.Meta.IsValidated())
	assert.False(t, service.Meta.HasError())

	// Проверяем Ready condition
	readyCondition := service.Meta.GetCondition(models.ConditionReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, models.ReasonPending, readyCondition.Reason)
	assert.Contains(t, readyCondition.Message, "no ingress ports configured")
}

func TestConditionManager_ProcessRuleS2SConditions_Success(t *testing.T) {
	ctx := context.Background()
	registry := NewMockRegistry()
	netguardService := NewNetguardService(registry)
	conditionManager := NewConditionManager(registry, netguardService)

	// Создаем правило RuleS2S
	rule := &models.RuleS2S{
		SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("test-rule", models.WithNamespace("default"))),
		ServiceLocalRef: models.NewServiceAliasRef("local-alias", models.WithNamespace("default")),
		ServiceRef:      models.NewServiceAliasRef("target-alias", models.WithNamespace("default")),
		Traffic:         models.EGRESS,
	}

	// Добавляем ServiceAlias в mock
	localAlias := models.ServiceAlias{
		SelfRef:    models.NewSelfRef(models.NewResourceIdentifier("local-alias", models.WithNamespace("default"))),
		ServiceRef: models.NewServiceRef("local-service", models.WithNamespace("default")),
	}
	targetAlias := models.ServiceAlias{
		SelfRef:    models.NewSelfRef(models.NewResourceIdentifier("target-alias", models.WithNamespace("default"))),
		ServiceRef: models.NewServiceRef("target-service", models.WithNamespace("default")),
	}
	registry.reader.AddServiceAlias(localAlias)
	registry.reader.AddServiceAlias(targetAlias)

	// Добавляем Services в mock
	localService := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("local-service", models.WithNamespace("default"))),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: models.NewResourceIdentifier("local-ag", models.WithNamespace("default"))},
		},
	}
	targetService := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("target-service", models.WithNamespace("default"))),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "443"},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: models.NewResourceIdentifier("target-ag", models.WithNamespace("default"))},
		},
	}
	registry.reader.AddService(localService)
	registry.reader.AddService(targetService)

	// Execute
	err := conditionManager.ProcessRuleS2SConditions(ctx, rule)

	// Verify
	assert.NoError(t, err)
	assert.True(t, rule.Meta.IsSynced())
	assert.True(t, rule.Meta.IsValidated())
}

func TestConditionManager_ProcessAddressGroupBindingConditions_Success(t *testing.T) {
	ctx := context.Background()
	registry := NewMockRegistry()
	netguardService := NewNetguardService(registry)
	conditionManager := NewConditionManager(registry, netguardService)

	// Создаем binding
	binding := &models.AddressGroupBinding{
		SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("test-binding", models.WithNamespace("default"))),
		ServiceRef:      models.NewServiceRef("test-service", models.WithNamespace("default")),
		AddressGroupRef: models.NewAddressGroupRef("test-ag", models.WithNamespace("default")),
	}

	// Добавляем Service и AddressGroup в mock
	service := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service", models.WithNamespace("default"))),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
	}
	addressGroup := models.AddressGroup{
		SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("test-ag", models.WithNamespace("default"))),
		DefaultAction: models.ActionAccept,
		Logs:          true,
		Trace:         false,
	}
	portMapping := models.AddressGroupPortMapping{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-ag", models.WithNamespace("default"))),
		AccessPorts: map[models.ServiceRef]models.ServicePorts{
			binding.ServiceRef: {
				Ports: models.ProtocolPorts{
					models.TCP: []models.PortRange{{Start: 80, End: 80}},
				},
			},
		},
	}

	registry.reader.AddService(service)
	registry.reader.AddAddressGroup(addressGroup)
	registry.reader.AddPortMapping(portMapping)

	// Execute
	err := conditionManager.ProcessAddressGroupBindingConditions(ctx, binding)

	// Verify
	assert.NoError(t, err)
	assert.True(t, binding.Meta.IsReady())
	assert.True(t, binding.Meta.IsSynced())
	assert.True(t, binding.Meta.IsValidated())
	assert.False(t, binding.Meta.HasError())
}

func TestConditionManager_SetDefaultConditions(t *testing.T) {
	registry := NewMockRegistry()
	netguardService := NewNetguardService(registry)
	conditionManager := NewConditionManager(registry, netguardService)

	service := &models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service", models.WithNamespace("default"))),
	}

	// Execute
	conditionManager.SetDefaultConditions(service)

	// Verify
	assert.False(t, service.Meta.IsReady())
	assert.NotNil(t, service.Meta.GetCondition(models.ConditionReady))
	assert.NotNil(t, service.Meta.GetCondition(models.ConditionSynced))
	assert.NotNil(t, service.Meta.GetCondition(models.ConditionValidated))

	// Проверяем начальные значения
	readyCondition := service.Meta.GetCondition(models.ConditionReady)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, models.ReasonPending, readyCondition.Reason)

	syncedCondition := service.Meta.GetCondition(models.ConditionSynced)
	assert.Equal(t, metav1.ConditionUnknown, syncedCondition.Status)
	assert.Equal(t, models.ReasonPending, syncedCondition.Reason)

	validatedCondition := service.Meta.GetCondition(models.ConditionValidated)
	assert.Equal(t, metav1.ConditionUnknown, validatedCondition.Status)
	assert.Equal(t, models.ReasonPending, validatedCondition.Reason)

	// Проверяем что метаданные инициализированы
	assert.NotEmpty(t, service.Meta.UID)
	assert.Equal(t, int64(1), service.Meta.Generation)
	assert.False(t, service.Meta.CreationTS.IsZero())
}

func TestServiceConditions_ConceptualSave(t *testing.T) {
	ctx := context.Background()
	registry := NewMockRegistry()
	netguardService := NewNetguardService(registry)
	conditionManager := NewConditionManager(registry, netguardService)

	// Создаем сервис
	service := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service", models.WithNamespace("default"))),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default"))},
		},
	}

	// Добавляем AddressGroup в mock reader для валидации
	addressGroup := models.AddressGroup{
		SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("test-ag", models.WithNamespace("default"))),
		DefaultAction: models.ActionAccept,
		Logs:          true,
		Trace:         false,
	}
	registry.reader.AddAddressGroup(addressGroup)

	// Тестируем что conditions формируются правильно
	conditionManager.SetDefaultConditions(&service)
	err := conditionManager.ProcessServiceConditions(ctx, &service)

	// Проверяем результат
	assert.NoError(t, err)

	// 🔥 КЛЮЧЕВАЯ ПРОВЕРКА: Conditions должны быть установлены
	assert.True(t, service.Meta.IsReady())
	assert.True(t, service.Meta.IsSynced())
	assert.True(t, service.Meta.IsValidated())
	assert.False(t, service.Meta.HasError())

	// Проверяем что ResourceVersion обновлен
	assert.Equal(t, "v1", service.Meta.ResourceVersion)

	// Проверяем что условия сохранены в Meta
	readyCondition := service.Meta.GetCondition(models.ConditionReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
	assert.Equal(t, models.ReasonReady, readyCondition.Reason)

	syncedCondition := service.Meta.GetCondition(models.ConditionSynced)
	assert.NotNil(t, syncedCondition)
	assert.Equal(t, metav1.ConditionTrue, syncedCondition.Status)
	assert.Equal(t, models.ReasonSynced, syncedCondition.Reason)

	validatedCondition := service.Meta.GetCondition(models.ConditionValidated)
	assert.NotNil(t, validatedCondition)
	assert.Equal(t, metav1.ConditionTrue, validatedCondition.Status)
	assert.Equal(t, models.ReasonValidated, validatedCondition.Reason)

	t.Logf("✅ PASSED: Conditions are properly formed and will be saved via saveResourceConditions method")
}
