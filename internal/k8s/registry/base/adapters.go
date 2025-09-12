package base

import (
	"context"

	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/client"
)

// ServiceOperations implements BackendOperations for Service using NetguardService
type ServiceOperations struct {
	netguardService *services.NetguardFacade
}

// NewServiceOperations creates a new ServiceOperations
func NewServiceOperations(netguardService *services.NetguardFacade) *ServiceOperations {
	return &ServiceOperations{
		netguardService: netguardService,
	}
}

func (s *ServiceOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return s.netguardService.GetServiceByID(ctx, id)
}

func (s *ServiceOperations) List(ctx context.Context, scope ports.Scope) ([]models.Service, error) {
	return s.netguardService.GetServices(ctx, scope)
}

func (s *ServiceOperations) Create(ctx context.Context, obj *models.Service) error {
	return s.netguardService.CreateService(ctx, *obj)
}

func (s *ServiceOperations) Update(ctx context.Context, obj *models.Service) error {
	return s.netguardService.UpdateService(ctx, *obj)
}

func (s *ServiceOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return s.netguardService.DeleteServicesByIDs(ctx, []models.ResourceIdentifier{id})
}

// NewServicePtrOpsWithNetguardService creates a new PtrBackendOperations for Service using NetguardService
func NewServicePtrOpsWithNetguardService(netguardService *services.NetguardFacade) BackendOperations[*models.Service] {
	return NewPtrBackendOperations[models.Service](NewServiceOperations(netguardService))
}

// ORIGINAL CLIENT-BASED OPERATIONS - DEPRECATED
// These are kept for reference but should be replaced with NetguardService operations

// ServiceBackendOperations implements BackendOperations for Service
type ServiceBackendOperations struct {
	client client.BackendClient
}

// NewServiceBackendOperations creates a new ServiceBackendOperations
func NewServiceBackendOperations(client client.BackendClient) *ServiceBackendOperations {
	return &ServiceBackendOperations{client: client}
}

func (s *ServiceBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return s.client.GetService(ctx, id)
}

func (s *ServiceBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.Service, error) {
	return s.client.ListServices(ctx, scope)
}

func (s *ServiceBackendOperations) Create(ctx context.Context, obj *models.Service) error {
	return s.client.CreateService(ctx, obj)
}

func (s *ServiceBackendOperations) Update(ctx context.Context, obj *models.Service) error {
	return s.client.UpdateService(ctx, obj)
}

func (s *ServiceBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return s.client.DeleteService(ctx, id)
}

// NewServicePtrOpsOld creates a new PtrBackendOperations for Service (old client-based)
func NewServicePtrOpsOld(client client.BackendClient) BackendOperations[*models.Service] {
	return NewPtrBackendOperations[models.Service](NewServiceBackendOperations(client))
}

// AddressGroupBackendOperations implements BackendOperations for AddressGroup resources
type AddressGroupBackendOperations struct {
	client client.BackendClient
}

func NewAddressGroupBackendOperations(client client.BackendClient) BackendOperations[models.AddressGroup] {
	return &AddressGroupBackendOperations{client: client}
}

func (a *AddressGroupBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return a.client.GetAddressGroup(ctx, id)
}

func (a *AddressGroupBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error) {
	return a.client.ListAddressGroups(ctx, scope)
}

func (a *AddressGroupBackendOperations) Create(ctx context.Context, obj *models.AddressGroup) error {
	return a.client.CreateAddressGroup(ctx, obj)
}

func (a *AddressGroupBackendOperations) Update(ctx context.Context, obj *models.AddressGroup) error {
	return a.client.UpdateAddressGroup(ctx, obj)
}

func (a *AddressGroupBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return a.client.DeleteAddressGroup(ctx, id)
}

// AddressGroupBindingBackendOperations implements BackendOperations for AddressGroupBinding resources
type AddressGroupBindingBackendOperations struct {
	client client.BackendClient
}

func NewAddressGroupBindingBackendOperations(client client.BackendClient) BackendOperations[models.AddressGroupBinding] {
	return &AddressGroupBindingBackendOperations{client: client}
}

func (a *AddressGroupBindingBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return a.client.GetAddressGroupBinding(ctx, id)
}

func (a *AddressGroupBindingBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBinding, error) {
	return a.client.ListAddressGroupBindings(ctx, scope)
}

func (a *AddressGroupBindingBackendOperations) Create(ctx context.Context, obj *models.AddressGroupBinding) error {
	return a.client.CreateAddressGroupBinding(ctx, obj)
}

func (a *AddressGroupBindingBackendOperations) Update(ctx context.Context, obj *models.AddressGroupBinding) error {
	return a.client.UpdateAddressGroupBinding(ctx, obj)
}

func (a *AddressGroupBindingBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return a.client.DeleteAddressGroupBinding(ctx, id)
}

// AddressGroupBindingPolicyBackendOperations implements BackendOperations for AddressGroupBindingPolicy resources
type AddressGroupBindingPolicyBackendOperations struct {
	client client.BackendClient
}

func NewAddressGroupBindingPolicyBackendOperations(client client.BackendClient) BackendOperations[models.AddressGroupBindingPolicy] {
	return &AddressGroupBindingPolicyBackendOperations{client: client}
}

func (a *AddressGroupBindingPolicyBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return a.client.GetAddressGroupBindingPolicy(ctx, id)
}

func (a *AddressGroupBindingPolicyBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBindingPolicy, error) {
	return a.client.ListAddressGroupBindingPolicies(ctx, scope)
}

func (a *AddressGroupBindingPolicyBackendOperations) Create(ctx context.Context, obj *models.AddressGroupBindingPolicy) error {
	return a.client.CreateAddressGroupBindingPolicy(ctx, obj)
}

func (a *AddressGroupBindingPolicyBackendOperations) Update(ctx context.Context, obj *models.AddressGroupBindingPolicy) error {
	return a.client.UpdateAddressGroupBindingPolicy(ctx, obj)
}

func (a *AddressGroupBindingPolicyBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return a.client.DeleteAddressGroupBindingPolicy(ctx, id)
}

// AddressGroupPortMappingBackendOperations implements BackendOperations for AddressGroupPortMapping resources
type AddressGroupPortMappingBackendOperations struct {
	client client.BackendClient
}

func NewAddressGroupPortMappingBackendOperations(client client.BackendClient) BackendOperations[models.AddressGroupPortMapping] {
	return &AddressGroupPortMappingBackendOperations{client: client}
}

func (a *AddressGroupPortMappingBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return a.client.GetAddressGroupPortMapping(ctx, id)
}

func (a *AddressGroupPortMappingBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.AddressGroupPortMapping, error) {
	return a.client.ListAddressGroupPortMappings(ctx, scope)
}

func (a *AddressGroupPortMappingBackendOperations) Create(ctx context.Context, obj *models.AddressGroupPortMapping) error {
	return a.client.CreateAddressGroupPortMapping(ctx, obj)
}

func (a *AddressGroupPortMappingBackendOperations) Update(ctx context.Context, obj *models.AddressGroupPortMapping) error {
	return a.client.UpdateAddressGroupPortMapping(ctx, obj)
}

func (a *AddressGroupPortMappingBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return a.client.DeleteAddressGroupPortMapping(ctx, id)
}

// RuleS2SBackendOperations implements BackendOperations for RuleS2S resources
type RuleS2SBackendOperations struct {
	client client.BackendClient
}

func NewRuleS2SBackendOperations(client client.BackendClient) BackendOperations[models.RuleS2S] {
	return &RuleS2SBackendOperations{client: client}
}

func (r *RuleS2SBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return r.client.GetRuleS2S(ctx, id)
}

func (r *RuleS2SBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.RuleS2S, error) {
	return r.client.ListRuleS2S(ctx, scope)
}

func (r *RuleS2SBackendOperations) Create(ctx context.Context, obj *models.RuleS2S) error {
	return r.client.CreateRuleS2S(ctx, obj)
}

func (r *RuleS2SBackendOperations) Update(ctx context.Context, obj *models.RuleS2S) error {
	return r.client.UpdateRuleS2S(ctx, obj)
}

func (r *RuleS2SBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return r.client.DeleteRuleS2S(ctx, id)
}

// ServiceAliasBackendOperations implements BackendOperations for ServiceAlias resources
type ServiceAliasBackendOperations struct {
	client client.BackendClient
}

func NewServiceAliasBackendOperations(client client.BackendClient) BackendOperations[models.ServiceAlias] {
	return &ServiceAliasBackendOperations{client: client}
}

func (s *ServiceAliasBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return s.client.GetServiceAlias(ctx, id)
}

func (s *ServiceAliasBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.ServiceAlias, error) {
	return s.client.ListServiceAliases(ctx, scope)
}

func (s *ServiceAliasBackendOperations) Create(ctx context.Context, obj *models.ServiceAlias) error {
	return s.client.CreateServiceAlias(ctx, obj)
}

func (s *ServiceAliasBackendOperations) Update(ctx context.Context, obj *models.ServiceAlias) error {
	return s.client.UpdateServiceAlias(ctx, obj)
}

func (s *ServiceAliasBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return s.client.DeleteServiceAlias(ctx, id)
}

// IEAgAgRuleBackendOperations implements BackendOperations for IEAgAgRule resources
type IEAgAgRuleBackendOperations struct {
	client client.BackendClient
}

func NewIEAgAgRuleBackendOperations(client client.BackendClient) BackendOperations[models.IEAgAgRule] {
	return &IEAgAgRuleBackendOperations{client: client}
}

func (i *IEAgAgRuleBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	return i.client.GetIEAgAgRule(ctx, id)
}

func (i *IEAgAgRuleBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.IEAgAgRule, error) {
	return i.client.ListIEAgAgRules(ctx, scope)
}

func (i *IEAgAgRuleBackendOperations) Create(ctx context.Context, obj *models.IEAgAgRule) error {
	return i.client.CreateIEAgAgRule(ctx, obj)
}

func (i *IEAgAgRuleBackendOperations) Update(ctx context.Context, obj *models.IEAgAgRule) error {
	return i.client.UpdateIEAgAgRule(ctx, obj)
}

func (i *IEAgAgRuleBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return i.client.DeleteIEAgAgRule(ctx, id)
}

// NetworkOperations implements BackendOperations for Network using NetguardService
type NetworkOperations struct {
	netguardService *services.NetguardFacade
}

// NewNetworkOperations creates a new NetworkOperations
func NewNetworkOperations(netguardService *services.NetguardFacade) *NetworkOperations {
	return &NetworkOperations{
		netguardService: netguardService,
	}
}

func (n *NetworkOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	return n.netguardService.GetNetworkByID(ctx, id)
}

func (n *NetworkOperations) List(ctx context.Context, scope ports.Scope) ([]models.Network, error) {
	return n.netguardService.GetNetworks(ctx, scope)
}

func (n *NetworkOperations) Create(ctx context.Context, obj *models.Network) error {
	return n.netguardService.CreateNetwork(ctx, *obj)
}

func (n *NetworkOperations) Update(ctx context.Context, obj *models.Network) error {
	return n.netguardService.UpdateNetwork(ctx, *obj)
}

func (n *NetworkOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return n.netguardService.DeleteNetwork(ctx, id)
}

// NewNetworkPtrOpsWithNetguardService creates a new PtrBackendOperations for Network using NetguardService
func NewNetworkPtrOpsWithNetguardService(netguardService *services.NetguardFacade) BackendOperations[*models.Network] {
	return NewPtrBackendOperations[models.Network](NewNetworkOperations(netguardService))
}

// NetworkBindingOperations implements BackendOperations for NetworkBinding using NetguardService
type NetworkBindingOperations struct {
	netguardService *services.NetguardFacade
}

// NewNetworkBindingOperations creates a new NetworkBindingOperations
func NewNetworkBindingOperations(netguardService *services.NetguardFacade) *NetworkBindingOperations {
	return &NetworkBindingOperations{
		netguardService: netguardService,
	}
}

func (nb *NetworkBindingOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	return nb.netguardService.GetNetworkBindingByID(ctx, id)
}

func (nb *NetworkBindingOperations) List(ctx context.Context, scope ports.Scope) ([]models.NetworkBinding, error) {
	return nb.netguardService.GetNetworkBindings(ctx, scope)
}

func (nb *NetworkBindingOperations) Create(ctx context.Context, obj *models.NetworkBinding) error {
	return nb.netguardService.CreateNetworkBinding(ctx, *obj)
}

func (nb *NetworkBindingOperations) Update(ctx context.Context, obj *models.NetworkBinding) error {
	return nb.netguardService.UpdateNetworkBinding(ctx, *obj)
}

func (nb *NetworkBindingOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return nb.netguardService.DeleteNetworkBinding(ctx, id)
}

// NewNetworkBindingPtrOpsWithNetguardService creates a new PtrBackendOperations for NetworkBinding using NetguardService
func NewNetworkBindingPtrOpsWithNetguardService(netguardService *services.NetguardFacade) BackendOperations[*models.NetworkBinding] {
	return NewPtrBackendOperations[models.NetworkBinding](NewNetworkBindingOperations(netguardService))
}

// NetworkBackendOperations implements BackendOperations for Network (DEPRECATED - use NetworkOperations)
type NetworkBackendOperations struct {
	client client.BackendClient
}

// NewNetworkBackendOperations creates a new NetworkBackendOperations
func NewNetworkBackendOperations(client client.BackendClient) *NetworkBackendOperations {
	return &NetworkBackendOperations{client: client}
}

func (n *NetworkBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	return n.client.GetNetwork(ctx, id)
}

func (n *NetworkBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.Network, error) {
	return n.client.ListNetworks(ctx, scope)
}

func (n *NetworkBackendOperations) Create(ctx context.Context, obj *models.Network) error {
	return n.client.CreateNetwork(ctx, obj)
}

func (n *NetworkBackendOperations) Update(ctx context.Context, obj *models.Network) error {
	return n.client.UpdateNetwork(ctx, obj)
}

func (n *NetworkBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return n.client.DeleteNetwork(ctx, id)
}

// NewNetworkPtrOpsOld creates a new PtrBackendOperations for Network (old client-based)
func NewNetworkPtrOpsOld(client client.BackendClient) BackendOperations[*models.Network] {
	return NewPtrBackendOperations[models.Network](NewNetworkBackendOperations(client))
}

// NetworkBindingBackendOperations implements BackendOperations for NetworkBinding (DEPRECATED - use NetworkBindingOperations)
type NetworkBindingBackendOperations struct {
	client client.BackendClient
}

// NewNetworkBindingBackendOperations creates a new NetworkBindingBackendOperations
func NewNetworkBindingBackendOperations(client client.BackendClient) *NetworkBindingBackendOperations {
	return &NetworkBindingBackendOperations{client: client}
}

func (nb *NetworkBindingBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	return nb.client.GetNetworkBinding(ctx, id)
}

func (nb *NetworkBindingBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.NetworkBinding, error) {
	return nb.client.ListNetworkBindings(ctx, scope)
}

func (nb *NetworkBindingBackendOperations) Create(ctx context.Context, obj *models.NetworkBinding) error {
	return nb.client.CreateNetworkBinding(ctx, obj)
}

func (nb *NetworkBindingBackendOperations) Update(ctx context.Context, obj *models.NetworkBinding) error {
	return nb.client.UpdateNetworkBinding(ctx, obj)
}

func (nb *NetworkBindingBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return nb.client.DeleteNetworkBinding(ctx, id)
}

// NewNetworkBindingPtrOpsOld creates a new PtrBackendOperations for NetworkBinding (old client-based)
func NewNetworkBindingPtrOpsOld(client client.BackendClient) BackendOperations[*models.NetworkBinding] {
	return NewPtrBackendOperations[models.NetworkBinding](NewNetworkBindingBackendOperations(client))
}

// HostBackendOperations implements BackendOperations for Host resources
type HostBackendOperations struct {
	client client.BackendClient
}

// NewHostBackendOperations creates a new HostBackendOperations
func NewHostBackendOperations(client client.BackendClient) *HostBackendOperations {
	return &HostBackendOperations{client: client}
}

func (h *HostBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error) {
	return h.client.GetHost(ctx, id)
}

func (h *HostBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.Host, error) {
	return h.client.ListHosts(ctx, scope)
}

func (h *HostBackendOperations) Create(ctx context.Context, obj *models.Host) error {
	return h.client.CreateHost(ctx, obj)
}

func (h *HostBackendOperations) Update(ctx context.Context, obj *models.Host) error {
	return h.client.UpdateHost(ctx, obj)
}

func (h *HostBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return h.client.DeleteHost(ctx, id)
}

// HostBindingBackendOperations implements BackendOperations for HostBinding resources
type HostBindingBackendOperations struct {
	client client.BackendClient
}

// NewHostBindingBackendOperations creates a new HostBindingBackendOperations
func NewHostBindingBackendOperations(client client.BackendClient) *HostBindingBackendOperations {
	return &HostBindingBackendOperations{client: client}
}

func (hb *HostBindingBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error) {
	return hb.client.GetHostBinding(ctx, id)
}

func (hb *HostBindingBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.HostBinding, error) {
	return hb.client.ListHostBindings(ctx, scope)
}

func (hb *HostBindingBackendOperations) Create(ctx context.Context, obj *models.HostBinding) error {
	return hb.client.CreateHostBinding(ctx, obj)
}

func (hb *HostBindingBackendOperations) Update(ctx context.Context, obj *models.HostBinding) error {
	return hb.client.UpdateHostBinding(ctx, obj)
}

func (hb *HostBindingBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return hb.client.DeleteHostBinding(ctx, id)
}
