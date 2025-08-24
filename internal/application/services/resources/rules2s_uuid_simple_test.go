package resources

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGenerateRuleName_Core tests the core UUID generation function in isolation
func TestGenerateRuleName_Core(t *testing.T) {
	// Create a minimal service instance to test the method
	service := &RuleS2SResourceService{}

	tests := []struct {
		name         string
		traffic      string
		localAGName  string
		targetAGName string
		protocol     string
		wantPrefix   string
	}{
		{
			name:         "INGRESS TCP rule",
			traffic:      "INGRESS",
			localAGName:  "web-servers",
			targetAGName: "database-servers",
			protocol:     "TCP",
			wantPrefix:   "ing-",
		},
		{
			name:         "EGRESS UDP rule",
			traffic:      "EGRESS",
			localAGName:  "api-servers",
			targetAGName: "cache-servers",
			protocol:     "UDP",
			wantPrefix:   "egr-",
		},
		{
			name:         "Case normalization",
			traffic:      "Ingress",
			localAGName:  "Web-Servers",
			targetAGName: "Database-Servers",
			protocol:     "tcp",
			wantPrefix:   "ing-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result1 := service.generateRuleName(tt.traffic, tt.localAGName, tt.targetAGName, tt.protocol)
			result2 := service.generateRuleName(tt.traffic, tt.localAGName, tt.targetAGName, tt.protocol)

			// Test determinism
			assert.Equal(t, result1, result2, "generateRuleName should be deterministic")

			// Test prefix
			assert.True(t, strings.HasPrefix(result1, tt.wantPrefix), "Rule name should start with correct prefix: %s", tt.wantPrefix)

			// Test length (prefix + dash + UUID = 3 + 1 + 36 = 40)
			assert.Equal(t, 40, len(result1), "Rule name should be 40 characters")

			// Test UUID format
			uuidPart := result1[4:]
			assert.Regexp(t, "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$", uuidPart)

			t.Logf("Generated: %s", result1)
		})
	}
}

// TestGenerateRuleName_SHA256Consistency verifies the SHA256 hash is computed correctly
func TestGenerateRuleName_SHA256Consistency(t *testing.T) {
	service := &RuleS2SResourceService{}

	// Test known SHA256 computation
	traffic := "ingress"
	localAG := "test-ag-1"
	targetAG := "test-ag-2"
	protocol := "tcp"

	// Generate rule name
	result := service.generateRuleName(traffic, localAG, targetAG, protocol)

	// Manually compute expected hash for verification
	input := fmt.Sprintf("%s-%s-%s-%s", strings.ToLower(traffic), localAG, targetAG, strings.ToLower(protocol))
	h := sha256.New()
	h.Write([]byte(input))
	hash := h.Sum(nil)

	expectedUUID := fmt.Sprintf("%x-%x-%x-%x-%x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
	expectedName := fmt.Sprintf("%s-%s", strings.ToLower(traffic)[:3], expectedUUID)

	assert.Equal(t, expectedName, result, "Generated name should match manually computed hash")
	t.Logf("Input: %s", input)
	t.Logf("Expected: %s", expectedName)
	t.Logf("Actual: %s", result)
}

// TestGenerateRuleName_ParameterSensitivity verifies different inputs produce different outputs
func TestGenerateRuleName_ParameterSensitivity(t *testing.T) {
	service := &RuleS2SResourceService{}

	base := service.generateRuleName("INGRESS", "web", "db", "TCP")

	variations := []struct {
		name   string
		args   []string
		expect string
	}{
		{"traffic change", []string{"EGRESS", "web", "db", "TCP"}, "different"},
		{"local AG change", []string{"INGRESS", "api", "db", "TCP"}, "different"},
		{"target AG change", []string{"INGRESS", "web", "cache", "TCP"}, "different"},
		{"protocol change", []string{"INGRESS", "web", "db", "UDP"}, "different"},
		{"identical params", []string{"INGRESS", "web", "db", "TCP"}, "same"},
	}

	for _, v := range variations {
		t.Run(v.name, func(t *testing.T) {
			result := service.generateRuleName(v.args[0], v.args[1], v.args[2], v.args[3])

			if v.expect == "same" {
				assert.Equal(t, base, result, "Identical parameters should produce same result")
			} else {
				assert.NotEqual(t, base, result, "Different parameters should produce different results")
			}

			t.Logf("Base: %s, Variant: %s", base, result)
		})
	}
}
