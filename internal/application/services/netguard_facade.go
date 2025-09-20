package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/application/services/resources"
	"netguard-pg-backend/internal/application/utils"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/interfaces"
)

// NetguardFacade provides a unified interface that coordinates all resource services
// This maintains backward compatibility with the original NetguardService API
// while leveraging the decomposed resource services internally
type NetguardFacade struct {
	// Resource-specific services
	serviceResourceService      *resources.ServiceResourceService
	addressGroupResourceService *resources.AddressGroupResourceService
	ruleS2SResourceService      *resources.RuleS2SResourceService
	validationService           *resources.ValidationService

	// Network resource services
	networkResourceService        *resources.NetworkResourceService
	networkBindingResourceService *resources.NetworkBindingResourceService

	// Host resource services
	hostResourceService        *resources.HostResourceService
	hostBindingResourceService *resources.HostBindingResourceService

	// Internal dependencies (preserved from original)
	registry         ports.Registry
	conditionManager *ConditionManager
	syncManager      interfaces.SyncManager

	// 🎯 SEQUENTIAL_PROCESSING: Mutex to serialize RuleS2S operations and prevent PostgreSQL contention
	// This eliminates database serialization conflicts during complex Cross-RuleS2S aggregation flows
	ruleS2SMutex sync.Mutex
}

// ConditionManager is imported from condition_manager.go - no redeclaration needed

// NewNetguardFacade creates a new NetguardFacade with all resource services
func NewNetguardFacade(
	registry ports.Registry,
	conditionManager *ConditionManager,
	syncManager interfaces.SyncManager,
) *NetguardFacade {
	serviceConditionAdapter := &serviceConditionManagerAdapter{conditionManager}
	addressGroupConditionAdapter := &addressGroupConditionManagerAdapter{conditionManager}
	networkConditionAdapter := &networkConditionManagerAdapter{conditionManager}
	networkBindingConditionAdapter := &networkBindingConditionManagerAdapter{conditionManager}
	hostConditionAdapter := &hostConditionManagerAdapter{conditionManager}
	hostBindingConditionAdapter := &hostBindingConditionManagerAdapter{conditionManager}
	ruleConditionAdapter := &ruleConditionManager{conditionManager}

	validationService := resources.NewValidationService(registry, syncManager)

	// Create host resource services with condition managers (needed first for AddressGroupResourceService)
	hostResourceService := resources.NewHostResourceService(registry, syncManager, hostConditionAdapter)

	// Create resource services with condition managers
	serviceResourceService := resources.NewServiceResourceService(registry, syncManager, serviceConditionAdapter)
	addressGroupResourceService := resources.NewAddressGroupResourceService(registry, syncManager, addressGroupConditionAdapter, validationService, hostResourceService)

	// Create network resource services with condition managers
	networkResourceService := resources.NewNetworkResourceService(registry, syncManager, networkConditionAdapter)
	networkBindingResourceService := resources.NewNetworkBindingResourceService(registry, networkResourceService, syncManager, networkBindingConditionAdapter)
	hostBindingResourceService := resources.NewHostBindingResourceService(registry, hostResourceService, addressGroupResourceService, syncManager, hostBindingConditionAdapter)

	// Create RuleS2S service with condition manager
	ruleS2SResourceService := resources.NewRuleS2SResourceService(registry, syncManager, ruleConditionAdapter)

	// Initialize NetguardFacade first to get access to the sequential mutex
	facade := &NetguardFacade{
		serviceResourceService:        serviceResourceService,
		addressGroupResourceService:   addressGroupResourceService,
		ruleS2SResourceService:        ruleS2SResourceService,
		validationService:             validationService,
		networkResourceService:        networkResourceService,
		networkBindingResourceService: networkBindingResourceService,
		hostResourceService:           hostResourceService,
		hostBindingResourceService:    hostBindingResourceService,
		registry:                      registry,
		conditionManager:              conditionManager,
		syncManager:                   syncManager,
	}

	// Inject the RuleS2S service into ConditionManager for IEAgAg generation and cleanup
	if conditionManager != nil {
		conditionManager.SetIEAgAgRuleManager(ruleS2SResourceService)
		conditionManager.SetRuleS2SService(ruleS2SResourceService)

		// 🚀 EXTERNAL_SYNC_FIX: Inject SyncManager for AddressGroup external sync
		conditionManager.SetSyncManager(syncManager)
		klog.Infof("🚀 EXTERNAL_SYNC_FIX: Injected SyncManager into ConditionManager for AddressGroup sync")

		// 🔒 SEQUENTIAL_PROCESSING: Share the sequential mutex with ConditionManager
		// This extends deadlock prevention to condition batching operations
		conditionManager.SetSequentialMutex(&facade.ruleS2SMutex)
		klog.Infof("🔒 DEADLOCK_FIX: Injected sequential processing mutex into ConditionManager")
	}

	// Wire up service dependencies to avoid circular imports
	// ServiceResourceService needs AddressGroupResourceService for port mapping regeneration
	serviceResourceService.SetPortMappingRegenerator(addressGroupResourceService)

	// CRITICAL: Wire up RuleS2SRegenerator dependencies for reactive IEAgAg rule updates
	// ServiceResourceService needs RuleS2SResourceService to regenerate IEAgAg rules when Service/ServiceAlias changes
	serviceResourceService.SetRuleS2SRegenerator(ruleS2SResourceService)

	// AddressGroupResourceService needs RuleS2SResourceService to regenerate IEAgAg rules when AddressGroupBinding changes
	addressGroupResourceService.SetRuleS2SRegenerator(ruleS2SResourceService)

	klog.Infof("🔗 NetguardFacade: Successfully wired dependency injections - Service ↔ RuleS2S, AddressGroup ↔ RuleS2S")

	return facade
}

// =============================================================================
// Service Operations - delegate to ServiceResourceService
// =============================================================================

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

// ServiceAlias operations
func (f *NetguardFacade) GetServiceAliases(ctx context.Context, scope ports.Scope) ([]models.ServiceAlias, error) {
	return f.serviceResourceService.GetServiceAliases(ctx, scope)
}

func (f *NetguardFacade) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return f.serviceResourceService.GetServiceAliasByID(ctx, id)
}

func (f *NetguardFacade) GetServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.ServiceAlias, error) {
	return f.serviceResourceService.GetServiceAliasesByIDs(ctx, ids)
}

func (f *NetguardFacade) CreateServiceAlias(ctx context.Context, alias models.ServiceAlias) error {
	return f.serviceResourceService.CreateServiceAlias(ctx, alias)
}

func (f *NetguardFacade) UpdateServiceAlias(ctx context.Context, alias models.ServiceAlias) error {
	return f.serviceResourceService.UpdateServiceAlias(ctx, alias)
}

func (f *NetguardFacade) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope) error {
	return f.serviceResourceService.SyncServiceAliases(ctx, aliases, scope, models.SyncOpUpsert)
}

func (f *NetguardFacade) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	return f.serviceResourceService.DeleteServiceAliasesByIDs(ctx, ids)
}

// =============================================================================
// AddressGroup Operations - delegate to AddressGroupResourceService
// =============================================================================

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

// AddressGroupBinding operations
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

	// Process conditions for related AddressGroupPortMapping after successful binding creation
	f.processAddressGroupPortMappingConditionsAfterBinding(ctx, binding)

	return nil
}

func (f *NetguardFacade) UpdateAddressGroupBinding(ctx context.Context, binding models.AddressGroupBinding) error {
	err := f.addressGroupResourceService.UpdateAddressGroupBinding(ctx, binding)
	if err != nil {
		return err
	}

	// Process conditions for related AddressGroupPortMapping after successful binding update
	f.processAddressGroupPortMappingConditionsAfterBinding(ctx, binding)

	return nil
}

func (f *NetguardFacade) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope) error {
	err := f.addressGroupResourceService.SyncAddressGroupBindings(ctx, bindings, scope, models.SyncOpUpsert)
	if err != nil {
		return err
	}

	// Process conditions for related AddressGroupPortMappings after successful bindings sync
	for _, binding := range bindings {
		f.processAddressGroupPortMappingConditionsAfterBinding(ctx, binding)
	}

	return nil
}

func (f *NetguardFacade) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	f.ruleS2SMutex.Lock()
	defer f.ruleS2SMutex.Unlock()

	klog.V(2).Infof("🔒 SEQUENTIAL_PROCESSING: Starting DeleteAddressGroupBindingsByIDs for %d bindings (serialized to prevent concurrent reactive cleanup)", len(ids))
	err := f.addressGroupResourceService.DeleteAddressGroupBindingsByIDs(ctx, ids)
	if err != nil {
		klog.Errorf("❌ SEQUENTIAL_PROCESSING: DeleteAddressGroupBindingsByIDs failed for %d bindings: %v", len(ids), err)
	} else {
		klog.V(2).Infof("✅ SEQUENTIAL_PROCESSING: DeleteAddressGroupBindingsByIDs completed for %d bindings", len(ids))
	}
	return err
}

// AddressGroupPortMapping operations
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

// Port mapping sync operations (complex methods)
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

// AddressGroupBindingPolicy operations
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

// =============================================================================
// RuleS2S Operations - delegate to RuleS2SResourceService
// =============================================================================

func (f *NetguardFacade) GetRuleS2S(ctx context.Context, scope ports.Scope) ([]models.RuleS2S, error) {
	return f.ruleS2SResourceService.GetRuleS2S(ctx, scope)
}

func (f *NetguardFacade) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return f.ruleS2SResourceService.GetRuleS2SByID(ctx, id)
}

func (f *NetguardFacade) GetRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.RuleS2S, error) {
	return f.ruleS2SResourceService.GetRuleS2SByIDs(ctx, ids)
}

func (f *NetguardFacade) CreateRuleS2S(ctx context.Context, rule models.RuleS2S) error {
	// 🎯 SEQUENTIAL_PROCESSING: Serialize RuleS2S operations to prevent PostgreSQL contention
	// This ensures only one RuleS2S processes at a time, eliminating database conflicts during
	// complex Cross-RuleS2S aggregation and IEAgAgRule generation flows
	f.ruleS2SMutex.Lock()
	defer f.ruleS2SMutex.Unlock()

	klog.V(2).Infof("🔒 SEQUENTIAL_PROCESSING: Starting CreateRuleS2S for %s/%s (serialized)", rule.Namespace, rule.Name)
	err := f.ruleS2SResourceService.CreateRuleS2S(ctx, rule)
	if err != nil {
		klog.V(2).Infof("❌ SEQUENTIAL_PROCESSING: CreateRuleS2S failed for %s/%s: %v", rule.Namespace, rule.Name, err)
	} else {
		klog.V(2).Infof("✅ SEQUENTIAL_PROCESSING: CreateRuleS2S completed for %s/%s", rule.Namespace, rule.Name)
	}
	return err
}

func (f *NetguardFacade) UpdateRuleS2S(ctx context.Context, rule models.RuleS2S) error {
	// 🎯 SEQUENTIAL_PROCESSING: Serialize RuleS2S operations to prevent PostgreSQL contention
	f.ruleS2SMutex.Lock()
	defer f.ruleS2SMutex.Unlock()

	klog.V(2).Infof("🔒 SEQUENTIAL_PROCESSING: Starting UpdateRuleS2S for %s/%s (serialized)", rule.Namespace, rule.Name)
	err := f.ruleS2SResourceService.UpdateRuleS2S(ctx, rule)
	if err != nil {
		klog.V(2).Infof("❌ SEQUENTIAL_PROCESSING: UpdateRuleS2S failed for %s/%s: %v", rule.Namespace, rule.Name, err)
	} else {
		klog.V(2).Infof("✅ SEQUENTIAL_PROCESSING: UpdateRuleS2S completed for %s/%s", rule.Namespace, rule.Name)
	}
	return err
}

func (f *NetguardFacade) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope) error {
	// 🎯 SEQUENTIAL_PROCESSING: Serialize RuleS2S operations to prevent PostgreSQL contention
	// This is the CRITICAL method - bulk RuleS2S sync operations trigger the most database conflicts
	f.ruleS2SMutex.Lock()
	defer f.ruleS2SMutex.Unlock()

	klog.V(2).Infof("🔒 SEQUENTIAL_PROCESSING: Starting SyncRuleS2S for %d rules (serialized)", len(rules))
	err := f.ruleS2SResourceService.SyncRuleS2S(ctx, rules, scope, models.SyncOpUpsert)
	if err != nil {
		klog.V(2).Infof("❌ SEQUENTIAL_PROCESSING: SyncRuleS2S failed for %d rules: %v", len(rules), err)
	} else {
		klog.V(2).Infof("✅ SEQUENTIAL_PROCESSING: SyncRuleS2S completed for %d rules", len(rules))
	}
	return err
}

func (f *NetguardFacade) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	// 🎯 SEQUENTIAL_PROCESSING: Serialize RuleS2S operations to prevent PostgreSQL contention
	f.ruleS2SMutex.Lock()
	defer f.ruleS2SMutex.Unlock()

	klog.V(2).Infof("🔒 SEQUENTIAL_PROCESSING: Starting DeleteRuleS2SByIDs for %d rules (serialized)", len(ids))
	err := f.ruleS2SResourceService.DeleteRuleS2SByIDs(ctx, ids)
	if err != nil {
		klog.V(2).Infof("❌ SEQUENTIAL_PROCESSING: DeleteRuleS2SByIDs failed for %d rules: %v", len(ids), err)
	} else {
		klog.V(2).Infof("✅ SEQUENTIAL_PROCESSING: DeleteRuleS2SByIDs completed for %d rules", len(ids))
	}
	return err
}

// IEAgAgRule operations
func (f *NetguardFacade) GetIEAgAgRules(ctx context.Context, scope ports.Scope) ([]models.IEAgAgRule, error) {
	return f.ruleS2SResourceService.GetIEAgAgRules(ctx, scope)
}

func (f *NetguardFacade) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	return f.ruleS2SResourceService.GetIEAgAgRuleByID(ctx, id)
}

func (f *NetguardFacade) GetIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.IEAgAgRule, error) {
	return f.ruleS2SResourceService.GetIEAgAgRulesByIDs(ctx, ids)
}

func (f *NetguardFacade) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope) error {
	return f.ruleS2SResourceService.SyncIEAgAgRules(ctx, rules, scope)
}

func (f *NetguardFacade) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	f.ruleS2SMutex.Lock()
	defer f.ruleS2SMutex.Unlock()

	klog.V(2).Infof("🔒 SEQUENTIAL_PROCESSING: Starting DeleteIEAgAgRulesByIDs for %d rules (serialized)", len(ids))
	err := f.ruleS2SResourceService.DeleteIEAgAgRulesByIDs(ctx, ids)
	if err != nil {
		klog.Errorf("❌ SEQUENTIAL_PROCESSING: DeleteIEAgAgRulesByIDs failed for %d rules: %v", len(ids), err)
	} else {
		klog.V(2).Infof("✅ SEQUENTIAL_PROCESSING: DeleteIEAgAgRulesByIDs completed for %d rules", len(ids))
	}
	return err
}

// Complex rule generation methods
func (f *NetguardFacade) GenerateIEAgAgRulesFromRuleS2S(ctx context.Context, ruleS2S models.RuleS2S) ([]models.IEAgAgRule, error) {
	return f.ruleS2SResourceService.GenerateIEAgAgRulesFromRuleS2S(ctx, ruleS2S)
}

func (f *NetguardFacade) GenerateIEAgAgRulesFromRuleS2SWithReader(ctx context.Context, reader ports.Reader, ruleS2S models.RuleS2S) ([]models.IEAgAgRule, error) {
	return f.ruleS2SResourceService.GenerateIEAgAgRulesFromRuleS2SWithReader(ctx, reader, ruleS2S)
}

// Universal recalculation for ALL scenarios that affect IEAgAg rules
func (f *NetguardFacade) RecalculateAllAffectedIEAgAgRules(ctx context.Context, reason string) error {
	return f.ruleS2SResourceService.RecalculateAllAffectedIEAgAgRules(ctx, reason)
}

// Rule/Service relationship methods
func (f *NetguardFacade) FindRuleS2SForServices(ctx context.Context, serviceIDs []models.ResourceIdentifier) ([]models.RuleS2S, error) {
	return f.ruleS2SResourceService.FindRuleS2SForServices(ctx, serviceIDs)
}

func (f *NetguardFacade) FindRuleS2SForServicesWithReader(ctx context.Context, reader ports.Reader, serviceIDs []models.ResourceIdentifier) ([]models.RuleS2S, error) {
	return f.ruleS2SResourceService.FindRuleS2SForServicesWithReader(ctx, reader, serviceIDs)
}

func (f *NetguardFacade) FindRuleS2SForServiceAliases(ctx context.Context, aliasIDs []models.ResourceIdentifier) ([]models.RuleS2S, error) {
	return f.ruleS2SResourceService.FindRuleS2SForServiceAliases(ctx, aliasIDs)
}

// Complex rule update methods
func (f *NetguardFacade) UpdateIEAgAgRulesForRuleS2S(ctx context.Context, writer ports.Writer, rules []models.RuleS2S, syncOp models.SyncOp) error {
	return f.ruleS2SResourceService.UpdateIEAgAgRulesForRuleS2S(ctx, writer, rules, syncOp)
}

func (f *NetguardFacade) UpdateIEAgAgRulesForRuleS2SWithReader(ctx context.Context, writer ports.Writer, reader ports.Reader, rules []models.RuleS2S, syncOp models.SyncOp) error {
	return f.ruleS2SResourceService.UpdateIEAgAgRulesForRuleS2SWithReader(ctx, writer, reader, rules, syncOp)
}

// =============================================================================
// Network Operations - delegate to NetworkService
// =============================================================================

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

// Network binding operations
func (f *NetguardFacade) ValidateNetworkBinding(ctx context.Context, networkID models.ResourceIdentifier, bindingID models.ResourceIdentifier) error {
	return f.networkResourceService.ValidateNetworkBinding(ctx, networkID, bindingID)
}

func (f *NetguardFacade) UpdateNetworkBindingRelationship(ctx context.Context, networkID models.ResourceIdentifier, bindingID models.ResourceIdentifier, addressGroupID models.ResourceIdentifier) error {
	return f.networkResourceService.UpdateNetworkBinding(ctx, networkID, bindingID, addressGroupID)
}

func (f *NetguardFacade) RemoveNetworkBinding(ctx context.Context, networkID models.ResourceIdentifier) error {
	return f.networkResourceService.RemoveNetworkBinding(ctx, networkID)
}

// NetworkBinding operations - delegate to NetworkBindingService if needed
func (f *NetguardFacade) GetNetworkBindings(ctx context.Context, scope ports.Scope) ([]models.NetworkBinding, error) {
	if f.networkBindingResourceService != nil {
		return f.networkBindingResourceService.ListNetworkBindings(ctx, scope)
	}
	// Fallback implementation or return empty slice
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

// =============================================================================
// Host Operations - Direct Registry Access (TODO: Create HostResourceService)
// =============================================================================

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
	// Use Sync API with Upsert operation
	return f.Sync(ctx, models.SyncOpUpsert, []models.Host{host})
}

func (f *NetguardFacade) UpdateHost(ctx context.Context, host models.Host) error {
	// Use Sync API with Upsert operation
	return f.Sync(ctx, models.SyncOpUpsert, []models.Host{host})
}

func (f *NetguardFacade) DeleteHost(ctx context.Context, id models.ResourceIdentifier) error {
	// Get the host first to pass to sync for deletion
	host, err := f.GetHostByID(ctx, id)
	if err != nil {
		return errors.Wrap(err, "failed to get host for deletion")
	}

	// Use Sync API with Delete operation
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
	// Use Sync API with Upsert operation
	return f.Sync(ctx, models.SyncOpUpsert, []models.HostBinding{binding})
}

func (f *NetguardFacade) UpdateHostBinding(ctx context.Context, binding models.HostBinding) error {
	// Use Sync API with Upsert operation
	return f.Sync(ctx, models.SyncOpUpsert, []models.HostBinding{binding})
}

func (f *NetguardFacade) DeleteHostBinding(ctx context.Context, id models.ResourceIdentifier) error {
	// Get the host binding first to pass to sync for deletion
	hostBinding, err := f.GetHostBindingByID(ctx, id)
	if err != nil {
		return errors.Wrap(err, "failed to get host binding for deletion")
	}

	// Use Sync API with Delete operation
	return f.Sync(ctx, models.SyncOpDelete, []models.HostBinding{*hostBinding})
}

// =============================================================================
// Cross-Service Coordination Methods
// =============================================================================

// FindServicesForAddressGroups finds services related to address groups (coordination between services)
func (f *NetguardFacade) FindServicesForAddressGroups(ctx context.Context, addressGroupIDs []models.ResourceIdentifier) ([]models.Service, error) {
	return f.addressGroupResourceService.FindServicesForAddressGroups(ctx, addressGroupIDs)
}

// =============================================================================
// Utility Methods (preserved from original interface)
// =============================================================================

// GetSyncStatus returns overall sync status (could coordinate between all services)
func (f *NetguardFacade) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return &models.SyncStatus{
		UpdatedAt: time.Now(),
	}, nil
}

// SetSyncStatus sets overall sync status
func (f *NetguardFacade) SetSyncStatus(ctx context.Context, status models.SyncStatus) error {
	log.Printf("SetSyncStatus: Updated sync status at %v", status.UpdatedAt)
	return nil
}

// =============================================================================
// Main Sync Method (core gRPC interface compatibility)
// =============================================================================

// Sync is the main synchronization method used by gRPC endpoints
func (f *NetguardFacade) Sync(ctx context.Context, syncOp models.SyncOp, resources interface{}) error {
	log.Printf("🔥 DEBUG: NetguardFacade.Sync called with syncOp=%v, resources type=%T", syncOp, resources)

	// Delegate to appropriate resource service based on resource type with proper syncOp
	switch typedResources := resources.(type) {
	case []models.Service:
		// 🔍 TRACE: Log services received from gRPC layer
		for i, service := range typedResources {
			fmt.Printf("🔍 TRACE [Facade-Entry]: Service[%d] %s description='%s'\n",
				i, service.Key(), service.Description)
		}

		return f.serviceResourceService.SyncServices(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.AddressGroup:
		return f.addressGroupResourceService.SyncAddressGroups(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.AddressGroupBinding:
		return f.addressGroupResourceService.SyncAddressGroupBindings(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.AddressGroupPortMapping:
		return f.addressGroupResourceService.SyncMultipleAddressGroupPortMappings(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.RuleS2S:
		return f.ruleS2SResourceService.SyncRuleS2S(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.ServiceAlias:
		return f.serviceResourceService.SyncServiceAliases(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.AddressGroupBindingPolicy:
		return f.addressGroupResourceService.SyncAddressGroupBindingPolicies(ctx, typedResources, ports.EmptyScope{}, syncOp)
	case []models.IEAgAgRule:
		return f.ruleS2SResourceService.SyncIEAgAgRules(ctx, typedResources, ports.EmptyScope{})
	case []models.Network:
		log.Printf("🔥 DEBUG: Processing %d Network(s) with syncOp=%v", len(typedResources), syncOp)
		// Handle different sync operations for Networks
		for i, network := range typedResources {
			log.Printf("🔥 DEBUG: Processing Network[%d]: %s with syncOp=%v", i, network.Key(), syncOp)
			switch syncOp {
			case models.SyncOpDelete:
				log.Printf("🔥 DEBUG: Calling NetworkResourceService.DeleteNetwork for %s", network.Key())
				if err := f.networkResourceService.DeleteNetwork(ctx, network.SelfRef.ResourceIdentifier); err != nil {
					log.Printf("❌ DEBUG: NetworkResourceService.DeleteNetwork failed for %s: %v", network.Key(), err)
					return errors.Wrapf(err, "failed to delete network %s", network.Key())
				}
				log.Printf("✅ DEBUG: NetworkResourceService.DeleteNetwork completed successfully for %s", network.Key())
			case models.SyncOpUpsert, models.SyncOpFullSync:
				log.Printf("🔥 DEBUG: Calling NetworkResourceService.CreateNetwork for %s", network.Key())
				if err := f.networkResourceService.CreateNetwork(ctx, &network); err != nil {
					log.Printf("❌ DEBUG: NetworkResourceService.CreateNetwork failed for %s: %v", network.Key(), err)
					return errors.Wrapf(err, "failed to create network %s", network.Key())
				}
				log.Printf("✅ DEBUG: NetworkResourceService.CreateNetwork completed successfully for %s", network.Key())
			default:
				log.Printf("❌ DEBUG: Unsupported sync operation for Network: %v", syncOp)
				return errors.New(fmt.Sprintf("unsupported sync operation for Network: %v", syncOp))
			}
		}
		return nil
	case []models.NetworkBinding:

		// Handle different sync operations for NetworkBindings
		for _, binding := range typedResources {
			switch syncOp {
			case models.SyncOpDelete:
				if err := f.networkBindingResourceService.DeleteNetworkBinding(ctx, binding.SelfRef.ResourceIdentifier); err != nil {
					log.Printf("❌ DEBUG: DeleteNetworkBinding failed for %s: %v", binding.Key(), err)
					return errors.Wrapf(err, "failed to delete network binding %s", binding.Key())
				}
			case models.SyncOpUpsert, models.SyncOpFullSync:
				if err := f.networkBindingResourceService.CreateNetworkBinding(ctx, &binding); err != nil {
					log.Printf("❌ DEBUG: CreateNetworkBinding failed for %s: %v", binding.Key(), err)
					return errors.Wrapf(err, "failed to create network binding %s", binding.Key())
				}
			default:
				log.Printf("❌ DEBUG: Unsupported sync operation for NetworkBinding: %v", syncOp)
				return errors.New(fmt.Sprintf("unsupported sync operation for NetworkBinding: %v", syncOp))
			}
		}
		return nil
	case []models.Host:
		for _, host := range typedResources {
			switch syncOp {
			case models.SyncOpDelete:
				if err := f.hostResourceService.DeleteHost(ctx, host.SelfRef.ResourceIdentifier); err != nil {
					log.Printf("❌ DEBUG: DeleteHost failed for %s: %v", host.Key(), err)
					return errors.Wrapf(err, "failed to delete host %s", host.Key())
				}
			case models.SyncOpUpsert, models.SyncOpFullSync:
				if err := f.hostResourceService.CreateHost(ctx, &host); err != nil {
					log.Printf("❌ DEBUG: CreateHost failed for %s: %v", host.Key(), err)
					return errors.Wrapf(err, "failed to create host %s", host.Key())
				}
			default:
				log.Printf("❌ DEBUG: Unsupported sync operation for Host: %v", syncOp)
				return errors.New(fmt.Sprintf("unsupported sync operation for Host: %v", syncOp))
			}
		}
		return nil
	case []models.HostBinding:
		for _, hostBinding := range typedResources {
			switch syncOp {
			case models.SyncOpDelete:
				if err := f.hostBindingResourceService.DeleteHostBinding(ctx, hostBinding.SelfRef.ResourceIdentifier); err != nil {
					log.Printf("❌ DEBUG: DeleteHostBinding failed for %s: %v", hostBinding.Key(), err)
					return errors.Wrapf(err, "failed to delete host binding %s", hostBinding.Key())
				}
			case models.SyncOpUpsert, models.SyncOpFullSync:
				if err := f.hostBindingResourceService.CreateHostBinding(ctx, &hostBinding); err != nil {
					log.Printf("❌ DEBUG: CreateHostBinding failed for %s: %v", hostBinding.Key(), err)
					return errors.Wrapf(err, "failed to create host binding %s", hostBinding.Key())
				}
			default:
				log.Printf("❌ DEBUG: Unsupported sync operation for HostBinding: %v", syncOp)
				return errors.New(fmt.Sprintf("unsupported sync operation for HostBinding: %v", syncOp))
			}
		}
		return nil
	default:
		return errors.New(fmt.Sprintf("unsupported resource type: %T", resources))
	}
}

// ProcessConditionsIfNeeded processes conditions for resources (preserved from original)
func (f *NetguardFacade) ProcessConditionsIfNeeded(ctx context.Context, resource interface{}, syncOp models.SyncOp) {
	if f.conditionManager == nil {
		return
	}

	if syncOp == models.SyncOpDelete {
		log.Printf("🚫 ConditionManager: Skipping condition processing for DELETE operation to prevent recreation of deleted resources")
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
	case *models.RuleS2S:
		if err := f.conditionManager.ProcessRuleS2SConditions(ctx, r); err != nil {
			klog.Errorf("Failed to process RuleS2S conditions: %v", err)
		}
	case *models.ServiceAlias:
		if err := f.conditionManager.ProcessServiceAliasConditions(ctx, r); err != nil {
			klog.Errorf("Failed to process ServiceAlias conditions: %v", err)
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
	case *models.IEAgAgRule:
		if err := f.conditionManager.ProcessIEAgAgRuleConditions(ctx, r); err != nil {
			klog.Errorf("Failed to process IEAgAgRule conditions: %v", err)
		}
	}
}

// =============================================================================
// Condition Manager Adapters
// =============================================================================

// Adapters to make ConditionManager compatible with resource service interfaces
type serviceConditionManagerAdapter struct {
	conditionManager *ConditionManager
}

func (a *serviceConditionManagerAdapter) ProcessServiceConditions(ctx context.Context, service *models.Service) error {
	return a.conditionManager.ProcessServiceConditions(ctx, service)
}

func (a *serviceConditionManagerAdapter) ProcessServiceAliasConditions(ctx context.Context, alias *models.ServiceAlias) error {
	return a.conditionManager.ProcessServiceAliasConditions(ctx, alias)
}

func (a *serviceConditionManagerAdapter) ProcessAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	return a.conditionManager.ProcessAddressGroupBindingConditions(ctx, binding)
}

type addressGroupConditionManagerAdapter struct {
	conditionManager *ConditionManager
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
	return a.conditionManager.saveAddressGroupConditions(ctx, addressGroup)
}

func (a *addressGroupConditionManagerAdapter) SaveAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	return a.conditionManager.saveAddressGroupBindingConditions(ctx, binding)
}

func (a *addressGroupConditionManagerAdapter) SaveAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	return a.conditionManager.saveAddressGroupPortMappingConditions(ctx, mapping)
}

func (a *addressGroupConditionManagerAdapter) SaveAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	return a.conditionManager.saveAddressGroupBindingPolicyConditions(ctx, policy)
}

type networkConditionManagerAdapter struct {
	conditionManager *ConditionManager
}

func (a *networkConditionManagerAdapter) ProcessNetworkConditions(ctx context.Context, network *models.Network, syncResult error) error {
	return a.conditionManager.ProcessNetworkConditions(ctx, network, syncResult)
}

type networkBindingConditionManagerAdapter struct {
	conditionManager *ConditionManager
}

func (a *networkBindingConditionManagerAdapter) ProcessNetworkBindingConditions(ctx context.Context, networkBinding *models.NetworkBinding) error {
	return a.conditionManager.ProcessNetworkBindingConditions(ctx, networkBinding)
}

// hostConditionManagerAdapter adapts the existing ConditionManager to the interface expected by HostResourceService
type hostConditionManagerAdapter struct {
	conditionManager *ConditionManager
}

func (a *hostConditionManagerAdapter) ProcessHostConditions(ctx context.Context, host *models.Host, syncResult error) error {
	if syncResult != nil {
		// Set failed condition if sync failed
		utils.SetSyncFailedCondition(host, syncResult)
		log.Printf("❌ Host %s sync failed: %v", host.Key(), syncResult)
	} else {
		// Set success condition if sync succeeded
		utils.SetSyncSuccessCondition(host)
		log.Printf("✅ Host %s sync succeeded", host.Key())
	}

	return nil
}

// hostBindingConditionManagerAdapter adapts the existing ConditionManager to the interface expected by HostBindingResourceService
type hostBindingConditionManagerAdapter struct {
	conditionManager *ConditionManager
}

func (a *hostBindingConditionManagerAdapter) ProcessHostBindingConditions(ctx context.Context, hostBinding *models.HostBinding, syncResult error) error {
	// For now, just return nil as host binding conditions are not yet implemented
	// This can be extended when host binding condition processing is needed
	return nil
}

// ruleConditionManager adapts the existing ConditionManager to the interface expected by RuleS2SResourceService
type ruleConditionManager struct {
	conditionManager *ConditionManager
}

func (r *ruleConditionManager) ProcessRuleS2SConditions(ctx context.Context, rule *models.RuleS2S) error {
	if r.conditionManager != nil {
		return r.conditionManager.ProcessRuleS2SConditions(ctx, rule)
	}
	return nil
}

func (r *ruleConditionManager) ProcessIEAgAgRuleConditions(ctx context.Context, rule *models.IEAgAgRule) error {
	if r.conditionManager != nil {
		return r.conditionManager.ProcessIEAgAgRuleConditions(ctx, rule)
	}
	return nil
}

func (r *ruleConditionManager) SaveResourceConditions(ctx context.Context, resource interface{}) error {
	if r.conditionManager == nil {
		return nil
	}

	switch typedResource := resource.(type) {
	case *models.RuleS2S:
		return r.conditionManager.saveRuleS2SConditions(ctx, typedResource)
	case *models.IEAgAgRule:
		return r.conditionManager.saveIEAgAgRuleConditions(ctx, typedResource)
	default:
		klog.Warningf("SaveResourceConditions: Unsupported resource type %T", resource)
		return nil
	}
}

// processAddressGroupPortMappingConditionsAfterBinding processes conditions for AddressGroupPortMapping
// that is related to the given AddressGroupBinding after binding operations
func (f *NetguardFacade) processAddressGroupPortMappingConditionsAfterBinding(ctx context.Context, binding models.AddressGroupBinding) {
	// Get the related AddressGroupPortMapping
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

	// Process conditions for the mapping
	if f.conditionManager != nil {
		klog.Infof("🔄 NetguardFacade: Processing conditions for AddressGroupPortMapping %s/%s after binding operation",
			mapping.Namespace, mapping.Name)
		if err := f.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, mapping); err != nil {
			klog.Errorf("Failed to process AddressGroupPortMapping conditions for %s/%s: %v",
				mapping.Namespace, mapping.Name, err)
		}
	}
}

// GetNetworkBindingResourceService returns the NetworkBindingResourceService for external use
// This is used by the FinalizerController to access NetworkBinding operations
func (f *NetguardFacade) GetNetworkBindingResourceService() *resources.NetworkBindingResourceService {
	return f.networkBindingResourceService
}
