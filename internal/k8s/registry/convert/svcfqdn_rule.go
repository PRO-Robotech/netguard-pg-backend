package convert

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// SvcFqdnRuleConverter converts between K8s SvcFqdnRule objects and domain models
type SvcFqdnRuleConverter struct{}

func NewSvcFqdnRuleConverter() *SvcFqdnRuleConverter {
	return &SvcFqdnRuleConverter{}
}

// ToDomain converts a Kubernetes SvcFqdnRule object to a domain SvcFqdnRule model
func (c *SvcFqdnRuleConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.SvcFqdnRule) (*models.SvcFqdnRule, error) {
	if err := ValidateNilObject(k8sObj, "k8s SvcFqdnRule"); err != nil {
		return nil, err
	}

	action, err := c.convertActionToDomain(string(k8sObj.Spec.Action))
	if err != nil {
		return nil, fmt.Errorf("failed to convert action: %w", err)
	}

	transport, err := c.convertTransportToDomain(string(k8sObj.Spec.Transport))
	if err != nil {
		return nil, fmt.Errorf("failed to convert transport: %w", err)
	}

	ports := make([]models.FqdnPortSpec, 0, len(k8sObj.Spec.Ports))
	for _, port := range k8sObj.Spec.Ports {
		ports = append(ports, models.FqdnPortSpec{
			Port: port.Port,
		})
	}

	domainRule := &models.SvcFqdnRule{
		SelfRef: models.SelfRef{ResourceIdentifier: models.ResourceIdentifier{
			Name:      k8sObj.Name,
			Namespace: k8sObj.Namespace,
		}},
		ServiceFromRef: EnsureNamespacedObjectReferenceFields(k8sObj.Spec.ServiceFrom, "Service"),
		FQDN:           k8sObj.Spec.FQDN,
		Transport:      transport,
		Ports:          ports,
		Logs:           k8sObj.Spec.Logs,
		Trace:          k8sObj.Spec.Trace,
		Action:         action,
		Priority:       k8sObj.Spec.Priority,
		Description:    k8sObj.Spec.Description,
		Meta:           ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	return domainRule, nil
}

// FromDomain converts a domain SvcFqdnRule model to a Kubernetes SvcFqdnRule object
func (c *SvcFqdnRuleConverter) FromDomain(ctx context.Context, domainObj *models.SvcFqdnRule) (*netguardv1beta1.SvcFqdnRule, error) {
	if err := ValidateNilObject(domainObj, "domain SvcFqdnRule"); err != nil {
		return nil, err
	}

	action, err := c.convertActionFromDomain(domainObj.Action)
	if err != nil {
		return nil, fmt.Errorf("failed to convert action: %w", err)
	}

	transport, err := c.convertTransportFromDomain(domainObj.Transport)
	if err != nil {
		return nil, fmt.Errorf("failed to convert transport: %w", err)
	}

	ports := make([]netguardv1beta1.SvcFqdnPortSpec, 0, len(domainObj.Ports))
	for _, port := range domainObj.Ports {
		ports = append(ports, netguardv1beta1.SvcFqdnPortSpec{
			Port: port.Port,
		})
	}

	k8sRule := &netguardv1beta1.SvcFqdnRule{
		TypeMeta:   CreateStandardTypeMetaForResource("SvcFqdnRule"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.SvcFqdnRuleSpec{
			ServiceFrom: EnsureNamespacedObjectReferenceFields(domainObj.ServiceFromRef, "Service"),
			FQDN:        domainObj.FQDN,
			Transport:   transport,
			Ports:       ports,
			Logs:        domainObj.Logs,
			Trace:       domainObj.Trace,
			Action:      action,
			Priority:    domainObj.Priority,
			Description: domainObj.Description,
		},
	}

	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sRule.Status = netguardv1beta1.SvcFqdnRuleStatus{
		ObservedGeneration: observedGeneration,
		Conditions:         conditions,
		SyncReady:          false,
	}

	return k8sRule, nil
}

// ToList converts a slice of domain SvcFqdnRule models to a Kubernetes list object
func (c *SvcFqdnRuleConverter) ToList(ctx context.Context, domainObjs []*models.SvcFqdnRule) (runtime.Object, error) {
	list := &netguardv1beta1.SvcFqdnRuleList{
		TypeMeta: CreateStandardTypeMetaForList("SvcFqdnRuleList"),
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.SvcFqdnRule, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain SvcFqdnRule %d: %w", i, err)
		}
		list.Items[i] = *k8sObj
	}

	return list, nil
}

// Helper conversions

func (c *SvcFqdnRuleConverter) convertActionToDomain(k8sAction string) (models.RuleAction, error) {
	switch k8sAction {
	case string(netguardv1beta1.ActionAccept):
		return models.ActionAccept, nil
	case string(netguardv1beta1.ActionDrop):
		return models.ActionDrop, nil
	case "":
		return models.ActionAccept, nil
	default:
		return "", fmt.Errorf("unknown action: %s", k8sAction)
	}
}

func (c *SvcFqdnRuleConverter) convertActionFromDomain(domainAction models.RuleAction) (netguardv1beta1.RuleAction, error) {
	switch domainAction {
	case models.ActionAccept:
		return netguardv1beta1.ActionAccept, nil
	case models.ActionDrop:
		return netguardv1beta1.ActionDrop, nil
	case "":
		return netguardv1beta1.ActionAccept, nil
	default:
		return "", fmt.Errorf("unknown action: %s", domainAction)
	}
}

func (c *SvcFqdnRuleConverter) convertTransportToDomain(k8sTransport string) (models.TransportProtocol, error) {
	switch k8sTransport {
	case string(netguardv1beta1.ProtocolTCP):
		return models.TCP, nil
	case string(netguardv1beta1.ProtocolUDP):
		return models.UDP, nil
	default:
		return "", fmt.Errorf("unknown transport: %s", k8sTransport)
	}
}

func (c *SvcFqdnRuleConverter) convertTransportFromDomain(domainTransport models.TransportProtocol) (netguardv1beta1.TransportProtocol, error) {
	switch domainTransport {
	case models.TCP:
		return netguardv1beta1.ProtocolTCP, nil
	case models.UDP:
		return netguardv1beta1.ProtocolUDP, nil
	case "":
		return netguardv1beta1.ProtocolTCP, nil
	default:
		return "", fmt.Errorf("unknown transport: %s", domainTransport)
	}
}
