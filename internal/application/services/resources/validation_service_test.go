package resources

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/services/resources/testutil"
	"netguard-pg-backend/internal/domain/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationService_ValidateServiceForCreation(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		service     models.Service
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid service creation with no references",
			setupData:   map[string]interface{}{},
			service:     testutil.CreateTestService("test-service", "test-namespace"),
			expectError: false,
		},
		{
			name: "valid service creation with existing address group references",
			setupData: map[string]interface{}{
				"addressgroup_test-namespace/test-ag": func() *models.AddressGroup {
					ag := testutil.CreateTestAddressGroup("test-ag", "test-namespace")
					return &ag
				}(),
			},
			service: func() models.Service {
				svc := testutil.CreateTestService("test-service", "test-namespace")
				svc.AddressGroups = []models.AddressGroupRef{
					models.NewAddressGroupRef("test-ag", models.WithNamespace("test-namespace")),
				}
				return svc
			}(),
			expectError: false,
		},
		{
			name:      "service creation with invalid address group reference should fail",
			setupData: map[string]interface{}{},
			service: func() models.Service {
				svc := testutil.CreateTestService("test-service", "test-namespace")
				svc.AddressGroups = []models.AddressGroupRef{
					models.NewAddressGroupRef("non-existent-ag", models.WithNamespace("test-namespace")),
				}
				return svc
			}(),
			expectError: true,
			errorMsg:    "invalid address group reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateServiceForCreation(context.Background(), tt.service)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateServiceForUpdate(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		oldService  models.Service
		newService  models.Service
		expectError bool
	}{
		{
			name: "valid service update",
			setupData: map[string]interface{}{
				"service_test-namespace/test-service": func() *models.Service {
					svc := testutil.CreateTestService("test-service", "test-namespace")
					return &svc
				}(),
			},
			oldService:  testutil.CreateTestService("test-service", "test-namespace"),
			newService:  testutil.CreateTestService("test-service", "test-namespace"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateServiceForUpdate(context.Background(), tt.oldService, tt.newService)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateServiceForDeletion(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		service     models.Service
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid service deletion - service exists",
			setupData: map[string]interface{}{
				"service_test-namespace/test-service": func() *models.Service {
					svc := testutil.CreateTestService("test-service", "test-namespace")
					return &svc
				}(),
			},
			service:     testutil.CreateTestService("test-service", "test-namespace"),
			expectError: false,
		},
		{
			name:        "service deletion - service doesn't exist",
			setupData:   map[string]interface{}{},
			service:     testutil.CreateTestService("non-existent-service", "test-namespace"),
			expectError: true,
			errorMsg:    "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateServiceForDeletion(context.Background(), tt.service)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateAddressGroupForCreation(t *testing.T) {
	tests := []struct {
		name         string
		setupData    map[string]interface{}
		addressGroup models.AddressGroup
		expectError  bool
		errorMsg     string
	}{
		{
			name:         "valid address group creation",
			setupData:    map[string]interface{}{},
			addressGroup: testutil.CreateTestAddressGroup("test-ag", "test-namespace"),
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateAddressGroupForCreation(context.Background(), tt.addressGroup)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateAddressGroupForUpdate(t *testing.T) {
	tests := []struct {
		name            string
		setupData       map[string]interface{}
		oldAddressGroup models.AddressGroup
		newAddressGroup models.AddressGroup
		expectError     bool
	}{
		{
			name: "valid address group update",
			setupData: map[string]interface{}{
				"addressgroup_test-namespace/test-ag": func() *models.AddressGroup {
					ag := testutil.CreateTestAddressGroup("test-ag", "test-namespace")
					return &ag
				}(),
			},
			oldAddressGroup: testutil.CreateTestAddressGroup("test-ag", "test-namespace"),
			newAddressGroup: testutil.CreateTestAddressGroup("test-ag", "test-namespace"),
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateAddressGroupForUpdate(context.Background(), tt.oldAddressGroup, tt.newAddressGroup)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateAddressGroupForDeletion(t *testing.T) {
	tests := []struct {
		name         string
		setupData    map[string]interface{}
		addressGroup models.AddressGroup
		expectError  bool
		errorMsg     string
	}{
		{
			name: "valid address group deletion - address group exists",
			setupData: map[string]interface{}{
				"addressgroup_test-namespace/test-ag": func() *models.AddressGroup {
					ag := testutil.CreateTestAddressGroup("test-ag", "test-namespace")
					return &ag
				}(),
			},
			addressGroup: testutil.CreateTestAddressGroup("test-ag", "test-namespace"),
			expectError:  false,
		},
		{
			name:         "address group deletion - address group doesn't exist",
			setupData:    map[string]interface{}{},
			addressGroup: testutil.CreateTestAddressGroup("non-existent-ag", "test-namespace"),
			expectError:  true,
			errorMsg:     "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateAddressGroupForDeletion(context.Background(), tt.addressGroup)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateRuleS2SForCreation(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		rule        models.RuleS2S
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid RuleS2S creation with dependencies",
			setupData: map[string]interface{}{
				// Add required services and service aliases for validation
				"service_test-namespace/test-local-service": func() *models.Service {
					svc := testutil.CreateTestService("test-local-service", "test-namespace")
					return &svc
				}(),
				"service_test-namespace/test-service": func() *models.Service {
					svc := testutil.CreateTestService("test-service", "test-namespace")
					return &svc
				}(),
				"servicealias_test-namespace/test-local-service": func() *models.ServiceAlias {
					alias := testutil.CreateTestServiceAlias("test-local-service", "test-namespace", "test-local-service")
					return &alias
				}(),
				"servicealias_test-namespace/test-service": func() *models.ServiceAlias {
					alias := testutil.CreateTestServiceAlias("test-service", "test-namespace", "test-service")
					return &alias
				}(),
			},
			rule:        testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace"),
			expectError: false,
		},
		{
			name:        "RuleS2S creation without dependencies should fail",
			setupData:   map[string]interface{}{},
			rule:        testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace"),
			expectError: true,
			errorMsg:    "validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateRuleS2SForCreation(context.Background(), tt.rule)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateRuleS2SForUpdate(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		oldRule     models.RuleS2S
		newRule     models.RuleS2S
		expectError bool
	}{
		{
			name: "valid RuleS2S update with dependencies",
			setupData: map[string]interface{}{
				"service_test-namespace/test-local-service": func() *models.Service {
					svc := testutil.CreateTestService("test-local-service", "test-namespace")
					return &svc
				}(),
				"service_test-namespace/test-service": func() *models.Service {
					svc := testutil.CreateTestService("test-service", "test-namespace")
					return &svc
				}(),
				"servicealias_test-namespace/test-local-service": func() *models.ServiceAlias {
					alias := testutil.CreateTestServiceAlias("test-local-service", "test-namespace", "test-local-service")
					return &alias
				}(),
				"servicealias_test-namespace/test-service": func() *models.ServiceAlias {
					alias := testutil.CreateTestServiceAlias("test-service", "test-namespace", "test-service")
					return &alias
				}(),
			},
			oldRule:     testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace"),
			newRule:     testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateRuleS2SForUpdate(context.Background(), tt.oldRule, tt.newRule)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateRuleS2SForDeletion(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		rule        models.RuleS2S
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid RuleS2S deletion - rule exists",
			setupData: map[string]interface{}{
				"rules2s_test-namespace/test-rule-s2s": func() *models.RuleS2S {
					rule := testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")
					return &rule
				}(),
			},
			rule:        testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace"),
			expectError: false,
		},
		{
			name:        "RuleS2S deletion - rule doesn't exist",
			setupData:   map[string]interface{}{},
			rule:        testutil.CreateTestRuleS2S("non-existent-rule", "test-namespace"),
			expectError: true,
			errorMsg:    "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateRuleS2SForDeletion(context.Background(), tt.rule)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateServiceAliasForCreation(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		alias       models.ServiceAlias
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid service alias creation with service dependency",
			setupData: map[string]interface{}{
				"service_test-namespace/test-service": func() *models.Service {
					svc := testutil.CreateTestService("test-service", "test-namespace")
					return &svc
				}(),
			},
			alias:       testutil.CreateTestServiceAlias("test-alias", "test-namespace", "test-service"),
			expectError: false,
		},
		{
			name:        "service alias creation without service dependency should fail",
			setupData:   map[string]interface{}{},
			alias:       testutil.CreateTestServiceAlias("test-alias", "test-namespace", "non-existent-service"),
			expectError: true,
			errorMsg:    "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateServiceAliasForCreation(context.Background(), tt.alias)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateNetworkForCreation(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		network     models.Network
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid network creation",
			setupData:   map[string]interface{}{},
			network:     testutil.CreateTestNetwork("test-network", "test-namespace", "10.0.0.0/24"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateNetworkForCreation(context.Background(), tt.network)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateNetworkBindingForCreation(t *testing.T) {
	tests := []struct {
		name           string
		setupData      map[string]interface{}
		networkBinding models.NetworkBinding
		expectError    bool
		errorMsg       string
	}{
		{
			name: "valid network binding creation with dependencies",
			setupData: map[string]interface{}{
				"network_test-namespace/test-network": func() *models.Network {
					network := testutil.CreateTestNetwork("test-network", "test-namespace", "10.0.0.0/24")
					return &network
				}(),
				"addressgroup_test-namespace/test-ag": func() *models.AddressGroup {
					ag := testutil.CreateTestAddressGroup("test-ag", "test-namespace")
					return &ag
				}(),
			},
			networkBinding: testutil.CreateTestNetworkBinding("test-binding", "test-namespace", "test-network", "test-ag"),
			expectError:    false,
		},
		{
			name:           "network binding creation without dependencies should fail",
			setupData:      map[string]interface{}{},
			networkBinding: testutil.CreateTestNetworkBinding("test-binding", "test-namespace", "non-existent-network", "non-existent-ag"),
			expectError:    true,
			errorMsg:       "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateNetworkBindingForCreation(context.Background(), tt.networkBinding)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateMultipleResourcesForOperation(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		resources   []interface{}
		operation   string
		expectError bool
		errorMsg    string
	}{
		{
			name:      "valid multiple service creation",
			setupData: map[string]interface{}{},
			resources: []interface{}{
				testutil.CreateTestService("service1", "test-namespace"),
				testutil.CreateTestService("service2", "test-namespace"),
			},
			operation:   "create",
			expectError: false,
		},
		{
			name:      "bulk update should fail - requires old version",
			setupData: map[string]interface{}{},
			resources: []interface{}{
				testutil.CreateTestService("service1", "test-namespace"),
			},
			operation:   "update",
			expectError: true,
			errorMsg:    "bulk update validation requires old version",
		},
		{
			name: "valid multiple service deletion",
			setupData: map[string]interface{}{
				"service_test-namespace/service1": func() *models.Service {
					svc := testutil.CreateTestService("service1", "test-namespace")
					return &svc
				}(),
				"service_test-namespace/service2": func() *models.Service {
					svc := testutil.CreateTestService("service2", "test-namespace")
					return &svc
				}(),
			},
			resources: []interface{}{
				testutil.CreateTestService("service1", "test-namespace"),
				testutil.CreateTestService("service2", "test-namespace"),
			},
			operation:   "delete",
			expectError: false,
		},
		{
			name:      "unsupported resource type should fail",
			setupData: map[string]interface{}{},
			resources: []interface{}{
				"unsupported resource type",
			},
			operation:   "create",
			expectError: true,
			errorMsg:    "unsupported resource type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateMultipleResourcesForOperation(context.Background(), tt.resources, tt.operation)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ValidateWithReader(t *testing.T) {
	t.Run("validate service creation with existing reader", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		validationService := NewValidationService(mockRegistry)

		reader, err := mockRegistry.Reader(context.Background())
		require.NoError(t, err)
		defer reader.Close()

		service := testutil.CreateTestService("test-service", "test-namespace")

		// Execute
		err = validationService.ValidateServiceForCreationWithReader(context.Background(), service, reader)

		// Assert
		assert.NoError(t, err)
	})

	t.Run("validate address group creation with existing reader", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		validationService := NewValidationService(mockRegistry)

		reader, err := mockRegistry.Reader(context.Background())
		require.NoError(t, err)
		defer reader.Close()

		addressGroup := testutil.CreateTestAddressGroup("test-ag", "test-namespace")

		// Execute
		err = validationService.ValidateAddressGroupForCreationWithReader(context.Background(), addressGroup, reader)

		// Assert
		assert.NoError(t, err)
	})

	t.Run("validate RuleS2S creation with existing reader", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		mockRegistry.SetupTestData(map[string]interface{}{
			"service_test-namespace/test-local-service": func() *models.Service {
				svc := testutil.CreateTestService("test-local-service", "test-namespace")
				return &svc
			}(),
			"service_test-namespace/test-service": func() *models.Service {
				svc := testutil.CreateTestService("test-service", "test-namespace")
				return &svc
			}(),
			"servicealias_test-namespace/test-local-service": func() *models.ServiceAlias {
				alias := testutil.CreateTestServiceAlias("test-local-service", "test-namespace", "test-local-service")
				return &alias
			}(),
			"servicealias_test-namespace/test-service": func() *models.ServiceAlias {
				alias := testutil.CreateTestServiceAlias("test-service", "test-namespace", "test-service")
				return &alias
			}(),
		})

		validationService := NewValidationService(mockRegistry)

		reader, err := mockRegistry.Reader(context.Background())
		require.NoError(t, err)
		defer reader.Close()

		rule := testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")

		// Execute
		err = validationService.ValidateRuleS2SForCreationWithReader(context.Background(), rule, reader)

		// Assert
		assert.NoError(t, err)
	})
}

func TestValidationService_ValidateResourceDependencies(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		resource    interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid address group binding dependencies",
			setupData: map[string]interface{}{
				"service_test-namespace/test-service": func() *models.Service {
					svc := testutil.CreateTestService("test-service", "test-namespace")
					return &svc
				}(),
				"addressgroup_test-namespace/test-ag": func() *models.AddressGroup {
					ag := testutil.CreateTestAddressGroup("test-ag", "test-namespace")
					return &ag
				}(),
			},
			resource: models.AddressGroupBinding{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-binding",
						Namespace: "test-namespace",
					},
				},
				ServiceRef:      models.NewServiceRef("test-service", models.WithNamespace("test-namespace")),
				AddressGroupRef: models.NewAddressGroupRef("test-ag", models.WithNamespace("test-namespace")),
			},
			expectError: false,
		},
		{
			name:      "invalid address group binding - missing service",
			setupData: map[string]interface{}{},
			resource: models.AddressGroupBinding{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-binding",
						Namespace: "test-namespace",
					},
				},
				ServiceRef:      models.NewServiceRef("non-existent-service", models.WithNamespace("test-namespace")),
				AddressGroupRef: models.NewAddressGroupRef("test-ag", models.WithNamespace("test-namespace")),
			},
			expectError: true,
			errorMsg:    "service dependency validation failed",
		},
		{
			name: "valid RuleS2S dependencies",
			setupData: map[string]interface{}{
				"servicealias_test-namespace/test-local-service": func() *models.ServiceAlias {
					alias := testutil.CreateTestServiceAlias("test-local-service", "test-namespace", "test-local-service")
					return &alias
				}(),
				"servicealias_test-namespace/test-service": func() *models.ServiceAlias {
					alias := testutil.CreateTestServiceAlias("test-service", "test-namespace", "test-service")
					return &alias
				}(),
			},
			resource:    testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace"),
			expectError: false,
		},
		{
			name:        "invalid RuleS2S - missing service aliases",
			setupData:   map[string]interface{}{},
			resource:    testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace"),
			expectError: true,
			errorMsg:    "service alias dependency validation failed",
		},
		{
			name: "valid service alias dependency",
			setupData: map[string]interface{}{
				"service_test-namespace/test-service": func() *models.Service {
					svc := testutil.CreateTestService("test-service", "test-namespace")
					return &svc
				}(),
			},
			resource:    testutil.CreateTestServiceAlias("test-alias", "test-namespace", "test-service"),
			expectError: false,
		},
		{
			name:        "invalid service alias - missing service",
			setupData:   map[string]interface{}{},
			resource:    testutil.CreateTestServiceAlias("test-alias", "test-namespace", "non-existent-service"),
			expectError: true,
			errorMsg:    "service dependency validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			validationService := NewValidationService(mockRegistry)

			// Execute
			err := validationService.ValidateResourceDependencies(context.Background(), tt.resource)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationService_ErrorHandling(t *testing.T) {
	t.Run("registry error handling", func(t *testing.T) {
		// Setup with closed registry to simulate error conditions
		mockRegistry := testutil.NewMockRegistry()
		mockRegistry.Close() // Close registry to force errors

		validationService := NewValidationService(mockRegistry)
		service := testutil.CreateTestService("test-service", "test-namespace")

		// Test that errors are properly handled
		err := validationService.ValidateServiceForCreation(context.Background(), service)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get reader")

		err = validationService.ValidateServiceForUpdate(context.Background(), service, service)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get reader")

		err = validationService.ValidateServiceForDeletion(context.Background(), service)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get reader")
	})
}

func TestValidationService_Lifecycle(t *testing.T) {
	t.Run("complete validation lifecycle for service", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		validationService := NewValidationService(mockRegistry)
		service := testutil.CreateTestService("test-service", "test-namespace")

		// Test creation validation
		err := validationService.ValidateServiceForCreation(context.Background(), service)
		require.NoError(t, err)

		// Simulate service creation by adding to registry
		mockRegistry.SetupTestData(map[string]interface{}{
			"service_test-namespace/test-service": &service,
		})

		// Test update validation
		updatedService := service
		updatedService.Description = "Updated description"
		err = validationService.ValidateServiceForUpdate(context.Background(), service, updatedService)
		require.NoError(t, err)

		// Test deletion validation
		err = validationService.ValidateServiceForDeletion(context.Background(), service)
		require.NoError(t, err)
	})

	t.Run("complete validation lifecycle for complex dependency", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		validationService := NewValidationService(mockRegistry)

		// First create service and service alias
		service := testutil.CreateTestService("test-service", "test-namespace")
		localService := testutil.CreateTestService("test-local-service", "test-namespace")
		serviceAlias := testutil.CreateTestServiceAlias("test-service", "test-namespace", "test-service")
		localServiceAlias := testutil.CreateTestServiceAlias("test-local-service", "test-namespace", "test-local-service")

		mockRegistry.SetupTestData(map[string]interface{}{
			"service_test-namespace/test-service":            &service,
			"service_test-namespace/test-local-service":      &localService,
			"servicealias_test-namespace/test-service":       &serviceAlias,
			"servicealias_test-namespace/test-local-service": &localServiceAlias,
		})

		// Test RuleS2S creation validation with dependencies
		rule := testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")
		err := validationService.ValidateRuleS2SForCreation(context.Background(), rule)
		require.NoError(t, err)

		// Test resource dependency validation
		err = validationService.ValidateResourceDependencies(context.Background(), rule)
		require.NoError(t, err)

		// Test service alias dependency validation
		err = validationService.ValidateResourceDependencies(context.Background(), serviceAlias)
		require.NoError(t, err)
	})
}
