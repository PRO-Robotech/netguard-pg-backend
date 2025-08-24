package resources

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
)

// RuleS2SRegenerator provides the ability to regenerate IEAgAg rules when service-related changes occur
// This interface is used to avoid circular dependencies between resource services
type RuleS2SRegenerator interface {
	// RegenerateIEAgAgRulesForService regenerates all IEAgAg rules that depend on a specific Service
	// Called when Service ports change or Service is created/updated/deleted
	RegenerateIEAgAgRulesForService(ctx context.Context, serviceID models.ResourceIdentifier) error

	// RegenerateIEAgAgRulesForServiceAlias regenerates all IEAgAg rules that depend on a specific ServiceAlias
	// Called when ServiceAlias is created/updated/deleted
	RegenerateIEAgAgRulesForServiceAlias(ctx context.Context, serviceAliasID models.ResourceIdentifier) error

	// RegenerateIEAgAgRulesForAddressGroupBinding regenerates IEAgAg rules affected by AddressGroupBinding changes
	// Called when AddressGroupBinding is created/updated/deleted
	RegenerateIEAgAgRulesForAddressGroupBinding(ctx context.Context, bindingID models.ResourceIdentifier) error

	// 🎯 NEW: NotifyServiceAddressGroupsChanged triggers RuleS2S condition recalculation when Service.AddressGroups changes
	// This method enables the reactive dependency chain: AddressGroupBinding → Service.AddressGroups → RuleS2S conditions
	// Called after updateServiceAddressGroups successfully updates a Service
	NotifyServiceAddressGroupsChanged(ctx context.Context, serviceID models.ResourceIdentifier) error
}
