package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestServiceValidator_ValidateExists tests the ValidateExists method of ServiceValidator
func TestServiceValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns a service for the test ID
	mockReader := &MockReaderForServiceValidator{
		serviceExists: true,
		serviceID:     "test-service",
	}

	validator := validation.NewServiceValidator(mockReader)
	serviceID := models.NewResourceIdentifier("test-service")

	// Test when service exists
	err := validator.ValidateExists(context.Background(), serviceID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when service does not exist
	mockReader.serviceExists = false
	err = validator.ValidateExists(context.Background(), serviceID)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestServiceValidator_CheckDependencies tests the CheckDependencies method of ServiceValidator
func TestServiceValidator_CheckDependencies(t *testing.T) {
	// Create a mock reader with no dependencies
	mockReader := &MockReaderForServiceValidator{
		serviceID:   "test-service",
		hasAliases:  false,
		hasBindings: false,
	}

	validator := validation.NewServiceValidator(mockReader)
	serviceID := models.NewResourceIdentifier("test-service")

	// Test when no dependencies exist
	err := validator.CheckDependencies(context.Background(), serviceID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when service alias dependency exists
	mockReader.hasAliases = true
	err = validator.CheckDependencies(context.Background(), serviceID)
	if err == nil {
		t.Error("Expected error for service alias dependency, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}

	// Test when address group binding dependency exists
	mockReader.hasAliases = false
	mockReader.hasBindings = true
	err = validator.CheckDependencies(context.Background(), serviceID)
	if err == nil {
		t.Error("Expected error for address group binding dependency, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}
}

// MockReaderForServiceValidator is a specialized mock for testing ServiceValidator
type MockReaderForServiceValidator struct {
	serviceExists bool
	serviceID     string
	hasAliases    bool
	hasBindings   bool

	// Дополнительные поля для тестирования новых методов
	addressGroups            map[string]models.AddressGroup
	addressGroupPortMappings map[string]models.AddressGroupPortMapping
	services                 map[string]models.Service
	addressGroupBindings     []models.AddressGroupBinding
}

func (m *MockReaderForServiceValidator) Close() error {
	return nil
}

func (m *MockReaderForServiceValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	if m.serviceExists {
		service := models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.serviceID),
			},
		}
		return consume(service)
	}
	return nil
}

func (m *MockReaderForServiceValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForServiceValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	if m.hasBindings {
		binding := models.AddressGroupBinding{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-binding"),
			},
			ServiceRef: models.ServiceRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.serviceID),
			},
		}
		return consume(binding)
	}
	return nil
}

func (m *MockReaderForServiceValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForServiceValidator) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForServiceValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	if m.hasAliases {
		alias := models.ServiceAlias{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-alias"),
			},
			ServiceRef: models.ServiceRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.serviceID),
			},
		}
		return consume(alias)
	}
	return nil
}

func (m *MockReaderForServiceValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForServiceValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	if m.services == nil {
		return nil, validation.NewEntityNotFoundError("service", id.Key())
	}

	service, ok := m.services[id.Key()]
	if !ok {
		return nil, validation.NewEntityNotFoundError("service", id.Key())
	}

	return &service, nil
}

func (m *MockReaderForServiceValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	if m.addressGroups == nil {
		return nil, validation.NewEntityNotFoundError("address_group", id.Key())
	}

	ag, ok := m.addressGroups[id.Key()]
	if !ok {
		return nil, validation.NewEntityNotFoundError("address_group", id.Key())
	}

	return &ag, nil
}

func (m *MockReaderForServiceValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, nil
}

func (m *MockReaderForServiceValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	if m.addressGroupPortMappings == nil {
		return nil, validation.NewEntityNotFoundError("address_group_port_mapping", id.Key())
	}

	agpm, ok := m.addressGroupPortMappings[id.Key()]
	if !ok {
		return nil, validation.NewEntityNotFoundError("address_group_port_mapping", id.Key())
	}

	return &agpm, nil
}

func (m *MockReaderForServiceValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, nil
}

func (m *MockReaderForServiceValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, nil
}

// Вспомогательные методы для настройки мока

// InitMockData инициализирует структуры данных мока
func (m *MockReaderForServiceValidator) InitMockData() {
	if m.services == nil {
		m.services = make(map[string]models.Service)
	}
	if m.addressGroups == nil {
		m.addressGroups = make(map[string]models.AddressGroup)
	}
	if m.addressGroupPortMappings == nil {
		m.addressGroupPortMappings = make(map[string]models.AddressGroupPortMapping)
	}
}

// AddService добавляет сервис в мок
func (m *MockReaderForServiceValidator) AddService(service models.Service) {
	m.InitMockData()
	m.services[service.Key()] = service
}

// AddAddressGroup добавляет адресную группу в мок
func (m *MockReaderForServiceValidator) AddAddressGroup(ag models.AddressGroup) {
	m.InitMockData()
	m.addressGroups[ag.Key()] = ag
}

// AddAddressGroupPortMapping добавляет маппинг портов адресной группы в мок
func (m *MockReaderForServiceValidator) AddAddressGroupPortMapping(agpm models.AddressGroupPortMapping) {
	m.InitMockData()
	m.addressGroupPortMappings[agpm.Key()] = agpm
}

// TestServiceValidator_ValidateNoDuplicatePorts тестирует метод ValidateNoDuplicatePorts
func TestServiceValidator_ValidateNoDuplicatePorts(t *testing.T) {
	mockReader := &MockReaderForServiceValidator{}
	validator := validation.NewServiceValidator(mockReader)

	tests := []struct {
		name         string
		ingressPorts []models.IngressPort
		expectError  bool
	}{
		{
			name: "No duplicate ports",
			ingressPorts: []models.IngressPort{
				{Protocol: models.TCP, Port: "80"},
				{Protocol: models.TCP, Port: "443"},
				{Protocol: models.UDP, Port: "53"},
			},
			expectError: false,
		},
		{
			name: "Duplicate TCP ports",
			ingressPorts: []models.IngressPort{
				{Protocol: models.TCP, Port: "80"},
				{Protocol: models.TCP, Port: "80"},
			},
			expectError: true,
		},
		{
			name: "Overlapping TCP port ranges",
			ingressPorts: []models.IngressPort{
				{Protocol: models.TCP, Port: "80-100"},
				{Protocol: models.TCP, Port: "90-110"},
			},
			expectError: true,
		},
		{
			name: "Same port different protocols",
			ingressPorts: []models.IngressPort{
				{Protocol: models.TCP, Port: "80"},
				{Protocol: models.UDP, Port: "80"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateNoDuplicatePorts(tt.ingressPorts)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for ports %v, but got nil", tt.ingressPorts)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for ports %v: %v", tt.ingressPorts, err)
			}
		})
	}
}

// TestServiceValidator_ValidateForCreation тестирует обновленный метод ValidateForCreation
func TestServiceValidator_ValidateForCreation(t *testing.T) {
	ctx := context.Background()
	mockReader := &MockReaderForServiceValidator{}
	mockReader.InitMockData()

	// Создаем AddressGroup
	ag := models.AddressGroup{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-ag")),
	}
	mockReader.AddAddressGroup(ag)

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
	mockReader.AddService(existingService)

	// Добавляем порты существующего сервиса в AddressGroupPortMapping
	servicePorts := models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 80, End: 80}},
		},
	}
	agpm.AccessPorts[models.NewServiceRef("existing-service")] = servicePorts
	mockReader.AddAddressGroupPortMapping(agpm)

	validator := validation.NewServiceValidator(mockReader)

	tests := []struct {
		name        string
		service     models.Service
		expectError bool
	}{
		{
			name: "Valid service with non-overlapping ports",
			service: models.Service{
				SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service")),
				IngressPorts: []models.IngressPort{
					{Protocol: models.TCP, Port: "443"},
				},
				AddressGroups: []models.AddressGroupRef{
					{ResourceIdentifier: models.NewResourceIdentifier("test-ag")},
				},
			},
			expectError: false,
		},
		{
			name: "Service with overlapping ports",
			service: models.Service{
				SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service-2")),
				IngressPorts: []models.IngressPort{
					{Protocol: models.TCP, Port: "80"},
				},
				AddressGroups: []models.AddressGroupRef{
					{ResourceIdentifier: models.NewResourceIdentifier("test-ag")},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateForCreation(ctx, tt.service)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for service %v, but got nil", tt.service)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for service %v: %v", tt.service, err)
			}
		})
	}
}

// TestServiceValidator_CheckBindingsPortOverlaps тестирует метод CheckBindingsPortOverlaps
func TestServiceValidator_CheckBindingsPortOverlaps(t *testing.T) {
	ctx := context.Background()
	mockReader := &MockReaderForServiceValidator{}
	mockReader.InitMockData()

	// Создаем AddressGroup
	ag := models.AddressGroup{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-ag")),
	}
	mockReader.AddAddressGroup(ag)

	// Создаем сервис, который будем проверять
	service := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "8080"},
		},
	}
	mockReader.AddService(service)

	// Создаем другой сервис с портом 80 TCP
	otherService := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("other-service")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"},
		},
	}
	mockReader.AddService(otherService)

	// Создаем AddressGroupBinding, связывающий test-service с test-ag
	binding := models.AddressGroupBinding{
		SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("test-binding")),
		ServiceRef:      models.NewServiceRef("test-service"),
		AddressGroupRef: models.NewAddressGroupRef("test-ag"),
	}

	// Добавляем binding в mock reader
	mockReader.bindings = append(mockReader.bindings, binding)

	// Создаем AddressGroupPortMapping для test-ag
	agpm := models.AddressGroupPortMapping{
		SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("test-ag")),
		AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
	}

	// Добавляем порты other-service в AddressGroupPortMapping
	otherServicePorts := models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 80, End: 80}},
		},
	}
	agpm.AccessPorts[models.NewServiceRef("other-service")] = otherServicePorts

	// Добавляем порты test-service в AddressGroupPortMapping
	servicePorts := models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 8080, End: 8080}},
		},
	}
	agpm.AccessPorts[models.NewServiceRef("test-service")] = servicePorts

	mockReader.AddAddressGroupPortMapping(agpm)

	validator := validation.NewServiceValidator(mockReader)

	// Тест 1: Сервис с неперекрывающимися портами
	err := validator.CheckBindingsPortOverlaps(ctx, service)
	if err != nil {
		t.Errorf("Unexpected error for service with non-overlapping ports: %v", err)
	}

	// Тест 2: Сервис с перекрывающимися портами
	serviceWithOverlap := models.Service{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service")),
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80"}, // Перекрывается с other-service
		},
	}
	err = validator.CheckBindingsPortOverlaps(ctx, serviceWithOverlap)
	if err == nil {
		t.Errorf("Expected error for service with overlapping ports, but got nil")
	}
}

// TestServiceValidator_ValidateForUpdate тестирует метод ValidateForUpdate
func TestServiceValidator_ValidateForUpdate(t *testing.T) {
	ctx := context.Background()
	mockReader := &MockReaderForServiceValidator{}
	mockReader.InitMockData()

	// Создаем AddressGroup
	ag := models.AddressGroup{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-ag")),
	}
	mockReader.AddAddressGroup(ag)

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
	mockReader.AddService(existingService)

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
	mockReader.AddService(oldService)

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

	mockReader.AddAddressGroupPortMapping(agpm)

	validator := validation.NewServiceValidator(mockReader)

	tests := []struct {
		name        string
		newService  models.Service
		expectError bool
	}{
		{
			name: "Valid update with non-overlapping ports",
			newService: models.Service{
				SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service")),
				IngressPorts: []models.IngressPort{
					{Protocol: models.TCP, Port: "8080"},
				},
				AddressGroups: []models.AddressGroupRef{
					{ResourceIdentifier: models.NewResourceIdentifier("test-ag")},
				},
			},
			expectError: false,
		},
		{
			name: "Update with overlapping ports",
			newService: models.Service{
				SelfRef: models.NewSelfRef(models.NewResourceIdentifier("test-service")),
				IngressPorts: []models.IngressPort{
					{Protocol: models.TCP, Port: "80"},
				},
				AddressGroups: []models.AddressGroupRef{
					{ResourceIdentifier: models.NewResourceIdentifier("test-ag")},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateForUpdate(ctx, oldService, tt.newService)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for service update %v, but got nil", tt.newService)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for service update %v: %v", tt.newService, err)
			}
		})
	}
}
