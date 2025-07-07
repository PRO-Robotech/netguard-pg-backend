package client

import (
	"context"
	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"k8s.io/klog/v2"
)

// BackendClient интерфейс для всех операций с backend
type BackendClient interface {
	// CRUD операции для всех ресурсов
	GetService(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error)
	ListServices(ctx context.Context, scope ports.Scope) ([]models.Service, error)
	CreateService(ctx context.Context, service *models.Service) error
	UpdateService(ctx context.Context, service *models.Service) error
	DeleteService(ctx context.Context, id models.ResourceIdentifier) error

	GetAddressGroup(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error)
	ListAddressGroups(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error)
	CreateAddressGroup(ctx context.Context, group *models.AddressGroup) error
	UpdateAddressGroup(ctx context.Context, group *models.AddressGroup) error
	DeleteAddressGroup(ctx context.Context, id models.ResourceIdentifier) error

	GetAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error)
	ListAddressGroupBindings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBinding, error)
	CreateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error
	UpdateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error
	DeleteAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) error

	GetAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error)
	ListAddressGroupPortMappings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupPortMapping, error)
	CreateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error
	UpdateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error
	DeleteAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) error

	GetRuleS2S(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error)
	ListRuleS2S(ctx context.Context, scope ports.Scope) ([]models.RuleS2S, error)
	CreateRuleS2S(ctx context.Context, rule *models.RuleS2S) error
	UpdateRuleS2S(ctx context.Context, rule *models.RuleS2S) error
	DeleteRuleS2S(ctx context.Context, id models.ResourceIdentifier) error

	GetServiceAlias(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error)
	ListServiceAliases(ctx context.Context, scope ports.Scope) ([]models.ServiceAlias, error)
	CreateServiceAlias(ctx context.Context, alias *models.ServiceAlias) error
	UpdateServiceAlias(ctx context.Context, alias *models.ServiceAlias) error
	DeleteServiceAlias(ctx context.Context, id models.ResourceIdentifier) error

	GetAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error)
	ListAddressGroupBindingPolicies(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBindingPolicy, error)
	CreateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error
	UpdateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error
	DeleteAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) error

	GetIEAgAgRule(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error)
	ListIEAgAgRules(ctx context.Context, scope ports.Scope) ([]models.IEAgAgRule, error)
	CreateIEAgAgRule(ctx context.Context, rule *models.IEAgAgRule) error
	UpdateIEAgAgRule(ctx context.Context, rule *models.IEAgAgRule) error
	DeleteIEAgAgRule(ctx context.Context, id models.ResourceIdentifier) error

	// Sync операции
	Sync(ctx context.Context, syncOp models.SyncOp, resources interface{}) error
	GetSyncStatus(ctx context.Context) (*models.SyncStatus, error)

	// Валидация (доступ к валидаторам backend)
	GetDependencyValidator() *validation.DependencyValidator
	GetReader(ctx context.Context) (ports.Reader, error)

	// Health check
	HealthCheck(ctx context.Context) error

	// Graceful shutdown
	Close() error
}

// NewBackendClient создает backend-клиент. Если ENDPOINT == "mock" (или пуст),
// используется моковый клиент. Иначе создаётся реальный gRPC-клиент.
func NewBackendClient(config BackendClientConfig) (BackendClient, error) {
	// Валидация входных параметров
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Если явно указан mock – возвращаем мок-клиент для локальных тестов.
	if config.Endpoint == "mock" {
		return NewMockBackendClient(), nil
	}

	// Иначе пытаемся установить gRPC-соединение с backend
	klog.Infof("Dialing backend gRPC %s …", config.Endpoint)
	grpcClient, err := NewGRPCBackendClient(config)
	if err != nil {
		klog.Errorf("backend connection failed: %v", err)
		return nil, err
	}
	klog.Infof("Connected to backend gRPC %s", config.Endpoint)
	return grpcClient, nil
}
