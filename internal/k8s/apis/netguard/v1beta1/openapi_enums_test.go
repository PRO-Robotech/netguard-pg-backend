package v1beta1_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	"github.com/go-openapi/jsonreference"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIEnumDefinitions_StructureAndContent(t *testing.T) {
	// Создаем reference callback
	refCallback := func(path string) spec.Ref {
		return spec.Ref{Ref: jsonreference.MustCreateRef("#/definitions/" + path)}
	}

	t.Run("GetEnumOpenAPIDefinitions_ContainsAllEnumTypes", func(t *testing.T) {
		// Act
		enumDefs := v1beta1.GetEnumOpenAPIDefinitions(refCallback)

		// Assert
		expectedTypes := []string{
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.TransportProtocol",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.Traffic",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.RuleAction",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.HostRegistrationSource",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.AddressGroupRegistrationSource",
		}

		for _, expectedType := range expectedTypes {
			assert.Contains(t, enumDefs, expectedType,
				"Should contain definition for %s", expectedType)
		}

		assert.Len(t, enumDefs, len(expectedTypes),
			"Should contain exactly the expected enum types")
	})

	t.Run("TransportProtocol_HasCorrectEnumValues", func(t *testing.T) {
		// Act
		enumDefs := v1beta1.GetEnumOpenAPIDefinitions(refCallback)
		transportDef, exists := enumDefs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.TransportProtocol"]

		// Assert
		require.True(t, exists, "TransportProtocol definition should exist")

		schema := transportDef.Schema
		assert.Len(t, schema.SchemaProps.Type, 1, "Type should have one element")
		assert.Equal(t, "string", schema.SchemaProps.Type[0],
			"TransportProtocol should be of type string")
		assert.Equal(t, "Transport protocol (TCP or UDP)", schema.SchemaProps.Description,
			"TransportProtocol should have correct description")

		expectedEnums := []interface{}{"TCP", "UDP"}
		assert.Equal(t, expectedEnums, schema.SchemaProps.Enum,
			"TransportProtocol should have correct enum values")
	})

	t.Run("Traffic_HasCorrectEnumValues", func(t *testing.T) {
		// Act
		enumDefs := v1beta1.GetEnumOpenAPIDefinitions(refCallback)
		trafficDef, exists := enumDefs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.Traffic"]

		// Assert
		require.True(t, exists, "Traffic definition should exist")

		schema := trafficDef.Schema
		assert.Len(t, schema.SchemaProps.Type, 1, "Type should have one element")
		assert.Equal(t, "string", schema.SchemaProps.Type[0],
			"Traffic should be of type string")
		assert.Equal(t, "Traffic direction (INGRESS or EGRESS)", schema.SchemaProps.Description,
			"Traffic should have correct description")

		expectedEnums := []interface{}{"INGRESS", "EGRESS"}
		assert.Equal(t, expectedEnums, schema.SchemaProps.Enum,
			"Traffic should have correct enum values")
	})

	t.Run("RuleAction_HasCorrectEnumValues", func(t *testing.T) {
		// Act
		enumDefs := v1beta1.GetEnumOpenAPIDefinitions(refCallback)
		actionDef, exists := enumDefs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.RuleAction"]

		// Assert
		require.True(t, exists, "RuleAction definition should exist")

		schema := actionDef.Schema
		assert.Len(t, schema.SchemaProps.Type, 1, "Type should have one element")
		assert.Equal(t, "string", schema.SchemaProps.Type[0],
			"RuleAction should be of type string")
		assert.Equal(t, "Rule action (ACCEPT or DROP)", schema.SchemaProps.Description,
			"RuleAction should have correct description")

		expectedEnums := []interface{}{"ACCEPT", "DROP"}
		assert.Equal(t, expectedEnums, schema.SchemaProps.Enum,
			"RuleAction should have correct enum values")
	})

	t.Run("HostRegistrationSource_HasCorrectEnumValues", func(t *testing.T) {
		// Act
		enumDefs := v1beta1.GetEnumOpenAPIDefinitions(refCallback)
		hostSourceDef, exists := enumDefs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.HostRegistrationSource"]

		// Assert
		require.True(t, exists, "HostRegistrationSource definition should exist")

		schema := hostSourceDef.Schema
		assert.Len(t, schema.SchemaProps.Type, 1, "Type should have one element")
		assert.Equal(t, "string", schema.SchemaProps.Type[0],
			"HostRegistrationSource should be of type string")
		assert.Equal(t, "Host registration source (spec or binding)", schema.SchemaProps.Description,
			"HostRegistrationSource should have correct description")

		expectedEnums := []interface{}{"spec", "binding"}
		assert.Equal(t, expectedEnums, schema.SchemaProps.Enum,
			"HostRegistrationSource should have correct enum values")
	})

	t.Run("AddressGroupRegistrationSource_HasCorrectEnumValues", func(t *testing.T) {
		// Act
		enumDefs := v1beta1.GetEnumOpenAPIDefinitions(refCallback)
		agSourceDef, exists := enumDefs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.AddressGroupRegistrationSource"]

		// Assert
		require.True(t, exists, "AddressGroupRegistrationSource definition should exist")

		schema := agSourceDef.Schema
		assert.Len(t, schema.SchemaProps.Type, 1, "Type should have one element")
		assert.Equal(t, "string", schema.SchemaProps.Type[0],
			"AddressGroupRegistrationSource should be of type string")
		assert.Equal(t, "AddressGroup registration source (spec or binding)", schema.SchemaProps.Description,
			"AddressGroupRegistrationSource should have correct description")

		expectedEnums := []interface{}{"spec", "binding"}
		assert.Equal(t, expectedEnums, schema.SchemaProps.Enum,
			"AddressGroupRegistrationSource should have correct enum values")
	})
}

func TestGetOpenAPIDefinitionsWithEnums_Integration(t *testing.T) {
	// Создаем reference callback
	refCallback := func(path string) spec.Ref {
		return spec.Ref{Ref: jsonreference.MustCreateRef("#/definitions/" + path)}
	}

	t.Run("IncludesAutoGeneratedDefinitions", func(t *testing.T) {
		// Act
		allDefs := v1beta1.GetOpenAPIDefinitionsWithEnums(refCallback)

		// Assert
		// Проверяем, что базовые автогенерированные определения присутствуют
		expectedBaseDefs := []string{
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.RuleS2S",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.RuleS2SSpec",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.IEAgAgRule",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.IEAgAgRuleSpec",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.IngressPort",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.AddressGroupSpec",
		}

		for _, expectedDef := range expectedBaseDefs {
			assert.Contains(t, allDefs, expectedDef,
				"Should contain auto-generated definition for %s", expectedDef)
		}
	})

	t.Run("IncludesCustomEnumDefinitions", func(t *testing.T) {
		// Act
		allDefs := v1beta1.GetOpenAPIDefinitionsWithEnums(refCallback)

		// Assert
		expectedEnumDefs := []string{
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.TransportProtocol",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.Traffic",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.RuleAction",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.HostRegistrationSource",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.AddressGroupRegistrationSource",
		}

		for _, expectedDef := range expectedEnumDefs {
			assert.Contains(t, allDefs, expectedDef,
				"Should contain custom enum definition for %s", expectedDef)
		}
	})

	t.Run("StructFieldsHaveEnumValues", func(t *testing.T) {
		// Act
		allDefs := v1beta1.GetOpenAPIDefinitionsWithEnums(refCallback)

		// Assert - IEAgAgRuleSpec fields should have enum values
		ieAgAgRuleDef, exists := allDefs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.IEAgAgRuleSpec"]
		require.True(t, exists, "IEAgAgRuleSpec definition should exist")

		// Check transport field
		transportField, exists := ieAgAgRuleDef.Schema.Properties["transport"]
		require.True(t, exists, "transport field should exist")
		assert.Equal(t, []interface{}{"TCP", "UDP"}, transportField.SchemaProps.Enum,
			"transport field should have enum values")

		// Check traffic field
		trafficField, exists := ieAgAgRuleDef.Schema.Properties["traffic"]
		require.True(t, exists, "traffic field should exist")
		assert.Equal(t, []interface{}{"INGRESS", "EGRESS"}, trafficField.SchemaProps.Enum,
			"traffic field should have enum values")

		// Check action field
		actionField, exists := ieAgAgRuleDef.Schema.Properties["action"]
		require.True(t, exists, "action field should exist")
		assert.Equal(t, []interface{}{"ACCEPT", "DROP"}, actionField.SchemaProps.Enum,
			"action field should have enum values")
	})

	t.Run("RuleS2SSpecFieldsHaveEnumValues", func(t *testing.T) {
		// Act
		allDefs := v1beta1.GetOpenAPIDefinitionsWithEnums(refCallback)

		// Assert - RuleS2SSpec traffic field should have enum values
		ruleS2SSpecDef, exists := allDefs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.RuleS2SSpec"]
		require.True(t, exists, "RuleS2SSpec definition should exist")

		trafficField, exists := ruleS2SSpecDef.Schema.Properties["traffic"]
		require.True(t, exists, "traffic field should exist")
		assert.Equal(t, []interface{}{"INGRESS", "EGRESS"}, trafficField.SchemaProps.Enum,
			"RuleS2SSpec traffic field should have enum values")
	})

	t.Run("IngressPortFieldsHaveEnumValues", func(t *testing.T) {
		// Act
		allDefs := v1beta1.GetOpenAPIDefinitionsWithEnums(refCallback)

		// Assert - IngressPort protocol field should have enum values
		ingressPortDef, exists := allDefs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.IngressPort"]
		require.True(t, exists, "IngressPort definition should exist")

		protocolField, exists := ingressPortDef.Schema.Properties["protocol"]
		require.True(t, exists, "protocol field should exist")
		assert.Equal(t, []interface{}{"TCP", "UDP"}, protocolField.SchemaProps.Enum,
			"IngressPort protocol field should have enum values")
	})

	t.Run("AddressGroupSpecFieldsHaveEnumValues", func(t *testing.T) {
		// Act
		allDefs := v1beta1.GetOpenAPIDefinitionsWithEnums(refCallback)

		// Assert - AddressGroupSpec defaultAction field should have enum values
		addressGroupSpecDef, exists := allDefs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.AddressGroupSpec"]
		require.True(t, exists, "AddressGroupSpec definition should exist")

		defaultActionField, exists := addressGroupSpecDef.Schema.Properties["defaultAction"]
		require.True(t, exists, "defaultAction field should exist")
		assert.Equal(t, []interface{}{"ACCEPT", "DROP"}, defaultActionField.SchemaProps.Enum,
			"AddressGroupSpec defaultAction field should have enum values")
	})

	t.Run("SvcSvcRuleSpecFieldsHaveEnumValues", func(t *testing.T) {
		// Act
		allDefs := v1beta1.GetOpenAPIDefinitionsWithEnums(refCallback)

		// Assert - SvcSvcRuleSpec action field should have enum values
		svcSvcRuleSpecDef, exists := allDefs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.SvcSvcRuleSpec"]
		require.True(t, exists, "SvcSvcRuleSpec definition should exist")

		actionField, exists := svcSvcRuleSpecDef.Schema.Properties["action"]
		require.True(t, exists, "action field should exist")
		assert.Equal(t, []interface{}{"ACCEPT", "DROP"}, actionField.SchemaProps.Enum,
			"SvcSvcRuleSpec action field should have enum values")
	})

	t.Run("SvcFqdnRuleSpecFieldsHaveEnumValues", func(t *testing.T) {
		// Act
		allDefs := v1beta1.GetOpenAPIDefinitionsWithEnums(refCallback)

		// Assert - SvcFqdnRuleSpec fields should have enum values
		svcFqdnRuleSpecDef, exists := allDefs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.SvcFqdnRuleSpec"]
		require.True(t, exists, "SvcFqdnRuleSpec definition should exist")

		// Check transport field
		transportField, exists := svcFqdnRuleSpecDef.Schema.Properties["transport"]
		require.True(t, exists, "transport field should exist")
		assert.Equal(t, []interface{}{"TCP", "UDP"}, transportField.SchemaProps.Enum,
			"SvcFqdnRuleSpec transport field should have enum values")

		// Check action field
		actionField, exists := svcFqdnRuleSpecDef.Schema.Properties["action"]
		require.True(t, exists, "action field should exist")
		assert.Equal(t, []interface{}{"ACCEPT", "DROP"}, actionField.SchemaProps.Enum,
			"SvcFqdnRuleSpec action field should have enum values")
	})
}

func TestOpenAPIEnumSerialization_JSONValidation(t *testing.T) {
	// Создаем reference callback
	refCallback := func(path string) spec.Ref {
		return spec.Ref{Ref: jsonreference.MustCreateRef("#/definitions/" + path)}
	}

	t.Run("EnumDefinitions_SerializeToValidJSON", func(t *testing.T) {
		// Act
		enumDefs := v1beta1.GetEnumOpenAPIDefinitions(refCallback)

		// Test each enum definition can be serialized to JSON
		for name, definition := range enumDefs {
			t.Run(name, func(t *testing.T) {
				// Act
				jsonData, err := json.Marshal(definition.Schema)

				// Assert
				require.NoError(t, err, "Schema should serialize to JSON without error")

				// Verify we can parse it back
				var parsedSchema spec.Schema
				err = json.Unmarshal(jsonData, &parsedSchema)
				require.NoError(t, err, "JSON should parse back to schema")

				// Verify enum field is preserved
				assert.NotEmpty(t, parsedSchema.SchemaProps.Enum,
					"Enum values should be preserved in JSON serialization")
			})
		}
	})

	t.Run("ModifiedStructDefinitions_SerializeToValidJSON", func(t *testing.T) {
		// Act
		allDefs := v1beta1.GetOpenAPIDefinitionsWithEnums(refCallback)

		// Test key modified definitions
		modifiedDefs := []string{
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.IEAgAgRuleSpec",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.RuleS2SSpec",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.IngressPort",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.AddressGroupSpec",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.SvcSvcRuleSpec",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.SvcFqdnRuleSpec",
		}

		for _, defName := range modifiedDefs {
			t.Run(defName, func(t *testing.T) {
				definition, exists := allDefs[defName]
				require.True(t, exists, "Definition should exist")

				// Act
				jsonData, err := json.Marshal(definition.Schema)

				// Assert
				require.NoError(t, err, "Modified schema should serialize to JSON")

				var parsedSchema spec.Schema
				err = json.Unmarshal(jsonData, &parsedSchema)
				require.NoError(t, err, "JSON should parse back to schema")

				// Verify properties with enum values exist
				hasEnumFields := false
				for fieldName, fieldSchema := range parsedSchema.Properties {
					if len(fieldSchema.SchemaProps.Enum) > 0 {
						hasEnumFields = true
						t.Logf("Field %s has enum values: %v", fieldName, fieldSchema.SchemaProps.Enum)
					}
				}
				assert.True(t, hasEnumFields, "Should have at least one field with enum values")
			})
		}
	})
}

func TestOpenAPIEnumDefinitions_CallbackUsage(t *testing.T) {
	t.Run("ReferenceCallback_IsUsedCorrectly", func(t *testing.T) {
		// Track callback usage
		callbackCalls := make(map[string]int)
		refCallback := func(path string) spec.Ref {
			callbackCalls[path]++
			return spec.Ref{Ref: jsonreference.MustCreateRef("#/definitions/" + path)}
		}

		// Act
		_ = v1beta1.GetOpenAPIDefinitionsWithEnums(refCallback)

		// Assert - callback should be used for struct field references
		// but not necessarily for our enum types (since they're standalone)
		// This verifies our callback integration is working
		assert.NotEmpty(t, callbackCalls, "Reference callback should be used")

		// Log the calls for debugging
		for path, count := range callbackCalls {
			t.Logf("Reference callback called %d times for path: %s", count, path)
		}
	})

	t.Run("GetOpenAPIDefinitionsWithEnums_DifferentCallbacks_SameResult", func(t *testing.T) {
		// Create two different callbacks that produce same refs
		callback1 := func(path string) spec.Ref {
			return spec.Ref{Ref: jsonreference.MustCreateRef("#/definitions/" + path)}
		}
		callback2 := func(path string) spec.Ref {
			return spec.Ref{Ref: jsonreference.MustCreateRef("#/definitions/" + path)}
		}

		// Act
		defs1 := v1beta1.GetOpenAPIDefinitionsWithEnums(callback1)
		defs2 := v1beta1.GetOpenAPIDefinitionsWithEnums(callback2)

		// Assert - both should produce same enum definitions
		enumTypes := []string{
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.TransportProtocol",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.Traffic",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.RuleAction",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.HostRegistrationSource",
			"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.AddressGroupRegistrationSource",
		}

		for _, enumType := range enumTypes {
			def1, exists1 := defs1[enumType]
			def2, exists2 := defs2[enumType]

			assert.True(t, exists1, "Enum type should exist in first result")
			assert.True(t, exists2, "Enum type should exist in second result")
			assert.True(t, reflect.DeepEqual(def1.Schema.SchemaProps.Enum, def2.Schema.SchemaProps.Enum),
				"Enum values should be identical regardless of callback")
		}
	})
}
