package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/application/services/conditions"
	"netguard-pg-backend/internal/application/services/resources"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/interfaces"
)

type NetguardFacade struct {
	serviceResourceService        *resources.ServiceResourceService
	addressGroupResourceService   *resources.AddressGroupResourceService
	svcSvcRuleResourceService     *resources.SvcSvcRuleResourceService
	svcFqdnRuleResourceService    *resources.SvcFqdnRuleResourceService
	validationService             *resources.ValidationService
	networkResourceService        *resources.NetworkResourceService
	networkBindingResourceService *resources.NetworkBindingResourceService
	hostResourceService           *resources.HostResourceService
	hostBindingResourceService    *resources.HostBindingResourceService
	registry                      ports.Registry
	conditionManager              *conditions.ConditionManager
	syncManager                   interfaces.SyncManager
	mutex                         sync.Mutex
}

func NewNetguardFacade(
	registry ports.Registry,
	conditionManager *conditions.ConditionManager,
	syncManager interfaces.SyncManager,
) *NetguardFacade {
	serviceConditionAdapter := &serviceConditionManagerAdapter{conditionManager}
	addressGroupConditionAdapter := &addressGroupConditionManagerAdapter{conditionManager}
	networkConditionAdapter := &networkConditionManagerAdapter{conditionManager}
	networkBindingConditionAdapter := &networkBindingConditionManagerAdapter{conditionManager}
	hostConditionAdapter := &hostConditionManagerAdapter{conditionManager}
	hostBindingConditionAdapter := &hostBindingConditionManagerAdapter{conditionManager}
	svcSvcRuleConditionAdapter := &svcSvcRuleConditionManager{conditionManager}
	validationService := resources.NewValidationService(registry, syncManager)
	hostResourceService := resources.NewHostResourceService(registry, syncManager, hostConditionAdapter)
	serviceResourceService := resources.NewServiceResourceService(registry, syncManager, serviceConditionAdapter)
	addressGroupResourceService := resources.NewAddressGroupResourceService(registry, syncManager,
		addressGroupConditionAdapter, validationService, hostResourceService)
	networkResourceService := resources.NewNetworkResourceService(registry, syncManager, networkConditionAdapter)
	networkBindingResourceService := resources.NewNetworkBindingResourceService(registry, networkResourceService,
		addressGroupResourceService,
		syncManager, networkBindingConditionAdapter)
	hostBindingResourceService := resources.NewHostBindingResourceService(registry, hostResourceService,
		addressGroupResourceService, syncManager, hostBindingConditionAdapter)
	svcSvcRuleResourceService := resources.NewSvcSvcRuleResourceService(registry, syncManager, svcSvcRuleConditionAdapter)
	svcFqdnRuleResourceService := resources.NewSvcFqdnRuleResourceService(registry)
	facade := &NetguardFacade{
		serviceResourceService:        serviceResourceService,
		addressGroupResourceService:   addressGroupResourceService,
		svcSvcRuleResourceService:     svcSvcRuleResourceService,
		svcFqdnRuleResourceService:    svcFqdnRuleResourceService,
		validationService:             validationService,
		networkResourceService:        networkResourceService,
		networkBindingResourceService: networkBindingResourceService,
		hostResourceService:           hostResourceService,
		hostBindingResourceService:    hostBindingResourceService,
		registry:                      registry,
		conditionManager:              conditionManager,
		syncManager:                   syncManager,
	}
	if conditionManager != nil {
		conditionManager.SetSyncManager(syncManager)
		conditionManager.SetSequentialMutex(&facade.mutex)
	}
	serviceResourceService.SetPortMappingRegenerator(addressGroupResourceService)
	return facade
}

func (f *NetguardFacade) AddressGroupResourceService() *resources.AddressGroupResourceService {
	return f.addressGroupResourceService
}
func (f *NetguardFacade) GetServices(ctx context.Context, scope ports.Scope) ([]models.Service, error) {
	return f.serviceResourceService.GetServices(ctx, scope)
}
func (f *NetguardFacade) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return f.serviceResourceService.GetServiceByID(ctx, id)
}
func (f *NetguardFacade) GetServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.Service, error) {
	return f.serviceResourceService.GetServicesByIDs(ctx, ids)
}
func (f *NetguardFacade) CreateService(ctx context.Context, service models.Service) error {
	return f.serviceResourceService.CreateService(ctx, service)
}
func (f *NetguardFacade) UpdateService(ctx context.Context, service models.Service) error {
	return f.serviceResourceService.UpdateService(ctx, service)
}
func (f *NetguardFacade) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope) error {
	return f.serviceResourceService.SyncServices(ctx, services, scope, models.SyncOpUpsert)
}
func (f *NetguardFacade) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	return f.serviceResourceService.DeleteServicesByIDs(ctx, ids)
}
func (f *NetguardFacade) GetAddressGroups(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error) {
	return f.addressGroupResourceService.GetAddressGroups(ctx, scope)
}
func (f *NetguardFacade) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return f.addressGroupResourceService.GetAddressGroupByID(ctx, id)
}
func (f *NetguardFacade) GetAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroup, error) {
	return f.addressGroupResourceService.GetAddressGroupsByIDs(ctx, ids)
}
func (f *NetguardFacade) CreateAddressGroup(ctx context.Context, addressGroup models.AddressGroup) error {
	return f.addressGroupResourceService.CreateAddressGroup(ctx, addressGroup)
}
func (f *NetguardFacade) UpdateAddressGroup(ctx context.Context, addressGroup models.AddressGroup) error {
	return f.addressGroupResourceService.UpdateAddressGroup(ctx, addressGroup)
}
func (f *NetguardFacade) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope) error {
	return f.addressGroupResourceService.SyncAddressGroups(ctx, addressGroups, scope, models.SyncOpUpsert)
}
func (f *NetguardFacade) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	return f.addressGroupResourceService.DeleteAddressGroupsByIDs(ctx, ids)
}
func (f *NetguardFacade) GetAddressGroupBindings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBinding, error) {
	return f.addressGroupResourceService.GetAddressGroupBindings(ctx, scope)
}
func (f *NetguardFacade) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return f.addressGroupResourceService.GetAddressGroupBindingByID(ctx, id)
}
func (f *NetguardFacade) GetAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroupBinding, error) {
	return f.addressGroupResourceService.GetAddressGroupBindingsByIDs(ctx, ids)
}
func (f *NetguardFacade) CreateAddressGroupBinding(ctx context.Context, binding models.AddressGroupBinding) error {
	err := f.addressGroupResourceService.CreateAddressGroupBinding(ctx, binding)
	if err != nil {
		return err
	}
	f.processAddressGroupPortMappingConditionsAfterBinding(ctx, binding)
	return nil
}
func (f *NetguardFacade) UpdateAddressGroupBinding(ctx context.Context, binding models.AddressGroupBinding) error {
	err := f.addressGroupResourceService.UpdateAddressGroupBinding(ctx, binding)
	if err != nil {
		return err
	}
	f.processAddressGroupPortMappingConditionsAfterBinding(ctx, binding)
	return nil
}
func (f *NetguardFacade) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope) error {
	err := f.addressGroupResourceService.SyncAddressGroupBindings(ctx, bindings, scope, models.SyncOpUpsert)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		f.processAddressGroupPortMappingConditionsAfterBinding(ctx, binding)
	}
	return nil
}
func (f *NetguardFacade) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	err := f.addressGroupResourceService.DeleteAddressGroupBindingsByIDs(ctx, ids)
	if err != nil {
		klog.Errorf("DeleteAddressGroupBindingsByIDs failed for %d bindings: %v", len(ids), err)
	}
	return err
}
func (f *NetguardFacade) GetAddressGroupPortMappings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupPortMapping, error) {
	return f.addressGroupResourceService.GetAddressGroupPortMappings(ctx, scope)
}
func (f *NetguardFacade) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return f.addressGroupResourceService.GetAddressGroupPortMappingByID(ctx, id)
}
func (f *NetguardFacade) GetAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroupPortMapping, error) {
	return f.addressGroupResourceService.GetAddressGroupPortMappingsByIDs(ctx, ids)
}
func (f *NetguardFacade) CreateAddressGroupPortMapping(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	return f.addressGroupResourceService.CreateAddressGroupPortMapping(ctx, mapping)
}
func (f *NetguardFacade) UpdateAddressGroupPortMapping(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	return f.addressGroupResourceService.UpdateAddressGroupPortMapping(ctx, mapping)
}
func (f *NetguardFacade) SyncMultipleAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope) error {
	return f.addressGroupResourceService.SyncMultipleAddressGroupPortMappings(ctx, mappings, scope, models.SyncOpUpsert)
}
func (f *NetguardFacade) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	return f.addressGroupResourceService.DeleteAddressGroupPortMappingsByIDs(ctx, ids)
}
func (f *NetguardFacade) SyncAddressGroupPortMappings(ctx context.Context, binding models.AddressGroupBinding) error {
	return f.addressGroupResourceService.SyncAddressGroupPortMappings(ctx, binding)
}
func (f *NetguardFacade) SyncAddressGroupPortMappingsWithSyncOp(ctx context.Context, binding models.AddressGroupBinding, syncOp models.SyncOp) error {
	return f.addressGroupResourceService.SyncAddressGroupPortMappingsWithSyncOp(ctx, binding, syncOp)
}
func (f *NetguardFacade) SyncAddressGroupPortMappingsWithWriter(ctx context.Context, writer ports.Writer, binding models.AddressGroupBinding, syncOp models.SyncOp) error {
	return f.addressGroupResourceService.SyncAddressGroupPortMappingsWithWriter(ctx, writer, binding, syncOp)
}
func (f *NetguardFacade) SyncAddressGroupPortMappingsWithWriterAndReader(ctx context.Context, writer ports.Writer, reader ports.Reader, binding models.AddressGroupBinding, syncOp models.SyncOp) error {
	return f.addressGroupResourceService.SyncAddressGroupPortMappingsWithWriterAndReader(ctx, writer, reader, binding, syncOp)
}
func (f *NetguardFacade) GetAddressGroupBindingPolicies(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBindingPolicy, error) {
	return f.addressGroupResourceService.GetAddressGroupBindingPolicies(ctx, scope)
}
func (f *NetguardFacade) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return f.addressGroupResourceService.GetAddressGroupBindingPolicyByID(ctx, id)
}
func (f *NetguardFacade) GetAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroupBindingPolicy, error) {
	return f.addressGroupResourceService.GetAddressGroupBindingPoliciesByIDs(ctx, ids)
}
func (f *NetguardFacade) CreateAddressGroupBindingPolicy(ctx context.Context, policy models.AddressGroupBindingPolicy) error {
	return f.addressGroupResourceService.CreateAddressGroupBindingPolicy(ctx, policy)
}
func (f *NetguardFacade) UpdateAddressGroupBindingPolicy(ctx context.Context, policy models.AddressGroupBindingPolicy) error {
	return f.addressGroupResourceService.UpdateAddressGroupBindingPolicy(ctx, policy)
}
func (f *NetguardFacade) SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope ports.Scope) error {
	return f.addressGroupResourceService.SyncAddressGroupBindingPolicies(ctx, policies, scope, models.SyncOpUpsert)
}
func (f *NetguardFacade) DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	return f.addressGroupResourceService.DeleteAddressGroupBindingPoliciesByIDs(ctx, ids)
}
func (f *NetguardFacade) GetSvcSvcRules(ctx context.Context, scope ports.Scope) ([]models.SvcSvcRule, error) {
	return f.svcSvcRuleResourceService.GetSvcSvcRules(ctx, scope)
}
func (f *NetguardFacade) GetSvcSvcRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error) {
	return f.svcSvcRuleResourceService.GetSvcSvcRuleByID(ctx, id)
}
func (f *NetguardFacade) GetSvcSvcRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.SvcSvcRule, error) {
	return f.svcSvcRuleResourceService.GetSvcSvcRulesByIDs(ctx, ids)
}
func (f *NetguardFacade) CreateSvcSvcRule(ctx context.Context, rule models.SvcSvcRule) error {
	return f.svcSvcRuleResourceService.CreateSvcSvcRule(ctx, rule)
}
func (f *NetguardFacade) UpdateSvcSvcRule(ctx context.Context, rule models.SvcSvcRule) error {
	return f.svcSvcRuleResourceService.UpdateSvcSvcRule(ctx, rule)
}
func (f *NetguardFacade) SyncSvcSvcRules(ctx context.Context, rules []models.SvcSvcRule, scope ports.Scope) error {
	return f.svcSvcRuleResourceService.SyncSvcSvcRules(ctx, rules, scope, models.SyncOpUpsert)
}
func (f *NetguardFacade) DeleteSvcSvcRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	return f.svcSvcRuleResourceService.DeleteSvcSvcRulesByIDs(ctx, ids)
}
func (f *NetguardFacade) GetSvcFqdnRules(ctx context.Context, scope ports.Scope) ([]models.SvcFqdnRule, error) {
	return f.svcFqdnRuleResourceService.GetSvcFqdnRules(ctx, scope)
}

func (f *NetguardFacade) GetSvcFqdnRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcFqdnRule, error) {
	return f.svcFqdnRuleResourceService.GetSvcFqdnRuleByID(ctx, id)
}

func (f *NetguardFacade) CreateSvcFqdnRule(ctx context.Context, rule models.SvcFqdnRule) error {
	return f.svcFqdnRuleResourceService.CreateSvcFqdnRule(ctx, rule)
}

func (f *NetguardFacade) UpdateSvcFqdnRule(ctx context.Context, rule models.SvcFqdnRule) error {
	return f.svcFqdnRuleResourceService.UpdateSvcFqdnRule(ctx, rule)
}

func (f *NetguardFacade) DeleteSvcFqdnRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	return f.svcFqdnRuleResourceService.DeleteSvcFqdnRulesByIDs(ctx, ids)
}

func (f *NetguardFacade) GetNetworks(ctx context.Context, scope ports.Scope) ([]models.Network, error) {
	return f.networkResourceService.ListNetworks(ctx, scope)
}
func (f *NetguardFacade) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	return f.networkResourceService.GetNetwork(ctx, id)
}
func (f *NetguardFacade) CreateNetwork(ctx context.Context, network models.Network) error {
	return f.networkResourceService.CreateNetwork(ctx, &network)
}
func (f *NetguardFacade) UpdateNetwork(ctx context.Context, network models.Network) error {
	return f.networkResourceService.UpdateNetwork(ctx, &network)
}
func (f *NetguardFacade) DeleteNetwork(ctx context.Context, id models.ResourceIdentifier) error {
	return f.networkResourceService.DeleteNetwork(ctx, id)
}
func (f *NetguardFacade) ValidateNetworkBinding(ctx context.Context, networkID models.ResourceIdentifier, bindingID models.ResourceIdentifier) error {
	return f.networkResourceService.ValidateNetworkBinding(ctx, networkID, bindingID)
}
func (f *NetguardFacade) UpdateNetworkBindingRelationship(ctx context.Context, networkID models.ResourceIdentifier, bindingID models.ResourceIdentifier, addressGroupID models.ResourceIdentifier) error {
	return f.networkResourceService.UpdateNetworkBinding(ctx, networkID, bindingID, addressGroupID)
}
func (f *NetguardFacade) RemoveNetworkBinding(ctx context.Context, networkID models.ResourceIdentifier) error {
	return f.networkResourceService.RemoveNetworkBinding(ctx, networkID)
}
func (f *NetguardFacade) GetNetworkBindings(ctx context.Context, scope ports.Scope) ([]models.NetworkBinding, error) {
	if f.networkBindingResourceService != nil {
		return f.networkBindingResourceService.ListNetworkBindings(ctx, scope)
	}
	return []models.NetworkBinding{}, nil
}
func (f *NetguardFacade) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	if f.networkBindingResourceService != nil {
		return f.networkBindingResourceService.GetNetworkBinding(ctx, id)
	}
	return nil, ports.ErrNotFound
}
func (f *NetguardFacade) CreateNetworkBinding(ctx context.Context, binding models.NetworkBinding) error {
	if f.networkBindingResourceService != nil {
		return f.networkBindingResourceService.CreateNetworkBinding(ctx, &binding)
	}
	return nil
}
func (f *NetguardFacade) UpdateNetworkBinding(ctx context.Context, binding models.NetworkBinding) error {
	if f.networkBindingResourceService != nil {
		return f.networkBindingResourceService.UpdateNetworkBinding(ctx, &binding)
	}
	return nil
}
func (f *NetguardFacade) DeleteNetworkBinding(ctx context.Context, id models.ResourceIdentifier) error {
	if f.networkBindingResourceService != nil {
		return f.networkBindingResourceService.DeleteNetworkBinding(ctx, id)
	}
	return nil
}
func (f *NetguardFacade) GetHosts(ctx context.Context, scope ports.Scope) ([]models.Host, error) {
	reader, err := f.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create registry reader")
	}
	defer reader.Close()
	var hosts []models.Host
	err = reader.ListHosts(ctx, func(host models.Host) error {
		hosts = append(hosts, host)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list hosts from registry")
	}
	return hosts, nil
}
func (f *NetguardFacade) GetHostByID(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error) {
	reader, err := f.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create registry reader")
	}
	defer reader.Close()
	host, err := reader.GetHostByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get host from registry")
	}
	return host, nil
}
func (f *NetguardFacade) CreateHost(ctx context.Context, host models.Host) error {
	return f.Sync(ctx, models.SyncOpUpsert, []models.Host{host})
}
func (f *NetguardFacade) UpdateHost(ctx context.Context, host models.Host) error {
	return f.Sync(ctx, models.SyncOpUpsert, []models.Host{host})
}
func (f *NetguardFacade) DeleteHost(ctx context.Context, id models.ResourceIdentifier) error {
	host, err := f.GetHostByID(ctx, id)
	if err != nil {
		return errors.Wrap(err, "failed to get host for deletion")
	}
	return f.Sync(ctx, models.SyncOpDelete, []models.Host{*host})
}
func (f *NetguardFacade) GetHostBindings(ctx context.Context, scope ports.Scope) ([]models.HostBinding, error) {
	reader, err := f.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create registry reader")
	}
	defer reader.Close()
	var hostBindings []models.HostBinding
	err = reader.ListHostBindings(ctx, func(hostBinding models.HostBinding) error {
		hostBindings = append(hostBindings, hostBinding)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list host bindings from registry")
	}
	return hostBindings, nil
}
func (f *NetguardFacade) GetHostBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error) {
	reader, err := f.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create registry reader")
	}
	defer reader.Close()
	hostBinding, err := reader.GetHostBindingByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get host binding from registry")
	}
	return hostBinding, nil
}
func (f *NetguardFacade) CreateHostBinding(ctx context.Context, binding models.HostBinding) error {
	return f.Sync(ctx, models.SyncOpUpsert, []models.HostBinding{binding})
}
func (f *NetguardFacade) UpdateHostBinding(ctx context.Context, binding models.HostBinding) error {
	return f.Sync(ctx, models.SyncOpUpsert, []models.HostBinding{binding})
}
func (f *NetguardFacade) DeleteHostBinding(ctx context.Context, id models.ResourceIdentifier) error {
	hostBinding, err := f.GetHostBindingByID(ctx, id)
	if err != nil {
		return errors.Wrap(err, "failed to get host binding for deletion")
	}
	return f.Sync(ctx, models.SyncOpDelete, []models.HostBinding{*hostBinding})
}
func (f *NetguardFacade) FindServicesForAddressGroups(ctx context.Context, addressGroupIDs []models.ResourceIdentifier) ([]models.Service, error) {
	return f.addressGroupResourceService.FindServicesForAddressGroups(ctx, addressGroupIDs)
}
func (f *NetguardFacade) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return &models.SyncStatus{
		UpdatedAt: time.Now(),
	}, nil
}
func (f *NetguardFacade) SetSyncStatus(ctx context.Context, status models.SyncStatus) error {
	return nil
}
func (f *NetguardFacade) Sync(ctx context.Context, syncOp models.SyncOp, resources interface{}) error {
	switch typedResources := resources.(type) {
	case []models.Service:
		return f.serviceResourceService.SyncServices(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.AddressGroup:
		return f.addressGroupResourceService.SyncAddressGroups(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.AddressGroupBinding:
		return f.addressGroupResourceService.SyncAddressGroupBindings(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.AddressGroupPortMapping:
		return f.addressGroupResourceService.SyncMultipleAddressGroupPortMappings(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.AddressGroupBindingPolicy:
		return f.addressGroupResourceService.SyncAddressGroupBindingPolicies(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.Network:
		return f.networkResourceService.SyncNetworks(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.NetworkBinding:
		return f.networkBindingResourceService.SyncNetworkBindings(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.Host:
		return f.hostResourceService.SyncHosts(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.HostBinding:
		return f.hostBindingResourceService.SyncHostBindings(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.SvcSvcRule:
		return f.svcSvcRuleResourceService.SyncSvcSvcRules(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.SvcFqdnRule:
		return f.svcFqdnRuleResourceService.SyncSvcFqdnRules(ctx, typedResources, ports.EmptyScope{}, syncOp)
	default:
		return errors.New(fmt.Sprintf("unsupported resource type: %T", resources))
	}
}
func (f *NetguardFacade) ProcessConditionsIfNeeded(ctx context.Context, resource interface{}, syncOp models.SyncOp) {
	if f.conditionManager == nil {
		return
	}
	if syncOp == models.SyncOpDelete {
		return
	}
	switch r := resource.(type) {
	case *models.Service:
		if err := f.conditionManager.ProcessServiceConditions(ctx, r); err != nil {
			klog.Errorf("Failed to process service conditions: %v", err)
		}
	case *models.AddressGroup:
		if err := f.conditionManager.ProcessAddressGroupConditions(ctx, r); err != nil {
			klog.Errorf("Failed to process address group conditions: %v", err)
		}
	case *models.Network:
		if err := f.conditionManager.ProcessNetworkConditions(ctx, r, nil); err != nil {
			klog.Errorf("Failed to process network conditions: %v", err)
		}
	case *models.NetworkBinding:
		if err := f.conditionManager.ProcessNetworkBindingConditions(ctx, r); err != nil {
			klog.Errorf("Failed to process network binding conditions: %v", err)
		}
	case *models.AddressGroupBinding:
		if err := f.conditionManager.ProcessAddressGroupBindingConditions(ctx, r); err != nil {
			klog.Errorf("Failed to process AddressGroupBinding conditions: %v", err)
		}
	case *models.AddressGroupPortMapping:
		if err := f.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, r); err != nil {
			klog.Errorf("Failed to process AddressGroupPortMapping conditions: %v", err)
		}
	case *models.AddressGroupBindingPolicy:
		if err := f.conditionManager.ProcessAddressGroupBindingPolicyConditions(ctx, r); err != nil {
			klog.Errorf("Failed to process AddressGroupBindingPolicy conditions: %v", err)
		}
	}
}

type serviceConditionManagerAdapter struct {
	conditionManager *conditions.ConditionManager
}

func (a *serviceConditionManagerAdapter) ProcessServiceConditions(ctx context.Context, service *models.Service) error {
	return a.conditionManager.ProcessServiceConditions(ctx, service)
}
func (a *serviceConditionManagerAdapter) ProcessAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	return a.conditionManager.ProcessAddressGroupBindingConditions(ctx, binding)
}

type addressGroupConditionManagerAdapter struct {
	conditionManager *conditions.ConditionManager
}

func (a *addressGroupConditionManagerAdapter) ProcessAddressGroupConditions(ctx context.Context, addressGroup *models.AddressGroup) error {
	return a.conditionManager.ProcessAddressGroupConditions(ctx, addressGroup)
}
func (a *addressGroupConditionManagerAdapter) ProcessAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	return a.conditionManager.ProcessAddressGroupBindingConditions(ctx, binding)
}
func (a *addressGroupConditionManagerAdapter) ProcessAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	return a.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, mapping)
}
func (a *addressGroupConditionManagerAdapter) ProcessAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	return a.conditionManager.ProcessAddressGroupBindingPolicyConditions(ctx, policy)
}
func (a *addressGroupConditionManagerAdapter) SaveAddressGroupConditions(ctx context.Context, addressGroup *models.AddressGroup) error {
	return a.conditionManager.SaveAddressGroupConditions(ctx, addressGroup)
}
func (a *addressGroupConditionManagerAdapter) SaveAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	return a.conditionManager.SaveAddressGroupBindingConditions(ctx, binding)
}
func (a *addressGroupConditionManagerAdapter) SaveAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	return a.conditionManager.SaveAddressGroupPortMappingConditions(ctx, mapping)
}
func (a *addressGroupConditionManagerAdapter) SaveAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	return a.conditionManager.SaveAddressGroupBindingPolicyConditions(ctx, policy)
}

type networkConditionManagerAdapter struct {
	conditionManager *conditions.ConditionManager
}

func (a *networkConditionManagerAdapter) ProcessNetworkConditions(ctx context.Context, network *models.Network, syncResult error) error {
	return a.conditionManager.ProcessNetworkConditions(ctx, network, syncResult)
}

type networkBindingConditionManagerAdapter struct {
	conditionManager *conditions.ConditionManager
}

func (a *networkBindingConditionManagerAdapter) ProcessNetworkBindingConditions(ctx context.Context, networkBinding *models.NetworkBinding) error {
	return a.conditionManager.ProcessNetworkBindingConditions(ctx, networkBinding)
}

type hostConditionManagerAdapter struct {
	conditionManager *conditions.ConditionManager
}

func (a *hostConditionManagerAdapter) ProcessHostConditions(ctx context.Context, host *models.Host, syncResult error) error {
	return a.conditionManager.ProcessHostConditions(ctx, host, syncResult)
}

type hostBindingConditionManagerAdapter struct {
	conditionManager *conditions.ConditionManager
}

func (a *hostBindingConditionManagerAdapter) ProcessHostBindingConditions(ctx context.Context, hostBinding *models.HostBinding, syncResult error) error {
	return a.conditionManager.ProcessHostBindingConditions(ctx, hostBinding, syncResult)
}

func (f *NetguardFacade) processAddressGroupPortMappingConditionsAfterBinding(ctx context.Context, binding models.AddressGroupBinding) {
	mappingID := models.ResourceIdentifier{
		Name:      binding.AddressGroupRef.Name,
		Namespace: binding.AddressGroupRef.Namespace,
	}
	mapping, err := f.addressGroupResourceService.GetAddressGroupPortMappingByID(ctx, mappingID)
	if err != nil {
		klog.V(4).Infof("processAddressGroupPortMappingConditionsAfterBinding: No mapping found for %s/%s (this is normal): %v",
			mappingID.Namespace, mappingID.Name, err)
		return
	}
	if f.conditionManager != nil {
		if err := f.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, mapping); err != nil {
			klog.Errorf("Failed to process AddressGroupPortMapping conditions for %s/%s: %v",
				mapping.Namespace, mapping.Name, err)
		}
	}
}
func (f *NetguardFacade) GetNetworkBindingResourceService() *resources.NetworkBindingResourceService {
	return f.networkBindingResourceService
}
func (f *NetguardFacade) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	klog.InfoS("NetguardFacade.MarkForDeletion invoked",
		"namespace", namespace,
		"name", name,
		"kind", kind)

	// Use ExecuteDeleteWithRetry to handle serialization failures (SQLSTATE 40001)
	err := f.registry.ExecuteDeleteWithRetry(ctx, func(writer ports.Writer) error {
		klog.V(2).InfoS("NetguardFacade.MarkForDeletion: Executing MarkForDeletionWithStatus",
			"namespace", namespace, "name", name, "kind", kind)
		return writer.MarkForDeletionWithStatus(namespace, name, kind)
	})

	if err != nil {
		klog.ErrorS(err, "NetguardFacade.MarkForDeletion: MarkForDeletionWithStatus failed",
			"namespace", namespace, "name", name, "kind", kind)
		return err
	}

	klog.InfoS("NetguardFacade.MarkForDeletion completed",
		"namespace", namespace, "name", name, "kind", kind)
	return nil
}

type svcSvcRuleConditionManager struct {
	conditionManager *conditions.ConditionManager
}

func (s *svcSvcRuleConditionManager) ProcessSvcSvcRuleConditions(ctx context.Context, rule *models.SvcSvcRule) error {
	if s.conditionManager != nil {
		return nil
	}
	return nil
}
