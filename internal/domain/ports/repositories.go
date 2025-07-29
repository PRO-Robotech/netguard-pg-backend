package ports

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/patterns"
)

type (
	// Scope defines the scope of operations
	Scope interface {
		IsEmpty() bool
		String() string
	}

	// Option defines options for operations
	Option interface{}

	// ReaderNoClose defines read operations without close
	ReaderNoClose interface {
		// List methods with scope
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
		GetSyncStatus(ctx context.Context) (*models.SyncStatus, error)

		// Get methods with ResourceIdentifier
		GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error)
		GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error)
		GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error)
		GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error)
		GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error)
		GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error)
		GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error)
		GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error)
		GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error)
		GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error)
	}

	// Reader defines read operations
	Reader interface {
		ReaderNoClose
		Close() error
	}

	// Writer defines write operations
	Writer interface {
		// Sync methods with scope
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

		// Delete methods with ResourceIdentifier
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

		Commit() error
		Abort()
	}

	// Registry defines the registry interface
	Registry interface {
		Subject() patterns.Subject
		Writer(ctx context.Context) (Writer, error)
		Reader(ctx context.Context) (Reader, error)
		// ReaderFromWriter returns a reader that can see changes made in the current transaction
		ReaderFromWriter(ctx context.Context, writer Writer) (Reader, error)
		Close() error
	}
)
