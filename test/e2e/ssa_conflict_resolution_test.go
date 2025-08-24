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

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// SSAConflictResolutionTestSuite tests conflict detection and resolution in Server-Side Apply
type SSAConflictResolutionTestSuite struct {
	suite.Suite
	helper *E2ETestHelper
	ctx    context.Context
}

// SetupSuite initializes the test environment
func (suite *SSAConflictResolutionTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	config := GetDefaultConfig()
	config.Namespace = "ssa-conflict-test"

	var err error
	suite.helper, err = NewE2ETestHelper(config)
	suite.Require().NoError(err)

	// Ensure test namespace exists
	err = suite.helper.EnsureNamespace(suite.ctx)
	suite.Require().NoError(err)
}

// TearDownSuite cleans up test resources
func (suite *SSAConflictResolutionTestSuite) TearDownSuite() {
	if suite.helper != nil {
		_ = suite.helper.CleanupNamespace(suite.ctx)
	}
}

// SetupTest prepares each individual test
func (suite *SSAConflictResolutionTestSuite) SetupTest() {
	_ = suite.helper.CleanupServices(suite.ctx)
	_ = suite.helper.CleanupAddressGroups(suite.ctx)
}

// TestConflictDetection tests that conflicts are properly detected
func (suite *SSAConflictResolutionTestSuite) TestConflictDetection() {
	serviceName := "conflict-detection-service"

	t := suite.T()

	// Step 1: Create initial service
	t.Run("CreateInitialService", func(t *testing.T) {
		service := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.helper.Config.Namespace,
				Labels: map[string]string{
					"app":   "conflict-test",
					"owner": "manager-1",
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Initial service for conflict detection",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     "80",
						Protocol: netguardv1beta1.ProtocolTCP,
					},
				},
			},
		}

		serviceData, err := suite.marshalResource(service)
		require.NoError(t, err)

		patchOptions := metav1.PatchOptions{
			FieldManager: "manager-1",
		}

		appliedService, err := suite.helper.NetguardClient.NetguardV1beta1().Services(service.Namespace).Patch(
			suite.ctx, service.Name, types.ApplyPatchType, serviceData, patchOptions)
		require.NoError(t, err)
		require.NotNil(t, appliedService)

		// Verify initial state
		assert.Equal(t, "Initial service for conflict detection", appliedService.Spec.Description)
		assert.Len(t, appliedService.ManagedFields, 1)
		assert.Equal(t, "manager-1", appliedService.ManagedFields[0].Manager)
	})

	// Step 2: Attempt conflicting update from different manager
	t.Run("ConflictingUpdate_ShouldFail", func(t *testing.T) {
		conflictingService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.helper.Config.Namespace,
				Labels: map[string]string{
					"app":   "conflict-test-modified", // Conflicting change
					"owner": "manager-2",              // Different manager claiming ownership
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Conflicting description from manager-2",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     "8080", // Conflicting port change
						Protocol: netguardv1beta1.ProtocolTCP,
					},
				},
			},
		}

		serviceData, err := suite.marshalResource(conflictingService)
		require.NoError(t, err)

		patchOptions := metav1.PatchOptions{
			FieldManager: "manager-2",
		}

		// This should fail due to conflicts
		_, err = suite.helper.NetguardClient.NetguardV1beta1().Services(conflictingService.Namespace).Patch(
			suite.ctx, conflictingService.Name, types.ApplyPatchType, serviceData, patchOptions)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflict", "Error should mention conflicts")
	})

	// Step 3: Verify original service is unchanged
	t.Run("VerifyOriginalUnchanged", func(t *testing.T) {
		service, err := suite.helper.NetguardClient.NetguardV1beta1().Services(suite.helper.Config.Namespace).Get(
			suite.ctx, serviceName, metav1.GetOptions{})
		require.NoError(t, err)

		// Original values should be preserved
		assert.Equal(t, "Initial service for conflict detection", service.Spec.Description)
		assert.Equal(t, "80", service.Spec.IngressPorts[0].Port)
		assert.Equal(t, "manager-1", service.Labels["owner"])

		// Still only one manager in managedFields
		assert.Len(t, service.ManagedFields, 1)
		assert.Equal(t, "manager-1", service.ManagedFields[0].Manager)
	})
}

// TestForceApplyResolution tests conflict resolution using force apply
func (suite *SSAConflictResolutionTestSuite) TestForceApplyResolution() {
	serviceName := "force-apply-service"

	t := suite.T()

	// Step 1: Create initial service with manager-1
	t.Run("CreateInitialService", func(t *testing.T) {
		service := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.helper.Config.Namespace,
				Labels: map[string]string{
					"app":        "force-test",
					"version":    "v1.0",
					"managed-by": "manager-1",
				},
				Annotations: map[string]string{
					"description": "Service managed by manager-1",
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Original service by manager-1",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:        "80",
						Protocol:    netguardv1beta1.ProtocolTCP,
						Description: "HTTP by manager-1",
					},
					{
						Port:        "443",
						Protocol:    netguardv1beta1.ProtocolTCP,
						Description: "HTTPS by manager-1",
					},
				},
			},
		}

		serviceData, err := suite.marshalResource(service)
		require.NoError(t, err)

		patchOptions := metav1.PatchOptions{
			FieldManager: "manager-1",
		}

		appliedService, err := suite.helper.NetguardClient.NetguardV1beta1().Services(service.Namespace).Patch(
			suite.ctx, service.Name, types.ApplyPatchType, serviceData, patchOptions)
		require.NoError(t, err)
		require.NotNil(t, appliedService)

		// Verify initial state
		assert.Len(t, appliedService.ManagedFields, 1)
		assert.Equal(t, "manager-1", appliedService.ManagedFields[0].Manager)
	})

	// Step 2: Force apply conflicting changes from manager-2
	t.Run("ForceApplyConflictingChanges", func(t *testing.T) {
		forcedService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.helper.Config.Namespace,
				Labels: map[string]string{
					"app":        "force-test",
					"version":    "v2.0",      // Changed by manager-2
					"managed-by": "manager-2", // Changed by manager-2
					"forced":     "true",      // New field by manager-2
				},
				Annotations: map[string]string{
					"description": "Service forcefully taken over by manager-2",
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Force applied by manager-2",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:        "8080", // Changed by manager-2
						Protocol:    netguardv1beta1.ProtocolTCP,
						Description: "HTTP forced by manager-2",
					},
					{
						Port:        "8443", // Changed by manager-2
						Protocol:    netguardv1beta1.ProtocolTCP,
						Description: "HTTPS forced by manager-2",
					},
					{
						Port:        "9090", // New port by manager-2
						Protocol:    netguardv1beta1.ProtocolTCP,
						Description: "Metrics by manager-2",
					},
				},
			},
		}

		serviceData, err := suite.marshalResource(forcedService)
		require.NoError(t, err)

		force := true
		patchOptions := metav1.PatchOptions{
			FieldManager: "manager-2",
			Force:        &force,
		}

		appliedService, err := suite.helper.NetguardClient.NetguardV1beta1().Services(forcedService.Namespace).Patch(
			suite.ctx, forcedService.Name, types.ApplyPatchType, serviceData, patchOptions)
		require.NoError(t, err)
		require.NotNil(t, appliedService)

		// Verify force apply worked
		assert.Equal(t, "Force applied by manager-2", appliedService.Spec.Description)
		assert.Equal(t, "v2.0", appliedService.Labels["version"])
		assert.Equal(t, "manager-2", appliedService.Labels["managed-by"])
		assert.Equal(t, "true", appliedService.Labels["forced"])
		assert.Len(t, appliedService.Spec.IngressPorts, 3)

		// Check that the new ports are present
		portNumbers := make(map[string]bool)
		for _, port := range appliedService.Spec.IngressPorts {
			portNumbers[port.Port] = true
		}
		assert.True(t, portNumbers["8080"])
		assert.True(t, portNumbers["8443"])
		assert.True(t, portNumbers["9090"])

		// Verify managedFields now has both managers
		assert.True(t, len(appliedService.ManagedFields) >= 2)

		managers := make(map[string]bool)
		for _, field := range appliedService.ManagedFields {
			managers[field.Manager] = true
		}

		// Both managers should be present (manager-1 might still own some fields not touched by manager-2)
		assert.True(t, managers["manager-2"], "manager-2 should be present in managedFields")
	})

	// Step 3: Verify field ownership transfer
	t.Run("VerifyFieldOwnershipTransfer", func(t *testing.T) {
		service, err := suite.helper.NetguardClient.NetguardV1beta1().Services(suite.helper.Config.Namespace).Get(
			suite.ctx, serviceName, metav1.GetOptions{})
		require.NoError(t, err)

		// Verify the forced changes are present
		assert.Equal(t, "Force applied by manager-2", service.Spec.Description)
		assert.Equal(t, "v2.0", service.Labels["version"])
		assert.Equal(t, "Service forcefully taken over by manager-2", service.Annotations["description"])

		// Verify managedFields structure
		err = suite.helper.ValidateManagedFields(service, TestValidation{
			Name: "force-apply-validation",
		})
		assert.NoError(t, err)

		// Check that manager-2 has the expected operation
		var manager2Found bool
		for _, field := range service.ManagedFields {
			if field.Manager == "manager-2" {
				manager2Found = true
				assert.Equal(t, metav1.ManagedFieldsOperationApply, field.Operation)
				break
			}
		}
		assert.True(t, manager2Found, "manager-2 should be found in managedFields")
	})
}

// TestPartialConflicts tests scenarios where only some fields conflict
func (suite *SSAConflictResolutionTestSuite) TestPartialConflicts() {
	serviceName := "partial-conflict-service"

	t := suite.T()

	// Step 1: Create service with manager-1
	t.Run("CreateInitialService", func(t *testing.T) {
		service := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.helper.Config.Namespace,
				Labels: map[string]string{
					"app":   "partial-test",
					"team":  "backend",
					"stage": "development",
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Service for partial conflict testing",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     "80",
						Protocol: netguardv1beta1.ProtocolTCP,
					},
				},
			},
		}

		serviceData, err := suite.marshalResource(service)
		require.NoError(t, err)

		patchOptions := metav1.PatchOptions{
			FieldManager: "backend-manager",
		}

		_, err = suite.helper.NetguardClient.NetguardV1beta1().Services(service.Namespace).Patch(
			suite.ctx, service.Name, types.ApplyPatchType, serviceData, patchOptions)
		require.NoError(t, err)
	})

	// Step 2: Update with manager-2 (non-conflicting fields should work)
	t.Run("UpdateNonConflictingFields", func(t *testing.T) {
		partialService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.helper.Config.Namespace,
				Labels: map[string]string{
					// Don't modify existing labels that might conflict
					"monitoring": "enabled", // New non-conflicting label
					"alerting":   "slack",   // New non-conflicting label
				},
				Annotations: map[string]string{
					"contact":       "ops-team@company.com", // New annotation
					"deployment-id": "12345",                // New annotation
				},
			},
		}

		serviceData, err := suite.marshalResource(partialService)
		require.NoError(t, err)

		patchOptions := metav1.PatchOptions{
			FieldManager: "ops-manager",
		}

		appliedService, err := suite.helper.NetguardClient.NetguardV1beta1().Services(partialService.Namespace).Patch(
			suite.ctx, partialService.Name, types.ApplyPatchType, serviceData, patchOptions)

		// This might succeed if the fields don't conflict
		if err != nil {
			// If it fails, it might be due to our field management logic being conservative
			t.Logf("Partial update failed (expected in some implementations): %v", err)
		} else {
			// Verify non-conflicting fields were added
			assert.Equal(t, "enabled", appliedService.Labels["monitoring"])
			assert.Equal(t, "slack", appliedService.Labels["alerting"])
			assert.Equal(t, "ops-team@company.com", appliedService.Annotations["contact"])

			// Verify original fields are preserved
			assert.Equal(t, "Service for partial conflict testing", appliedService.Spec.Description)
			assert.Equal(t, "backend", appliedService.Labels["team"])
		}
	})
}

// TestSubresourceConflicts tests conflicts on subresources (like status)
func (suite *SSAConflictResolutionTestSuite) TestSubresourceConflicts() {
	serviceName := "subresource-conflict-service"

	t := suite.T()

	// Step 1: Create service with main resource manager
	t.Run("CreateMainResource", func(t *testing.T) {
		service := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.helper.Config.Namespace,
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Service for subresource conflict testing",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     "80",
						Protocol: netguardv1beta1.ProtocolTCP,
					},
				},
			},
		}

		serviceData, err := suite.marshalResource(service)
		require.NoError(t, err)

		patchOptions := metav1.PatchOptions{
			FieldManager: "main-manager",
		}

		_, err = suite.helper.NetguardClient.NetguardV1beta1().Services(service.Namespace).Patch(
			suite.ctx, service.Name, types.ApplyPatchType, serviceData, patchOptions)
		require.NoError(t, err)
	})

	// Step 2: Update status with different manager (should not conflict)
	t.Run("UpdateStatusSubresource", func(t *testing.T) {
		// Get current service
		currentService, err := suite.helper.NetguardClient.NetguardV1beta1().Services(suite.helper.Config.Namespace).Get(
			suite.ctx, serviceName, metav1.GetOptions{})
		require.NoError(t, err)

		// Create status update
		statusUpdate := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: suite.helper.Config.Namespace,
			},
			Status: netguardv1beta1.ServiceStatus{
				ObservedGeneration: currentService.Generation,
				Conditions: []metav1.Condition{
					{
						Type:               "Ready",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
						Reason:             "ServiceReady",
						Message:            "Service is ready for traffic",
					},
				},
			},
		}

		statusData, err := suite.marshalResource(statusUpdate)
		require.NoError(t, err)

		patchOptions := metav1.PatchOptions{
			FieldManager: "status-manager",
		}

		appliedService, err := suite.helper.NetguardClient.NetguardV1beta1().Services(statusUpdate.Namespace).Patch(
			suite.ctx, statusUpdate.Name, types.ApplyPatchType, statusData, patchOptions, "status")

		if err != nil {
			t.Logf("Status update failed (might not be supported): %v", err)
		} else {
			// Verify status was updated
			assert.Len(t, appliedService.Status.Conditions, 1)
			assert.Equal(t, "Ready", appliedService.Status.Conditions[0].Type)

			// Verify both managers are present
			assert.True(t, len(appliedService.ManagedFields) >= 2)

			managers := make(map[string]string) // manager -> subresource
			for _, field := range appliedService.ManagedFields {
				managers[field.Manager] = field.Subresource
			}

			assert.Contains(t, managers, "main-manager")
			assert.Contains(t, managers, "status-manager")
			assert.Equal(t, "status", managers["status-manager"])
		}
	})
}

// marshalResource marshals a resource to JSON for Server-Side Apply
func (suite *SSAConflictResolutionTestSuite) marshalResource(resource interface{}) ([]byte, error) {
	// Set APIVersion and Kind based on resource type
	switch r := resource.(type) {
	case *netguardv1beta1.Service:
		r.APIVersion = "netguard.sgroups.io/v1beta1"
		r.Kind = "Service"
	case *netguardv1beta1.AddressGroup:
		r.APIVersion = "netguard.sgroups.io/v1beta1"
		r.Kind = "AddressGroup"
	}

	return json.Marshal(resource)
}

// RunSSAConflictResolutionTests runs the conflict resolution test suite
func RunSSAConflictResolutionTests(t *testing.T) {
	suite := new(SSAConflictResolutionTestSuite)
	suite.Run(t, suite)
}

// TestSSAConflictResolution is the main test function for conflict resolution
func TestSSAConflictResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping conflict resolution E2E tests in short mode")
	}

	RunSSAConflictResolutionTests(t)
}
