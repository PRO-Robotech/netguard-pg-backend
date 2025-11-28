package ports

import (
	"context"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/patterns"
)

type (
	Scope interface {
		IsEmpty() bool
		String() string
	}
	Option                 interface{}
	ConditionOnlyOperation struct{}
	ReaderNoClose          interface {
		ListServices(ctx context.Context, consume func(models.Service) error, scope Scope) error
		ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope Scope) error
		ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope Scope) error
		ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope Scope) error
		ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope Scope) error
		ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope Scope) error
		ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope Scope) error
		ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope Scope) error
		ListNetworks(ctx context.Context, consume func(models.Network) error, scope Scope) error
		ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope Scope) error
		ListHosts(ctx context.Context, consume func(models.Host) error, scope Scope) error
		ListHostBindings(ctx context.Context, consume func(models.HostBinding) error, scope Scope) error
		ListSvcSvcRules(ctx context.Context, consume func(models.SvcSvcRule) error, scope Scope) error
		ListSvcFqdnRules(ctx context.Context, consume func(models.SvcFqdnRule) error, scope Scope) error
		GetSyncStatus(ctx context.Context) (*models.SyncStatus, error)
		GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error)
		GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error)
		GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error)
		GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error)
		GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error)
		GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error)
		GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error)
		GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error)
		GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error)
		GetNetworkByCIDR(ctx context.Context, cidr string) (*models.Network, error)
		GetNetworksOverlappingCIDR(ctx context.Context, cidr string) ([]*models.Network, error)
		GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error)
		GetHostByID(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error)
		GetHostBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error)
		GetSvcSvcRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error)
		GetSvcFqdnRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcFqdnRule, error)
	}
	Reader interface {
		ReaderNoClose
		Close() error
	}
	Writer interface {
		SyncServices(ctx context.Context, services []models.Service, scope Scope, opts ...Option) error
		SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope Scope, opts ...Option) error
		SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope Scope, opts ...Option) error
		SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope Scope, opts ...Option) error
		SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope Scope, opts ...Option) error
		SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope Scope, opts ...Option) error
		SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope Scope, opts ...Option) error
		SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope Scope, opts ...Option) error
		SyncNetworks(ctx context.Context, networks []models.Network, scope Scope, opts ...Option) error
		SyncNetworkBindings(ctx context.Context, bindings []models.NetworkBinding, scope Scope, opts ...Option) error
		SyncHosts(ctx context.Context, hosts []models.Host, scope Scope, opts ...Option) error
		SyncHostBindings(ctx context.Context, bindings []models.HostBinding, scope Scope, opts ...Option) error
		SyncSvcSvcRules(ctx context.Context, rules []models.SvcSvcRule, scope Scope, opts ...Option) error
		SyncSvcFqdnRules(ctx context.Context, rules []models.SvcFqdnRule, scope Scope, opts ...Option) error
		DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteNetworksByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteNetworkBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteHostsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteHostBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteSvcSvcRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		DeleteSvcFqdnRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...Option) error
		MarkForDeletionWithStatus(namespace, name, kind string) error
		Commit() error
		Abort()
	}
	Registry interface {
		Subject() patterns.Subject
		Writer(ctx context.Context) (Writer, error)
		Reader(ctx context.Context) (Reader, error)
		ReaderFromWriter(ctx context.Context, writer Writer) (Reader, error)
		ReaderWithReadCommitted(ctx context.Context) (Reader, error)
		ExecuteDeleteWithRetry(ctx context.Context, fn func(Writer) error) error
		Close() error
	}
)
