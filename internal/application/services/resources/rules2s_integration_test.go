package resources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/application/services/resources/testutil"
	"netguard-pg-backend/internal/domain/models"
)

// TestRuleS2SToIEAgAgGeneration_IntegrationFlow tests the full flow from RuleS2S creation to IEAgAg rule generation
func TestRuleS2SToIEAgAgGeneration_IntegrationFlow(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()
	mockConditionManager := testutil.NewMockConditionManager()

	service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)

	ctx := context.Background()

	// Create test address groups
	webAddressGroup := testutil.CreateTestAddressGroup("web-ag", "default")
	dbAddressGroup := testutil.CreateTestAddressGroup("db-ag", "default")

	// Create test services with address groups
	webService := testutil.CreateTestService("web-service", "default")
	webService.IngressPorts = []models.IngressPort{
		{Port: "80", Protocol: models.TCP},
		{Port: "443", Protocol: models.TCP},
		{Port: "53", Protocol: models.UDP},
	}
	webService.AddressGroups = []models.AddressGroupRef{
		models.NewAddressGroupRef("web-ag", models.WithNamespace("default")),
	}

	dbService := testutil.CreateTestService("db-service", "default")
	dbService.IngressPorts = []models.IngressPort{
		{Port: "3306", Protocol: models.TCP},
		{Port: "5432", Protocol: models.TCP},
	}
	dbService.AddressGroups = []models.AddressGroupRef{
		models.NewAddressGroupRef("db-ag", models.WithNamespace("default")),
	}

	// Create test service aliases
	webAlias := testutil.CreateTestServiceAlias("web-alias", "default", "web-service")
	dbAlias := testutil.CreateTestServiceAlias("db-alias", "default", "db-service")

	// Setup test data in mock registry using the correct format: "resourcetype_namespace/name"
	testData := map[string]interface{}{
		"addressgroup_default/web-ag":    &webAddressGroup,
		"addressgroup_default/db-ag":     &dbAddressGroup,
		"service_default/web-service":    &webService,
		"service_default/db-service":     &dbService,
		"servicealias_default/web-alias": &webAlias,
		"servicealias_default/db-alias":  &dbAlias,
	}
	mockRegistry.SetupTestData(testData)

	// Create test RuleS2S
	rule := testutil.CreateTestRuleS2S("test-rule", "default")
	rule.ServiceLocalRef.Name = "web-alias"
	rule.ServiceRef.Name = "db-alias"
	rule.Traffic = models.INGRESS
	rule.Trace = true // Enable trace (will be copied to generated IEAgAg rule Logs)

	t.Run("GenerateIEAgAgRulesFromRuleS2S", func(t *testing.T) {
		// Generate IEAgAg rules from RuleS2S
		generatedRules, err := service.GenerateIEAgAgRulesFromRuleS2S(ctx, rule)
		require.NoError(t, err)

		// Verify rules were generated
		assert.Greater(t, len(generatedRules), 0, "Should generate at least one IEAgAg rule")

		for i, ieRule := range generatedRules {
			t.Logf("Generated IEAgAg rule %d:", i+1)
			t.Logf("  Name: %s", ieRule.Name)
			t.Logf("  Traffic: %s", ieRule.Traffic)
			t.Logf("  Transport: %s", ieRule.Transport)
			t.Logf("  LocalAG: %s", ieRule.AddressGroupLocal.Name)
			t.Logf("  TargetAG: %s", ieRule.AddressGroup.Name)
			t.Logf("  Ports: %v", ieRule.Ports)

			// Verify UUID-based naming
			assert.True(t, len(ieRule.Name) == 40, "Rule name should be 40 characters (UUID format)")
			assert.True(t, ieRule.Name[:4] == "ing-", "INGRESS rule should have 'ing-' prefix")
			assert.Regexp(t, "^ing-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$", ieRule.Name, "Should match UUID format")

			// Verify essential fields are set
			assert.Equal(t, models.INGRESS, ieRule.Traffic, "Traffic should match RuleS2S")
			assert.NotEmpty(t, ieRule.AddressGroupLocal.Name, "Local AddressGroup should be set")
			assert.NotEmpty(t, ieRule.AddressGroup.Name, "Target AddressGroup should be set")
			assert.NotEmpty(t, ieRule.Ports, "Ports should be set")

			// Verify restored fields
			assert.Equal(t, models.ActionAccept, ieRule.Action, "Action should be Accept")
			assert.False(t, ieRule.Logs, "Logs should be disabled by default")
			assert.True(t, ieRule.Trace, "Trace should be enabled")
			assert.Equal(t, int32(100), ieRule.Priority, "Priority should be 100")

			// Verify port aggregation - ports should be joined as comma-separated string
			if len(ieRule.Ports) > 0 {
				portStr := ieRule.Ports[0].Destination
				if ieRule.Transport == models.TCP {
					// TCP rules should contain web service ports
					assert.Contains(t, portStr, "80", "TCP rule should include port 80")
					assert.Contains(t, portStr, "443", "TCP rule should include port 443")
				} else if ieRule.Transport == models.UDP {
					// UDP rules should contain UDP ports
					assert.Contains(t, portStr, "53", "UDP rule should include port 53")
				}
			}
		}
	})

	t.Run("DeterministicRuleNameGeneration", func(t *testing.T) {
		// Generate rules multiple times and verify names are consistent
		rules1, err := service.GenerateIEAgAgRulesFromRuleS2S(ctx, rule)
		require.NoError(t, err)

		rules2, err := service.GenerateIEAgAgRulesFromRuleS2S(ctx, rule)
		require.NoError(t, err)

		assert.Equal(t, len(rules1), len(rules2), "Should generate same number of rules")

		// Create maps for easy comparison
		rules1Map := make(map[string]models.IEAgAgRule)
		rules2Map := make(map[string]models.IEAgAgRule)

		for _, r := range rules1 {
			rules1Map[r.Name] = r
		}
		for _, r := range rules2 {
			rules2Map[r.Name] = r
		}

		// Verify same rule names are generated
		for name, rule1 := range rules1Map {
			rule2, exists := rules2Map[name]
			assert.True(t, exists, "Rule %s should exist in both generations", name)
			if exists {
				assert.Equal(t, rule1.Name, rule2.Name, "Rule names should be identical")
				assert.Equal(t, rule1.Traffic, rule2.Traffic, "Traffic should be identical")
				assert.Equal(t, rule1.Transport, rule2.Transport, "Transport should be identical")
			}
		}
	})

	t.Run("PortAggregationLogic", func(t *testing.T) {
		// Create service with multiple ports of same protocol
		multiPortService := testutil.CreateTestService("multi-port-service", "default")
		multiPortService.IngressPorts = []models.IngressPort{
			{Port: "8080", Protocol: models.TCP},
			{Port: "8081", Protocol: models.TCP},
			{Port: "8082", Protocol: models.TCP},
		}
		// Add AddressGroups so rule generation works
		multiPortService.AddressGroups = []models.AddressGroupRef{
			models.NewAddressGroupRef("web-ag", models.WithNamespace("default")),
		}

		multiPortAlias := testutil.CreateTestServiceAlias("multi-port-alias", "default", "multi-port-service")

		// Add new test data to mock registry
		testData["service_default/multi-port-service"] = &multiPortService
		testData["servicealias_default/multi-port-alias"] = &multiPortAlias
		mockRegistry.SetupTestData(testData)

		// Create rule referencing multi-port service
		multiPortRule := testutil.CreateTestRuleS2S("multi-port-rule", "default")
		multiPortRule.ServiceLocalRef.Name = "multi-port-alias"
		multiPortRule.ServiceRef.Name = "db-alias"
		multiPortRule.Traffic = models.INGRESS

		rules, err := service.GenerateIEAgAgRulesFromRuleS2S(ctx, multiPortRule)
		require.NoError(t, err)

		// Find TCP rule and verify port aggregation
		var tcpRule *models.IEAgAgRule
		for _, r := range rules {
			if r.Transport == models.TCP {
				tcpRule = &r
				break
			}
		}

		require.NotNil(t, tcpRule, "Should generate TCP rule")
		require.Len(t, tcpRule.Ports, 1, "Should have single aggregated port spec")

		portStr := tcpRule.Ports[0].Destination
		t.Logf("Aggregated ports: %s", portStr)

		// Verify ports are comma-separated and sorted
		assert.Contains(t, portStr, "8080", "Should contain port 8080")
		assert.Contains(t, portStr, "8081", "Should contain port 8081")
		assert.Contains(t, portStr, "8082", "Should contain port 8082")
		assert.Contains(t, portStr, ",", "Ports should be comma-separated")
	})
}
