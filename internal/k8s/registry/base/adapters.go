package base

import (
	"context"
	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/client"
)

type ServiceOperations struct {
	netguardService *services.NetguardFacade
}

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
func (s *ServiceOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return s.netguardService.MarkForDeletion(ctx, namespace, name, kind)
}
func NewServicePtrOpsWithNetguardService(netguardService *services.NetguardFacade) BackendOperations[*models.Service] {
	return NewPtrBackendOperations[models.Service](NewServiceOperations(netguardService))
}

type ServiceBackendOperations struct {
	client client.BackendClient
}

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
func NewServicePtrOpsOld(client client.BackendClient) BackendOperations[*models.Service] {
	return NewPtrBackendOperations[models.Service](NewServiceBackendOperations(client))
}

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

type NetworkOperations struct {
	netguardService *services.NetguardFacade
}

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
func (n *NetworkOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return n.netguardService.MarkForDeletion(ctx, namespace, name, kind)
}
func NewNetworkPtrOpsWithNetguardService(netguardService *services.NetguardFacade) BackendOperations[*models.Network] {
	return NewPtrBackendOperations[models.Network](NewNetworkOperations(netguardService))
}

type NetworkBindingOperations struct {
	netguardService *services.NetguardFacade
}

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
func (nb *NetworkBindingOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return nb.netguardService.MarkForDeletion(ctx, namespace, name, kind)
}
func NewNetworkBindingPtrOpsWithNetguardService(netguardService *services.NetguardFacade) BackendOperations[*models.NetworkBinding] {
	return NewPtrBackendOperations[models.NetworkBinding](NewNetworkBindingOperations(netguardService))
}

type NetworkBackendOperations struct {
	client client.BackendClient
}

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
func NewNetworkPtrOpsOld(client client.BackendClient) BackendOperations[*models.Network] {
	return NewPtrBackendOperations[models.Network](NewNetworkBackendOperations(client))
}

type NetworkBindingBackendOperations struct {
	client client.BackendClient
}

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
func NewNetworkBindingPtrOpsOld(client client.BackendClient) BackendOperations[*models.NetworkBinding] {
	return NewPtrBackendOperations[models.NetworkBinding](NewNetworkBindingBackendOperations(client))
}

type HostBackendOperations struct {
	client client.BackendClient
}

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

type HostBindingBackendOperations struct {
	client client.BackendClient
}

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

type SvcSvcRuleBackendOperations struct {
	client client.BackendClient
}

func NewSvcSvcRuleBackendOperations(client client.BackendClient) BackendOperations[models.SvcSvcRule] {
	return &SvcSvcRuleBackendOperations{client: client}
}
func (s *SvcSvcRuleBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error) {
	return s.client.GetSvcSvcRule(ctx, id)
}
func (s *SvcSvcRuleBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.SvcSvcRule, error) {
	return s.client.ListSvcSvcRules(ctx, scope)
}
func (s *SvcSvcRuleBackendOperations) Create(ctx context.Context, obj *models.SvcSvcRule) error {
	return s.client.CreateSvcSvcRule(ctx, obj)
}
func (s *SvcSvcRuleBackendOperations) Update(ctx context.Context, obj *models.SvcSvcRule) error {
	return s.client.UpdateSvcSvcRule(ctx, obj)
}
func (s *SvcSvcRuleBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return s.client.DeleteSvcSvcRule(ctx, id)
}

type SvcFqdnRuleBackendOperations struct {
	client client.BackendClient
}

func NewSvcFqdnRuleBackendOperations(client client.BackendClient) BackendOperations[models.SvcFqdnRule] {
	return &SvcFqdnRuleBackendOperations{client: client}
}

func (s *SvcFqdnRuleBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.SvcFqdnRule, error) {
	return s.client.GetSvcFqdnRule(ctx, id)
}

func (s *SvcFqdnRuleBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.SvcFqdnRule, error) {
	return s.client.ListSvcFqdnRules(ctx, scope)
}

func (s *SvcFqdnRuleBackendOperations) Create(ctx context.Context, obj *models.SvcFqdnRule) error {
	return s.client.CreateSvcFqdnRule(ctx, obj)
}

func (s *SvcFqdnRuleBackendOperations) Update(ctx context.Context, obj *models.SvcFqdnRule) error {
	return s.client.UpdateSvcFqdnRule(ctx, obj)
}

func (s *SvcFqdnRuleBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return s.client.DeleteSvcFqdnRule(ctx, id)
}
func (s *SvcFqdnRuleBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return s.client.MarkForDeletion(ctx, namespace, name, kind)
}
func (s *SvcSvcRuleBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return s.client.MarkForDeletion(ctx, namespace, name, kind)
}
func (s *ServiceBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return s.client.MarkForDeletion(ctx, namespace, name, kind)
}
func (a *AddressGroupBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return a.client.MarkForDeletion(ctx, namespace, name, kind)
}
func (a *AddressGroupBindingBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return a.client.MarkForDeletion(ctx, namespace, name, kind)
}
func (a *AddressGroupBindingPolicyBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return a.client.MarkForDeletion(ctx, namespace, name, kind)
}
func (a *AddressGroupPortMappingBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return a.client.MarkForDeletion(ctx, namespace, name, kind)
}
func (n *NetworkBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return n.client.MarkForDeletion(ctx, namespace, name, kind)
}
func (n *NetworkBindingBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return n.client.MarkForDeletion(ctx, namespace, name, kind)
}
func (h *HostBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return h.client.MarkForDeletion(ctx, namespace, name, kind)
}
func (h *HostBindingBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return h.client.MarkForDeletion(ctx, namespace, name, kind)
}

type IECidrSvcRuleBackendOperations struct {
	client client.BackendClient
}

func NewIECidrSvcRuleBackendOperations(client client.BackendClient) BackendOperations[models.IECidrSvcRule] {
	return &IECidrSvcRuleBackendOperations{client: client}
}

func (i *IECidrSvcRuleBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (*models.IECidrSvcRule, error) {
	return i.client.GetIECidrSvcRule(ctx, id)
}

func (i *IECidrSvcRuleBackendOperations) List(ctx context.Context, scope ports.Scope) ([]models.IECidrSvcRule, error) {
	return i.client.ListIECidrSvcRules(ctx, scope)
}

func (i *IECidrSvcRuleBackendOperations) Create(ctx context.Context, obj *models.IECidrSvcRule) error {
	return i.client.CreateIECidrSvcRule(ctx, obj)
}

func (i *IECidrSvcRuleBackendOperations) Update(ctx context.Context, obj *models.IECidrSvcRule) error {
	return i.client.UpdateIECidrSvcRule(ctx, obj)
}

func (i *IECidrSvcRuleBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return i.client.DeleteIECidrSvcRule(ctx, id)
}

func (i *IECidrSvcRuleBackendOperations) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return i.client.MarkForDeletion(ctx, namespace, name, kind)
}
