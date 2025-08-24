package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// SSATestRunner orchestrates the complete Server-Side Apply test suite
type SSATestRunner struct {
	config    *TestConfig
	helper    *E2ETestHelper
	ctx       context.Context
	startTime time.Time
}

// NewSSATestRunner creates a new test runner
func NewSSATestRunner() (*SSATestRunner, error) {
	config := GetDefaultConfig()

	// Override config from environment variables if available
	if namespace := os.Getenv("E2E_TEST_NAMESPACE"); namespace != "" {
		config.Namespace = namespace
	}
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config.Kubeconfig = kubeconfig
	}
	if timeout := os.Getenv("E2E_TEST_TIMEOUT"); timeout != "" {
		if duration, err := time.ParseDuration(timeout); err == nil {
			config.TimeoutDuration = duration
		}
	}

	helper, err := NewE2ETestHelper(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize E2E test helper: %w", err)
	}

	return &SSATestRunner{
		config:    config,
		helper:    helper,
		ctx:       context.Background(),
		startTime: time.Now(),
	}, nil
}

// RunAllTests executes the complete Server-Side Apply test suite
func (r *SSATestRunner) RunAllTests(t *testing.T) {
	// Register NetGuard scheme
	netguardv1beta1.AddToScheme(scheme.Scheme)

	r.logTestStart(t)

	// Ensure test environment is ready
	if err := r.setupTestEnvironment(t); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}
	defer r.cleanupTestEnvironment(t)

	// Run main Server-Side Apply E2E tests
	t.Run("ServerSideApply_E2E_Tests", func(t *testing.T) {
		RunServerSideApplyE2ETests(t)
	})

	// Run conflict resolution tests
	t.Run("SSA_Conflict_Resolution_Tests", func(t *testing.T) {
		RunSSAConflictResolutionTests(t)
	})

	// Run scenario-based tests
	t.Run("SSA_Scenario_Based_Tests", func(t *testing.T) {
		r.runScenarioBasedTests(t)
	})

	// Run performance tests (if enabled)
	if !testing.Short() && os.Getenv("RUN_PERFORMANCE_TESTS") == "1" {
		t.Run("SSA_Performance_Tests", func(t *testing.T) {
			r.runPerformanceTests(t)
		})
	}

	r.logTestCompletion(t)
}

// setupTestEnvironment prepares the test environment
func (r *SSATestRunner) setupTestEnvironment(t *testing.T) error {
	// Create test namespace
	if err := r.helper.EnsureNamespace(r.ctx); err != nil {
		return fmt.Errorf("failed to create test namespace: %w", err)
	}

	// Cleanup any existing resources
	if err := r.helper.CleanupServices(r.ctx); err != nil {
		t.Logf("Warning: failed to cleanup existing services: %v", err)
	}

	if err := r.helper.CleanupAddressGroups(r.ctx); err != nil {
		t.Logf("Warning: failed to cleanup existing address groups: %v", err)
	}

	t.Logf("Test environment ready in namespace: %s", r.config.Namespace)
	return nil
}

// cleanupTestEnvironment cleans up the test environment
func (r *SSATestRunner) cleanupTestEnvironment(t *testing.T) {
	if r.config.CleanupPolicy == "auto" {
		if err := r.helper.CleanupNamespace(r.ctx); err != nil {
			t.Logf("Warning: failed to cleanup test namespace: %v", err)
		} else {
			t.Logf("Test namespace %s cleaned up successfully", r.config.Namespace)
		}
	} else {
		t.Logf("Skipping cleanup (policy: %s). Test namespace: %s", r.config.CleanupPolicy, r.config.Namespace)
	}
}

// runScenarioBasedTests executes configuration-driven scenario tests
func (r *SSATestRunner) runScenarioBasedTests(t *testing.T) {
	if r.helper.TestScenarios == nil {
		t.Skip("No test scenarios loaded")
		return
	}

	t.Logf("Running %d scenario-based tests", len(r.helper.TestScenarios.Scenarios))

	for _, scenario := range r.helper.TestScenarios.Scenarios {
		scenario := scenario // Capture for closure

		t.Run(scenario.Name, func(t *testing.T) {
			// Skip if depends on another scenario that hasn't run
			if scenario.DependsOn != "" {
				t.Logf("Scenario depends on: %s", scenario.DependsOn)
			}

			// Execute scenario
			result, err := r.helper.ApplyScenario(r.ctx, scenario)

			if scenario.ExpectError {
				if err == nil {
					t.Errorf("Expected error for scenario %s, but got none", scenario.Name)
					return
				}
				t.Logf("Got expected error: %v", err)
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error in scenario %s: %v", scenario.Name, err)
			}

			if result == nil {
				t.Fatalf("Scenario %s returned nil result", scenario.Name)
			}

			t.Logf("Scenario %s completed successfully", scenario.Name)

			// Validate managedFields if applicable
			if obj, ok := result.(interface{ GetManagedFields() []interface{} }); ok {
				managedFields := obj.GetManagedFields()
				if len(managedFields) == 0 {
					t.Errorf("Expected managedFields to be set for scenario %s", scenario.Name)
				}
			}
		})
	}
}

// runPerformanceTests executes performance-focused tests
func (r *SSATestRunner) runPerformanceTests(t *testing.T) {
	if r.helper.TestScenarios == nil || len(r.helper.TestScenarios.PerformanceScenarios) == 0 {
		t.Skip("No performance scenarios configured")
		return
	}

	t.Logf("Running %d performance tests", len(r.helper.TestScenarios.PerformanceScenarios))

	for _, perfScenario := range r.helper.TestScenarios.PerformanceScenarios {
		perfScenario := perfScenario // Capture for closure

		t.Run(perfScenario.Name, func(t *testing.T) {
			start := time.Now()

			switch perfScenario.Name {
			case "large_service_apply":
				r.runLargeServiceTest(t, perfScenario)
			case "concurrent_applies":
				r.runConcurrentApplyTest(t, perfScenario)
			case "large_managed_fields":
				r.runLargeManagedFieldsTest(t, perfScenario)
			default:
				t.Skipf("Unknown performance scenario: %s", perfScenario.Name)
			}

			duration := time.Since(start)
			t.Logf("Performance test %s completed in %v", perfScenario.Name, duration)

			// Log performance metrics
			if duration > 30*time.Second {
				t.Logf("WARNING: Performance test took longer than expected: %v", duration)
			}
		})
	}
}

// runLargeServiceTest tests Server-Side Apply with large services
func (r *SSATestRunner) runLargeServiceTest(t *testing.T, scenario PerformanceScenario) {
	// Create a service with many ingress ports
	service := &netguardv1beta1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "large-service-test",
			Namespace: r.config.Namespace,
		},
		Spec: netguardv1beta1.ServiceSpec{
			Description:  "Large service for performance testing",
			IngressPorts: make([]netguardv1beta1.IngressPort, 100), // 100 ports
		},
	}

	// Generate 100 ingress ports
	for i := 0; i < 100; i++ {
		service.Spec.IngressPorts[i] = netguardv1beta1.IngressPort{
			Port:        fmt.Sprintf("%d", 8000+i),
			Protocol:    netguardv1beta1.ProtocolTCP,
			Description: fmt.Sprintf("Port %d for performance testing", 8000+i),
		}
	}

	testScenario := TestScenario{
		Name:         scenario.Name,
		ResourceType: "Service",
		FieldManager: scenario.FieldManager,
		Force:        false,
		Resource:     convertServiceToMap(service),
	}

	// Apply the large service
	result, err := r.helper.ApplyScenario(r.ctx, testScenario)
	if err != nil {
		t.Fatalf("Failed to apply large service: %v", err)
	}

	if appliedService, ok := result.(*netguardv1beta1.Service); ok {
		if len(appliedService.Spec.IngressPorts) != 100 {
			t.Errorf("Expected 100 ingress ports, got %d", len(appliedService.Spec.IngressPorts))
		}
		t.Logf("Large service applied successfully with %d ports", len(appliedService.Spec.IngressPorts))
	}
}

// runConcurrentApplyTest tests concurrent Server-Side Apply operations
func (r *SSATestRunner) runConcurrentApplyTest(t *testing.T, scenario PerformanceScenario) {
	concurrency := scenario.ConcurrentOperations
	if concurrency == 0 {
		concurrency = 10
	}

	t.Logf("Running %d concurrent apply operations", concurrency)

	// Use channels to coordinate concurrent operations
	errChan := make(chan error, concurrency)
	doneChan := make(chan bool, concurrency)

	// Launch concurrent operations
	for i := 0; i < concurrency; i++ {
		go func(index int) {
			defer func() { doneChan <- true }()

			serviceName := fmt.Sprintf("%s-%d", scenario.BaseResourceName, index)
			service := &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: r.config.Namespace,
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: fmt.Sprintf("Concurrent test service %d", index),
					IngressPorts: []netguardv1beta1.IngressPort{
						{
							Port:     "80",
							Protocol: netguardv1beta1.ProtocolTCP,
						},
					},
				},
			}

			testScenario := TestScenario{
				Name:         fmt.Sprintf("concurrent-%d", index),
				ResourceType: "Service",
				FieldManager: fmt.Sprintf("concurrent-manager-%d", index),
				Force:        false,
				Resource:     convertServiceToMap(service),
			}

			_, err := r.helper.ApplyScenario(r.ctx, testScenario)
			if err != nil {
				errChan <- fmt.Errorf("concurrent operation %d failed: %w", index, err)
			}
		}(i)
	}

	// Wait for all operations to complete
	completed := 0
	errors := 0

	for completed < concurrency {
		select {
		case <-doneChan:
			completed++
		case err := <-errChan:
			t.Errorf("Concurrent operation error: %v", err)
			errors++
		case <-time.After(60 * time.Second):
			t.Fatalf("Concurrent operations timed out after 60 seconds")
		}
	}

	t.Logf("Concurrent test completed: %d operations, %d errors", completed, errors)
}

// runLargeManagedFieldsTest tests with many field managers
func (r *SSATestRunner) runLargeManagedFieldsTest(t *testing.T, scenario PerformanceScenario) {
	managersCount := scenario.ManagersCount
	if managersCount == 0 {
		managersCount = 20
	}

	serviceName := "large-managed-fields-service"

	// Apply the service with multiple managers sequentially
	for i := 0; i < managersCount; i++ {
		service := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: r.config.Namespace,
				Labels: map[string]string{
					fmt.Sprintf("manager-%d", i): fmt.Sprintf("label-%d", i),
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: fmt.Sprintf("Service updated by manager-%d", i),
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     fmt.Sprintf("%d", 8000+i),
						Protocol: netguardv1beta1.ProtocolTCP,
					},
				},
			},
		}

		testScenario := TestScenario{
			Name:         fmt.Sprintf("manager-%d-update", i),
			ResourceType: "Service",
			FieldManager: fmt.Sprintf("perf-manager-%d", i),
			Force:        false, // Don't force, see if we get conflicts
			Resource:     convertServiceToMap(service),
		}

		result, err := r.helper.ApplyScenario(r.ctx, testScenario)
		if err != nil {
			t.Logf("Manager %d failed (expected): %v", i, err)
			continue
		}

		if i%10 == 0 {
			t.Logf("Applied update from manager %d", i)
		}

		// Check managedFields growth
		if appliedService, ok := result.(*netguardv1beta1.Service); ok {
			t.Logf("Service now has %d managedFields entries", len(appliedService.ManagedFields))
		}
	}

	// Get final service state
	finalService, err := r.helper.NetguardClient.NetguardV1beta1().Services(r.config.Namespace).Get(
		r.ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get final service: %v", err)
	}

	t.Logf("Final service has %d managedFields entries", len(finalService.ManagedFields))
}

// logTestStart logs test suite start
func (r *SSATestRunner) logTestStart(t *testing.T) {
	t.Logf("=== Starting Server-Side Apply E2E Test Suite ===")
	t.Logf("Test Config:")
	t.Logf("  Namespace: %s", r.config.Namespace)
	t.Logf("  Timeout: %v", r.config.TimeoutDuration)
	t.Logf("  Cleanup Policy: %s", r.config.CleanupPolicy)
	t.Logf("  Test Data Dir: %s", r.config.TestDataDir)
	t.Logf("=====================================================")
}

// logTestCompletion logs test suite completion
func (r *SSATestRunner) logTestCompletion(t *testing.T) {
	duration := time.Since(r.startTime)
	t.Logf("=== Server-Side Apply E2E Test Suite Completed ===")
	t.Logf("Total Duration: %v", duration)
	t.Logf("==================================================")
}

// convertServiceToMap converts a service to map[string]interface{} for scenarios
func convertServiceToMap(service *netguardv1beta1.Service) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "netguard.sgroups.io/v1beta1",
		"kind":       "Service",
		"metadata": map[string]interface{}{
			"name":      service.Name,
			"namespace": service.Namespace,
			"labels":    service.Labels,
		},
		"spec": map[string]interface{}{
			"description":  service.Spec.Description,
			"ingressPorts": service.Spec.IngressPorts,
		},
	}
}

// RunCompleteSSATestSuite is the main entry point for running all tests
func RunCompleteSSATestSuite(t *testing.T) {
	runner, err := NewSSATestRunner()
	if err != nil {
		t.Fatalf("Failed to create test runner: %v", err)
	}

	runner.RunAllTests(t)
}
