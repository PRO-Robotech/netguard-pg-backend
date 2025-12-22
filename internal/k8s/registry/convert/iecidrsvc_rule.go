package convert

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// IECidrSvcRuleConverter converts between K8s IECidrSvcRule objects and domain models
type IECidrSvcRuleConverter struct{}

func NewIECidrSvcRuleConverter() *IECidrSvcRuleConverter {
	return &IECidrSvcRuleConverter{}
}

// ToDomain converts a Kubernetes IECidrSvcRule object to a domain IECidrSvcRule model
func (c *IECidrSvcRuleConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.IECidrSvcRule) (*models.IECidrSvcRule, error) {
	if err := ValidateNilObject(k8sObj, "k8s IECidrSvcRule"); err != nil {
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

	traffic, err := c.convertTrafficToDomain(string(k8sObj.Spec.Traffic))
	if err != nil {
		return nil, fmt.Errorf("failed to convert traffic: %w", err)
	}

	ports := make([]models.IECidrSvcPortSpec, 0, len(k8sObj.Spec.Ports))
	for _, port := range k8sObj.Spec.Ports {
		ports = append(ports, models.IECidrSvcPortSpec{
			S: port.S,
			D: port.D,
		})
	}

	domainRule := &models.IECidrSvcRule{
		SelfRef: models.SelfRef{ResourceIdentifier: models.ResourceIdentifier{
			Name:      k8sObj.Name,
			Namespace: k8sObj.Namespace,
		}},
		Transport:   transport,
		CIDR:        k8sObj.Spec.CIDR,
		ServiceRef:  EnsureNamespacedObjectReferenceFields(k8sObj.Spec.Svc, "Service"),
		Traffic:     traffic,
		Ports:       ports,
		Logs:        k8sObj.Spec.Logs,
		Trace:       k8sObj.Spec.Trace,
		Action:      action,
		Priority:    k8sObj.Spec.Priority,
		Description: k8sObj.Spec.Description,
		Comment:     k8sObj.Spec.Comment,
		Meta:        ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	return domainRule, nil
}

// FromDomain converts a domain IECidrSvcRule model to a Kubernetes IECidrSvcRule object
func (c *IECidrSvcRuleConverter) FromDomain(ctx context.Context, domainObj *models.IECidrSvcRule) (*netguardv1beta1.IECidrSvcRule, error) {
	if err := ValidateNilObject(domainObj, "domain IECidrSvcRule"); err != nil {
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

	traffic, err := c.convertTrafficFromDomain(domainObj.Traffic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert traffic: %w", err)
	}

	ports := make([]netguardv1beta1.IECidrSvcPortSpec, 0, len(domainObj.Ports))
	for _, port := range domainObj.Ports {
		ports = append(ports, netguardv1beta1.IECidrSvcPortSpec{
			S: port.S,
			D: port.D,
		})
	}

	k8sRule := &netguardv1beta1.IECidrSvcRule{
		TypeMeta:   CreateStandardTypeMetaForResource("IECidrSvcRule"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.IECidrSvcRuleSpec{
			Transport:   transport,
			CIDR:        domainObj.CIDR,
			Svc:         EnsureNamespacedObjectReferenceFields(domainObj.ServiceRef, "Service"),
			Traffic:     traffic,
			Ports:       ports,
			Logs:        domainObj.Logs,
			Trace:       domainObj.Trace,
			Action:      action,
			Priority:    domainObj.Priority,
			Description: domainObj.Description,
			Comment:     domainObj.Comment,
		},
	}

	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sRule.Status = netguardv1beta1.IECidrSvcRuleStatus{
		ObservedGeneration: observedGeneration,
		Conditions:         conditions,
		SyncReady:          false,
	}

	return k8sRule, nil
}

// ToList converts a slice of domain IECidrSvcRule models to a Kubernetes list object
func (c *IECidrSvcRuleConverter) ToList(ctx context.Context, domainObjs []*models.IECidrSvcRule) (runtime.Object, error) {
	list := &netguardv1beta1.IECidrSvcRuleList{
		TypeMeta: CreateStandardTypeMetaForList("IECidrSvcRuleList"),
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.IECidrSvcRule, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain IECidrSvcRule %d: %w", i, err)
		}
		list.Items[i] = *k8sObj
	}

	return list, nil
}

// Helper conversions

func (c *IECidrSvcRuleConverter) convertActionToDomain(k8sAction string) (models.RuleAction, error) {
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

func (c *IECidrSvcRuleConverter) convertActionFromDomain(domainAction models.RuleAction) (netguardv1beta1.RuleAction, error) {
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

func (c *IECidrSvcRuleConverter) convertTransportToDomain(k8sTransport string) (models.TransportProtocol, error) {
	switch k8sTransport {
	case string(netguardv1beta1.ProtocolTCP):
		return models.TCP, nil
	case string(netguardv1beta1.ProtocolUDP):
		return models.UDP, nil
	default:
		return "", fmt.Errorf("unknown transport: %s", k8sTransport)
	}
}

func (c *IECidrSvcRuleConverter) convertTransportFromDomain(domainTransport models.TransportProtocol) (netguardv1beta1.TransportProtocol, error) {
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

func (c *IECidrSvcRuleConverter) convertTrafficToDomain(k8sTraffic string) (models.Traffic, error) {
	switch k8sTraffic {
	case string(netguardv1beta1.INGRESS):
		return models.INGRESS, nil
	case string(netguardv1beta1.EGRESS):
		return models.EGRESS, nil
	default:
		return "", fmt.Errorf("unknown traffic direction: %s", k8sTraffic)
	}
}

func (c *IECidrSvcRuleConverter) convertTrafficFromDomain(domainTraffic models.Traffic) (netguardv1beta1.Traffic, error) {
	switch domainTraffic {
	case models.INGRESS:
		return netguardv1beta1.INGRESS, nil
	case models.EGRESS:
		return netguardv1beta1.EGRESS, nil
	default:
		return "", fmt.Errorf("unknown traffic direction: %s", domainTraffic)
	}
}
