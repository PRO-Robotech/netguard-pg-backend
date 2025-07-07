package client

import (
	"context"
	"log"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/sony/gobreaker"
)

// CircuitBreakerClient слой circuit breaker для BackendClient

type CircuitBreakerClient struct {
	backend BackendClient
	breaker *gobreaker.CircuitBreaker
}

func NewCircuitBreakerClient(backend BackendClient, config BackendClientConfig) *CircuitBreakerClient {
	settings := gobreaker.Settings{
		Name:        "backend-client",
		MaxRequests: config.CBMaxRequests,
		Interval:    config.CBInterval,
		Timeout:     config.CBTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= config.CBFailureThreshold
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			log.Printf("Circuit breaker %s changed from %s to %s", name, from, to)
		},
	}
	return &CircuitBreakerClient{
		backend: backend,
		breaker: gobreaker.NewCircuitBreaker(settings),
	}
}

// --- Делегирование всех методов BackendClient через breaker.Execute ---
func (c *CircuitBreakerClient) GetService(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.GetService(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.Service), nil
}

func (c *CircuitBreakerClient) ListServices(ctx context.Context, scope ports.Scope) ([]models.Service, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.ListServices(ctx, scope)
	})
	if err != nil {
		return nil, err
	}
	return result.([]models.Service), nil
}

func (c *CircuitBreakerClient) CreateService(ctx context.Context, service *models.Service) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.CreateService(ctx, service)
	})
	return err
}

func (c *CircuitBreakerClient) UpdateService(ctx context.Context, service *models.Service) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.UpdateService(ctx, service)
	})
	return err
}

func (c *CircuitBreakerClient) DeleteService(ctx context.Context, id models.ResourceIdentifier) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.DeleteService(ctx, id)
	})
	return err
}

// AddressGroup
func (c *CircuitBreakerClient) GetAddressGroup(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.GetAddressGroup(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.AddressGroup), nil
}
func (c *CircuitBreakerClient) ListAddressGroups(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.ListAddressGroups(ctx, scope)
	})
	if err != nil {
		return nil, err
	}
	return result.([]models.AddressGroup), nil
}
func (c *CircuitBreakerClient) CreateAddressGroup(ctx context.Context, group *models.AddressGroup) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.CreateAddressGroup(ctx, group)
	})
	return err
}
func (c *CircuitBreakerClient) UpdateAddressGroup(ctx context.Context, group *models.AddressGroup) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.UpdateAddressGroup(ctx, group)
	})
	return err
}
func (c *CircuitBreakerClient) DeleteAddressGroup(ctx context.Context, id models.ResourceIdentifier) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.DeleteAddressGroup(ctx, id)
	})
	return err
}

// AddressGroupBinding
func (c *CircuitBreakerClient) GetAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.GetAddressGroupBinding(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.AddressGroupBinding), nil
}
func (c *CircuitBreakerClient) ListAddressGroupBindings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBinding, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.ListAddressGroupBindings(ctx, scope)
	})
	if err != nil {
		return nil, err
	}
	return result.([]models.AddressGroupBinding), nil
}
func (c *CircuitBreakerClient) CreateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.CreateAddressGroupBinding(ctx, binding)
	})
	return err
}
func (c *CircuitBreakerClient) UpdateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.UpdateAddressGroupBinding(ctx, binding)
	})
	return err
}
func (c *CircuitBreakerClient) DeleteAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.DeleteAddressGroupBinding(ctx, id)
	})
	return err
}

// AddressGroupPortMapping
func (c *CircuitBreakerClient) GetAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.GetAddressGroupPortMapping(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.AddressGroupPortMapping), nil
}
func (c *CircuitBreakerClient) ListAddressGroupPortMappings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupPortMapping, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.ListAddressGroupPortMappings(ctx, scope)
	})
	if err != nil {
		return nil, err
	}
	return result.([]models.AddressGroupPortMapping), nil
}
func (c *CircuitBreakerClient) CreateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.CreateAddressGroupPortMapping(ctx, mapping)
	})
	return err
}
func (c *CircuitBreakerClient) UpdateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.UpdateAddressGroupPortMapping(ctx, mapping)
	})
	return err
}
func (c *CircuitBreakerClient) DeleteAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.DeleteAddressGroupPortMapping(ctx, id)
	})
	return err
}

// RuleS2S
func (c *CircuitBreakerClient) GetRuleS2S(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.GetRuleS2S(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.RuleS2S), nil
}
func (c *CircuitBreakerClient) ListRuleS2S(ctx context.Context, scope ports.Scope) ([]models.RuleS2S, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.ListRuleS2S(ctx, scope)
	})
	if err != nil {
		return nil, err
	}
	return result.([]models.RuleS2S), nil
}
func (c *CircuitBreakerClient) CreateRuleS2S(ctx context.Context, rule *models.RuleS2S) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.CreateRuleS2S(ctx, rule)
	})
	return err
}
func (c *CircuitBreakerClient) UpdateRuleS2S(ctx context.Context, rule *models.RuleS2S) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.UpdateRuleS2S(ctx, rule)
	})
	return err
}
func (c *CircuitBreakerClient) DeleteRuleS2S(ctx context.Context, id models.ResourceIdentifier) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.DeleteRuleS2S(ctx, id)
	})
	return err
}

// ServiceAlias
func (c *CircuitBreakerClient) GetServiceAlias(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.GetServiceAlias(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.ServiceAlias), nil
}
func (c *CircuitBreakerClient) ListServiceAliases(ctx context.Context, scope ports.Scope) ([]models.ServiceAlias, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.ListServiceAliases(ctx, scope)
	})
	if err != nil {
		return nil, err
	}
	return result.([]models.ServiceAlias), nil
}
func (c *CircuitBreakerClient) CreateServiceAlias(ctx context.Context, alias *models.ServiceAlias) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.CreateServiceAlias(ctx, alias)
	})
	return err
}
func (c *CircuitBreakerClient) UpdateServiceAlias(ctx context.Context, alias *models.ServiceAlias) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.UpdateServiceAlias(ctx, alias)
	})
	return err
}
func (c *CircuitBreakerClient) DeleteServiceAlias(ctx context.Context, id models.ResourceIdentifier) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.DeleteServiceAlias(ctx, id)
	})
	return err
}

// AddressGroupBindingPolicy
func (c *CircuitBreakerClient) GetAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.GetAddressGroupBindingPolicy(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.AddressGroupBindingPolicy), nil
}
func (c *CircuitBreakerClient) ListAddressGroupBindingPolicies(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBindingPolicy, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.ListAddressGroupBindingPolicies(ctx, scope)
	})
	if err != nil {
		return nil, err
	}
	return result.([]models.AddressGroupBindingPolicy), nil
}
func (c *CircuitBreakerClient) CreateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.CreateAddressGroupBindingPolicy(ctx, policy)
	})
	return err
}
func (c *CircuitBreakerClient) UpdateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.UpdateAddressGroupBindingPolicy(ctx, policy)
	})
	return err
}
func (c *CircuitBreakerClient) DeleteAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.DeleteAddressGroupBindingPolicy(ctx, id)
	})
	return err
}

// IEAgAgRule
func (c *CircuitBreakerClient) GetIEAgAgRule(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.GetIEAgAgRule(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.IEAgAgRule), nil
}
func (c *CircuitBreakerClient) ListIEAgAgRules(ctx context.Context, scope ports.Scope) ([]models.IEAgAgRule, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.ListIEAgAgRules(ctx, scope)
	})
	if err != nil {
		return nil, err
	}
	return result.([]models.IEAgAgRule), nil
}
func (c *CircuitBreakerClient) CreateIEAgAgRule(ctx context.Context, rule *models.IEAgAgRule) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.CreateIEAgAgRule(ctx, rule)
	})
	return err
}
func (c *CircuitBreakerClient) UpdateIEAgAgRule(ctx context.Context, rule *models.IEAgAgRule) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.UpdateIEAgAgRule(ctx, rule)
	})
	return err
}
func (c *CircuitBreakerClient) DeleteIEAgAgRule(ctx context.Context, id models.ResourceIdentifier) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.DeleteIEAgAgRule(ctx, id)
	})
	return err
}

// Sync, HealthCheck, Close, GetDependencyValidator, GetSyncStatus
func (c *CircuitBreakerClient) Sync(ctx context.Context, syncOp models.SyncOp, resources interface{}) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.Sync(ctx, syncOp, resources)
	})
	return err
}
func (c *CircuitBreakerClient) GetDependencyValidator() *validation.DependencyValidator {
	return c.backend.GetDependencyValidator()
}
func (c *CircuitBreakerClient) GetReader(ctx context.Context) (ports.Reader, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.GetReader(ctx)
	})
	if err != nil {
		return nil, err
	}
	return result.(ports.Reader), nil
}
func (c *CircuitBreakerClient) HealthCheck(ctx context.Context) error {
	_, err := c.breaker.Execute(func() (interface{}, error) {
		return nil, c.backend.HealthCheck(ctx)
	})
	return err
}
func (c *CircuitBreakerClient) Close() error {
	return c.backend.Close()
}
func (c *CircuitBreakerClient) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.backend.GetSyncStatus(ctx)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.SyncStatus), nil
}
