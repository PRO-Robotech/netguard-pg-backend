# NetGuard Test Resources Library

This directory contains a comprehensive library of valid YAML files for testing all NetGuard v1beta1 resources.

## 📁 Directory Structure

```
test/resources/
├── 01-basic/           # Independent resources (no dependencies)
├── 02-dependencies/    # Resources requiring other resources  
├── 03-complex/         # Complex scenarios with multiple dependencies
├── 04-integration/     # Complete integration scenarios
└── README.md          # This file
```

## 🎯 Resource Types (All v1beta1)

### ✅ Implemented Resources (10/10):

1. **Service** - Network service definitions with ingress ports
2. **AddressGroup** - Security groups with default actions
3. **ServiceAlias** - Aliases for services (requires Service)
4. **AddressGroupBinding** - Service ↔ AddressGroup bindings
5. **AddressGroupBindingPolicy** - Binding policies 
6. **AddressGroupPortMapping** - Port access definitions
7. **NetworkBinding** - Network ↔ AddressGroup bindings
8. **Network** - CIDR network definitions
9. **RuleS2S** - Service-to-service communication rules
10. **IEAgAgRule** - Ingress/Egress AddressGroup rules

## 🔄 Dependencies Order

### Level 1: Independent (01-basic/)
- **Service** - No dependencies
- **AddressGroup** - No dependencies  
- **Network** - No dependencies

### Level 2: Single Dependency (02-dependencies/)
- **ServiceAlias** - Requires: Service
- **AddressGroupBinding** - Requires: Service + AddressGroup
- **AddressGroupBindingPolicy** - Requires: Service + AddressGroup
- **AddressGroupPortMapping** - Requires: AddressGroup
- **NetworkBinding** - Requires: Network + AddressGroup

### Level 3: Complex Dependencies (03-complex/)
- **RuleS2S** - Requires: ServiceAlias (which requires Service)
- **IEAgAgRule** - Requires: AddressGroup

### Level 4: Integration Scenarios (04-integration/)
- Multi-resource scenarios demonstrating complete workflows

## 🚀 Quick Test Commands

### Create Test Namespace
```bash
kubectl create namespace netguard-test --dry-run=client -o yaml | kubectl apply -f -
```

### Basic Resources (Level 1)
```bash
# Create all basic resources
kubectl apply -f test/resources/01-basic/

# Verify creation
kubectl get services.v1beta1.netguard.sgroups.io -n netguard-test
kubectl get addressgroups.v1beta1.netguard.sgroups.io -n netguard-test  
kubectl get networks.v1beta1.netguard.sgroups.io -n netguard-test
```

### Dependency Resources (Level 2)
```bash
# Prerequisites: Level 1 must be created first
kubectl apply -f test/resources/02-dependencies/

# Verify creation
kubectl get servicealiases.v1beta1.netguard.sgroups.io -n netguard-test
kubectl get addressgroupbindings.v1beta1.netguard.sgroups.io -n netguard-test
kubectl get networkbindings.v1beta1.netguard.sgroups.io -n netguard-test
```

### Complex Resources (Level 3)
```bash
# Prerequisites: Level 1 and 2 must be created first
kubectl apply -f test/resources/03-complex/

# Verify creation
kubectl get rules2s.v1beta1.netguard.sgroups.io -n netguard-test
kubectl get ieagagrules.v1beta1.netguard.sgroups.io -n netguard-test
```

### Integration Scenarios (Level 4)
```bash
# Complete scenarios (may overwrite basic resources)
kubectl apply -f test/resources/04-integration/

# Check complete setup
kubectl get all -n netguard-test --show-labels
```

## 🧪 Test Operations

### CRUD Testing
```bash
# CREATE - Apply resources
kubectl apply -f test/resources/01-basic/service.yaml

# READ - Get resource
kubectl get service.v1beta1.netguard.sgroups.io test-service -n netguard-test -o yaml

# UPDATE - Patch resource
kubectl patch service.v1beta1.netguard.sgroups.io test-service -n netguard-test \
  --type='merge' -p='{"metadata":{"annotations":{"updated":"true"}}}'

# DELETE - Remove resource
kubectl delete -f test/resources/01-basic/service.yaml
```

### PATCH Testing
```bash
# JSON Patch
kubectl patch service.v1beta1.netguard.sgroups.io test-service -n netguard-test \
  --type='json' -p='[{"op": "replace", "path": "/spec/description", "value": "Updated via JSON Patch"}]'

# Merge Patch  
kubectl patch service.v1beta1.netguard.sgroups.io test-service -n netguard-test \
  --type='merge' -p='{"spec":{"description":"Updated via Merge Patch"}}'

# Strategic Merge Patch (default)
kubectl patch service.v1beta1.netguard.sgroups.io test-service -n netguard-test \
  -p='{"spec":{"description":"Updated via Strategic Merge Patch"}}'
```

## 🧹 Cleanup

### Clean All Test Resources
```bash
# Delete by namespace (removes everything)
kubectl delete namespace netguard-test

# Or delete by directory
kubectl delete -f test/resources/04-integration/
kubectl delete -f test/resources/03-complex/
kubectl delete -f test/resources/02-dependencies/
kubectl delete -f test/resources/01-basic/
```

### Selective Cleanup
```bash
# Delete specific resource types
kubectl delete services.v1beta1.netguard.sgroups.io --all -n netguard-test
kubectl delete addressgroups.v1beta1.netguard.sgroups.io --all -n netguard-test
```

## 🔍 Validation

### Verify All Resources Are Working
```bash
# Check API availability
kubectl api-resources --api-group=netguard.sgroups.io | grep v1beta1

# List all resources
kubectl get services.v1beta1.netguard.sgroups.io -A
kubectl get addressgroups.v1beta1.netguard.sgroups.io -A
kubectl get servicealiases.v1beta1.netguard.sgroups.io -A
kubectl get addressgroupbindings.v1beta1.netguard.sgroups.io -A
kubectl get addressgroupbindingpolicies.v1beta1.netguard.sgroups.io -A
kubectl get addressgroupportmappings.v1beta1.netguard.sgroups.io -A
kubectl get networkbindings.v1beta1.netguard.sgroups.io -A  
kubectl get networks.v1beta1.netguard.sgroups.io -A
kubectl get rules2s.v1beta1.netguard.sgroups.io -A
kubectl get ieagagrules.v1beta1.netguard.sgroups.io -A
```

## 📋 Common Issues

### Port Format
- ✅ Use `port: "80"` (string) 
- ❌ Don't use `port: 80` (number)

### ObjectReference Fields
- ✅ Always include `apiVersion: netguard.sgroups.io/v1beta1`
- ✅ Always include `kind: ResourceType`
- ✅ Always include `name` and `namespace`

### Resource Dependencies
- ✅ Create resources in dependency order (Level 1 → 2 → 3 → 4)
- ❌ Don't create dependent resources before their prerequisites

## 🔗 Integration with Scripts

See `test/scenarios/` directory for automated test scripts that use these resources.