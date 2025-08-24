package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/pkg/k8s/clientset/versioned"
)

// TestConfig holds configuration for E2E tests
type TestConfig struct {
	Namespace       string
	Kubeconfig      string
	APIServerURL    string
	TestDataDir     string
	CleanupPolicy   string
	TimeoutDuration time.Duration
}

// TestScenario represents a single test scenario
type TestScenario struct {
	Name         string                 `yaml:"name"`
	Description  string                 `yaml:"description"`
	ResourceType string                 `yaml:"resource_type"`
	FieldManager string                 `yaml:"field_manager"`
	Force        bool                   `yaml:"force"`
	DependsOn    string                 `yaml:"depends_on,omitempty"`
	ExpectError  bool                   `yaml:"expect_error,omitempty"`
	Resource     map[string]interface{} `yaml:"resource"`
}

// TestValidation represents validation checks
type TestValidation struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Checks      []ValidationCheck `yaml:"checks"`
}

// ValidationCheck represents individual validation check
type ValidationCheck struct {
	Field       string      `yaml:"field,omitempty"`
	Required    bool        `yaml:"required,omitempty"`
	Type        string      `yaml:"type,omitempty"`
	Value       interface{} `yaml:"value,omitempty"`
	Enum        []string    `yaml:"enum,omitempty"`
	MinLength   int         `yaml:"min_length,omitempty"`
	Description string      `yaml:"description,omitempty"`
}

// TestScenariosConfig holds the complete test configuration
type TestScenariosConfig struct {
	Scenarios            []TestScenario       `yaml:"scenarios"`
	TestValidations      []TestValidation     `yaml:"test_validations"`
	CleanupPolicies      CleanupPolicies      `yaml:"cleanup_policies"`
	ErrorScenarios       ErrorScenarios       `yaml:"error_scenarios"`
	PerformanceScenarios PerformanceScenarios `yaml:"performance_scenarios"`
}

// CleanupPolicies defines cleanup behavior
type CleanupPolicies struct {
	AfterEachTest      bool     `yaml:"after_each_test"`
	AfterSuite         bool     `yaml:"after_suite"`
	ResourcesToCleanup []string `yaml:"resources_to_cleanup"`
}

// ErrorScenarios defines error testing scenarios
type ErrorScenarios struct {
	InvalidFieldManagerName InvalidManagerTest `yaml:"invalid_field_manager_name"`
	MalformedApplyPatches   MalformedPatchTest `yaml:"malformed_apply_patches"`
}

type InvalidManagerTest struct {
	Name            string   `yaml:"name"`
	Description     string   `yaml:"description"`
	InvalidManagers []string `yaml:"invalid_managers"`
}

type MalformedPatchTest struct {
	Name           string   `yaml:"name"`
	Description    string   `yaml:"description"`
	InvalidPatches []string `yaml:"invalid_patches"`
}

// PerformanceScenarios defines performance testing scenarios
type PerformanceScenarios []PerformanceScenario

type PerformanceScenario struct {
	Name                 string `yaml:"name"`
	Description          string `yaml:"description"`
	ResourceType         string `yaml:"resource_type,omitempty"`
	FieldManager         string `yaml:"field_manager,omitempty"`
	ConcurrentOperations int    `yaml:"concurrent_operations,omitempty"`
	BaseResourceName     string `yaml:"base_resource_name,omitempty"`
	ManagersCount        int    `yaml:"managers_count,omitempty"`
}

// E2ETestHelper provides utilities for E2E testing
type E2ETestHelper struct {
	Config           *TestConfig
	NetguardClient   versioned.Interface
	KubernetesClient kubernetes.Interface
	RestConfig       *rest.Config
	TestScenarios    *TestScenariosConfig
}

// NewE2ETestHelper creates a new E2E test helper
func NewE2ETestHelper(config *TestConfig) (*E2ETestHelper, error) {
	helper := &E2ETestHelper{
		Config: config,
	}

	// Initialize Kubernetes clients
	if err := helper.initializeClients(); err != nil {
		return nil, fmt.Errorf("failed to initialize clients: %w", err)
	}

	// Load test scenarios
	if err := helper.loadTestScenarios(); err != nil {
		return nil, fmt.Errorf("failed to load test scenarios: %w", err)
	}

	return helper, nil
}

// initializeClients initializes Kubernetes and NetGuard clients
func (h *E2ETestHelper) initializeClients() error {
	var config *rest.Config
	var err error

	if h.Config.Kubeconfig != "" {
		// Load from kubeconfig file
		config, err = clientcmd.BuildConfigFromFlags(h.Config.APIServerURL, h.Config.Kubeconfig)
	} else {
		// Try in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			// Fallback to default kubeconfig location
			kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
			config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	h.RestConfig = config

	// Create Kubernetes client
	h.KubernetesClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create NetGuard client
	h.NetguardClient, err = versioned.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create netguard client: %w", err)
	}

	return nil
}

// loadTestScenarios loads test scenarios from YAML file
func (h *E2ETestHelper) loadTestScenarios() error {
	scenariosFile := filepath.Join(h.Config.TestDataDir, "ssa_test_scenarios.yaml")

	data, err := os.ReadFile(scenariosFile)
	if err != nil {
		return fmt.Errorf("failed to read scenarios file %s: %w", scenariosFile, err)
	}

	h.TestScenarios = &TestScenariosConfig{}
	if err := yaml.Unmarshal(data, h.TestScenarios); err != nil {
		return fmt.Errorf("failed to unmarshal scenarios: %w", err)
	}

	return nil
}

// EnsureNamespace creates the test namespace if it doesn't exist
func (h *E2ETestHelper) EnsureNamespace(ctx context.Context) error {
	namespace := &metav1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: h.Config.Namespace,
			Labels: map[string]string{
				"e2e-test": "server-side-apply",
				"cleanup":  "auto",
			},
		},
	}

	_, err := h.KubernetesClient.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace %s: %w", h.Config.Namespace, err)
	}

	return nil
}

// CleanupNamespace removes the test namespace and all resources
func (h *E2ETestHelper) CleanupNamespace(ctx context.Context) error {
	err := h.KubernetesClient.CoreV1().Namespaces().Delete(ctx, h.Config.Namespace, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete namespace %s: %w", h.Config.Namespace, err)
	}

	// Wait for namespace to be fully deleted
	return wait.PollImmediate(time.Second, h.Config.TimeoutDuration, func() (bool, error) {
		_, err := h.KubernetesClient.CoreV1().Namespaces().Get(ctx, h.Config.Namespace, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

// CleanupServices removes all test services
func (h *E2ETestHelper) CleanupServices(ctx context.Context) error {
	services, err := h.NetguardClient.NetguardV1beta1().Services(h.Config.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	for _, service := range services.Items {
		err := h.NetguardClient.NetguardV1beta1().Services(h.Config.Namespace).Delete(ctx, service.Name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete service %s: %w", service.Name, err)
		}
	}

	return nil
}

// CleanupAddressGroups removes all test address groups
func (h *E2ETestHelper) CleanupAddressGroups(ctx context.Context) error {
	addressGroups, err := h.NetguardClient.NetguardV1beta1().AddressGroups(h.Config.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list address groups: %w", err)
	}

	for _, ag := range addressGroups.Items {
		err := h.NetguardClient.NetguardV1beta1().AddressGroups(h.Config.Namespace).Delete(ctx, ag.Name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete address group %s: %w", ag.Name, err)
		}
	}

	return nil
}

// ApplyScenario executes a test scenario
func (h *E2ETestHelper) ApplyScenario(ctx context.Context, scenario TestScenario) (runtime.Object, error) {
	// Convert scenario resource to typed object
	resourceData, err := json.Marshal(scenario.Resource)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal scenario resource: %w", err)
	}

	switch strings.ToLower(scenario.ResourceType) {
	case "service":
		return h.applyServiceScenario(ctx, scenario, resourceData)
	case "addressgroup":
		return h.applyAddressGroupScenario(ctx, scenario, resourceData)
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", scenario.ResourceType)
	}
}

// applyServiceScenario applies a Service scenario
func (h *E2ETestHelper) applyServiceScenario(ctx context.Context, scenario TestScenario, resourceData []byte) (*netguardv1beta1.Service, error) {
	var service netguardv1beta1.Service
	if err := json.Unmarshal(resourceData, &service); err != nil {
		return nil, fmt.Errorf("failed to unmarshal service: %w", err)
	}

	// Set namespace if not specified
	if service.Namespace == "" {
		service.Namespace = h.Config.Namespace
	}

	patchOptions := metav1.PatchOptions{
		FieldManager: scenario.FieldManager,
		Force:        &scenario.Force,
	}

	return h.NetguardClient.NetguardV1beta1().Services(service.Namespace).Patch(
		ctx, service.Name, metav1.PatchType("application/apply-patch+yaml"), resourceData, patchOptions)
}

// applyAddressGroupScenario applies an AddressGroup scenario
func (h *E2ETestHelper) applyAddressGroupScenario(ctx context.Context, scenario TestScenario, resourceData []byte) (*netguardv1beta1.AddressGroup, error) {
	var addressGroup netguardv1beta1.AddressGroup
	if err := json.Unmarshal(resourceData, &addressGroup); err != nil {
		return nil, fmt.Errorf("failed to unmarshal address group: %w", err)
	}

	// Set namespace if not specified
	if addressGroup.Namespace == "" {
		addressGroup.Namespace = h.Config.Namespace
	}

	patchOptions := metav1.PatchOptions{
		FieldManager: scenario.FieldManager,
		Force:        &scenario.Force,
	}

	return h.NetguardClient.NetguardV1beta1().AddressGroups(addressGroup.Namespace).Patch(
		ctx, addressGroup.Name, metav1.PatchType("application/apply-patch+yaml"), resourceData, patchOptions)
}

// ValidateManagedFields validates managedFields structure
func (h *E2ETestHelper) ValidateManagedFields(obj metav1.Object, validation TestValidation) error {
	managedFields := obj.GetManagedFields()
	if managedFields == nil {
		return fmt.Errorf("managedFields is nil")
	}

	if len(managedFields) == 0 {
		return fmt.Errorf("managedFields is empty")
	}

	for i, field := range managedFields {
		// Validate required fields
		if field.Manager == "" {
			return fmt.Errorf("managedFields[%d].manager is empty", i)
		}

		if field.Operation == "" {
			return fmt.Errorf("managedFields[%d].operation is empty", i)
		}

		if field.APIVersion == "" {
			return fmt.Errorf("managedFields[%d].apiVersion is empty", i)
		}

		if field.Time == nil {
			return fmt.Errorf("managedFields[%d].time is nil", i)
		}

		if field.FieldsType != "FieldsV1" {
			return fmt.Errorf("managedFields[%d].fieldsType expected 'FieldsV1', got '%s'", i, field.FieldsType)
		}

		if field.FieldsV1 == nil {
			return fmt.Errorf("managedFields[%d].fieldsV1 is nil", i)
		}

		if len(field.FieldsV1.Raw) == 0 {
			return fmt.Errorf("managedFields[%d].fieldsV1.raw is empty", i)
		}

		// Validate FieldsV1.Raw is valid JSON
		var fieldsMap map[string]interface{}
		if err := json.Unmarshal(field.FieldsV1.Raw, &fieldsMap); err != nil {
			return fmt.Errorf("managedFields[%d].fieldsV1.raw is not valid JSON: %w", i, err)
		}
	}

	return nil
}

// ValidateResourceConsistency validates that resource data is consistent
func (h *E2ETestHelper) ValidateResourceConsistency(original, retrieved runtime.Object) error {
	// Convert both objects to JSON for comparison
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return fmt.Errorf("failed to marshal original object: %w", err)
	}

	retrievedJSON, err := json.Marshal(retrieved)
	if err != nil {
		return fmt.Errorf("failed to marshal retrieved object: %w", err)
	}

	var originalMap, retrievedMap map[string]interface{}

	if err := json.Unmarshal(originalJSON, &originalMap); err != nil {
		return fmt.Errorf("failed to unmarshal original JSON: %w", err)
	}

	if err := json.Unmarshal(retrievedJSON, &retrievedMap); err != nil {
		return fmt.Errorf("failed to unmarshal retrieved JSON: %w", err)
	}

	// Compare spec fields (ignoring metadata differences like resourceVersion)
	return h.compareSpecs(originalMap, retrievedMap)
}

// compareSpecs compares the spec portions of two objects
func (h *E2ETestHelper) compareSpecs(original, retrieved map[string]interface{}) error {
	originalSpec, originalOk := original["spec"].(map[string]interface{})
	retrievedSpec, retrievedOk := retrieved["spec"].(map[string]interface{})

	if originalOk != retrievedOk {
		return fmt.Errorf("spec presence mismatch: original has spec: %t, retrieved has spec: %t", originalOk, retrievedOk)
	}

	if !originalOk {
		return nil // Both don't have spec, which is fine
	}

	return h.deepCompare("spec", originalSpec, retrievedSpec)
}

// deepCompare performs deep comparison of two maps
func (h *E2ETestHelper) deepCompare(path string, original, retrieved interface{}) error {
	switch originalVal := original.(type) {
	case map[string]interface{}:
		retrievedMap, ok := retrieved.(map[string]interface{})
		if !ok {
			return fmt.Errorf("type mismatch at %s: original is map, retrieved is %T", path, retrieved)
		}

		for key, originalValue := range originalVal {
			retrievedValue, exists := retrievedMap[key]
			if !exists {
				return fmt.Errorf("missing key at %s.%s", path, key)
			}

			if err := h.deepCompare(fmt.Sprintf("%s.%s", path, key), originalValue, retrievedValue); err != nil {
				return err
			}
		}

	case []interface{}:
		retrievedSlice, ok := retrieved.([]interface{})
		if !ok {
			return fmt.Errorf("type mismatch at %s: original is slice, retrieved is %T", path, retrieved)
		}

		if len(originalVal) != len(retrievedSlice) {
			return fmt.Errorf("slice length mismatch at %s: original %d, retrieved %d", path, len(originalVal), len(retrievedSlice))
		}

		for i, originalItem := range originalVal {
			if err := h.deepCompare(fmt.Sprintf("%s[%d]", path, i), originalItem, retrievedSlice[i]); err != nil {
				return err
			}
		}

	case string, int, int64, float64, bool:
		if originalVal != retrieved {
			return fmt.Errorf("value mismatch at %s: original %v, retrieved %v", path, originalVal, retrieved)
		}

	default:
		// For other types, convert to string and compare
		originalStr := fmt.Sprintf("%v", originalVal)
		retrievedStr := fmt.Sprintf("%v", retrieved)
		if originalStr != retrievedStr {
			return fmt.Errorf("value mismatch at %s: original %v, retrieved %v", path, originalVal, retrieved)
		}
	}

	return nil
}

// WaitForResource waits for a resource to reach expected state
func (h *E2ETestHelper) WaitForResource(ctx context.Context, resourceType, namespace, name string, checkFn func(obj runtime.Object) (bool, error)) error {
	return wait.PollImmediate(time.Second, h.Config.TimeoutDuration, func() (bool, error) {
		var obj runtime.Object
		var err error

		switch strings.ToLower(resourceType) {
		case "service":
			obj, err = h.NetguardClient.NetguardV1beta1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		case "addressgroup":
			obj, err = h.NetguardClient.NetguardV1beta1().AddressGroups(namespace).Get(ctx, name, metav1.GetOptions{})
		default:
			return false, fmt.Errorf("unsupported resource type: %s", resourceType)
		}

		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil // Keep waiting
			}
			return false, err
		}

		return checkFn(obj)
	})
}

// GetDefaultConfig returns default E2E test configuration
func GetDefaultConfig() *TestConfig {
	return &TestConfig{
		Namespace:       "netguard-e2e-test",
		TestDataDir:     "test/e2e",
		CleanupPolicy:   "auto",
		TimeoutDuration: 30 * time.Second,
	}
}
