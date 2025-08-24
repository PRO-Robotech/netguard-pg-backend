/*
Copyright 2025 The Netguard Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// GetEnumOpenAPIDefinitions returns OpenAPI definitions with enum support for our custom types
func GetEnumOpenAPIDefinitions(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return map[string]common.OpenAPIDefinition{
		"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.TransportProtocol": {
			Schema: spec.Schema{
				SchemaProps: spec.SchemaProps{
					Description: "Transport protocol (TCP or UDP)",
					Type:        []string{"string"},
					Enum: []interface{}{
						"TCP",
						"UDP",
					},
				},
			},
		},

		"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.Traffic": {
			Schema: spec.Schema{
				SchemaProps: spec.SchemaProps{
					Description: "Traffic direction (INGRESS or EGRESS)",
					Type:        []string{"string"},
					Enum: []interface{}{
						"INGRESS",
						"EGRESS",
					},
				},
			},
		},

		"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.RuleAction": {
			Schema: spec.Schema{
				SchemaProps: spec.SchemaProps{
					Description: "Rule action (ACCEPT or DROP)",
					Type:        []string{"string"},
					Enum: []interface{}{
						"ACCEPT",
						"DROP",
					},
				},
			},
		},
	}
}

// GetOpenAPIDefinitionsWithEnums returns all OpenAPI definitions including our custom enum types
func GetOpenAPIDefinitionsWithEnums(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	defs := GetOpenAPIDefinitions(ref)
	enumDefs := GetEnumOpenAPIDefinitions(ref)
	for key, value := range enumDefs {
		defs[key] = value
	}

	modifyStructFieldsWithEnums(defs)

	return defs
}

// modifyStructFieldsWithEnums modifies only the enum fields in existing struct definitions
func modifyStructFieldsWithEnums(defs map[string]common.OpenAPIDefinition) {
	if ieSpec, exists := defs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.IEAgAgRuleSpec"]; exists {
		if ieSpec.Schema.Properties != nil {
			if transportProp, ok := ieSpec.Schema.Properties["transport"]; ok {
				transportProp.SchemaProps.Enum = []interface{}{"TCP", "UDP"}
				transportProp.SchemaProps.Description = "Transport protocol (TCP or UDP)"
				ieSpec.Schema.Properties["transport"] = transportProp
			}
			if trafficProp, ok := ieSpec.Schema.Properties["traffic"]; ok {
				trafficProp.SchemaProps.Enum = []interface{}{"INGRESS", "EGRESS"}
				trafficProp.SchemaProps.Description = "Traffic direction (INGRESS or EGRESS)"
				ieSpec.Schema.Properties["traffic"] = trafficProp
			}

			if actionProp, ok := ieSpec.Schema.Properties["action"]; ok {
				actionProp.SchemaProps.Enum = []interface{}{"ACCEPT", "DROP"}
				actionProp.SchemaProps.Description = "Action for the rule (ACCEPT or DROP)"
				ieSpec.Schema.Properties["action"] = actionProp
			}

			defs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.IEAgAgRuleSpec"] = ieSpec
		}
	}

	if r2sSpec, exists := defs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.RuleS2SSpec"]; exists {
		if r2sSpec.Schema.Properties != nil {
			if trafficProp, ok := r2sSpec.Schema.Properties["traffic"]; ok {
				trafficProp.SchemaProps.Enum = []interface{}{"INGRESS", "EGRESS"}
				trafficProp.SchemaProps.Description = "Traffic direction (INGRESS or EGRESS)"
				r2sSpec.Schema.Properties["traffic"] = trafficProp
			}

			defs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.RuleS2SSpec"] = r2sSpec
		}
	}

	if ingressPort, exists := defs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.IngressPort"]; exists {
		if ingressPort.Schema.Properties != nil {
			if protocolProp, ok := ingressPort.Schema.Properties["protocol"]; ok {
				protocolProp.SchemaProps.Enum = []interface{}{"TCP", "UDP"}
				protocolProp.SchemaProps.Description = "Transport protocol (TCP or UDP)"
				ingressPort.Schema.Properties["protocol"] = protocolProp
			}

			defs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.IngressPort"] = ingressPort
		}
	}

	if agSpec, exists := defs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.AddressGroupSpec"]; exists {
		if agSpec.Schema.Properties != nil {
			if actionProp, ok := agSpec.Schema.Properties["defaultAction"]; ok {
				actionProp.SchemaProps.Enum = []interface{}{"ACCEPT", "DROP"}
				actionProp.SchemaProps.Description = "Default action for the address group (ACCEPT or DROP)"
				agSpec.Schema.Properties["defaultAction"] = actionProp
			}

			defs["netguard-pg-backend/internal/k8s/apis/netguard/v1beta1.AddressGroupSpec"] = agSpec
		}
	}
}
