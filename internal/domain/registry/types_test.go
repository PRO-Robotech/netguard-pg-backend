package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateRegistry tests that the registry passes validation
func TestValidateRegistry(t *testing.T) {
	err := ValidateRegistry()
	assert.NoError(t, err, "Registry validation should pass")
}

// TestRegistryInitialization tests that all required resources are registered
func TestRegistryInitialization(t *testing.T) {
	// Check registry has exactly 8 resources
	assert.Len(t, resourceRegistry, 8, "Registry should contain exactly 8 resource types")

	// Check all required types exist
	requiredTypes := []ResourceType{
		TypeHost, TypeAddressGroup, TypeNetwork, TypeService,
		TypeSvcSvcRule,
		TypeHostBinding, TypeNetworkBinding, TypeAddressGroupBinding,
	}

	for _, resType := range requiredTypes {
		_, exists := resourceRegistry[resType]
		assert.True(t, exists, "Resource type %s should be registered", resType)
	}
}

// TestGetResourceDefinition tests retrieving resource definitions
func TestGetResourceDefinition(t *testing.T) {
	tests := []struct {
		name        string
		resType     ResourceType
		shouldExist bool
		checkFunc   func(t *testing.T, def ResourceDefinition)
	}{
		{
			name:        "Host exists and is an Entity",
			resType:     TypeHost,
			shouldExist: true,
			checkFunc: func(t *testing.T, def ResourceDefinition) {
				assert.Equal(t, CategoryEntity, def.Category)
				assert.Equal(t, TargetSGROUP, def.TargetSystem)
				assert.True(t, def.SupportsCreate)
				assert.True(t, def.SupportsUpdate)
				assert.True(t, def.SupportsDelete)
				assert.Empty(t, def.AffectsResources)
			},
		},
		{
			name:        "AddressGroup exists and is an Entity",
			resType:     TypeAddressGroup,
			shouldExist: true,
			checkFunc: func(t *testing.T, def ResourceDefinition) {
				assert.Equal(t, CategoryEntity, def.Category)
				assert.Equal(t, TargetSGROUP, def.TargetSystem)
				assert.True(t, def.SupportsCreate)
				assert.True(t, def.SupportsUpdate)
				assert.True(t, def.SupportsDelete)
				assert.Empty(t, def.AffectsResources)
			},
		},
		{
			name:        "Network exists and is an Entity",
			resType:     TypeNetwork,
			shouldExist: true,
			checkFunc: func(t *testing.T, def ResourceDefinition) {
				assert.Equal(t, CategoryEntity, def.Category)
				assert.Equal(t, TargetSGROUP, def.TargetSystem)
			},
		},
		{
			name:        "Service exists and is an Entity",
			resType:     TypeService,
			shouldExist: true,
			checkFunc: func(t *testing.T, def ResourceDefinition) {
				assert.Equal(t, CategoryEntity, def.Category)
				assert.Equal(t, TargetSGROUP, def.TargetSystem)
			},
		},
		{
			name:        "HostBinding exists and is a Process",
			resType:     TypeHostBinding,
			shouldExist: true,
			checkFunc: func(t *testing.T, def ResourceDefinition) {
				assert.Equal(t, CategoryProcess, def.Category)
				assert.Equal(t, TargetInternal, def.TargetSystem)
				assert.True(t, def.SupportsCreate)
				assert.False(t, def.SupportsUpdate)
				assert.True(t, def.SupportsDelete)
				assert.Len(t, def.AffectsResources, 1)
				assert.Contains(t, def.AffectsResources, TypeHost)
			},
		},
		{
			name:        "NetworkBinding exists and is a Process",
			resType:     TypeNetworkBinding,
			shouldExist: true,
			checkFunc: func(t *testing.T, def ResourceDefinition) {
				assert.Equal(t, CategoryProcess, def.Category)
				assert.Equal(t, TargetInternal, def.TargetSystem)
				assert.True(t, def.SupportsCreate)
				assert.False(t, def.SupportsUpdate)
				assert.True(t, def.SupportsDelete)
				assert.Len(t, def.AffectsResources, 1)
				assert.Contains(t, def.AffectsResources, TypeNetwork)
			},
		},
		{
			name:        "Unknown resource type",
			resType:     ResourceType("UnknownType"),
			shouldExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, exists := GetResourceDefinition(tt.resType)
			assert.Equal(t, tt.shouldExist, exists)

			if tt.shouldExist && tt.checkFunc != nil {
				tt.checkFunc(t, def)
			}
		})
	}
}

// TestIsProcessResource tests identifying process resources
func TestIsProcessResource(t *testing.T) {
	tests := []struct {
		name      string
		resType   ResourceType
		isProcess bool
	}{
		{"HostBinding is Process", TypeHostBinding, true},
		{"NetworkBinding is Process", TypeNetworkBinding, true},
		{"Host is not Process", TypeHost, false},
		{"AddressGroup is not Process", TypeAddressGroup, false},
		{"Network is not Process", TypeNetwork, false},
		{"Service is not Process", TypeService, false},
		{"Unknown type is not Process", ResourceType("Unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsProcessResource(tt.resType)
			assert.Equal(t, tt.isProcess, result)
		})
	}
}

// TestIsEntityResource tests identifying entity resources
func TestIsEntityResource(t *testing.T) {
	tests := []struct {
		name     string
		resType  ResourceType
		isEntity bool
	}{
		{"Host is Entity", TypeHost, true},
		{"AddressGroup is Entity", TypeAddressGroup, true},
		{"Network is Entity", TypeNetwork, true},
		{"Service is Entity", TypeService, true},
		{"HostBinding is not Entity", TypeHostBinding, false},
		{"NetworkBinding is not Entity", TypeNetworkBinding, false},
		{"Unknown type is not Entity", ResourceType("Unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsEntityResource(tt.resType)
			assert.Equal(t, tt.isEntity, result)
		})
	}
}

// TestGetAffectedResources tests retrieving affected resources for process resources
func TestGetAffectedResources(t *testing.T) {
	tests := []struct {
		name             string
		resType          ResourceType
		expectedAffected []ResourceType
		shouldBeNil      bool
	}{
		{
			name:             "HostBinding affects Host",
			resType:          TypeHostBinding,
			expectedAffected: []ResourceType{TypeHost},
			shouldBeNil:      false,
		},
		{
			name:             "NetworkBinding affects Network",
			resType:          TypeNetworkBinding,
			expectedAffected: []ResourceType{TypeNetwork},
			shouldBeNil:      false,
		},
		{
			name:        "Host (Entity) has no affected resources",
			resType:     TypeHost,
			shouldBeNil: true,
		},
		{
			name:        "AddressGroup (Entity) has no affected resources",
			resType:     TypeAddressGroup,
			shouldBeNil: true,
		},
		{
			name:        "Unknown type has no affected resources",
			resType:     ResourceType("Unknown"),
			shouldBeNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affected := GetAffectedResources(tt.resType)

			if tt.shouldBeNil {
				assert.Nil(t, affected)
			} else {
				require.NotNil(t, affected)
				assert.Equal(t, len(tt.expectedAffected), len(affected))
				for _, expected := range tt.expectedAffected {
					assert.Contains(t, affected, expected)
				}
			}
		})
	}
}

// TestGetTargetSystem tests retrieving target system for resources
func TestGetTargetSystem(t *testing.T) {
	tests := []struct {
		name           string
		resType        ResourceType
		expectedSystem TargetSystem
		shouldExist    bool
	}{
		{"Host targets SGROUP", TypeHost, TargetSGROUP, true},
		{"AddressGroup targets SGROUP", TypeAddressGroup, TargetSGROUP, true},
		{"Network targets SGROUP", TypeNetwork, TargetSGROUP, true},
		{"Service targets SGROUP", TypeService, TargetSGROUP, true},
		{"HostBinding targets INTERNAL", TypeHostBinding, TargetInternal, true},
		{"NetworkBinding targets INTERNAL", TypeNetworkBinding, TargetInternal, true},
		{"Unknown type has no target", ResourceType("Unknown"), "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			system, exists := GetTargetSystem(tt.resType)
			assert.Equal(t, tt.shouldExist, exists)
			if tt.shouldExist {
				assert.Equal(t, tt.expectedSystem, system)
			}
		})
	}
}

// TestListEntityResources tests listing all entity resources
func TestListEntityResources(t *testing.T) {
	entities := ListEntityResources()

	// Should have exactly 5 entity resources
	assert.Len(t, entities, 5, "Should have 5 entity resources")

	// Check all expected entities are present
	expectedEntities := []ResourceType{
		TypeHost, TypeAddressGroup, TypeNetwork, TypeService, TypeSvcSvcRule,
	}

	for _, expected := range expectedEntities {
		assert.Contains(t, entities, expected, "Should contain %s", expected)
	}

	// Check no process resources are included
	assert.NotContains(t, entities, TypeHostBinding)
	assert.NotContains(t, entities, TypeNetworkBinding)
}

// TestListProcessResources tests listing all process resources
func TestListProcessResources(t *testing.T) {
	processes := ListProcessResources()

	// Should have exactly 3 process resources
	assert.Len(t, processes, 3, "Should have 3 process resources")

	// Check all expected processes are present
	expectedProcesses := []ResourceType{
		TypeHostBinding, TypeNetworkBinding, TypeAddressGroupBinding,
	}

	for _, expected := range expectedProcesses {
		assert.Contains(t, processes, expected, "Should contain %s", expected)
	}

	// Check no entity resources are included
	assert.NotContains(t, processes, TypeHost)
	assert.NotContains(t, processes, TypeAddressGroup)
	assert.NotContains(t, processes, TypeNetwork)
	assert.NotContains(t, processes, TypeService)
}

// TestGetRegistry tests retrieving a copy of the entire registry
func TestGetRegistry(t *testing.T) {
	registry := GetRegistry()

	// Should have all 8 resources
	assert.Len(t, registry, 8)

	// Modifying the returned copy should not affect the original
	originalLen := len(resourceRegistry)
	delete(registry, TypeHost)
	assert.Len(t, resourceRegistry, originalLen, "Original registry should not be modified")
}

// TestValidateResourceDefinition_EntityResources tests validation for entity resources
func TestValidateResourceDefinition_EntityResources(t *testing.T) {
	tests := []struct {
		name    string
		resType ResourceType
		def     ResourceDefinition
		wantErr bool
		errMsg  string
	}{
		{
			name:    "Valid entity resource",
			resType: TypeHost,
			def: ResourceDefinition{
				Type:             TypeHost,
				Category:         CategoryEntity,
				TargetSystem:     TargetSGROUP,
				SupportsCreate:   true,
				SupportsUpdate:   true,
				SupportsDelete:   true,
				AffectsResources: nil,
			},
			wantErr: false,
		},
		{
			name:    "Entity with wrong target system",
			resType: TypeHost,
			def: ResourceDefinition{
				Type:           TypeHost,
				Category:       CategoryEntity,
				TargetSystem:   TargetInternal, // Wrong!
				SupportsCreate: true,
			},
			wantErr: true,
			errMsg:  "entity resources must target SGROUP",
		},
		{
			name:    "Entity with affected resources",
			resType: TypeHost,
			def: ResourceDefinition{
				Type:             TypeHost,
				Category:         CategoryEntity,
				TargetSystem:     TargetSGROUP,
				SupportsCreate:   true,
				AffectsResources: []ResourceType{TypeAddressGroup}, // Wrong!
			},
			wantErr: true,
			errMsg:  "entity resources must not have affected resources",
		},
		{
			name:    "Resource with no operations",
			resType: TypeHost,
			def: ResourceDefinition{
				Type:           TypeHost,
				Category:       CategoryEntity,
				TargetSystem:   TargetSGROUP,
				SupportsCreate: false,
				SupportsUpdate: false,
				SupportsDelete: false,
			},
			wantErr: true,
			errMsg:  "resource must support at least one operation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResourceDefinition(tt.resType, tt.def)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateResourceDefinition_ProcessResources tests validation for process resources
func TestValidateResourceDefinition_ProcessResources(t *testing.T) {
	tests := []struct {
		name    string
		resType ResourceType
		def     ResourceDefinition
		wantErr bool
		errMsg  string
	}{
		{
			name:    "Valid process resource",
			resType: TypeHostBinding,
			def: ResourceDefinition{
				Type:             TypeHostBinding,
				Category:         CategoryProcess,
				TargetSystem:     TargetInternal,
				SupportsCreate:   true,
				SupportsDelete:   true,
				AffectsResources: []ResourceType{TypeHost},
			},
			wantErr: false,
		},
		{
			name:    "Process with wrong target system",
			resType: TypeHostBinding,
			def: ResourceDefinition{
				Type:             TypeHostBinding,
				Category:         CategoryProcess,
				TargetSystem:     TargetSGROUP, // Wrong!
				SupportsCreate:   true,
				AffectsResources: []ResourceType{TypeHost},
			},
			wantErr: true,
			errMsg:  "process resources must target INTERNAL",
		},
		{
			name:    "Process with no affected resources",
			resType: TypeHostBinding,
			def: ResourceDefinition{
				Type:             TypeHostBinding,
				Category:         CategoryProcess,
				TargetSystem:     TargetInternal,
				SupportsCreate:   true,
				AffectsResources: nil, // Wrong!
			},
			wantErr: true,
			errMsg:  "process resources must have at least one affected resource",
		},
		{
			name:    "Process with unregistered affected resource",
			resType: TypeHostBinding,
			def: ResourceDefinition{
				Type:             TypeHostBinding,
				Category:         CategoryProcess,
				TargetSystem:     TargetInternal,
				SupportsCreate:   true,
				AffectsResources: []ResourceType{ResourceType("UnknownType")},
			},
			wantErr: true,
			errMsg:  "affected resource UnknownType is not registered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResourceDefinition(tt.resType, tt.def)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestResourceCategoryConstants tests resource category constants
func TestResourceCategoryConstants(t *testing.T) {
	assert.Equal(t, ResourceCategory("ENTITY"), CategoryEntity)
	assert.Equal(t, ResourceCategory("PROCESS"), CategoryProcess)
}

// TestTargetSystemConstants tests target system constants
func TestTargetSystemConstants(t *testing.T) {
	assert.Equal(t, TargetSystem("SGROUP"), TargetSGROUP)
	assert.Equal(t, TargetSystem("INTERNAL"), TargetInternal)
}

// TestResourceTypeConstants tests resource type constants
func TestResourceTypeConstants(t *testing.T) {
	assert.Equal(t, ResourceType("Host"), TypeHost)
	assert.Equal(t, ResourceType("AddressGroup"), TypeAddressGroup)
	assert.Equal(t, ResourceType("Network"), TypeNetwork)
	assert.Equal(t, ResourceType("Service"), TypeService)
	assert.Equal(t, ResourceType("HostBinding"), TypeHostBinding)
	assert.Equal(t, ResourceType("NetworkBinding"), TypeNetworkBinding)
}

// TestEntityResourcesOperations tests that all entity resources support full CRUD
func TestEntityResourcesOperations(t *testing.T) {
	entityTypes := []ResourceType{TypeHost, TypeAddressGroup, TypeNetwork, TypeService}

	for _, resType := range entityTypes {
		t.Run(string(resType), func(t *testing.T) {
			def, exists := GetResourceDefinition(resType)
			require.True(t, exists)

			assert.True(t, def.SupportsCreate, "%s should support CREATE", resType)
			assert.True(t, def.SupportsUpdate, "%s should support UPDATE", resType)
			assert.True(t, def.SupportsDelete, "%s should support DELETE", resType)
		})
	}
}

// TestProcessResourcesOperations tests that process resources only support CREATE/DELETE
func TestProcessResourcesOperations(t *testing.T) {
	processTypes := []ResourceType{TypeHostBinding, TypeNetworkBinding}

	for _, resType := range processTypes {
		t.Run(string(resType), func(t *testing.T) {
			def, exists := GetResourceDefinition(resType)
			require.True(t, exists)

			assert.True(t, def.SupportsCreate, "%s should support CREATE", resType)
			assert.False(t, def.SupportsUpdate, "%s should NOT support UPDATE", resType)
			assert.True(t, def.SupportsDelete, "%s should support DELETE", resType)
		})
	}
}
