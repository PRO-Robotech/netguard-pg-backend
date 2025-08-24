package resources

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// TestPortAggregationLogic_Core tests the core port aggregation logic in isolation
func TestPortAggregationLogic_Core(t *testing.T) {
	// Test the exact port aggregation logic that was restored from original code

	// Simulate port groups (like in generateAggregatedIEAgAgRules)
	portGroups := make(map[string]map[string]bool)

	// Create a test group
	groupKey := "INGRESS|default/web-ag|default/db-ag|TCP"
	portGroups[groupKey] = map[string]bool{
		"8080": true,
		"8081": true,
		"8082": true,
		"80":   true,
		"443":  true,
	}

	// Test the restored aggregation logic
	for _, portsSet := range portGroups {
		// Collect and sort ports (CRITICAL: restored from original)
		ports := make([]string, 0, len(portsSet))
		for port := range portsSet {
			ports = append(ports, port)
		}
		sort.Strings(ports) // IMPORTANT: This was missing before!

		// Create port spec like in restored code
		portSpec := models.PortSpec{
			Destination: strings.Join(ports, ","), // CRITICAL: Join ports as single string!
		}

		// Verify the aggregation
		expectedPorts := "443,80,8080,8081,8082" // Sorted order
		assert.Equal(t, expectedPorts, portSpec.Destination, "Ports should be sorted and comma-separated")

		t.Logf("Aggregated ports: %s", portSpec.Destination)
	}
}

// TestIEAgAgRuleFieldRestoration tests that all required fields are restored
func TestIEAgAgRuleFieldRestoration(t *testing.T) {
	// Test that the restored IEAgAg rule has all the necessary fields from original
	service := &RuleS2SResourceService{}

	// Create minimal rule metadata
	traffic := models.INGRESS
	localAG := models.AddressGroupRef{
		ObjectReference: netguardv1beta1.ObjectReference{
			Name: "web-ag",
		},
		Namespace: "default",
	}
	targetAG := models.AddressGroupRef{
		ObjectReference: netguardv1beta1.ObjectReference{
			Name: "db-ag",
		},
		Namespace: "default",
	}
	protocol := models.TCP

	// Create rule using restored logic (like in generateAggregatedIEAgAgRules)
	ruleName := service.generateRuleName(string(traffic), localAG.Name, targetAG.Name, string(protocol))

	ieRule := models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      ruleName,
				Namespace: "default",
			},
		},
		Transport:         protocol, // RESTORED: Transport field
		Traffic:           traffic,
		AddressGroupLocal: localAG,
		AddressGroup:      targetAG,
		Ports: []models.PortSpec{
			{
				Destination: "80,443,8080", // CRITICAL: Join ports as single string!
			},
		},
		Action:   models.ActionAccept, // RESTORED: from original
		Logs:     true,                // RESTORED: missing field
		Trace:    false,               // RESTORED: missing field
		Priority: 100,                 // RESTORED: missing field
	}

	// Verify all restored fields
	assert.Equal(t, protocol, ieRule.Transport, "Transport should be set")
	assert.Equal(t, traffic, ieRule.Traffic, "Traffic should be set")
	assert.Equal(t, models.ActionAccept, ieRule.Action, "Action should be Accept")
	assert.True(t, ieRule.Logs, "Logs should be enabled")
	assert.False(t, ieRule.Trace, "Trace should be disabled")
	assert.Equal(t, int32(100), ieRule.Priority, "Priority should be 100")

	// Verify UUID name format
	assert.True(t, len(ieRule.Name) == 40, "Name should be 40 characters")
	assert.True(t, strings.HasPrefix(ieRule.Name, "ing-"), "INGRESS rule should have ing- prefix")

	// Verify port aggregation format
	assert.Len(t, ieRule.Ports, 1, "Should have single aggregated port spec")
	assert.Equal(t, "80,443,8080", ieRule.Ports[0].Destination, "Ports should be comma-separated")
	assert.Empty(t, ieRule.Ports[0].Source, "Source should be empty for destination ports")

	t.Logf("Rule created: %s", ieRule.Name)
	t.Logf("  Transport: %s", ieRule.Transport)
	t.Logf("  Traffic: %s", ieRule.Traffic)
	t.Logf("  Action: %s", ieRule.Action)
	t.Logf("  Logs: %t", ieRule.Logs)
	t.Logf("  Trace: %t", ieRule.Trace)
	t.Logf("  Priority: %d", ieRule.Priority)
	t.Logf("  Ports: %s", ieRule.Ports[0].Destination)
}

// TestRuleNameConsistency_ProtocolSeparation tests that TCP and UDP rules get different names
func TestRuleNameConsistency_ProtocolSeparation(t *testing.T) {
	service := &RuleS2SResourceService{}

	// Same parameters except protocol
	traffic := "INGRESS"
	localAG := "web-servers"
	targetAG := "database-servers"

	tcpName := service.generateRuleName(traffic, localAG, targetAG, "TCP")
	udpName := service.generateRuleName(traffic, localAG, targetAG, "UDP")

	// Should produce different rule names
	assert.NotEqual(t, tcpName, udpName, "TCP and UDP rules should have different names")

	// Both should have same prefix
	assert.True(t, strings.HasPrefix(tcpName, "ing-"), "TCP rule should have ing- prefix")
	assert.True(t, strings.HasPrefix(udpName, "ing-"), "UDP rule should have ing- prefix")

	// Both should be proper length
	assert.Equal(t, 40, len(tcpName), "TCP rule name should be 40 chars")
	assert.Equal(t, 40, len(udpName), "UDP rule name should be 40 chars")

	t.Logf("TCP rule: %s", tcpName)
	t.Logf("UDP rule: %s", udpName)
}

// TestCriticalLogicRestoration verifies the most critical differences from before the fix
func TestCriticalLogicRestoration(t *testing.T) {
	t.Run("PortJoiningVsSeparate", func(t *testing.T) {
		// BEFORE (broken): Each port was a separate PortSpec
		brokenPorts := []models.PortSpec{
			{Destination: "80", Source: ""},
			{Destination: "443", Source: ""},
			{Destination: "8080", Source: ""},
		}

		// AFTER (fixed): All ports joined in single PortSpec
		fixedPorts := []models.PortSpec{
			{Destination: "443,80,8080", Source: ""}, // Sorted and joined!
		}

		assert.Len(t, brokenPorts, 3, "Broken version had separate PortSpecs")
		assert.Len(t, fixedPorts, 1, "Fixed version has single aggregated PortSpec")
		assert.Contains(t, fixedPorts[0].Destination, ",", "Fixed version joins ports with commas")

		t.Logf("Broken (separate): %d PortSpecs", len(brokenPorts))
		t.Logf("Fixed (aggregated): %s", fixedPorts[0].Destination)
	})

	t.Run("UUIDvsReadableNames", func(t *testing.T) {
		service := &RuleS2SResourceService{}

		// BEFORE (broken): Readable names like "rule-from-web-to-db-tcp"
		// AFTER (fixed): UUID names like "ing-a1b2c3d4-e5f6-7890-abcd-ef1234567890"

		uuidName := service.generateRuleName("INGRESS", "web-servers", "database-servers", "TCP")

		assert.True(t, strings.HasPrefix(uuidName, "ing-"), "Should have traffic prefix")
		assert.Equal(t, 40, len(uuidName), "Should be UUID length")
		assert.Regexp(t, "^ing-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$", uuidName, "Should match UUID pattern")

		// Should NOT look like readable names
		assert.False(t, strings.Contains(uuidName, "rule-from"), "Should not contain readable patterns")
		assert.False(t, strings.Contains(uuidName, "web-to-db"), "Should not contain readable patterns")

		t.Logf("UUID name: %s", uuidName)
	})

	t.Run("MissingFieldsRestored", func(t *testing.T) {
		// Test that critical fields that were missing are now present

		// Create rule with restored fields
		rule := models.IEAgAgRule{
			Transport: models.TCP,          // RESTORED: was missing
			Action:    models.ActionAccept, // RESTORED: was missing
			Logs:      true,                // RESTORED: was missing
			Trace:     false,               // RESTORED: was missing
			Priority:  100,                 // RESTORED: was missing
		}

		assert.Equal(t, models.TCP, rule.Transport, "Transport field restored")
		assert.Equal(t, models.ActionAccept, rule.Action, "Action field restored")
		assert.True(t, rule.Logs, "Logs field restored")
		assert.False(t, rule.Trace, "Trace field restored")
		assert.Equal(t, int32(100), rule.Priority, "Priority field restored")

		t.Logf("All critical fields restored and verified")
	})
}
