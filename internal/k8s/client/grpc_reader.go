package client

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// GRPCReader реализует ports.Reader интерфейс используя gRPC клиент
type GRPCReader struct {
	grpcClient *GRPCBackendClient
}

func NewGRPCReader(grpcClient *GRPCBackendClient) *GRPCReader {
	return &GRPCReader{
		grpcClient: grpcClient,
	}
}

func (r *GRPCReader) Close() error {
	// gRPC Reader не нуждается в закрытии - соединение управляется grpcClient
	return nil
}

// ListServices реализует ports.Reader интерфейс
func (r *GRPCReader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	services, err := r.grpcClient.ListServices(ctx, scope)
	if err != nil {
		return err
	}

	for _, service := range services {
		if err := consume(service); err != nil {
			return err
		}
	}

	return nil
}

// ListAddressGroups реализует ports.Reader интерфейс
func (r *GRPCReader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	groups, err := r.grpcClient.ListAddressGroups(ctx, scope)
	if err != nil {
		return err
	}

	for _, group := range groups {
		if err := consume(group); err != nil {
			return err
		}
	}

	return nil
}

// ListAddressGroupBindings реализует ports.Reader интерфейс
func (r *GRPCReader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	bindings, err := r.grpcClient.ListAddressGroupBindings(ctx, scope)
	if err != nil {
		return err
	}

	for _, binding := range bindings {
		if err := consume(binding); err != nil {
			return err
		}
	}

	return nil
}

// ListAddressGroupPortMappings реализует ports.Reader интерфейс
func (r *GRPCReader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	mappings, err := r.grpcClient.ListAddressGroupPortMappings(ctx, scope)
	if err != nil {
		return err
	}

	for _, mapping := range mappings {
		if err := consume(mapping); err != nil {
			return err
		}
	}

	return nil
}

// ListRuleS2S реализует ports.Reader интерфейс
func (r *GRPCReader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	rules, err := r.grpcClient.ListRuleS2S(ctx, scope)
	if err != nil {
		return err
	}

	for _, rule := range rules {
		if err := consume(rule); err != nil {
			return err
		}
	}

	return nil
}

// ListServiceAliases реализует ports.Reader интерфейс
func (r *GRPCReader) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	aliases, err := r.grpcClient.ListServiceAliases(ctx, scope)
	if err != nil {
		return err
	}

	for _, alias := range aliases {
		if err := consume(alias); err != nil {
			return err
		}
	}

	return nil
}

// ListAddressGroupBindingPolicies реализует ports.Reader интерфейс
func (r *GRPCReader) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	policies, err := r.grpcClient.ListAddressGroupBindingPolicies(ctx, scope)
	if err != nil {
		return err
	}

	for _, policy := range policies {
		if err := consume(policy); err != nil {
			return err
		}
	}

	return nil
}

// ListIEAgAgRules реализует ports.Reader интерфейс
func (r *GRPCReader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	rules, err := r.grpcClient.ListIEAgAgRules(ctx, scope)
	if err != nil {
		return err
	}

	for _, rule := range rules {
		if err := consume(rule); err != nil {
			return err
		}
	}

	return nil
}

// GetSyncStatus реализует ports.Reader интерфейс
func (r *GRPCReader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return r.grpcClient.GetSyncStatus(ctx)
}

// GetServiceByID реализует ports.Reader интерфейс
func (r *GRPCReader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return r.grpcClient.GetService(ctx, id)
}

// GetAddressGroupByID реализует ports.Reader интерфейс
func (r *GRPCReader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return r.grpcClient.GetAddressGroup(ctx, id)
}

// GetAddressGroupBindingByID реализует ports.Reader интерфейс
func (r *GRPCReader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return r.grpcClient.GetAddressGroupBinding(ctx, id)
}

// GetAddressGroupPortMappingByID реализует ports.Reader интерфейс
func (r *GRPCReader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return r.grpcClient.GetAddressGroupPortMapping(ctx, id)
}

// GetRuleS2SByID реализует ports.Reader интерфейс
func (r *GRPCReader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return r.grpcClient.GetRuleS2S(ctx, id)
}

// GetServiceAliasByID реализует ports.Reader интерфейс
func (r *GRPCReader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return r.grpcClient.GetServiceAlias(ctx, id)
}

// GetAddressGroupBindingPolicyByID реализует ports.Reader интерфейс
func (r *GRPCReader) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return r.grpcClient.GetAddressGroupBindingPolicy(ctx, id)
}

// GetIEAgAgRuleByID реализует ports.Reader интерфейс
func (r *GRPCReader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	return r.grpcClient.GetIEAgAgRule(ctx, id)
}

// GetNetworkByID реализует ports.Reader интерфейс
func (r *GRPCReader) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	return r.grpcClient.GetNetwork(ctx, id)
}

// GetNetworkBindingByID реализует ports.Reader интерфейс
func (r *GRPCReader) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	return r.grpcClient.GetNetworkBinding(ctx, id)
}

// ListNetworks реализует ports.Reader интерфейс
func (r *GRPCReader) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	networks, err := r.grpcClient.ListNetworks(ctx, scope)
	if err != nil {
		return err
	}

	for _, network := range networks {
		if err := consume(network); err != nil {
			return err
		}
	}

	return nil
}

// ListNetworkBindings реализует ports.Reader интерфейс
func (r *GRPCReader) ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope ports.Scope) error {
	bindings, err := r.grpcClient.ListNetworkBindings(ctx, scope)
	if err != nil {
		return err
	}

	for _, binding := range bindings {
		if err := consume(binding); err != nil {
			return err
		}
	}

	return nil
}
