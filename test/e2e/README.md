# Server-Side Apply E2E Test Suite

This directory contains comprehensive End-to-End tests for Server-Side Apply functionality in the NetGuard API server.

## Test Structure

### Core Test Files

- **`server_side_apply_test.go`** - Main E2E test suite with Service and AddressGroup scenarios
- **`ssa_conflict_resolution_test.go`** - Specialized conflict detection and resolution tests  
- **`ssa_test_helpers.go`** - Helper utilities and client management
- **`ssa_test_runner.go`** - Orchestrates the complete test suite with performance tests
- **`ssa_test_scenarios.yaml`** - Configuration-driven test scenarios

### Unit Test Files

- **`storage_preserve_managed_fields_test.go`** - Unit tests for managedFields preservation

## Test Coverage

### Functional Tests

✅ **CREATE operations** via Server-Side Apply  
✅ **UPDATE operations** with same/different managers  
✅ **Conflict detection** when different managers modify same fields  
✅ **Force apply resolution** to override conflicts  
✅ **Subresource updates** (status handling)  
✅ **Round-trip consistency** validation  
✅ **Complex scenarios** with multiple managers  
✅ **ManagedFields validation** and structure verification  

### Performance Tests

✅ **Large service apply** - Services with 100+ ingress ports  
✅ **Concurrent applies** - Multiple simultaneous operations  
✅ **Large managed fields** - Services managed by 20+ field managers  

### Error Scenarios

✅ **Invalid field manager names**  
✅ **Malformed apply patches**  
✅ **Conflict resolution failures**  

## Running Tests

### Prerequisites

1. **API Server Running**: The NetGuard API server must be running and accessible
2. **Kubeconfig**: Valid kubeconfig with access to the cluster
3. **Namespace Permissions**: Ability to create/delete namespaces and resources

### Environment Variables

```bash
# Optional: Custom test namespace (default: netguard-e2e-test)
export E2E_TEST_NAMESPACE=my-test-namespace

# Optional: Custom kubeconfig path
export KUBECONFIG=/path/to/kubeconfig

# Optional: Test timeout (default: 30s)
export E2E_TEST_TIMEOUT=60s

# Optional: Enable performance tests
export RUN_PERFORMANCE_TESTS=1

# Optional: Enable E2E tests (some tests check this)
export RUN_E2E_TESTS=1
```

### Running All Tests

```bash
# Run complete test suite
go test -v ./test/e2e -run TestCompleteSSATestSuite

# Run with performance tests
RUN_PERFORMANCE_TESTS=1 go test -v ./test/e2e -run TestCompleteSSATestSuite

# Run individual test suites
go test -v ./test/e2e -run TestServerSideApplyE2E
go test -v ./test/e2e -run TestSSAConflictResolution

# Skip long-running tests
go test -short -v ./test/e2e
```

### Development Testing

```bash
# Run unit tests only (no E2E environment needed)
go test -v ./internal/k8s/registry/base -run TestBaseStorage_PreserveManagedFields
go test -v ./internal/k8s/registry/convert -run TestServiceConverter_ManagedFields
```

## Test Scenarios

The test suite includes both programmatic and configuration-driven scenarios:

### Programmatic Scenarios

1. **Basic Service Lifecycle**
   - Create service via Server-Side Apply
   - Update with same manager
   - Update with different manager (conflict)
   - Force apply resolution

2. **AddressGroup Complex Scenarios**
   - Create with networks
   - Update networks from different manager
   - Conflict resolution

3. **Status Subresource Testing**
   - Main resource creation
   - Status updates by different manager
   - Verify separate field management

### Configuration-Driven Scenarios

Defined in `ssa_test_scenarios.yaml`:

- **basic-service-create** - Simple service creation
- **complex-service-with-multiple-ports** - Service with many ports
- **address-group-with-networks** - AddressGroup with network bindings
- **service-conflict-scenario** - Intentional conflicts
- **service-force-apply** - Force conflict resolution

## Test Architecture

### Helper Framework

The `E2ETestHelper` provides:

- **Client Management** - Kubernetes and NetGuard clients
- **Namespace Management** - Auto-creation and cleanup
- **Resource Cleanup** - Automatic cleanup between tests
- **Validation Utilities** - ManagedFields structure validation
- **Scenario Execution** - YAML-driven test scenarios

### Test Runner Framework

The `SSATestRunner` orchestrates:

- **Environment Setup** - Namespace creation and preparation
- **Test Execution** - All test suites in proper order
- **Performance Testing** - Concurrent and large-scale scenarios
- **Cleanup Management** - Configurable cleanup policies
- **Metrics Collection** - Test duration and performance data

## Validation Features

### ManagedFields Validation

Tests verify that managedFields entries contain:

- ✅ **Required fields** - manager, operation, apiVersion, time
- ✅ **Valid structure** - fieldsType="FieldsV1", valid fieldsV1.raw JSON
- ✅ **Proper ownership** - Field managers own correct fields
- ✅ **Conflict detection** - Overlapping field ownership detected
- ✅ **Merge behavior** - Multiple managers properly merged

### Data Consistency

Tests validate:

- ✅ **Round-trip consistency** - Applied data matches retrieved data
- ✅ **Special character preservation** - Unicode, emojis, complex strings
- ✅ **Array ordering** - List order preserved across operations
- ✅ **Nested structure integrity** - Complex objects preserved

## Performance Benchmarks

### Large Service Apply
- **Scale**: 100+ ingress ports per service
- **Metrics**: Apply time, memory usage, managedFields size
- **Expected**: < 5 seconds for 100 ports

### Concurrent Operations
- **Scale**: 10+ simultaneous apply operations
- **Metrics**: Success rate, conflict detection rate, total time
- **Expected**: > 90% success rate, < 30 seconds total

### Large ManagedFields
- **Scale**: 20+ field managers per resource
- **Metrics**: ManagedFields entry count, apply time, conflict resolution
- **Expected**: Proper field ownership tracking, reasonable performance

## Troubleshooting

### Common Issues

1. **API Server Not Running**
   ```
   Error: failed to initialize clients: connection refused
   Solution: Ensure NetGuard API server is running and accessible
   ```

2. **Permission Denied**
   ```
   Error: failed to create namespace: forbidden
   Solution: Ensure kubeconfig has proper permissions
   ```

3. **Test Timeout**
   ```
   Error: test timed out after 30s
   Solution: Increase E2E_TEST_TIMEOUT or check API server performance
   ```

4. **Resource Conflicts**
   ```
   Error: resource already exists
   Solution: Ensure cleanup is working or use different namespace
   ```

### Debug Mode

Enable verbose logging:

```bash
go test -v -args -test.v=2 ./test/e2e
```

### Manual Cleanup

If tests fail to cleanup:

```bash
kubectl delete namespace netguard-e2e-test
# Or your custom namespace
kubectl delete namespace $E2E_TEST_NAMESPACE
```

## Contributing

When adding new tests:

1. **Follow naming convention** - `TestSSA_FeatureName`
2. **Add cleanup** - Ensure resources are cleaned up
3. **Add validation** - Verify managedFields and data consistency
4. **Add documentation** - Update this README with new scenarios
5. **Consider performance** - Add performance scenarios for complex features

### Adding New Scenarios

1. Add to `ssa_test_scenarios.yaml`
2. Update test runner if needed
3. Add validation checks
4. Test both success and failure cases