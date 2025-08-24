package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/pkg/k8s/clientset/versioned"
)

// ServerSideApplyE2ETestSuite is a comprehensive test suite for Server-Side Apply functionality
type ServerSideApplyE2ETestSuite struct {
	suite.Suite
	client    versioned.Interface
	ctx       context.Context
	namespace string
}

// SetupSuite initializes the test environment
func (suite *ServerSideApplyE2ETestSuite) SetupSuite() {
	suite.ctx = context.Background()
	suite.namespace = "ssa-test"

	// Initialize test client
	// Note: This assumes the API server is running and accessible
	// In real E2E setup, this would use kubeconfig or in-cluster config
	suite.client = suite.createTestClient()

	// Ensure test namespace exists
	suite.createTestNamespace()
}

// TearDownSuite cleans up test resources
func (suite *ServerSideApplyE2ETestSuite) TearDownSuite() {
	suite.cleanupTestResources()
}

// SetupTest prepares each individual test
func (suite *ServerSideApplyE2ETestSuite) SetupTest() {
	// Clean up any leftover resources from previous tests
	suite.cleanupTestServices()
	suite.cleanupTestAddressGroups()
}

// TestServerSideApply_Service_CreateAndUpdate tests the complete lifecycle of Server-Side Apply for Services
func (suite *ServerSideApplyE2ETestSuite) TestServerSideApply_Service_CreateAndUpdate() {
	serviceName := "ssa-test-service"

	t := suite.T()

	// Test 1: Create service via Server-Side Apply
	t.Run("CREATE_via_ServerSideApply", func(t *testing.T) {
		// Define initial service spec
		initialService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.namespace,
				Labels: map[string]string{
					"app":     "ssa-test",
					"version": "v1",
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Initial service via Server-Side Apply",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     "80",
						Protocol: netguardv1beta1.ProtocolTCP,
					},
				},
			},
		}

		// Apply with first field manager
		appliedService, err := suite.applyService(initialService, "test-manager-1", false)
		require.NoError(t, err)
		require.NotNil(t, appliedService)

		// Verify service was created
		assert.Equal(t, serviceName, appliedService.Name)
		assert.Equal(t, suite.namespace, appliedService.Namespace)
		assert.Equal(t, "Initial service via Server-Side Apply", appliedService.Spec.Description)
		assert.Len(t, appliedService.Spec.IngressPorts, 1)
		assert.Equal(t, "80", appliedService.Spec.IngressPorts[0].Port)

		// Verify managedFields were created
		assert.NotNil(t, appliedService.ManagedFields)
		assert.Len(t, appliedService.ManagedFields, 1)
		assert.Equal(t, "test-manager-1", appliedService.ManagedFields[0].Manager)
		assert.Equal(t, metav1.ManagedFieldsOperationApply, appliedService.ManagedFields[0].Operation)
	})

	// Test 2: Update service via Server-Side Apply with same manager
	t.Run("UPDATE_via_ServerSideApply_SameManager", func(t *testing.T) {
		// Define updated service spec
		updatedService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.namespace,
				Labels: map[string]string{
					"app":     "ssa-test",
					"version": "v1",
					"updated": "true", // New label
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Updated service via Server-Side Apply",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     "80",
						Protocol: netguardv1beta1.ProtocolTCP,
					},
					{
						Port:        "443",
						Protocol:    netguardv1beta1.ProtocolTCP,
						Description: "HTTPS port",
					},
				},
			},
		}

		// Apply with same field manager
		appliedService, err := suite.applyService(updatedService, "test-manager-1", false)
		require.NoError(t, err)
		require.NotNil(t, appliedService)

		// Verify service was updated
		assert.Equal(t, "Updated service via Server-Side Apply", appliedService.Spec.Description)
		assert.Len(t, appliedService.Spec.IngressPorts, 2)
		assert.Equal(t, "443", appliedService.Spec.IngressPorts[1].Port)

		// Verify updated label
		assert.Equal(t, "true", appliedService.Labels["updated"])

		// Verify managedFields still show single manager
		assert.Len(t, appliedService.ManagedFields, 1)
		assert.Equal(t, "test-manager-1", appliedService.ManagedFields[0].Manager)
	})

	// Test 3: Update service with different manager (should cause conflicts)
	t.Run("UPDATE_via_ServerSideApply_DifferentManager_Conflict", func(t *testing.T) {
		// Define conflicting update
		conflictingService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.namespace,
				Labels: map[string]string{
					"app":      "ssa-test",
					"version":  "v2", // Conflicting change
					"manager2": "true",
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Conflicting update from different manager",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     "8080", // Conflicting change
						Protocol: netguardv1beta1.ProtocolTCP,
					},
				},
			},
		}

		// Apply with different field manager (should fail due to conflicts)
		_, err := suite.applyService(conflictingService, "test-manager-2", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflict", "Should report field management conflicts")
	})

	// Test 4: Force apply to resolve conflicts
	t.Run("UPDATE_via_ServerSideApply_ForceApply", func(t *testing.T) {
		// Same conflicting update but with force=true
		conflictingService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.namespace,
				Labels: map[string]string{
					"app":      "ssa-test",
					"version":  "v2",
					"manager2": "true",
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Force applied update from different manager",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     "8080",
						Protocol: netguardv1beta1.ProtocolTCP,
					},
				},
			},
		}

		// Apply with force=true
		appliedService, err := suite.applyService(conflictingService, "test-manager-2", true)
		require.NoError(t, err)
		require.NotNil(t, appliedService)

		// Verify force apply worked
		assert.Equal(t, "Force applied update from different manager", appliedService.Spec.Description)
		assert.Equal(t, "v2", appliedService.Labels["version"])
		assert.Equal(t, "8080", appliedService.Spec.IngressPorts[0].Port)

		// Verify managedFields now show multiple managers
		assert.Len(t, appliedService.ManagedFields, 2)

		managers := make(map[string]bool)
		for _, field := range appliedService.ManagedFields {
			managers[field.Manager] = true
		}
		assert.True(t, managers["test-manager-1"])
		assert.True(t, managers["test-manager-2"])
	})

	// Test 5: Verify final state integrity
	t.Run("VERIFY_Final_State_Integrity", func(t *testing.T) {
		// Get current service state
		service, err := suite.client.NetguardV1beta1().Services(suite.namespace).Get(
			suite.ctx, serviceName, metav1.GetOptions{})
		require.NoError(t, err)

		// Verify all expected fields are present
		assert.Equal(t, "Force applied update from different manager", service.Spec.Description)
		assert.Equal(t, "v2", service.Labels["version"])
		assert.Equal(t, "true", service.Labels["manager2"])
		assert.Len(t, service.Spec.IngressPorts, 1)
		assert.Equal(t, "8080", service.Spec.IngressPorts[0].Port)

		// Verify managedFields integrity
		assert.NotNil(t, service.ManagedFields)
		assert.Len(t, service.ManagedFields, 2)

		// Verify each managedFields entry has proper structure
		for _, field := range service.ManagedFields {
			assert.NotEmpty(t, field.Manager)
			assert.NotNil(t, field.Time)
			assert.Equal(t, "FieldsV1", field.FieldsType)
			assert.NotNil(t, field.FieldsV1)
			assert.NotEmpty(t, field.FieldsV1.Raw)
		}
	})
}

// TestServerSideApply_AddressGroup_ComplexScenarios tests Server-Side Apply with AddressGroups
func (suite *ServerSideApplyE2ETestSuite) TestServerSideApply_AddressGroup_ComplexScenarios() {
	addressGroupName := "ssa-test-address-group"

	t := suite.T()

	// Test 1: Create AddressGroup with Networks
	t.Run("CREATE_AddressGroup_WithNetworks", func(t *testing.T) {
		initialAddressGroup := &netguardv1beta1.AddressGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      addressGroupName,
				Namespace: suite.namespace,
				Labels: map[string]string{
					"security-zone": "dmz",
				},
			},
			Spec: netguardv1beta1.AddressGroupSpec{
				DefaultAction: netguardv1beta1.ActionAccept,
				Logs:          true,
				Trace:         false,
			},
			Networks: []netguardv1beta1.NetworkItem{
				{
					Name:      "dmz-network",
					CIDR:      "192.168.1.0/24",
					Namespace: suite.namespace,
				},
			},
		}

		appliedAG, err := suite.applyAddressGroup(initialAddressGroup, "security-manager", false)
		require.NoError(t, err)
		require.NotNil(t, appliedAG)

		// Verify AddressGroup creation
		assert.Equal(t, addressGroupName, appliedAG.Name)
		assert.Equal(t, netguardv1beta1.ActionAccept, appliedAG.Spec.DefaultAction)
		assert.True(t, appliedAG.Spec.Logs)
		assert.Len(t, appliedAG.Networks, 1)
		assert.Equal(t, "dmz-network", appliedAG.Networks[0].Name)
		assert.Equal(t, "192.168.1.0/24", appliedAG.Networks[0].CIDR)

		// Verify managedFields
		assert.NotNil(t, appliedAG.ManagedFields)
		assert.Len(t, appliedAG.ManagedFields, 1)
		assert.Equal(t, "security-manager", appliedAG.ManagedFields[0].Manager)
	})

	// Test 2: Update AddressGroup networks from different manager
	t.Run("UPDATE_AddressGroup_Networks_DifferentManager", func(t *testing.T) {
		// Try to update networks with different manager
		updatedAG := &netguardv1beta1.AddressGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      addressGroupName,
				Namespace: suite.namespace,
				Labels: map[string]string{
					"security-zone":   "dmz",
					"network-manager": "true",
				},
			},
			Networks: []netguardv1beta1.NetworkItem{
				{
					Name:      "dmz-network",
					CIDR:      "192.168.1.0/24",
					Namespace: suite.namespace,
				},
				{
					Name:      "internal-network",
					CIDR:      "10.0.0.0/16",
					Namespace: suite.namespace,
				},
			},
		}

		// This should work as networks might not conflict with spec fields
		appliedAG, err := suite.applyAddressGroup(updatedAG, "network-manager", false)

		if err != nil {
			// If it fails due to conflicts, try with force
			appliedAG, err = suite.applyAddressGroup(updatedAG, "network-manager", true)
			require.NoError(t, err)
		}

		require.NotNil(t, appliedAG)

		// Verify networks were updated
		assert.Len(t, appliedAG.Networks, 2)

		networkNames := make(map[string]bool)
		for _, network := range appliedAG.Networks {
			networkNames[network.Name] = true
		}
		assert.True(t, networkNames["dmz-network"])
		assert.True(t, networkNames["internal-network"])
	})
}

// TestServerSideApply_SubresourceUpdates tests status subresource updates
func (suite *ServerSideApplyE2ETestSuite) TestServerSideApply_SubresourceUpdates() {
	serviceName := "ssa-status-test-service"

	t := suite.T()

	// Test 1: Create service first
	t.Run("CREATE_Service_For_Status_Test", func(t *testing.T) {
		service := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.namespace,
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Service for status testing",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     "80",
						Protocol: netguardv1beta1.ProtocolTCP,
					},
				},
			},
		}

		appliedService, err := suite.applyService(service, "main-manager", false)
		require.NoError(t, err)
		require.NotNil(t, appliedService)
	})

	// Test 2: Update status subresource
	t.Run("UPDATE_Status_Subresource", func(t *testing.T) {
		// Get current service
		service, err := suite.client.NetguardV1beta1().Services(suite.namespace).Get(
			suite.ctx, serviceName, metav1.GetOptions{})
		require.NoError(t, err)

		// Update status
		service.Status = netguardv1beta1.ServiceStatus{
			ObservedGeneration: service.Generation,
			Conditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "ServiceReady",
					Message:            "Service is ready for traffic",
				},
			},
		}

		// Apply status update (this would normally be done by a controller)
		updatedService, err := suite.applyServiceStatus(service, "status-manager", false)
		require.NoError(t, err)
		require.NotNil(t, updatedService)

		// Verify status was updated
		assert.Len(t, updatedService.Status.Conditions, 1)
		assert.Equal(t, "Ready", updatedService.Status.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionTrue, updatedService.Status.Conditions[0].Status)

		// Verify managedFields now includes status entries
		assert.NotNil(t, updatedService.ManagedFields)
		assert.True(t, len(updatedService.ManagedFields) >= 2) // main + status managers

		// Find status manager entry
		var statusManagerFound bool
		for _, field := range updatedService.ManagedFields {
			if field.Manager == "status-manager" && field.Subresource == "status" {
				statusManagerFound = true
				assert.Equal(t, metav1.ManagedFieldsOperationApply, field.Operation)
				break
			}
		}
		assert.True(t, statusManagerFound, "Status manager should be found in managedFields")
	})
}

// TestServerSideApply_RoundTripConsistency tests data consistency across Server-Side Apply operations
func (suite *ServerSideApplyE2ETestSuite) TestServerSideApply_RoundTripConsistency() {
	serviceName := "ssa-consistency-test"

	t := suite.T()

	t.Run("RoundTrip_Consistency", func(t *testing.T) {
		// Create service with complex structure
		originalService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.namespace,
				Labels: map[string]string{
					"app":         "consistency-test",
					"version":     "v1.0.0",
					"environment": "test",
				},
				Annotations: map[string]string{
					"description":        "Complex service for consistency testing",
					"last-updated-by":    "e2e-test",
					"special-characters": "test-with-ñ-and-中文-and-🚀",
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Consistency test service with special chars: ñ, 中文, 🚀",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:        "80",
						Protocol:    netguardv1beta1.ProtocolTCP,
						Description: "HTTP port",
					},
					{
						Port:        "443",
						Protocol:    netguardv1beta1.ProtocolTCP,
						Description: "HTTPS port with special chars: ñ 中文 🚀",
					},
					{
						Port:        "8080",
						Protocol:    netguardv1beta1.ProtocolTCP,
						Description: "Alt HTTP",
					},
				},
			},
		}

		// Apply service
		appliedService, err := suite.applyService(originalService, "consistency-manager", false)
		require.NoError(t, err)
		require.NotNil(t, appliedService)

		// Verify all fields were preserved correctly
		assert.Equal(t, originalService.Spec.Description, appliedService.Spec.Description)
		assert.Equal(t, len(originalService.Spec.IngressPorts), len(appliedService.Spec.IngressPorts))

		for i, originalPort := range originalService.Spec.IngressPorts {
			appliedPort := appliedService.Spec.IngressPorts[i]
			assert.Equal(t, originalPort.Port, appliedPort.Port)
			assert.Equal(t, originalPort.Protocol, appliedPort.Protocol)
			assert.Equal(t, originalPort.Description, appliedPort.Description)
		}

		// Verify labels and annotations
		for key, value := range originalService.Labels {
			assert.Equal(t, value, appliedService.Labels[key])
		}

		for key, value := range originalService.Annotations {
			assert.Equal(t, value, appliedService.Annotations[key])
		}

		// Get service again and verify consistency
		retrievedService, err := suite.client.NetguardV1beta1().Services(suite.namespace).Get(
			suite.ctx, serviceName, metav1.GetOptions{})
		require.NoError(t, err)

		// Verify retrieved service matches applied service
		assert.Equal(t, appliedService.Spec.Description, retrievedService.Spec.Description)
		assert.Equal(t, len(appliedService.Spec.IngressPorts), len(retrievedService.Spec.IngressPorts))

		// Verify managedFields structure
		assert.NotNil(t, retrievedService.ManagedFields)
		for _, field := range retrievedService.ManagedFields {
			assert.NotEmpty(t, field.Manager)
			assert.NotNil(t, field.Time)
			assert.Equal(t, "FieldsV1", field.FieldsType)
			assert.NotNil(t, field.FieldsV1)
			assert.NotEmpty(t, field.FieldsV1.Raw)

			// Verify FieldsV1 contains valid JSON
			var fieldsMap map[string]interface{}
			err := json.Unmarshal(field.FieldsV1.Raw, &fieldsMap)
			assert.NoError(t, err, "FieldsV1.Raw should contain valid JSON")
		}
	})
}

// Helper methods for test execution

// applyService applies a service using Server-Side Apply
func (suite *ServerSideApplyE2ETestSuite) applyService(service *netguardv1beta1.Service, fieldManager string, force bool) (*netguardv1beta1.Service, error) {
	// Set API version and kind
	service.APIVersion = "netguard.sgroups.io/v1beta1"
	service.Kind = "Service"

	// Convert service to unstructured for apply
	serviceData, err := json.Marshal(service)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal service: %w", err)
	}

	// Perform Server-Side Apply
	// Note: This would use the actual Kubernetes client-go Patch method with ApplyPatchType
	patchOptions := metav1.PatchOptions{
		FieldManager: fieldManager,
		Force:        &force,
	}

	return suite.client.NetguardV1beta1().Services(suite.namespace).Patch(
		suite.ctx, service.Name, types.ApplyPatchType, serviceData, patchOptions)
}

// applyAddressGroup applies an address group using Server-Side Apply
func (suite *ServerSideApplyE2ETestSuite) applyAddressGroup(ag *netguardv1beta1.AddressGroup, fieldManager string, force bool) (*netguardv1beta1.AddressGroup, error) {
	// Set API version and kind
	ag.APIVersion = "netguard.sgroups.io/v1beta1"
	ag.Kind = "AddressGroup"

	// Convert to JSON for apply
	agData, err := json.Marshal(ag)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal address group: %w", err)
	}

	patchOptions := metav1.PatchOptions{
		FieldManager: fieldManager,
		Force:        &force,
	}

	return suite.client.NetguardV1beta1().AddressGroups(suite.namespace).Patch(
		suite.ctx, ag.Name, types.ApplyPatchType, agData, patchOptions)
}

// applyServiceStatus applies service status using Server-Side Apply
func (suite *ServerSideApplyE2ETestSuite) applyServiceStatus(service *netguardv1beta1.Service, fieldManager string, force bool) (*netguardv1beta1.Service, error) {
	// Create a copy with only status fields for applying
	statusService := &netguardv1beta1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
		},
		Status: service.Status,
	}
	statusService.APIVersion = "netguard.sgroups.io/v1beta1"
	statusService.Kind = "Service"

	statusData, err := json.Marshal(statusService)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal service status: %w", err)
	}

	patchOptions := metav1.PatchOptions{
		FieldManager: fieldManager,
		Force:        &force,
	}

	return suite.client.NetguardV1beta1().Services(suite.namespace).Patch(
		suite.ctx, service.Name, types.ApplyPatchType, statusData, patchOptions, "status")
}

// createTestClient creates a test client for the API server
func (suite *ServerSideApplyE2ETestSuite) createTestClient() versioned.Interface {
	// Try to create client using default kubeconfig or in-cluster config
	config := GetDefaultConfig()

	helper, err := NewE2ETestHelper(config)
	if err != nil {
		// In real E2E environment, this should never fail
		// For unit testing, we can skip with a clear message
		suite.T().Skip("E2E test environment not available: " + err.Error())
		return nil
	}

	return helper.NetguardClient
}

// createTestNamespace ensures the test namespace exists
func (suite *ServerSideApplyE2ETestSuite) createTestNamespace() {
	// Use helper to create namespace
	config := GetDefaultConfig()
	config.Namespace = suite.namespace

	helper, err := NewE2ETestHelper(config)
	if err != nil {
		suite.T().Skip("E2E test environment not available: " + err.Error())
		return
	}

	err = helper.EnsureNamespace(suite.ctx)
	if err != nil {
		suite.T().Fatalf("Failed to create test namespace: %v", err)
	}
}

// cleanupTestResources removes all test resources
func (suite *ServerSideApplyE2ETestSuite) cleanupTestResources() {
	suite.cleanupTestServices()
	suite.cleanupTestAddressGroups()
}

// cleanupTestServices removes all test services
func (suite *ServerSideApplyE2ETestSuite) cleanupTestServices() {
	if suite.client == nil {
		return // No client available for cleanup
	}

	services, err := suite.client.NetguardV1beta1().Services(suite.namespace).List(
		suite.ctx, metav1.ListOptions{})
	if err != nil {
		suite.T().Logf("Failed to list services for cleanup: %v", err)
		return
	}

	for _, service := range services.Items {
		err := suite.client.NetguardV1beta1().Services(suite.namespace).Delete(
			suite.ctx, service.Name, metav1.DeleteOptions{})
		if err != nil {
			suite.T().Logf("Failed to delete service %s: %v", service.Name, err)
		}
	}
}

// cleanupTestAddressGroups removes all test address groups
func (suite *ServerSideApplyE2ETestSuite) cleanupTestAddressGroups() {
	if suite.client == nil {
		return // No client available for cleanup
	}

	addressGroups, err := suite.client.NetguardV1beta1().AddressGroups(suite.namespace).List(
		suite.ctx, metav1.ListOptions{})
	if err != nil {
		suite.T().Logf("Failed to list address groups for cleanup: %v", err)
		return
	}

	for _, ag := range addressGroups.Items {
		err := suite.client.NetguardV1beta1().AddressGroups(suite.namespace).Delete(
			suite.ctx, ag.Name, metav1.DeleteOptions{})
		if err != nil {
			suite.T().Logf("Failed to delete address group %s: %v", ag.Name, err)
		}
	}
}

// RunServerSideApplyE2ETests runs the complete Server-Side Apply E2E test suite
func RunServerSideApplyE2ETests(t *testing.T) {
	// Register NetGuard scheme
	netguardv1beta1.AddToScheme(scheme.Scheme)

	suite := new(ServerSideApplyE2ETestSuite)
	suite.Run(t, suite)
}

// TestServerSideApplyE2E is the main test function that can be run via go test
func TestServerSideApplyE2E(t *testing.T) {
	// Skip E2E tests unless explicitly enabled
	if testing.Short() {
		t.Skip("Skipping E2E tests in short mode")
	}

	// Check for environment variable to enable E2E tests
	// export RUN_E2E_TESTS=1 to enable
	// if os.Getenv("RUN_E2E_TESTS") == "" {
	//     t.Skip("E2E tests disabled. Set RUN_E2E_TESTS=1 to enable.")
	// }

	RunServerSideApplyE2ETests(t)
}
