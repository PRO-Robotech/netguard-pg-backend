package watch

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
)

// ServiceConverter конвертер для Service ресурсов
type ServiceConverter struct{}

func (c *ServiceConverter) ConvertToK8s(resource interface{}) runtime.Object {
	service, ok := resource.(models.Service)
	if !ok {
		return nil
	}

	k8sService := &netguardv1beta1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "netguard.sgroups.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.ResourceIdentifier.Name,
			Namespace: service.ResourceIdentifier.Namespace,
		},
		Spec: netguardv1beta1.ServiceSpec{
			Description: service.Description,
		},
	}

	// Convert IngressPorts
	for _, port := range service.IngressPorts {
		k8sPort := netguardv1beta1.IngressPort{
			Protocol:    netguardv1beta1.TransportProtocol(port.Protocol),
			Port:        port.Port,
			Description: port.Description,
		}
		k8sService.Spec.IngressPorts = append(k8sService.Spec.IngressPorts, k8sPort)
	}

	return k8sService
}

func (c *ServiceConverter) ListResources(ctx context.Context, backend client.BackendClient) ([]interface{}, error) {
	services, err := backend.ListServices(ctx, nil)
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, len(services))
	for i, service := range services {
		result[i] = service
	}
	return result, nil
}

func (c *ServiceConverter) GetResourceKey(resource interface{}) string {
	service, ok := resource.(models.Service)
	if !ok {
		return ""
	}
	return service.ResourceIdentifier.Key()
}

// AddressGroupConverter конвертер для AddressGroup ресурсов
type AddressGroupConverter struct{}

func (c *AddressGroupConverter) ConvertToK8s(resource interface{}) runtime.Object {
	group, ok := resource.(models.AddressGroup)
	if !ok {
		return nil
	}

	k8sGroup := &netguardv1beta1.AddressGroup{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AddressGroup",
			APIVersion: "netguard.sgroups.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      group.ResourceIdentifier.Name,
			Namespace: group.ResourceIdentifier.Namespace,
		},
		Spec: netguardv1beta1.AddressGroupSpec{
			DefaultAction: netguardv1beta1.RuleAction(group.DefaultAction),
			Logs:          group.Logs,
			Trace:         group.Trace,
		},
	}

	return k8sGroup
}

func (c *AddressGroupConverter) ListResources(ctx context.Context, backend client.BackendClient) ([]interface{}, error) {
	groups, err := backend.ListAddressGroups(ctx, nil)
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, len(groups))
	for i, group := range groups {
		result[i] = group
	}
	return result, nil
}

func (c *AddressGroupConverter) GetResourceKey(resource interface{}) string {
	group, ok := resource.(models.AddressGroup)
	if !ok {
		return ""
	}
	return group.ResourceIdentifier.Key()
}

// Заглушки для остальных конверторов (TODO: реализовать полностью)

type AddressGroupBindingConverter struct{}

func (c *AddressGroupBindingConverter) ConvertToK8s(resource interface{}) runtime.Object {
	// TODO: реализовать
	return nil
}

func (c *AddressGroupBindingConverter) ListResources(ctx context.Context, backend client.BackendClient) ([]interface{}, error) {
	bindings, err := backend.ListAddressGroupBindings(ctx, nil)
	if err != nil {
		return nil, err
	}
	result := make([]interface{}, len(bindings))
	for i, binding := range bindings {
		result[i] = binding
	}
	return result, nil
}

func (c *AddressGroupBindingConverter) GetResourceKey(resource interface{}) string {
	binding, ok := resource.(models.AddressGroupBinding)
	if !ok {
		return ""
	}
	return binding.ResourceIdentifier.Key()
}

type AddressGroupPortMappingConverter struct{}

func (c *AddressGroupPortMappingConverter) ConvertToK8s(resource interface{}) runtime.Object {
	// TODO: реализовать
	return nil
}

func (c *AddressGroupPortMappingConverter) ListResources(ctx context.Context, backend client.BackendClient) ([]interface{}, error) {
	mappings, err := backend.ListAddressGroupPortMappings(ctx, nil)
	if err != nil {
		return nil, err
	}
	result := make([]interface{}, len(mappings))
	for i, mapping := range mappings {
		result[i] = mapping
	}
	return result, nil
}

func (c *AddressGroupPortMappingConverter) GetResourceKey(resource interface{}) string {
	mapping, ok := resource.(models.AddressGroupPortMapping)
	if !ok {
		return ""
	}
	return mapping.ResourceIdentifier.Key()
}

type RuleS2SConverter struct{}

func (c *RuleS2SConverter) ConvertToK8s(resource interface{}) runtime.Object {
	// TODO: реализовать
	return nil
}

func (c *RuleS2SConverter) ListResources(ctx context.Context, backend client.BackendClient) ([]interface{}, error) {
	rules, err := backend.ListRuleS2S(ctx, nil)
	if err != nil {
		return nil, err
	}
	result := make([]interface{}, len(rules))
	for i, rule := range rules {
		result[i] = rule
	}
	return result, nil
}

func (c *RuleS2SConverter) GetResourceKey(resource interface{}) string {
	rule, ok := resource.(models.RuleS2S)
	if !ok {
		return ""
	}
	return rule.ResourceIdentifier.Key()
}

type ServiceAliasConverter struct{}

func (c *ServiceAliasConverter) ConvertToK8s(resource interface{}) runtime.Object {
	// TODO: реализовать
	return nil
}

func (c *ServiceAliasConverter) ListResources(ctx context.Context, backend client.BackendClient) ([]interface{}, error) {
	aliases, err := backend.ListServiceAliases(ctx, nil)
	if err != nil {
		return nil, err
	}
	result := make([]interface{}, len(aliases))
	for i, alias := range aliases {
		result[i] = alias
	}
	return result, nil
}

func (c *ServiceAliasConverter) GetResourceKey(resource interface{}) string {
	alias, ok := resource.(models.ServiceAlias)
	if !ok {
		return ""
	}
	return alias.ResourceIdentifier.Key()
}

type AddressGroupBindingPolicyConverter struct{}

func (c *AddressGroupBindingPolicyConverter) ConvertToK8s(resource interface{}) runtime.Object {
	pol, ok := resource.(models.AddressGroupBindingPolicy)
	if !ok {
		return nil
	}
	return &netguardv1beta1.AddressGroupBindingPolicy{
		TypeMeta: metav1.TypeMeta{Kind: "AddressGroupBindingPolicy", APIVersion: "netguard.sgroups.io/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pol.ResourceIdentifier.Name,
			Namespace: pol.ResourceIdentifier.Namespace,
		},
		Spec: netguardv1beta1.AddressGroupBindingPolicySpec{
			AddressGroupRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{APIVersion: "netguard.sgroups.io/v1beta1", Kind: "AddressGroup", Name: pol.AddressGroupRef.Name},
				Namespace:       pol.AddressGroupRef.Namespace,
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{APIVersion: "netguard.sgroups.io/v1beta1", Kind: "Service", Name: pol.ServiceRef.Name},
				Namespace:       pol.ServiceRef.Namespace,
			},
		},
	}
}

func (c *AddressGroupBindingPolicyConverter) ListResources(ctx context.Context, backend client.BackendClient) ([]interface{}, error) {
	policies, err := backend.ListAddressGroupBindingPolicies(ctx, nil)
	if err != nil {
		return nil, err
	}
	result := make([]interface{}, len(policies))
	for i, policy := range policies {
		result[i] = policy
	}
	return result, nil
}

func (c *AddressGroupBindingPolicyConverter) GetResourceKey(resource interface{}) string {
	policy, ok := resource.(models.AddressGroupBindingPolicy)
	if !ok {
		return ""
	}
	return policy.ResourceIdentifier.Key()
}

type IEAgAgRuleConverter struct{}

func (c *IEAgAgRuleConverter) ConvertToK8s(resource interface{}) runtime.Object {
	rule, ok := resource.(models.IEAgAgRule)
	if !ok {
		return nil
	}

	k8sRule := &netguardv1beta1.IEAgAgRule{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IEAgAgRule",
			APIVersion: "netguard.sgroups.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        rule.ResourceIdentifier.Name,
			Namespace:   rule.ResourceIdentifier.Namespace,
			Labels:      rule.Meta.Labels,
			Annotations: rule.Meta.Annotations,
		},
		Spec: netguardv1beta1.IEAgAgRuleSpec{
			Description: fmt.Sprintf("IEAgAgRule: %s traffic from %s to %s",
				rule.Traffic, rule.AddressGroupLocal.Name, rule.AddressGroup.Name),
			Transport: convertTransportFromDomain(rule.Transport),
			Traffic:   convertTrafficFromDomain(rule.Traffic),
			AddressGroupLocal: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       rule.AddressGroupLocal.Name,
				},
				Namespace: rule.Namespace,
			},
			AddressGroup: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       rule.AddressGroup.Name,
				},
				Namespace: rule.Namespace,
			},
			Action:   netguardv1beta1.RuleAction(rule.Action),
			Priority: rule.Priority,
		},
		Status: netguardv1beta1.IEAgAgRuleStatus{
			ObservedGeneration: rule.Meta.ObservedGeneration,
			Conditions:         rule.Meta.Conditions,
		},
	}

	// Convert ports
	for _, portSpec := range rule.Ports {
		if portSpec.Destination != "" {
			ports := strings.Split(portSpec.Destination, ",")
			for _, portStr := range ports {
				portStr = strings.TrimSpace(portStr)
				if portStr == "" {
					continue
				}

				k8sPortSpec := netguardv1beta1.PortSpec{}

				if strings.Contains(portStr, "-") {
					// Port range
					var from, to int32
					if _, err := fmt.Sscanf(portStr, "%d-%d", &from, &to); err == nil {
						k8sPortSpec.PortRange = &netguardv1beta1.PortRange{
							From: from,
							To:   to,
						}
					}
				} else {
					// Single port
					var port int32
					if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil {
						k8sPortSpec.Port = port
					}
				}

				k8sRule.Spec.Ports = append(k8sRule.Spec.Ports, k8sPortSpec)
			}
		}
	}

	return k8sRule
}

func (c *IEAgAgRuleConverter) ListResources(ctx context.Context, backend client.BackendClient) ([]interface{}, error) {
	rules, err := backend.ListIEAgAgRules(ctx, nil)
	if err != nil {
		return nil, err
	}
	result := make([]interface{}, len(rules))
	for i, rule := range rules {
		result[i] = rule
	}
	return result, nil
}

func (c *IEAgAgRuleConverter) GetResourceKey(resource interface{}) string {
	rule, ok := resource.(models.IEAgAgRule)
	if !ok {
		return ""
	}
	return rule.ResourceIdentifier.Key()
}

// Helper functions for enum conversion
func convertTransportFromDomain(domainTransport models.TransportProtocol) netguardv1beta1.TransportProtocol {
	switch domainTransport {
	case models.TCP:
		return netguardv1beta1.ProtocolTCP
	case models.UDP:
		return netguardv1beta1.ProtocolUDP
	default:
		return netguardv1beta1.ProtocolTCP // default
	}
}

func convertTrafficFromDomain(domainTraffic models.Traffic) netguardv1beta1.Traffic {
	switch domainTraffic {
	case models.INGRESS:
		return netguardv1beta1.INGRESS
	case models.EGRESS:
		return netguardv1beta1.EGRESS
	default:
		return netguardv1beta1.INGRESS // default
	}
}
