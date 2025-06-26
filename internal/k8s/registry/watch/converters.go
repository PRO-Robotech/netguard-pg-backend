package watch

import (
	"context"

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
			Description: group.Description,
		},
	}

	// Convert Addresses
	for _, addr := range group.Addresses {
		k8sAddr := netguardv1beta1.Address{
			Address: addr,
		}
		k8sGroup.Spec.Addresses = append(k8sGroup.Spec.Addresses, k8sAddr)
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
	// TODO: реализовать
	return nil
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
	// TODO: реализовать
	return nil
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
