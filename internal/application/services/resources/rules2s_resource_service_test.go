package resources

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/services/resources/testutil"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuleS2SResourceService_GetRuleS2S(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		scope       ports.Scope
		expectedLen int
	}{
		{
			name: "get all RuleS2S",
			setupData: map[string]interface{}{
				"rules2s_test-namespace/test-rule-s2s-1": func() *models.RuleS2S {
					rule := testutil.CreateTestRuleS2S("test-rule-s2s-1", "test-namespace")
					return &rule
				}(),
				"rules2s_test-namespace/test-rule-s2s-2": func() *models.RuleS2S {
					rule := testutil.CreateTestRuleS2S("test-rule-s2s-2", "test-namespace")
					return &rule
				}(),
			},
			scope:       ports.EmptyScope{},
			expectedLen: 2,
		},
		{
			name:        "get RuleS2S from empty registry",
			setupData:   map[string]interface{}{},
			scope:       ports.EmptyScope{},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()

			service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			rules, err := service.GetRuleS2S(context.Background(), tt.scope)

			// Assert
			require.NoError(t, err)
			assert.Len(t, rules, tt.expectedLen)

			if tt.expectedLen > 0 {
				// Verify basic structure of returned RuleS2S
				assert.NotEmpty(t, rules[0].SelfRef.Name)
				assert.NotEmpty(t, rules[0].SelfRef.Namespace)
				assert.NotEmpty(t, rules[0].ServiceRef.Name)
				assert.NotEmpty(t, rules[0].ServiceLocalRef.Name)
			}
		})
	}
}

func TestRuleS2SResourceService_GetRuleS2SByID(t *testing.T) {
	tests := []struct {
		name         string
		setupData    map[string]interface{}
		resourceID   models.ResourceIdentifier
		expectError  bool
		expectedRule *models.RuleS2S
	}{
		{
			name: "get existing RuleS2S",
			setupData: map[string]interface{}{
				"rules2s_test-namespace/test-rule-s2s": func() *models.RuleS2S {
					rule := testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")
					return &rule
				}(),
			},
			resourceID:  models.ResourceIdentifier{Name: "test-rule-s2s", Namespace: "test-namespace"},
			expectError: false,
			expectedRule: func() *models.RuleS2S {
				rule := testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")
				return &rule
			}(),
		},
		{
			name:         "get non-existent RuleS2S",
			setupData:    map[string]interface{}{},
			resourceID:   models.ResourceIdentifier{Name: "test-rule-s2s", Namespace: "test-namespace"},
			expectError:  true,
			expectedRule: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()

			service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			result, err := service.GetRuleS2SByID(context.Background(), tt.resourceID)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedRule.SelfRef.Name, result.SelfRef.Name)
				assert.Equal(t, tt.expectedRule.SelfRef.Namespace, result.SelfRef.Namespace)
			}
		})
	}
}

func TestRuleS2SResourceService_GetRuleS2SByIDs(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		resourceIDs []models.ResourceIdentifier
		expectedLen int
	}{
		{
			name: "get multiple existing RuleS2S",
			setupData: map[string]interface{}{
				"rules2s_test-namespace/test-rule-s2s": func() *models.RuleS2S {
					rule := testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")
					return &rule
				}(),
				"rules2s_test-namespace/test-rule-s2s-2": func() *models.RuleS2S {
					rule := testutil.CreateTestRuleS2S("test-rule-s2s-2", "test-namespace")
					return &rule
				}(),
			},
			resourceIDs: []models.ResourceIdentifier{
				{Name: "test-rule-s2s", Namespace: "test-namespace"},
				{Name: "test-rule-s2s-2", Namespace: "test-namespace"},
			},
			expectedLen: 2,
		},
		{
			name: "get mixed existing and non-existing RuleS2S",
			setupData: map[string]interface{}{
				"rules2s_test-namespace/test-rule-s2s": func() *models.RuleS2S {
					rule := testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")
					return &rule
				}(),
			},
			resourceIDs: []models.ResourceIdentifier{
				{Name: "test-rule-s2s", Namespace: "test-namespace"},
				{Name: "non-existing", Namespace: "test-namespace"},
			},
			expectedLen: 1, // Only the existing one should be returned
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()

			service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			results, err := service.GetRuleS2SByIDs(context.Background(), tt.resourceIDs)

			// Assert
			require.NoError(t, err)
			assert.Len(t, results, tt.expectedLen)
		})
	}
}

func TestRuleS2SResourceService_GetIEAgAgRules(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		scope       ports.Scope
		expectedLen int
	}{
		{
			name: "get all IEAgAgRules",
			setupData: map[string]interface{}{
				"ieagagrule_test-namespace/test-ieagag-rule-1": func() *models.IEAgAgRule {
					rule := testutil.CreateTestIEAgAgRule("test-ieagag-rule-1", "test-namespace")
					return &rule
				}(),
				"ieagagrule_test-namespace/test-ieagag-rule-2": func() *models.IEAgAgRule {
					rule := testutil.CreateTestIEAgAgRule("test-ieagag-rule-2", "test-namespace")
					return &rule
				}(),
			},
			scope:       ports.EmptyScope{},
			expectedLen: 2,
		},
		{
			name:        "get IEAgAgRules from empty registry",
			setupData:   map[string]interface{}{},
			scope:       ports.EmptyScope{},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()

			service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			rules, err := service.GetIEAgAgRules(context.Background(), tt.scope)

			// Assert
			require.NoError(t, err)
			assert.Len(t, rules, tt.expectedLen)

			if tt.expectedLen > 0 && len(rules) > 0 {
				// Verify basic structure of returned IEAgAgRules
				assert.NotEmpty(t, rules[0].SelfRef.Name)
				assert.NotEmpty(t, rules[0].SelfRef.Namespace)
				assert.NotEmpty(t, rules[0].AddressGroupLocal.Name)
				assert.NotEmpty(t, rules[0].AddressGroup.Name)
			}
		})
	}
}

func TestRuleS2SResourceService_GetIEAgAgRuleByID(t *testing.T) {
	tests := []struct {
		name         string
		setupData    map[string]interface{}
		resourceID   models.ResourceIdentifier
		expectError  bool
		expectedRule *models.IEAgAgRule
	}{
		{
			name: "get existing IEAgAgRule",
			setupData: map[string]interface{}{
				"ieagagrule_test-namespace/test-ieagag-rule": func() *models.IEAgAgRule {
					rule := testutil.CreateTestIEAgAgRule("test-ieagag-rule", "test-namespace")
					return &rule
				}(),
			},
			resourceID:  models.ResourceIdentifier{Name: "test-ieagag-rule", Namespace: "test-namespace"},
			expectError: false,
			expectedRule: func() *models.IEAgAgRule {
				rule := testutil.CreateTestIEAgAgRule("test-ieagag-rule", "test-namespace")
				return &rule
			}(),
		},
		{
			name:         "get non-existent IEAgAgRule",
			setupData:    map[string]interface{}{},
			resourceID:   models.ResourceIdentifier{Name: "test-ieagag-rule", Namespace: "test-namespace"},
			expectError:  true,
			expectedRule: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()

			service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			result, err := service.GetIEAgAgRuleByID(context.Background(), tt.resourceID)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedRule.SelfRef.Name, result.SelfRef.Name)
				assert.Equal(t, tt.expectedRule.SelfRef.Namespace, result.SelfRef.Namespace)
			}
		})
	}
}

func TestRuleS2SResourceService_SyncRuleS2S(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		rules       []models.RuleS2S
		scope       ports.Scope
		expectError bool
	}{
		{
			name: "sync valid RuleS2S",
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
			rules:       []models.RuleS2S{testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")},
			scope:       ports.EmptyScope{},
			expectError: false,
		},
		{
			name: "sync multiple RuleS2S",
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
			rules: []models.RuleS2S{
				testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace"),
				testutil.CreateTestRuleS2S("test-rule-s2s-2", "test-namespace"),
			},
			scope:       ports.EmptyScope{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()

			service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			err := service.SyncRuleS2S(context.Background(), tt.rules, tt.scope)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify rules were synced
				for _, rule := range tt.rules {
					result, getErr := service.GetRuleS2SByID(context.Background(), rule.SelfRef.ResourceIdentifier)
					require.NoError(t, getErr)
					assert.Equal(t, rule.SelfRef.Name, result.SelfRef.Name)
				}
			}
		})
	}
}

func TestRuleS2SResourceService_DeleteRuleS2SByIDs(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		idsToDelete []models.ResourceIdentifier
		expectError bool
	}{
		{
			name: "delete existing RuleS2S",
			setupData: map[string]interface{}{
				"rules2s_test-namespace/test-rule-s2s": func() *models.RuleS2S {
					rule := testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")
					return &rule
				}(),
			},
			idsToDelete: []models.ResourceIdentifier{
				{Name: "test-rule-s2s", Namespace: "test-namespace"},
			},
			expectError: false,
		},
		{
			name:      "delete non-existent RuleS2S",
			setupData: map[string]interface{}{},
			idsToDelete: []models.ResourceIdentifier{
				{Name: "non-existent", Namespace: "test-namespace"},
			},
			expectError: false, // Deletion is typically idempotent
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()

			service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			err := service.DeleteRuleS2SByIDs(context.Background(), tt.idsToDelete)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify rules were deleted
				for _, id := range tt.idsToDelete {
					_, getErr := service.GetRuleS2SByID(context.Background(), id)
					assert.Error(t, getErr) // Should not be found
				}
			}
		})
	}
}

func TestRuleS2SResourceService_SyncIEAgAgRules(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		rules       []models.IEAgAgRule
		scope       ports.Scope
		expectError bool
	}{
		{
			name:        "sync valid IEAgAgRules",
			setupData:   map[string]interface{}{},
			rules:       []models.IEAgAgRule{testutil.CreateTestIEAgAgRule("test-ieagag-rule", "test-namespace")},
			scope:       ports.EmptyScope{},
			expectError: false,
		},
		{
			name:      "sync multiple IEAgAgRules",
			setupData: map[string]interface{}{},
			rules: []models.IEAgAgRule{
				testutil.CreateTestIEAgAgRule("test-ieagag-rule", "test-namespace"),
				testutil.CreateTestIEAgAgRule("test-ieagag-rule-2", "test-namespace"),
			},
			scope:       ports.EmptyScope{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()

			service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			err := service.SyncIEAgAgRules(context.Background(), tt.rules, tt.scope)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify rules were synced
				for _, rule := range tt.rules {
					result, getErr := service.GetIEAgAgRuleByID(context.Background(), rule.SelfRef.ResourceIdentifier)
					require.NoError(t, getErr)
					assert.Equal(t, rule.SelfRef.Name, result.SelfRef.Name)
				}
			}
		})
	}
}

func TestRuleS2SResourceService_DeleteIEAgAgRulesByIDs(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		idsToDelete []models.ResourceIdentifier
		expectError bool
	}{
		{
			name: "delete existing IEAgAgRules",
			setupData: map[string]interface{}{
				"ieagagrule_test-namespace/test-ieagag-rule": func() *models.IEAgAgRule {
					rule := testutil.CreateTestIEAgAgRule("test-ieagag-rule", "test-namespace")
					return &rule
				}(),
			},
			idsToDelete: []models.ResourceIdentifier{
				{Name: "test-ieagag-rule", Namespace: "test-namespace"},
			},
			expectError: false,
		},
		{
			name:      "delete non-existent IEAgAgRules",
			setupData: map[string]interface{}{},
			idsToDelete: []models.ResourceIdentifier{
				{Name: "non-existent", Namespace: "test-namespace"},
			},
			expectError: false, // Deletion is typically idempotent
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()

			service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			err := service.DeleteIEAgAgRulesByIDs(context.Background(), tt.idsToDelete)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify rules were deleted
				for _, id := range tt.idsToDelete {
					_, getErr := service.GetIEAgAgRuleByID(context.Background(), id)
					assert.Error(t, getErr) // Should not be found
				}
			}
		})
	}
}

func TestRuleS2SResourceService_ErrorHandling(t *testing.T) {
	t.Run("registry error handling", func(t *testing.T) {
		// Setup with closed registry to simulate error conditions
		mockRegistry := testutil.NewMockRegistry()
		mockRegistry.Close() // Close registry to force errors

		mockSyncManager := testutil.NewMockSyncManager()
		mockConditionManager := testutil.NewMockConditionManager()
		service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)

		// Test that errors are properly handled
		_, err := service.GetRuleS2S(context.Background(), ports.EmptyScope{})
		assert.Error(t, err)

		err = service.SyncRuleS2S(context.Background(), []models.RuleS2S{testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")}, ports.EmptyScope{})
		assert.Error(t, err)

		_, err = service.GetIEAgAgRules(context.Background(), ports.EmptyScope{})
		assert.Error(t, err)
	})
}

func TestRuleS2SResourceService_Lifecycle(t *testing.T) {
	t.Run("complete RuleS2S lifecycle", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		// Add required services and service aliases for validation
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

		mockSyncManager := testutil.NewMockSyncManager()
		mockConditionManager := testutil.NewMockConditionManager()

		service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)
		testRule := testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")

		// Sync RuleS2S
		err := service.SyncRuleS2S(context.Background(), []models.RuleS2S{testRule}, ports.EmptyScope{})
		require.NoError(t, err)

		// Verify RuleS2S exists
		retrieved, err := service.GetRuleS2SByID(context.Background(), testRule.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, testRule.SelfRef.Name, retrieved.SelfRef.Name)

		// Delete RuleS2S
		err = service.DeleteRuleS2SByIDs(context.Background(), []models.ResourceIdentifier{testRule.SelfRef.ResourceIdentifier})
		require.NoError(t, err)

		// Verify deletion
		_, err = service.GetRuleS2SByID(context.Background(), testRule.SelfRef.ResourceIdentifier)
		assert.Error(t, err) // Should not be found
	})

	t.Run("complete IEAgAgRule lifecycle", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		mockSyncManager := testutil.NewMockSyncManager()
		mockConditionManager := testutil.NewMockConditionManager()

		service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)
		testRule := testutil.CreateTestIEAgAgRule("test-ieagag-rule", "test-namespace")

		// Sync IEAgAgRule
		err := service.SyncIEAgAgRules(context.Background(), []models.IEAgAgRule{testRule}, ports.EmptyScope{})
		require.NoError(t, err)

		// Verify IEAgAgRule exists
		retrieved, err := service.GetIEAgAgRuleByID(context.Background(), testRule.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, testRule.SelfRef.Name, retrieved.SelfRef.Name)

		// Delete IEAgAgRule
		err = service.DeleteIEAgAgRulesByIDs(context.Background(), []models.ResourceIdentifier{testRule.SelfRef.ResourceIdentifier})
		require.NoError(t, err)

		// Verify deletion
		_, err = service.GetIEAgAgRuleByID(context.Background(), testRule.SelfRef.ResourceIdentifier)
		assert.Error(t, err) // Should not be found
	})
}
