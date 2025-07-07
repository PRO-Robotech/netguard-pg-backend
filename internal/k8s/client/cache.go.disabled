package client

import (
	"context"
	"fmt"
	"log"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	cache "github.com/patrickmn/go-cache"
)

// CachedBackendClient слой кэширования для BackendClient

type CachedBackendClient struct {
	backend BackendClient
	cache   *cache.Cache
	config  BackendClientConfig
}

func NewCachedBackendClient(backend BackendClient, config BackendClientConfig) *CachedBackendClient {
	return &CachedBackendClient{
		backend: backend,
		cache:   cache.New(config.CacheDefaultTTL, config.CacheCleanupInterval),
		config:  config,
	}
}

// --- Service ---
func (c *CachedBackendClient) GetService(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	key := fmt.Sprintf("service:%s", id.Key())
	service, err := c.backend.GetService(ctx, id)
	if err == nil {
		c.cache.Set(key, service, cache.DefaultExpiration)
		return service, nil
	}
	if cached, found := c.cache.Get(key); found {
		log.Printf("Backend unavailable, serving from cache: %s", key)
		return cached.(*models.Service), nil
	}
	return nil, fmt.Errorf("service not found in backend or cache: %w", err)
}

func (c *CachedBackendClient) ListServices(ctx context.Context, scope ports.Scope) ([]models.Service, error) {
	var cacheKey string
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			if ris.Identifiers[0].Namespace != "" {
				cacheKey = fmt.Sprintf("services:list:%s", ris.Identifiers[0].Namespace)
			}
		}
	}
	services, err := c.backend.ListServices(ctx, scope)
	if err == nil && cacheKey != "" {
		c.cache.Set(cacheKey, services, cache.DefaultExpiration)
		return services, nil
	}
	if err != nil && cacheKey != "" {
		if cached, found := c.cache.Get(cacheKey); found {
			log.Printf("Backend unavailable, serving list from cache: %s", cacheKey)
			return cached.([]models.Service), nil
		}
	}
	return services, err
}

func (c *CachedBackendClient) CreateService(ctx context.Context, service *models.Service) error {
	err := c.backend.CreateService(ctx, service)
	if err != nil {
		return err
	}
	c.invalidateServiceCache(service.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) UpdateService(ctx context.Context, service *models.Service) error {
	err := c.backend.UpdateService(ctx, service)
	if err != nil {
		return err
	}
	c.invalidateServiceCache(service.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) DeleteService(ctx context.Context, id models.ResourceIdentifier) error {
	err := c.backend.DeleteService(ctx, id)
	if err != nil {
		return err
	}
	c.invalidateServiceCache(id)
	return nil
}

func (c *CachedBackendClient) invalidateServiceCache(id models.ResourceIdentifier) {
	key := fmt.Sprintf("service:%s", id.Key())
	c.cache.Delete(key)
	listKey := fmt.Sprintf("services:list:%s", id.Namespace)
	c.cache.Delete(listKey)
}

// AddressGroup
func (c *CachedBackendClient) GetAddressGroup(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	key := fmt.Sprintf("addressgroup:%s", id.Key())
	group, err := c.backend.GetAddressGroup(ctx, id)
	if err == nil {
		c.cache.Set(key, group, cache.DefaultExpiration)
		return group, nil
	}
	if cached, found := c.cache.Get(key); found {
		log.Printf("Backend unavailable, serving from cache: %s", key)
		return cached.(*models.AddressGroup), nil
	}
	return nil, fmt.Errorf("address group not found in backend or cache: %w", err)
}

func (c *CachedBackendClient) ListAddressGroups(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error) {
	var cacheKey string
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			if ris.Identifiers[0].Namespace != "" {
				cacheKey = fmt.Sprintf("addressgroups:list:%s", ris.Identifiers[0].Namespace)
			}
		}
	}
	groups, err := c.backend.ListAddressGroups(ctx, scope)
	if err == nil && cacheKey != "" {
		c.cache.Set(cacheKey, groups, cache.DefaultExpiration)
		return groups, nil
	}
	if err != nil && cacheKey != "" {
		if cached, found := c.cache.Get(cacheKey); found {
			log.Printf("Backend unavailable, serving list from cache: %s", cacheKey)
			return cached.([]models.AddressGroup), nil
		}
	}
	return groups, err
}

func (c *CachedBackendClient) CreateAddressGroup(ctx context.Context, group *models.AddressGroup) error {
	err := c.backend.CreateAddressGroup(ctx, group)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupCache(group.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) UpdateAddressGroup(ctx context.Context, group *models.AddressGroup) error {
	err := c.backend.UpdateAddressGroup(ctx, group)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupCache(group.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) DeleteAddressGroup(ctx context.Context, id models.ResourceIdentifier) error {
	err := c.backend.DeleteAddressGroup(ctx, id)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupCache(id)
	return nil
}

func (c *CachedBackendClient) invalidateAddressGroupCache(id models.ResourceIdentifier) {
	key := fmt.Sprintf("addressgroup:%s", id.Key())
	c.cache.Delete(key)
	listKey := fmt.Sprintf("addressgroups:list:%s", id.Namespace)
	c.cache.Delete(listKey)
}

// AddressGroupBinding
func (c *CachedBackendClient) GetAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	key := fmt.Sprintf("addressgroupbinding:%s", id.Key())
	binding, err := c.backend.GetAddressGroupBinding(ctx, id)
	if err == nil {
		c.cache.Set(key, binding, cache.DefaultExpiration)
		return binding, nil
	}
	if cached, found := c.cache.Get(key); found {
		log.Printf("Backend unavailable, serving from cache: %s", key)
		return cached.(*models.AddressGroupBinding), nil
	}
	return nil, fmt.Errorf("address group binding not found in backend or cache: %w", err)
}

func (c *CachedBackendClient) ListAddressGroupBindings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBinding, error) {
	var cacheKey string
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			if ris.Identifiers[0].Namespace != "" {
				cacheKey = fmt.Sprintf("addressgroupbindings:list:%s", ris.Identifiers[0].Namespace)
			}
		}
	}
	bindings, err := c.backend.ListAddressGroupBindings(ctx, scope)
	if err == nil && cacheKey != "" {
		c.cache.Set(cacheKey, bindings, cache.DefaultExpiration)
		return bindings, nil
	}
	if err != nil && cacheKey != "" {
		if cached, found := c.cache.Get(cacheKey); found {
			log.Printf("Backend unavailable, serving list from cache: %s", cacheKey)
			return cached.([]models.AddressGroupBinding), nil
		}
	}
	return bindings, err
}

func (c *CachedBackendClient) CreateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error {
	err := c.backend.CreateAddressGroupBinding(ctx, binding)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupBindingCache(binding.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) UpdateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error {
	err := c.backend.UpdateAddressGroupBinding(ctx, binding)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupBindingCache(binding.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) DeleteAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) error {
	err := c.backend.DeleteAddressGroupBinding(ctx, id)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupBindingCache(id)
	return nil
}

func (c *CachedBackendClient) invalidateAddressGroupBindingCache(id models.ResourceIdentifier) {
	key := fmt.Sprintf("addressgroupbinding:%s", id.Key())
	c.cache.Delete(key)
	listKey := fmt.Sprintf("addressgroupbindings:list:%s", id.Namespace)
	c.cache.Delete(listKey)
}

// AddressGroupPortMapping
func (c *CachedBackendClient) GetAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	key := fmt.Sprintf("addressgroupportmapping:%s", id.Key())
	mapping, err := c.backend.GetAddressGroupPortMapping(ctx, id)
	if err == nil {
		c.cache.Set(key, mapping, cache.DefaultExpiration)
		return mapping, nil
	}
	if cached, found := c.cache.Get(key); found {
		log.Printf("Backend unavailable, serving from cache: %s", key)
		return cached.(*models.AddressGroupPortMapping), nil
	}
	return nil, fmt.Errorf("address group port mapping not found in backend or cache: %w", err)
}

func (c *CachedBackendClient) ListAddressGroupPortMappings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupPortMapping, error) {
	var cacheKey string
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			if ris.Identifiers[0].Namespace != "" {
				cacheKey = fmt.Sprintf("addressgroupportmappings:list:%s", ris.Identifiers[0].Namespace)
			}
		}
	}
	mappings, err := c.backend.ListAddressGroupPortMappings(ctx, scope)
	if err == nil && cacheKey != "" {
		c.cache.Set(cacheKey, mappings, cache.DefaultExpiration)
		return mappings, nil
	}
	if err != nil && cacheKey != "" {
		if cached, found := c.cache.Get(cacheKey); found {
			log.Printf("Backend unavailable, serving list from cache: %s", cacheKey)
			return cached.([]models.AddressGroupPortMapping), nil
		}
	}
	return mappings, err
}

func (c *CachedBackendClient) CreateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	err := c.backend.CreateAddressGroupPortMapping(ctx, mapping)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupPortMappingCache(mapping.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) UpdateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	err := c.backend.UpdateAddressGroupPortMapping(ctx, mapping)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupPortMappingCache(mapping.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) DeleteAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) error {
	err := c.backend.DeleteAddressGroupPortMapping(ctx, id)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupPortMappingCache(id)
	return nil
}

func (c *CachedBackendClient) invalidateAddressGroupPortMappingCache(id models.ResourceIdentifier) {
	key := fmt.Sprintf("addressgroupportmapping:%s", id.Key())
	c.cache.Delete(key)
	listKey := fmt.Sprintf("addressgroupportmappings:list:%s", id.Namespace)
	c.cache.Delete(listKey)
}

// RuleS2S
func (c *CachedBackendClient) GetRuleS2S(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	key := fmt.Sprintf("rules2s:%s", id.Key())
	rule, err := c.backend.GetRuleS2S(ctx, id)
	if err == nil {
		c.cache.Set(key, rule, cache.DefaultExpiration)
		return rule, nil
	}
	if cached, found := c.cache.Get(key); found {
		log.Printf("Backend unavailable, serving from cache: %s", key)
		return cached.(*models.RuleS2S), nil
	}
	return nil, fmt.Errorf("ruleS2S not found in backend or cache: %w", err)
}

func (c *CachedBackendClient) ListRuleS2S(ctx context.Context, scope ports.Scope) ([]models.RuleS2S, error) {
	var cacheKey string
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			if ris.Identifiers[0].Namespace != "" {
				cacheKey = fmt.Sprintf("rules2s:list:%s", ris.Identifiers[0].Namespace)
			}
		}
	}
	rules, err := c.backend.ListRuleS2S(ctx, scope)
	if err == nil && cacheKey != "" {
		c.cache.Set(cacheKey, rules, cache.DefaultExpiration)
		return rules, nil
	}
	if err != nil && cacheKey != "" {
		if cached, found := c.cache.Get(cacheKey); found {
			log.Printf("Backend unavailable, serving list from cache: %s", cacheKey)
			return cached.([]models.RuleS2S), nil
		}
	}
	return rules, err
}

func (c *CachedBackendClient) CreateRuleS2S(ctx context.Context, rule *models.RuleS2S) error {
	err := c.backend.CreateRuleS2S(ctx, rule)
	if err != nil {
		return err
	}
	c.invalidateRuleS2SCache(rule.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) UpdateRuleS2S(ctx context.Context, rule *models.RuleS2S) error {
	err := c.backend.UpdateRuleS2S(ctx, rule)
	if err != nil {
		return err
	}
	c.invalidateRuleS2SCache(rule.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) DeleteRuleS2S(ctx context.Context, id models.ResourceIdentifier) error {
	err := c.backend.DeleteRuleS2S(ctx, id)
	if err != nil {
		return err
	}
	c.invalidateRuleS2SCache(id)
	return nil
}

func (c *CachedBackendClient) invalidateRuleS2SCache(id models.ResourceIdentifier) {
	key := fmt.Sprintf("rules2s:%s", id.Key())
	c.cache.Delete(key)
	listKey := fmt.Sprintf("rules2s:list:%s", id.Namespace)
	c.cache.Delete(listKey)
}

// ServiceAlias
func (c *CachedBackendClient) GetServiceAlias(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	key := fmt.Sprintf("servicealias:%s", id.Key())
	alias, err := c.backend.GetServiceAlias(ctx, id)
	if err == nil {
		c.cache.Set(key, alias, cache.DefaultExpiration)
		return alias, nil
	}
	if cached, found := c.cache.Get(key); found {
		log.Printf("Backend unavailable, serving from cache: %s", key)
		return cached.(*models.ServiceAlias), nil
	}
	return nil, fmt.Errorf("service alias not found in backend or cache: %w", err)
}

func (c *CachedBackendClient) ListServiceAliases(ctx context.Context, scope ports.Scope) ([]models.ServiceAlias, error) {
	var cacheKey string
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			if ris.Identifiers[0].Namespace != "" {
				cacheKey = fmt.Sprintf("servicealiases:list:%s", ris.Identifiers[0].Namespace)
			}
		}
	}
	aliases, err := c.backend.ListServiceAliases(ctx, scope)
	if err == nil && cacheKey != "" {
		c.cache.Set(cacheKey, aliases, cache.DefaultExpiration)
		return aliases, nil
	}
	if err != nil && cacheKey != "" {
		if cached, found := c.cache.Get(cacheKey); found {
			log.Printf("Backend unavailable, serving list from cache: %s", cacheKey)
			return cached.([]models.ServiceAlias), nil
		}
	}
	return aliases, err
}

func (c *CachedBackendClient) CreateServiceAlias(ctx context.Context, alias *models.ServiceAlias) error {
	err := c.backend.CreateServiceAlias(ctx, alias)
	if err != nil {
		return err
	}
	c.invalidateServiceAliasCache(alias.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) UpdateServiceAlias(ctx context.Context, alias *models.ServiceAlias) error {
	err := c.backend.UpdateServiceAlias(ctx, alias)
	if err != nil {
		return err
	}
	c.invalidateServiceAliasCache(alias.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) DeleteServiceAlias(ctx context.Context, id models.ResourceIdentifier) error {
	err := c.backend.DeleteServiceAlias(ctx, id)
	if err != nil {
		return err
	}
	c.invalidateServiceAliasCache(id)
	return nil
}

func (c *CachedBackendClient) invalidateServiceAliasCache(id models.ResourceIdentifier) {
	key := fmt.Sprintf("servicealias:%s", id.Key())
	c.cache.Delete(key)
	listKey := fmt.Sprintf("servicealiases:list:%s", id.Namespace)
	c.cache.Delete(listKey)
}

// AddressGroupBindingPolicy
func (c *CachedBackendClient) GetAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	key := fmt.Sprintf("addressgroupbindingpolicy:%s", id.Key())
	policy, err := c.backend.GetAddressGroupBindingPolicy(ctx, id)
	if err == nil {
		c.cache.Set(key, policy, cache.DefaultExpiration)
		return policy, nil
	}
	if cached, found := c.cache.Get(key); found {
		log.Printf("Backend unavailable, serving from cache: %s", key)
		return cached.(*models.AddressGroupBindingPolicy), nil
	}
	return nil, fmt.Errorf("address group binding policy not found in backend or cache: %w", err)
}

func (c *CachedBackendClient) ListAddressGroupBindingPolicies(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBindingPolicy, error) {
	var cacheKey string
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			if ris.Identifiers[0].Namespace != "" {
				cacheKey = fmt.Sprintf("addressgroupbindingpolicies:list:%s", ris.Identifiers[0].Namespace)
			}
		}
	}
	policies, err := c.backend.ListAddressGroupBindingPolicies(ctx, scope)
	if err == nil && cacheKey != "" {
		c.cache.Set(cacheKey, policies, cache.DefaultExpiration)
		return policies, nil
	}
	if err != nil && cacheKey != "" {
		if cached, found := c.cache.Get(cacheKey); found {
			log.Printf("Backend unavailable, serving list from cache: %s", cacheKey)
			return cached.([]models.AddressGroupBindingPolicy), nil
		}
	}
	return policies, err
}

func (c *CachedBackendClient) CreateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	err := c.backend.CreateAddressGroupBindingPolicy(ctx, policy)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupBindingPolicyCache(policy.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) UpdateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	err := c.backend.UpdateAddressGroupBindingPolicy(ctx, policy)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupBindingPolicyCache(policy.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) DeleteAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) error {
	err := c.backend.DeleteAddressGroupBindingPolicy(ctx, id)
	if err != nil {
		return err
	}
	c.invalidateAddressGroupBindingPolicyCache(id)
	return nil
}

func (c *CachedBackendClient) invalidateAddressGroupBindingPolicyCache(id models.ResourceIdentifier) {
	key := fmt.Sprintf("addressgroupbindingpolicy:%s", id.Key())
	c.cache.Delete(key)
	listKey := fmt.Sprintf("addressgroupbindingpolicies:list:%s", id.Namespace)
	c.cache.Delete(listKey)
}

// IEAgAgRule
func (c *CachedBackendClient) GetIEAgAgRule(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	key := fmt.Sprintf("ieagagrule:%s", id.Key())
	rule, err := c.backend.GetIEAgAgRule(ctx, id)
	if err == nil {
		c.cache.Set(key, rule, cache.DefaultExpiration)
		return rule, nil
	}
	if cached, found := c.cache.Get(key); found {
		log.Printf("Backend unavailable, serving from cache: %s", key)
		return cached.(*models.IEAgAgRule), nil
	}
	return nil, fmt.Errorf("ieagagrule not found in backend or cache: %w", err)
}

func (c *CachedBackendClient) ListIEAgAgRules(ctx context.Context, scope ports.Scope) ([]models.IEAgAgRule, error) {
	var cacheKey string
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			if ris.Identifiers[0].Namespace != "" {
				cacheKey = fmt.Sprintf("ieagagrules:list:%s", ris.Identifiers[0].Namespace)
			}
		}
	}
	rules, err := c.backend.ListIEAgAgRules(ctx, scope)
	if err == nil && cacheKey != "" {
		c.cache.Set(cacheKey, rules, cache.DefaultExpiration)
		return rules, nil
	}
	if err != nil && cacheKey != "" {
		if cached, found := c.cache.Get(cacheKey); found {
			log.Printf("Backend unavailable, serving list from cache: %s", cacheKey)
			return cached.([]models.IEAgAgRule), nil
		}
	}
	return rules, err
}

func (c *CachedBackendClient) CreateIEAgAgRule(ctx context.Context, rule *models.IEAgAgRule) error {
	err := c.backend.CreateIEAgAgRule(ctx, rule)
	if err != nil {
		return err
	}
	c.invalidateIEAgAgRuleCache(rule.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) UpdateIEAgAgRule(ctx context.Context, rule *models.IEAgAgRule) error {
	err := c.backend.UpdateIEAgAgRule(ctx, rule)
	if err != nil {
		return err
	}
	c.invalidateIEAgAgRuleCache(rule.ResourceIdentifier)
	return nil
}

func (c *CachedBackendClient) DeleteIEAgAgRule(ctx context.Context, id models.ResourceIdentifier) error {
	err := c.backend.DeleteIEAgAgRule(ctx, id)
	if err != nil {
		return err
	}
	c.invalidateIEAgAgRuleCache(id)
	return nil
}

func (c *CachedBackendClient) invalidateIEAgAgRuleCache(id models.ResourceIdentifier) {
	key := fmt.Sprintf("ieagagrule:%s", id.Key())
	c.cache.Delete(key)
	listKey := fmt.Sprintf("ieagagrules:list:%s", id.Namespace)
	c.cache.Delete(listKey)
}

// Sync, HealthCheck, Close, GetDependencyValidator, GetSyncStatus
func (c *CachedBackendClient) Sync(ctx context.Context, syncOp models.SyncOp, resources interface{}) error {
	return c.backend.Sync(ctx, syncOp, resources)
}

func (c *CachedBackendClient) GetDependencyValidator() *validation.DependencyValidator {
	return c.backend.GetDependencyValidator()
}

func (c *CachedBackendClient) GetReader(ctx context.Context) (ports.Reader, error) {
	return c.backend.GetReader(ctx)
}

func (c *CachedBackendClient) HealthCheck(ctx context.Context) error {
	return c.backend.HealthCheck(ctx)
}

func (c *CachedBackendClient) Close() error {
	return c.backend.Close()
}

func (c *CachedBackendClient) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return c.backend.GetSyncStatus(ctx)
}
