# Resource Registry

Universal resource registry for dynamic resource routing in the OutboxWorker.

## Overview

The Resource Registry is a centralized system that defines all syncable resources in the netguard-pg-backend system. It provides metadata about each resource type including:

- **Category**: Entity or Process
- **Target System**: SGROUP or INTERNAL
- **Supported Operations**: CREATE, UPDATE, DELETE
- **Resource Relationships**: Which resources are affected by process resources

## Resource Categories

### Entity Resources (4)

Entity resources are synced to external SGROUP system:

- **Host**: Single host with IP address
- **AddressGroup**: Collection of Hosts and Networks
- **Network**: CIDR network range
- **Service**: Network service with port mappings

All entity resources:
- Target: `SGROUP`
- Support: CREATE, UPDATE, DELETE operations
- Have NO affected resources

### Process Resources (2)

Process resources are processed internally and trigger updates to entity resources:

- **HostBinding**: Binds a Host to an AddressGroup
- **NetworkBinding**: Binds a Network to an AddressGroup

All process resources:
- Target: `INTERNAL`
- Support: CREATE, DELETE only (NO UPDATE)
- Affect: AddressGroup resources

## Usage Examples

### Check if resource is a Process

```go
import "netguard-pg-backend/internal/domain/registry"

if registry.IsProcessResource(registry.TypeHostBinding) {
    // Handle process resource
    affected := registry.GetAffectedResources(registry.TypeHostBinding)
    // affected = [TypeAddressGroup]
}
```

### Check if resource is an Entity

```go
if registry.IsEntityResource(registry.TypeHost) {
    // Handle entity resource - sync to SGROUP
    target, _ := registry.GetTargetSystem(registry.TypeHost)
    // target = "SGROUP"
}
```

### Get resource definition

```go
def, exists := registry.GetResourceDefinition(registry.TypeHost)
if exists {
    fmt.Printf("Category: %s\n", def.Category)        // ENTITY
    fmt.Printf("Target: %s\n", def.TargetSystem)      // SGROUP
    fmt.Printf("Supports CREATE: %t\n", def.SupportsCreate)  // true
    fmt.Printf("Supports UPDATE: %t\n", def.SupportsUpdate)  // true
    fmt.Printf("Supports DELETE: %t\n", def.SupportsDelete)  // true
}
```

### List all entity resources

```go
entities := registry.ListEntityResources()
// Returns: [Host, AddressGroup, Network, Service]

for _, resType := range entities {
    fmt.Printf("Entity: %s\n", resType)
}
```

### List all process resources

```go
processes := registry.ListProcessResources()
// Returns: [HostBinding, NetworkBinding]

for _, resType := range processes {
    affected := registry.GetAffectedResources(resType)
    fmt.Printf("Process %s affects: %v\n", resType, affected)
}
```

### Validate registry at startup

```go
func main() {
    if err := registry.ValidateRegistry(); err != nil {
        log.Fatalf("Registry validation failed: %v", err)
    }

    // Registry is valid, continue with application startup
}
```

## Integration with OutboxWorker

The OutboxWorker uses the registry to dynamically route resources:

```go
func (w *OutboxWorker) ProcessMessage(msg OutboxMessage) error {
    resType := registry.ResourceType(msg.ResourceType)

    // Check if this is a process resource
    if registry.IsProcessResource(resType) {
        // Process internally
        affected := registry.GetAffectedResources(resType)
        for _, affectedType := range affected {
            // Trigger sync for affected entities
            w.syncEntity(affectedType, msg.ResourceID)
        }
        return nil
    }

    // This is an entity resource - sync to SGROUP
    if registry.IsEntityResource(resType) {
        target, _ := registry.GetTargetSystem(resType)
        return w.syncToExternalSystem(target, msg)
    }

    return fmt.Errorf("unknown resource type: %s", resType)
}
```

## Adding New Resource Types

To add a new resource type:

1. Define the constant in `types.go`:
   ```go
   const (
       TypeNewResource ResourceType = "NewResource"
   )
   ```

2. Add the definition to `resourceRegistry` in `init()`:
   ```go
   resourceRegistry[TypeNewResource] = ResourceDefinition{
       Type:             TypeNewResource,
       Category:         CategoryEntity,
       TargetSystem:     TargetSGROUP,
       SupportsCreate:   true,
       SupportsUpdate:   true,
       SupportsDelete:   true,
       AffectsResources: nil,
   }
   ```

3. Update validation in `ValidateRegistry()` to include the new type in `requiredTypes`.

4. Add test cases in `types_test.go`.

5. Run tests: `go test ./internal/domain/registry/... -v`

## Validation Rules

The registry enforces these rules:

1. Registry must contain exactly 6 resource types
2. Entity resources MUST target SGROUP
3. Process resources MUST target INTERNAL
4. Process resources MUST have at least one affected resource
5. Entity resources MUST NOT have affected resources
6. All resources MUST support at least one operation
7. Affected resources MUST exist in the registry

Validation runs at startup via `ValidateRegistry()`.

## Test Coverage

Current test coverage: **90.9%**

Run tests:
```bash
go test ./internal/domain/registry/... -v -cover
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   Resource Registry                      │
│                                                          │
│  ┌──────────────────┐      ┌──────────────────┐        │
│  │ Entity Resources │      │ Process Resources│        │
│  │                  │      │                  │        │
│  │ • Host           │      │ • HostBinding    │        │
│  │ • AddressGroup   │      │ • NetworkBinding │        │
│  │ • Network        │      │                  │        │
│  │ • Service        │      │ Affects:         │        │
│  │                  │◄─────│ • AddressGroup   │        │
│  │ Target: SGROUP   │      │ Target: INTERNAL │        │
│  └──────────────────┘      └──────────────────┘        │
│                                                          │
└─────────────────────────────────────────────────────────┘
                    ▲
                    │
                    │ Used by
                    │
         ┌──────────┴──────────┐
         │                     │
    OutboxWorker          SyncManager
    (Dynamic Routing)     (Operation Support)
```

## Related Files

- `types.go` - Type definitions and registry implementation
- `types_test.go` - Comprehensive test suite
- `README.md` - This documentation

## Version History

- **v1.0.0** (2025-10-13): Initial implementation with 6 resource types
