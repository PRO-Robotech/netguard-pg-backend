package e2e

import (
	"fmt"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// TestCompleteSSATestSuite is the main entry point for running all Server-Side Apply E2E tests
func TestCompleteSSATestSuite(t *testing.T) {
	// Skip E2E tests in short mode
	if testing.Short() {
		t.Skip("Skipping Server-Side Apply E2E tests in short mode")
	}

	// Check if E2E tests are explicitly disabled
	if os.Getenv("SKIP_E2E_TESTS") == "1" {
		t.Skip("E2E tests disabled via SKIP_E2E_TESTS=1")
	}

	// Use the comprehensive test runner
	RunCompleteSSATestSuite(t)
}

// TestServerSideApplyE2EBasic runs just the basic E2E tests (faster subset)
func TestServerSideApplyE2EBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E tests in short mode")
	}

	// Run only the core Server-Side Apply tests
	RunServerSideApplyE2ETests(t)
}

// TestSSAConflictResolutionOnly runs only conflict resolution tests
func TestSSAConflictResolutionOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E tests in short mode")
	}

	// Run only conflict resolution tests
	RunSSAConflictResolutionTests(t)
}

// TestSSAPerformanceOnly runs only performance tests
func TestSSAPerformanceOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	if os.Getenv("RUN_PERFORMANCE_TESTS") != "1" {
		t.Skip("Performance tests disabled. Set RUN_PERFORMANCE_TESTS=1 to enable.")
	}

	// Create runner and run only performance tests
	runner, err := NewSSATestRunner()
	if err != nil {
		t.Fatalf("Failed to create test runner: %v", err)
	}

	// Setup environment
	if err := runner.setupTestEnvironment(t); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}
	defer runner.cleanupTestEnvironment(t)

	// Run performance tests only
	runner.runPerformanceTests(t)
}

// Benchmark functions for performance measurement

func BenchmarkSSAServiceCreate(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmarks in short mode")
	}

	runner, err := NewSSATestRunner()
	if err != nil {
		b.Fatalf("Failed to create test runner: %v", err)
	}

	// Setup environment
	if err := runner.setupTestEnvironment(&testing.T{}); err != nil {
		b.Fatalf("Failed to setup test environment: %v", err)
	}
	defer runner.cleanupTestEnvironment(&testing.T{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		serviceName := fmt.Sprintf("benchmark-service-%d", i)

		service := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: runner.config.Namespace,
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Benchmark service for performance testing",
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     "80",
						Protocol: netguardv1beta1.ProtocolTCP,
					},
				},
			},
		}

		scenario := TestScenario{
			Name:         fmt.Sprintf("benchmark-%d", i),
			ResourceType: "Service",
			FieldManager: fmt.Sprintf("benchmark-manager-%d", i),
			Force:        false,
			Resource:     convertServiceToMap(service),
		}

		_, err := runner.helper.ApplyScenario(runner.ctx, scenario)
		if err != nil {
			b.Fatalf("Benchmark iteration %d failed: %v", i, err)
		}
	}
}

func BenchmarkSSAServiceUpdate(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmarks in short mode")
	}

	runner, err := NewSSATestRunner()
	if err != nil {
		b.Fatalf("Failed to create test runner: %v", err)
	}

	// Setup environment
	if err := runner.setupTestEnvironment(&testing.T{}); err != nil {
		b.Fatalf("Failed to setup test environment: %v", err)
	}
	defer runner.cleanupTestEnvironment(&testing.T{})

	// Create initial service
	serviceName := "benchmark-update-service"
	initialService := &netguardv1beta1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: runner.config.Namespace,
		},
		Spec: netguardv1beta1.ServiceSpec{
			Description: "Initial benchmark service",
			IngressPorts: []netguardv1beta1.IngressPort{
				{
					Port:     "80",
					Protocol: netguardv1beta1.ProtocolTCP,
				},
			},
		},
	}

	initialScenario := TestScenario{
		Name:         "initial-service",
		ResourceType: "Service",
		FieldManager: "benchmark-manager",
		Force:        false,
		Resource:     convertServiceToMap(initialService),
	}

	_, err = runner.helper.ApplyScenario(runner.ctx, initialScenario)
	if err != nil {
		b.Fatalf("Failed to create initial service: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Update service with additional port
		updatedService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: runner.config.Namespace,
				Labels: map[string]string{
					"iteration": fmt.Sprintf("%d", i),
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: fmt.Sprintf("Updated benchmark service - iteration %d", i),
				IngressPorts: []netguardv1beta1.IngressPort{
					{
						Port:     "80",
						Protocol: netguardv1beta1.ProtocolTCP,
					},
					{
						Port:        fmt.Sprintf("%d", 8000+i),
						Protocol:    netguardv1beta1.ProtocolTCP,
						Description: fmt.Sprintf("Dynamic port %d", i),
					},
				},
			},
		}

		updateScenario := TestScenario{
			Name:         fmt.Sprintf("update-%d", i),
			ResourceType: "Service",
			FieldManager: "benchmark-manager",
			Force:        false,
			Resource:     convertServiceToMap(updatedService),
		}

		_, err := runner.helper.ApplyScenario(runner.ctx, updateScenario)
		if err != nil {
			b.Fatalf("Benchmark update iteration %d failed: %v", i, err)
		}
	}
}
