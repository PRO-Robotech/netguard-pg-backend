package registry

import "fmt"

// ResourceCategory categorizes resources into Entity or Process types
type ResourceCategory string

const (
	// CategoryEntity represents entity resources that are synced to SGROUP
	// Examples: Host, AddressGroup, Network, Service
	CategoryEntity ResourceCategory = "ENTITY"

	// CategoryProcess represents process resources that are processed internally
	// and trigger updates to entity resources
	// Examples: HostBinding, NetworkBinding
	CategoryProcess ResourceCategory = "PROCESS"
)

// ResourceType identifies specific resource types in the system
type ResourceType string

const (
	// TypeHost represents a single host with an IP address
	TypeHost ResourceType = "Host"

	// TypeAddressGroup represents a collection of Hosts and Networks
	TypeAddressGroup ResourceType = "AddressGroup"

	// TypeNetwork represents a CIDR network range
	TypeNetwork ResourceType = "Network"

	// TypeService represents a network service with port mappings
	TypeService ResourceType = "Service"

	// TypeSvcSvcRule represents a service-to-service firewall rule
	TypeSvcSvcRule ResourceType = "SvcSvcRule"

	// TypeHostBinding represents a binding between a Host and an AddressGroup
	TypeHostBinding ResourceType = "HostBinding"

	// TypeNetworkBinding represents a binding between a Network and an AddressGroup
	TypeNetworkBinding ResourceType = "NetworkBinding"

	// TypeAddressGroupBinding represents a binding between a Service and an AddressGroup
	TypeAddressGroupBinding ResourceType = "AddressGroupBinding"
)

// TargetSystem identifies external systems for resource synchronization
type TargetSystem string

const (
	// TargetSGROUP represents the SGROUP external system
	// Entity resources (Host, AddressGroup, Network, Service) are synced to SGROUP
	TargetSGROUP TargetSystem = "SGROUP"

	// TargetInternal represents internal processing
	// Process resources (HostBinding, NetworkBinding) are processed internally
	TargetInternal TargetSystem = "INTERNAL"
)

// ResourceDefinition defines a syncable resource with its metadata and relationships
type ResourceDefinition struct {
	// Type is the resource type identifier
	Type ResourceType

	// Category indicates if this is an Entity or Process resource
	Category ResourceCategory

	// TargetSystem indicates where this resource is synced (SGROUP or INTERNAL)
	TargetSystem TargetSystem

	// SupportsCreate indicates if CREATE operations are supported
	SupportsCreate bool

	// SupportsUpdate indicates if UPDATE operations are supported
	SupportsUpdate bool

	// SupportsDelete indicates if DELETE operations are supported
	SupportsDelete bool

	// AffectsResources lists resource types affected by this Process resource
	// Only populated for Process resources (e.g., HostBinding affects [Host])
	// Empty for Entity resources
	AffectsResources []ResourceType
}

// resourceRegistry is the global registry of all resource definitions
// Initialized in init() function
var resourceRegistry map[ResourceType]ResourceDefinition

func init() {
	resourceRegistry = make(map[ResourceType]ResourceDefinition)

	// Entity Resources - synced to SGROUP
	// All entity resources support CREATE, UPDATE, DELETE operations

	resourceRegistry[TypeAddressGroup] = ResourceDefinition{
		Type:             TypeAddressGroup,
		Category:         CategoryEntity,
		TargetSystem:     TargetSGROUP,
		SupportsCreate:   true,
		SupportsUpdate:   true,
		SupportsDelete:   true,
		AffectsResources: nil, // Entity resources don't affect others
	}

	resourceRegistry[TypeHost] = ResourceDefinition{
		Type:             TypeHost,
		Category:         CategoryEntity,
		TargetSystem:     TargetSGROUP,
		SupportsCreate:   true,
		SupportsUpdate:   true,
		SupportsDelete:   true,
		AffectsResources: nil,
	}

	resourceRegistry[TypeNetwork] = ResourceDefinition{
		Type:             TypeNetwork,
		Category:         CategoryEntity,
		TargetSystem:     TargetSGROUP,
		SupportsCreate:   true,
		SupportsUpdate:   true,
		SupportsDelete:   true,
		AffectsResources: nil,
	}

	resourceRegistry[TypeService] = ResourceDefinition{
		Type:             TypeService,
		Category:         CategoryEntity,
		TargetSystem:     TargetSGROUP,
		SupportsCreate:   true,
		SupportsUpdate:   true,
		SupportsDelete:   true,
		AffectsResources: nil,
	}

	resourceRegistry[TypeSvcSvcRule] = ResourceDefinition{
		Type:             TypeSvcSvcRule,
		Category:         CategoryEntity,
		TargetSystem:     TargetSGROUP,
		SupportsCreate:   true,
		SupportsUpdate:   true,
		SupportsDelete:   true,
		AffectsResources: nil,
	}

	// Process Resources - processed internally
	// Process resources only support CREATE and DELETE (no UPDATE)
	// Each process resource affects one or more Entity resources

	resourceRegistry[TypeHostBinding] = ResourceDefinition{
		Type:           TypeHostBinding,
		Category:       CategoryProcess,
		TargetSystem:   TargetInternal,
		SupportsCreate: true,
		SupportsUpdate: false, // HostBinding only supports CREATE/DELETE
		SupportsDelete: true,
		// HostBinding now coordinates Host readiness only (AddressGroup handled via triggers)
		AffectsResources: []ResourceType{TypeHost},
	}

	resourceRegistry[TypeNetworkBinding] = ResourceDefinition{
		Type:           TypeNetworkBinding,
		Category:       CategoryProcess,
		TargetSystem:   TargetInternal,
		SupportsCreate: true,
		SupportsUpdate: false, // NetworkBinding only supports CREATE/DELETE
		SupportsDelete: true,
		// NetworkBinding now coordinates Network readiness only (AddressGroup handled via triggers)
		AffectsResources: []ResourceType{TypeNetwork},
	}

	resourceRegistry[TypeAddressGroupBinding] = ResourceDefinition{
		Type:           TypeAddressGroupBinding,
		Category:       CategoryProcess,
		TargetSystem:   TargetInternal,
		SupportsCreate: true,
		SupportsUpdate: false, // AddressGroupBinding only supports CREATE/DELETE
		SupportsDelete: true,
		// AddressGroupBinding affects Service and AddressGroup
		AffectsResources: []ResourceType{TypeService, TypeAddressGroup},
	}
}

// GetRegistry returns a read-only copy of the entire resource registry
func GetRegistry() map[ResourceType]ResourceDefinition {
	copy := make(map[ResourceType]ResourceDefinition, len(resourceRegistry))
	for k, v := range resourceRegistry {
		copy[k] = v
	}
	return copy
}

// GetResourceDefinition retrieves the definition for a specific resource type
// Returns error if resource type is not registered
func GetResourceDefinition(resType ResourceType) (ResourceDefinition, bool) {
	def, exists := resourceRegistry[resType]
	return def, exists
}

// IsProcessResource checks if the given resource type is a Process resource
// Returns false if resource type is not registered
func IsProcessResource(resType ResourceType) bool {
	def, exists := resourceRegistry[resType]
	return exists && def.Category == CategoryProcess
}

// IsEntityResource checks if the given resource type is an Entity resource
// Returns false if resource type is not registered
func IsEntityResource(resType ResourceType) bool {
	def, exists := resourceRegistry[resType]
	return exists && def.Category == CategoryEntity
}

// GetAffectedResources returns the list of resource types affected by a Process resource
// Returns nil if:
// - Resource type is not registered
// - Resource is not a Process resource
// - Process resource has no affected resources
func GetAffectedResources(resType ResourceType) []ResourceType {
	def, exists := resourceRegistry[resType]
	if !exists || def.Category != CategoryProcess {
		return nil
	}
	return def.AffectsResources
}

// GetTargetSystem returns the target system for the given resource type
// Returns empty string and false if resource type is not registered
func GetTargetSystem(resType ResourceType) (TargetSystem, bool) {
	def, exists := resourceRegistry[resType]
	if !exists {
		return "", false
	}
	return def.TargetSystem, true
}

// ListEntityResources returns all registered Entity resource types
func ListEntityResources() []ResourceType {
	var entities []ResourceType
	for resType, def := range resourceRegistry {
		if def.Category == CategoryEntity {
			entities = append(entities, resType)
		}
	}
	return entities
}

// ListProcessResources returns all registered Process resource types
func ListProcessResources() []ResourceType {
	var processes []ResourceType
	for resType, def := range resourceRegistry {
		if def.Category == CategoryProcess {
			processes = append(processes, resType)
		}
	}
	return processes
}

// ValidateRegistry validates the registry for consistency at startup
// Should be called during application initialization
// Returns error if validation fails
func ValidateRegistry() error {
	// Check registry is not empty
	if len(resourceRegistry) == 0 {
		return fmt.Errorf("registry is empty")
	}

	// Check expected count (6 resource types: 4 entities + 2 processes)
	requiredTypes := []ResourceType{
		TypeHost, TypeAddressGroup, TypeNetwork, TypeService,
		TypeHostBinding, TypeNetworkBinding,
	}

	for _, resType := range requiredTypes {
		if _, exists := resourceRegistry[resType]; !exists {
			return fmt.Errorf("required resource type not registered: %s", resType)
		}
	}

	// Validate each resource definition
	for resType, def := range resourceRegistry {
		if err := validateResourceDefinition(resType, def); err != nil {
			return fmt.Errorf("invalid resource definition for %s: %w", resType, err)
		}
	}

	return nil
}

// validateResourceDefinition validates a single resource definition
func validateResourceDefinition(resType ResourceType, def ResourceDefinition) error {
	// Check type consistency
	if def.Type != resType {
		return fmt.Errorf("type mismatch: registry key=%s, definition.Type=%s", resType, def.Type)
	}

	// Check category is valid
	if def.Category != CategoryEntity && def.Category != CategoryProcess {
		return fmt.Errorf("invalid category: %s (must be ENTITY or PROCESS)", def.Category)
	}

	// Check target system is valid
	if def.TargetSystem != TargetSGROUP && def.TargetSystem != TargetInternal {
		return fmt.Errorf("invalid target system: %s (must be SGROUP or INTERNAL)", def.TargetSystem)
	}

	// Entity resources must target SGROUP
	if def.Category == CategoryEntity && def.TargetSystem != TargetSGROUP {
		return fmt.Errorf("entity resources must target SGROUP, got: %s", def.TargetSystem)
	}

	// Process resources must target INTERNAL
	if def.Category == CategoryProcess && def.TargetSystem != TargetInternal {
		return fmt.Errorf("process resources must target INTERNAL, got: %s", def.TargetSystem)
	}

	// Process resources must have at least one affected resource
	if def.Category == CategoryProcess && len(def.AffectsResources) == 0 {
		return fmt.Errorf("process resources must have at least one affected resource")
	}

	// Entity resources must NOT have affected resources
	if def.Category == CategoryEntity && len(def.AffectsResources) > 0 {
		return fmt.Errorf("entity resources must not have affected resources")
	}

	// Validate affected resources exist in registry
	if def.Category == CategoryProcess {
		for _, affected := range def.AffectsResources {
			if _, exists := resourceRegistry[affected]; !exists {
				return fmt.Errorf("affected resource %s is not registered", affected)
			}
		}
	}

	// All resources must support at least one operation
	if !def.SupportsCreate && !def.SupportsUpdate && !def.SupportsDelete {
		return fmt.Errorf("resource must support at least one operation (CREATE, UPDATE, or DELETE)")
	}

	return nil
}
