package resources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/application/services/resources/testutil"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestRuleS2SDynamicPortAggregation tests the complete dynamic port aggregation functionality
// This demonstrates how multiple RuleS2S that generate the same IEAgAg rule (same AddressGroups + Transport)
// properly aggregate their ports even when services change dynamically
func TestRuleS2SDynamicPortAggregation(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()
	mockConditionManager := testutil.NewMockConditionManager()

	service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)
	ctx := context.Background()

	// Create address groups
	webAG := testutil.CreateTestAddressGroup("web-ag", "default")
	dbAG := testutil.CreateTestAddressGroup("db-ag", "default")

	// Create services with different ports
	webService := testutil.CreateTestService("web-service", "default")
	webService.IngressPorts = []models.IngressPort{
		{Port: "80", Protocol: models.TCP},
		{Port: "443", Protocol: models.TCP},
	}
	webService.AddressGroups = []models.AddressGroupRef{
		models.NewAddressGroupRef("web-ag", models.WithNamespace("default")),
	}

	apiService := testutil.CreateTestService("api-service", "default")
	apiService.IngressPorts = []models.IngressPort{
		{Port: "8080", Protocol: models.TCP},
		{Port: "8081", Protocol: models.TCP},
	}
	apiService.AddressGroups = []models.AddressGroupRef{
		models.NewAddressGroupRef("web-ag", models.WithNamespace("default")), // Same AddressGroup!
	}

	dbService := testutil.CreateTestService("db-service", "default")
	dbService.IngressPorts = []models.IngressPort{
		{Port: "3306", Protocol: models.TCP},
	}
	dbService.AddressGroups = []models.AddressGroupRef{
		models.NewAddressGroupRef("db-ag", models.WithNamespace("default")),
	}

	// Create service aliases
	webAlias := testutil.CreateTestServiceAlias("web-alias", "default", "web-service")
	apiAlias := testutil.CreateTestServiceAlias("api-alias", "default", "api-service")
	dbAlias := testutil.CreateTestServiceAlias("db-alias", "default", "db-service")

	// Setup test data
	testData := map[string]interface{}{
		"addressgroup_default/web-ag":    &webAG,
		"addressgroup_default/db-ag":     &dbAG,
		"service_default/web-service":    &webService,
		"service_default/api-service":    &apiService,
		"service_default/db-service":     &dbService,
		"servicealias_default/web-alias": &webAlias,
		"servicealias_default/api-alias": &apiAlias,
		"servicealias_default/db-alias":  &dbAlias,
	}
	mockRegistry.SetupTestData(testData)

	t.Run("MultipleRuleS2S_GenerateSameAggregatedIEAgAg", func(t *testing.T) {
		// Create two different RuleS2S that should generate the SAME IEAgAg rule
		// Both rules have: web-ag -> db-ag, TCP, INGRESS
		// But they reference different services that both belong to web-ag

		rule1 := testutil.CreateTestRuleS2S("web-to-db-rule", "default")
		rule1.ServiceLocalRef.Name = "web-alias" // web-service -> web-ag
		rule1.ServiceRef.Name = "db-alias"       // db-service -> db-ag
		rule1.Traffic = models.INGRESS

		rule2 := testutil.CreateTestRuleS2S("api-to-db-rule", "default")
		rule2.ServiceLocalRef.Name = "api-alias" // api-service -> web-ag (SAME!)
		rule2.ServiceRef.Name = "db-alias"       // db-service -> db-ag (SAME!)
		rule2.Traffic = models.INGRESS

		// Test the core aggregation functionality
		reader, err := mockRegistry.Reader(ctx)
		require.NoError(t, err)
		defer reader.Close()

		writer, err := mockRegistry.Writer(ctx)
		require.NoError(t, err)
		defer writer.Abort()

		// Add rules to registry
		err = writer.SyncRuleS2S(ctx, []models.RuleS2S{rule1, rule2}, ports.EmptyScope{})
		require.NoError(t, err)
		err = writer.Commit()
		require.NoError(t, err)

		// Now test the new aggregated update logic
		writer2, err := mockRegistry.Writer(ctx)
		require.NoError(t, err)
		defer writer2.Abort()

		// Update both rules - this should trigger aggregated generation
		err = service.updateIEAgAgRulesForRuleS2SWithReader(ctx, writer2, reader, []models.RuleS2S{rule1, rule2})
		require.NoError(t, err)

		// Get the generated IEAgAg rules
		reader2, err := mockRegistry.ReaderFromWriter(ctx, writer2)
		require.NoError(t, err)
		defer reader2.Close()

		var generatedRules []models.IEAgAgRule
		err = reader2.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
			generatedRules = append(generatedRules, rule)
			return nil
		}, ports.EmptyScope{})
		require.NoError(t, err)

		// CRITICAL: Should generate only 1 aggregated rule, not 2 separate rules
		require.Len(t, generatedRules, 1, "Should generate 1 aggregated IEAgAg rule, not separate rules")

		aggregatedRule := generatedRules[0]

		// Verify the aggregated rule has all ports from both services
		require.Len(t, aggregatedRule.Ports, 1, "Should have 1 aggregated port spec")
		portStr := aggregatedRule.Ports[0].Destination

		t.Logf("Aggregated ports: %s", portStr)

		// Should contain ports from both web-service (80,443) and api-service (8080,8081)
		assert.Contains(t, portStr, "80", "Should contain web-service port 80")
		assert.Contains(t, portStr, "443", "Should contain web-service port 443")
		assert.Contains(t, portStr, "8080", "Should contain api-service port 8080")
		assert.Contains(t, portStr, "8081", "Should contain api-service port 8081")
		assert.Contains(t, portStr, ",", "Ports should be comma-separated")

		// Verify other fields
		assert.Equal(t, models.TCP, aggregatedRule.Transport)
		assert.Equal(t, models.INGRESS, aggregatedRule.Traffic)
		assert.Equal(t, "web-ag", aggregatedRule.AddressGroupLocal.Name)
		assert.Equal(t, "db-ag", aggregatedRule.AddressGroup.Name)
	})

	t.Run("ServicePortChange_UpdatesAggregatedRule", func(t *testing.T) {
		// Create the same two rules as above
		rule1 := testutil.CreateTestRuleS2S("web-to-db-rule", "default")
		rule1.ServiceLocalRef.Name = "web-alias"
		rule1.ServiceRef.Name = "db-alias"
		rule1.Traffic = models.INGRESS

		rule2 := testutil.CreateTestRuleS2S("api-to-db-rule", "default")
		rule2.ServiceLocalRef.Name = "api-alias"
		rule2.ServiceRef.Name = "db-alias"
		rule2.Traffic = models.INGRESS

		// Create initial aggregated rule
		writer, err := mockRegistry.Writer(ctx)
		require.NoError(t, err)
		defer writer.Abort()

		err = writer.SyncRuleS2S(ctx, []models.RuleS2S{rule1, rule2}, ports.EmptyScope{})
		require.NoError(t, err)

		reader, err := mockRegistry.ReaderFromWriter(ctx, writer)
		require.NoError(t, err)
		defer reader.Close()

		err = service.updateIEAgAgRulesForRuleS2SWithReader(ctx, writer, reader, []models.RuleS2S{rule1, rule2})
		require.NoError(t, err)
		err = writer.Commit()
		require.NoError(t, err)

		// NOW: Change ports on api-service (add port 9090)
		apiService.IngressPorts = append(apiService.IngressPorts, models.IngressPort{
			Port: "9090", Protocol: models.TCP,
		})

		// Update test data
		testData["service_default/api-service"] = &apiService
		mockRegistry.SetupTestData(testData)

		// Simulate what happens when a service changes:
		// The system should find all aggregation groups affected and regenerate with ALL contributing rules

		writer3, err := mockRegistry.Writer(ctx)
		require.NoError(t, err)
		defer writer3.Abort()

		reader3, err := mockRegistry.Reader(ctx)
		require.NoError(t, err)
		defer reader3.Close()

		// Find aggregation groups affected by api-service change
		serviceIDs := []models.ResourceIdentifier{
			{Name: "api-service", Namespace: "default"},
		}

		affectedGroups, err := service.FindAggregationGroupsForServices(ctx, reader3, serviceIDs)
		require.NoError(t, err)
		require.Greater(t, len(affectedGroups), 0, "Should find affected aggregation groups")

		// For each affected group, find ALL contributing rules and regenerate
		var allAffectedRules []models.RuleS2S
		for _, group := range affectedGroups {
			groupRules, err := service.FindAllRuleS2SForAggregationGroup(ctx, reader3, group)
			require.NoError(t, err)
			allAffectedRules = append(allAffectedRules, groupRules...)
		}

		// Should find both rule1 and rule2 since they both contribute to the same aggregation group
		require.GreaterOrEqual(t, len(allAffectedRules), 2, "Should find all contributing rules")

		// Regenerate with the complete set
		err = service.updateIEAgAgRulesForRuleS2SWithReader(ctx, writer3, reader3, allAffectedRules)
		require.NoError(t, err)

		// Check the updated rule
		reader4, err := mockRegistry.ReaderFromWriter(ctx, writer3)
		require.NoError(t, err)
		defer reader4.Close()

		var updatedRules []models.IEAgAgRule
		err = reader4.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
			updatedRules = append(updatedRules, rule)
			return nil
		}, ports.EmptyScope{})
		require.NoError(t, err)

		require.Len(t, updatedRules, 1, "Should still have 1 aggregated rule")

		updatedRule := updatedRules[0]
		portStr := updatedRule.Ports[0].Destination

		t.Logf("Updated aggregated ports: %s", portStr)

		// Should now contain the new port 9090
		assert.Contains(t, portStr, "9090", "Should contain new port 9090")
		assert.Contains(t, portStr, "80", "Should still contain port 80")
		assert.Contains(t, portStr, "443", "Should still contain port 443")
		assert.Contains(t, portStr, "8080", "Should still contain port 8080")
		assert.Contains(t, portStr, "8081", "Should still contain port 8081")
	})

	t.Run("ServiceUnbinding_RemovesFromAggregation", func(t *testing.T) {
		// Test what happens when a service is unbound from an AddressGroup
		// This should remove its ports from the aggregated rule

		rule1 := testutil.CreateTestRuleS2S("web-to-db-rule", "default")
		rule1.ServiceLocalRef.Name = "web-alias"
		rule1.ServiceRef.Name = "db-alias"
		rule1.Traffic = models.INGRESS

		rule2 := testutil.CreateTestRuleS2S("api-to-db-rule", "default")
		rule2.ServiceLocalRef.Name = "api-alias"
		rule2.ServiceRef.Name = "db-alias"
		rule2.Traffic = models.INGRESS

		// Create initial state
		writer, err := mockRegistry.Writer(ctx)
		require.NoError(t, err)
		defer writer.Abort()

		err = writer.SyncRuleS2S(ctx, []models.RuleS2S{rule1, rule2}, ports.EmptyScope{})
		require.NoError(t, err)

		reader, err := mockRegistry.ReaderFromWriter(ctx, writer)
		require.NoError(t, err)
		defer reader.Close()

		err = service.updateIEAgAgRulesForRuleS2SWithReader(ctx, writer, reader, []models.RuleS2S{rule1, rule2})
		require.NoError(t, err)
		err = writer.Commit()
		require.NoError(t, err)

		// Remove api-service from web-ag (unbind it)
		apiService.AddressGroups = []models.AddressGroupRef{} // No address groups

		// Update test data
		testData["service_default/api-service"] = &apiService
		mockRegistry.SetupTestData(testData)

		// Regenerate rules
		writer2, err := mockRegistry.Writer(ctx)
		require.NoError(t, err)
		defer writer2.Abort()

		reader2, err := mockRegistry.Reader(ctx)
		require.NoError(t, err)
		defer reader2.Close()

		// Find all rules that might be affected
		serviceIDs := []models.ResourceIdentifier{
			{Name: "api-service", Namespace: "default"},
		}

		_, err = service.FindAggregationGroupsForServices(ctx, reader2, serviceIDs)
		require.NoError(t, err)

		// Since api-service is no longer bound, we need to find rules that USED to reference it
		// For this test, we'll manually trigger regeneration of the remaining rules
		err = service.updateIEAgAgRulesForRuleS2SWithReader(ctx, writer2, reader2, []models.RuleS2S{rule1, rule2})
		require.NoError(t, err)

		// Check the result
		reader3, err := mockRegistry.ReaderFromWriter(ctx, writer2)
		require.NoError(t, err)
		defer reader3.Close()

		var finalRules []models.IEAgAgRule
		err = reader3.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
			finalRules = append(finalRules, rule)
			return nil
		}, ports.EmptyScope{})
		require.NoError(t, err)

		// Should still have 1 rule, but only with web-service ports (since api-service is unbound)
		require.Len(t, finalRules, 1, "Should have 1 rule for remaining bound service")

		finalRule := finalRules[0]
		portStr := finalRule.Ports[0].Destination

		t.Logf("Final aggregated ports after unbinding: %s", portStr)

		// Should only contain web-service ports, not api-service ports
		assert.Contains(t, portStr, "80", "Should still contain web-service port 80")
		assert.Contains(t, portStr, "443", "Should still contain web-service port 443")
		assert.NotContains(t, portStr, "8080", "Should NOT contain unbound api-service port 8080")
		assert.NotContains(t, portStr, "8081", "Should NOT contain unbound api-service port 8081")
	})
}

// TestAggregationGroupDiscovery tests the smart rule discovery functionality
func TestAggregationGroupDiscovery(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()
	mockConditionManager := testutil.NewMockConditionManager()

	service := NewRuleS2SResourceService(mockRegistry, mockSyncManager, mockConditionManager)
	ctx := context.Background()

	// Create test setup similar to the previous test
	webAG := testutil.CreateTestAddressGroup("web-ag", "default")
	dbAG := testutil.CreateTestAddressGroup("db-ag", "default")

	webService := testutil.CreateTestService("web-service", "default")
	webService.IngressPorts = []models.IngressPort{
		{Port: "80", Protocol: models.TCP},
		{Port: "53", Protocol: models.UDP},
	}
	webService.AddressGroups = []models.AddressGroupRef{
		models.NewAddressGroupRef("web-ag", models.WithNamespace("default")),
	}

	dbService := testutil.CreateTestService("db-service", "default")
	dbService.IngressPorts = []models.IngressPort{
		{Port: "3306", Protocol: models.TCP},
	}
	dbService.AddressGroups = []models.AddressGroupRef{
		models.NewAddressGroupRef("db-ag", models.WithNamespace("default")),
	}

	webAlias := testutil.CreateTestServiceAlias("web-alias", "default", "web-service")
	dbAlias := testutil.CreateTestServiceAlias("db-alias", "default", "db-service")

	testData := map[string]interface{}{
		"addressgroup_default/web-ag":    &webAG,
		"addressgroup_default/db-ag":     &dbAG,
		"service_default/web-service":    &webService,
		"service_default/db-service":     &dbService,
		"servicealias_default/web-alias": &webAlias,
		"servicealias_default/db-alias":  &dbAlias,
	}
	mockRegistry.SetupTestData(testData)

	t.Run("FindAggregationGroupsForServices", func(t *testing.T) {
		reader, err := mockRegistry.Reader(ctx)
		require.NoError(t, err)
		defer reader.Close()

		// Create a RuleS2S
		rule := testutil.CreateTestRuleS2S("test-rule", "default")
		rule.ServiceLocalRef.Name = "web-alias"
		rule.ServiceRef.Name = "db-alias"
		rule.Traffic = models.INGRESS

		writer, err := mockRegistry.Writer(ctx)
		require.NoError(t, err)
		defer writer.Abort()

		err = writer.SyncRuleS2S(ctx, []models.RuleS2S{rule}, ports.EmptyScope{})
		require.NoError(t, err)

		reader2, err := mockRegistry.ReaderFromWriter(ctx, writer)
		require.NoError(t, err)
		defer reader2.Close()

		// Find aggregation groups affected by web-service
		serviceIDs := []models.ResourceIdentifier{
			{Name: "web-service", Namespace: "default"},
		}

		groups, err := service.FindAggregationGroupsForServices(ctx, reader2, serviceIDs)
		require.NoError(t, err)

		// Should find 2 groups:
		// 1. INGRESS|default/web-ag|default/db-ag|TCP
		// 2. INGRESS|default/web-ag|default/db-ag|UDP
		assert.Len(t, groups, 2, "Should find 2 aggregation groups (TCP + UDP)")

		// Verify the groups
		var tcpGroup, udpGroup *AggregationGroup
		for _, group := range groups {
			if group.Protocol == models.TCP {
				tcpGroup = &group
			} else if group.Protocol == models.UDP {
				udpGroup = &group
			}
		}

		require.NotNil(t, tcpGroup, "Should find TCP aggregation group")
		require.NotNil(t, udpGroup, "Should find UDP aggregation group")

		assert.Equal(t, models.INGRESS, tcpGroup.Traffic)
		assert.Equal(t, "web-ag", tcpGroup.LocalAG.Name)
		assert.Equal(t, "db-ag", tcpGroup.TargetAG.Name)
		assert.Equal(t, models.TCP, tcpGroup.Protocol)
	})

	t.Run("FindAllRuleS2SForAggregationGroup", func(t *testing.T) {
		reader, err := mockRegistry.Reader(ctx)
		require.NoError(t, err)
		defer reader.Close()

		// Create multiple RuleS2S that contribute to the same aggregation group
		rule1 := testutil.CreateTestRuleS2S("rule1", "default")
		rule1.ServiceLocalRef.Name = "web-alias"
		rule1.ServiceRef.Name = "db-alias"
		rule1.Traffic = models.INGRESS

		rule2 := testutil.CreateTestRuleS2S("rule2", "default")
		rule2.ServiceLocalRef.Name = "web-alias"
		rule2.ServiceRef.Name = "db-alias"
		rule2.Traffic = models.INGRESS

		// Different traffic direction - should not match
		rule3 := testutil.CreateTestRuleS2S("rule3", "default")
		rule3.ServiceLocalRef.Name = "web-alias"
		rule3.ServiceRef.Name = "db-alias"
		rule3.Traffic = models.EGRESS

		writer, err := mockRegistry.Writer(ctx)
		require.NoError(t, err)
		defer writer.Abort()

		err = writer.SyncRuleS2S(ctx, []models.RuleS2S{rule1, rule2, rule3}, ports.EmptyScope{})
		require.NoError(t, err)

		reader2, err := mockRegistry.ReaderFromWriter(ctx, writer)
		require.NoError(t, err)
		defer reader2.Close()

		// Define the aggregation group we're looking for
		group := AggregationGroup{
			Traffic:   models.INGRESS,
			LocalAG:   models.NewAddressGroupRef("web-ag", models.WithNamespace("default")),
			TargetAG:  models.NewAddressGroupRef("db-ag", models.WithNamespace("default")),
			Protocol:  models.TCP,
			Namespace: "default",
		}

		// Find all rules that contribute to this group
		matchingRules, err := service.FindAllRuleS2SForAggregationGroup(ctx, reader2, group)
		require.NoError(t, err)

		// Should find rule1 and rule2, but not rule3 (different traffic direction)
		assert.Len(t, matchingRules, 2, "Should find 2 matching rules")

		ruleNames := make([]string, len(matchingRules))
		for i, rule := range matchingRules {
			ruleNames[i] = rule.Name
		}

		assert.Contains(t, ruleNames, "rule1")
		assert.Contains(t, ruleNames, "rule2")
		assert.NotContains(t, ruleNames, "rule3")
	})
}
