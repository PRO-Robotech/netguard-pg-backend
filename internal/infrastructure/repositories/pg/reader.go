package pg

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/readers"
)

// reader implements the PostgreSQL reader
// This is now a lightweight wrapper that delegates to modular readers
type reader struct {
	registry      *Registry
	pool          *pgxpool.Pool
	tx            pgx.Tx // Optional transaction for consistency with writer
	ctx           context.Context
	modularReader *readers.Reader // Delegate to modular reader
}

// Close closes the reader
func (r *reader) Close() error {
	return nil // Connection returned to pool automatically
}

// query executes a query using either transaction or pool connection
func (r *reader) query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	if r.tx != nil {
		return r.tx.Query(ctx, query, args...)
	}
	return r.pool.Query(ctx, query, args...)
}

// queryRow executes a single-row query
func (r *reader) queryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	if r.tx != nil {
		return r.tx.QueryRow(ctx, query, args...)
	}
	return r.pool.QueryRow(ctx, query, args...)
}

// Implemented resource methods - delegated to modular readers

// Service methods - delegated to readers/service.go
func (r *reader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	return r.modularReader.ListServices(ctx, consume, scope)
}

func (r *reader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return r.modularReader.GetServiceByID(ctx, id)
}

// AddressGroup methods - delegated to readers/address_group.go
func (r *reader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	return r.modularReader.ListAddressGroups(ctx, consume, scope)
}

func (r *reader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return r.modularReader.GetAddressGroupByID(ctx, id)
}

// AddressGroupBinding methods - delegated to readers/address_group_binding.go
func (r *reader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return r.modularReader.ListAddressGroupBindings(ctx, consume, scope)
}

func (r *reader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return r.modularReader.GetAddressGroupBindingByID(ctx, id)
}

// AddressGroupPortMapping methods - delegated to readers/address_group_port_mapping.go
func (r *reader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return r.modularReader.ListAddressGroupPortMappings(ctx, consume, scope)
}

func (r *reader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return r.modularReader.GetAddressGroupPortMappingByID(ctx, id)
}

// SyncStatus methods - delegated to readers/sync_status.go
func (r *reader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return r.modularReader.GetSyncStatus(ctx)
}

// Placeholder methods for not-yet-implemented resources

// RuleS2S methods - delegated to readers/rule_s2s.go
func (r *reader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return r.modularReader.ListRuleS2S(ctx, consume, scope)
}

func (r *reader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return r.modularReader.GetRuleS2SByID(ctx, id)
}

// ServiceAlias methods - delegated to readers/service_alias.go
func (r *reader) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return r.modularReader.ListServiceAliases(ctx, consume, scope)
}

func (r *reader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return r.modularReader.GetServiceAliasByID(ctx, id)
}

// AddressGroupBindingPolicy methods - delegated to readers/address_group_binding_policy.go
func (r *reader) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return r.modularReader.ListAddressGroupBindingPolicies(ctx, consume, scope)
}

func (r *reader) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return r.modularReader.GetAddressGroupBindingPolicyByID(ctx, id)
}

// IEAgAgRule methods - delegated to readers/ieagag_rule.go
func (r *reader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	return r.modularReader.ListIEAgAgRules(ctx, consume, scope)
}

func (r *reader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	return r.modularReader.GetIEAgAgRuleByID(ctx, id)
}

// Network methods - delegated to readers/network.go
func (r *reader) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	return r.modularReader.ListNetworks(ctx, consume, scope)
}

func (r *reader) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	return r.modularReader.GetNetworkByID(ctx, id)
}

// NetworkBinding methods - delegated to readers/network_binding.go
func (r *reader) ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope ports.Scope) error {
	return r.modularReader.ListNetworkBindings(ctx, consume, scope)
}

func (r *reader) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	return r.modularReader.GetNetworkBindingByID(ctx, id)
}
