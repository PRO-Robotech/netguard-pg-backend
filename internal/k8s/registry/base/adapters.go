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
	netguardService *services.NetguardService
}

// NewServiceOperations creates a new ServiceOperations
func NewServiceOperations(netguardService *services.NetguardService) *ServiceOperations {
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
func NewServicePtrOpsWithNetguardService(netguardService *services.NetguardService) BackendOperations[*models.Service] {
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
