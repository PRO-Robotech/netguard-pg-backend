package convert

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// IEAgAgRuleConverter implements conversion between k8s IEAgAgRule and domain IEAgAgRule
type IEAgAgRuleConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.IEAgAgRule, *models.IEAgAgRule] = &IEAgAgRuleConverter{}

// ToDomain converts a Kubernetes IEAgAgRule object to a domain IEAgAgRule model
func (c *IEAgAgRuleConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.IEAgAgRule) (*models.IEAgAgRule, error) {
	if err := ValidateNilObject(k8sObj, "k8s IEAgAgRule"); err != nil {
		return nil, err
	}

	// Convert Transport protocol
	transport, err := c.convertTransportToDomain(k8sObj.Spec.Transport)
	if err != nil {
		return nil, fmt.Errorf("failed to convert transport: %w", err)
	}

	// Convert Traffic direction
	traffic, err := c.convertTrafficToDomain(k8sObj.Spec.Traffic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert traffic: %w", err)
	}

	// Convert Action
	action, err := c.convertActionToDomain(string(k8sObj.Spec.Action))
	if err != nil {
		return nil, fmt.Errorf("failed to convert action: %w", err)
	}

	// Create domain IEAgAgRule
	domainRule := &models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		Transport:         transport,
		Traffic:           traffic,
		AddressGroupLocal: k8sObj.Spec.AddressGroupLocal,
		AddressGroup:      k8sObj.Spec.AddressGroup,
		Action:            action,
		Logs:              false,             // Not exposed in k8s API for now
		Trace:             k8sObj.Spec.Trace, // Copy trace field from spec
		Priority:          k8sObj.Spec.Priority,
		Meta:              ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	// Convert ports
	if len(k8sObj.Spec.Ports) > 0 {
		domainRule.Ports = make([]models.PortSpec, len(k8sObj.Spec.Ports))
		for i, portSpec := range k8sObj.Spec.Ports {
			domainPortSpec := models.PortSpec{
				Destination: fmt.Sprintf("%d", portSpec.Port),
			}

			// Handle port range
			if portSpec.PortRange != nil {
				domainPortSpec.Destination = fmt.Sprintf("%d-%d", portSpec.PortRange.From, portSpec.PortRange.To)
			}

			domainRule.Ports[i] = domainPortSpec
		}
	}

	// Metadata already converted by ConvertMetadataToDomain helper

	return domainRule, nil
}

// FromDomain converts a domain IEAgAgRule model to a Kubernetes IEAgAgRule object
func (c *IEAgAgRuleConverter) FromDomain(ctx context.Context, domainObj *models.IEAgAgRule) (*netguardv1beta1.IEAgAgRule, error) {
	if err := ValidateNilObject(domainObj, "domain IEAgAgRule"); err != nil {
		return nil, err
	}

	// Convert Transport protocol
	transport, err := c.convertTransportFromDomain(domainObj.Transport)
	if err != nil {
		return nil, fmt.Errorf("failed to convert transport: %w", err)
	}

	// Convert Traffic direction
	traffic, err := c.convertTrafficFromDomain(domainObj.Traffic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert traffic: %w", err)
	}

	// Convert Action
	action, err := c.convertActionFromDomain(domainObj.Action)
	if err != nil {
		return nil, fmt.Errorf("failed to convert action: %w", err)
	}

	// Create k8s IEAgAgRule with standard metadata conversion
	k8sRule := &netguardv1beta1.IEAgAgRule{
		TypeMeta:   CreateStandardTypeMetaForResource("IEAgAgRule"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.IEAgAgRuleSpec{
			Description: fmt.Sprintf("IEAgAgRule: %s traffic from %s to %s",
				domainObj.Traffic, domainObj.AddressGroupLocal.Name, domainObj.AddressGroup.Name),
			Transport:         transport,
			Traffic:           traffic,
			AddressGroupLocal: EnsureNamespacedObjectReferenceFields(domainObj.AddressGroupLocal, "AddressGroup"),
			AddressGroup:      EnsureNamespacedObjectReferenceFields(domainObj.AddressGroup, "AddressGroup"),
			Action:            action,
			Trace:             domainObj.Trace, // Copy trace field from domain
			Priority:          domainObj.Priority,
		},
	}

	// Status will be set at the end to avoid duplication

	// Convert ports
	if len(domainObj.Ports) > 0 {
		for _, portSpec := range domainObj.Ports {
			// Handle comma-separated ports in destination
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
						_, err := fmt.Sscanf(portStr, "%d-%d", &from, &to)
						if err != nil {
							continue
						}
						k8sPortSpec.PortRange = &netguardv1beta1.PortRange{
							From: from,
							To:   to,
						}
					} else {
						// Single port
						var port int32
						_, err := fmt.Sscanf(portStr, "%d", &port)
						if err != nil {
							continue
						}
						k8sPortSpec.Port = port
					}

					k8sRule.Spec.Ports = append(k8sRule.Spec.Ports, k8sPortSpec)
				}
			}
		}
	}

	// Convert status using standard helper
	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sRule.Status = netguardv1beta1.IEAgAgRuleStatus{
		ObservedGeneration: observedGeneration,
		Conditions:         conditions,
	}

	return k8sRule, nil
}

// ToList converts a slice of domain IEAgAgRule models to a Kubernetes IEAgAgRuleList object
func (c *IEAgAgRuleConverter) ToList(ctx context.Context, domainObjs []*models.IEAgAgRule) (runtime.Object, error) {
	ruleList := &netguardv1beta1.IEAgAgRuleList{
		TypeMeta: CreateStandardTypeMetaForList("IEAgAgRuleList"),
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.IEAgAgRule, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain IEAgAgRule %d to k8s: %w", i, err)
		}
		ruleList.Items[i] = *k8sObj
	}

	return ruleList, nil
}

// Helper methods for enum conversions

// convertTransportToDomain converts k8s Transport enum to domain TransportProtocol enum
func (c *IEAgAgRuleConverter) convertTransportToDomain(k8sTransport netguardv1beta1.TransportProtocol) (models.TransportProtocol, error) {
	switch k8sTransport {
	case netguardv1beta1.ProtocolTCP:
		return models.TCP, nil
	case netguardv1beta1.ProtocolUDP:
		return models.UDP, nil
	default:
		return "", fmt.Errorf("unknown transport protocol: %s", k8sTransport)
	}
}

// convertTransportFromDomain converts domain TransportProtocol enum to k8s Transport enum
func (c *IEAgAgRuleConverter) convertTransportFromDomain(domainTransport models.TransportProtocol) (netguardv1beta1.TransportProtocol, error) {
	switch domainTransport {
	case models.TCP:
		return netguardv1beta1.ProtocolTCP, nil
	case models.UDP:
		return netguardv1beta1.ProtocolUDP, nil
	case "": // Handle empty transport - default to TCP
		return netguardv1beta1.ProtocolTCP, nil
	case "Networks_NetIP_TCP": // Handle old protobuf enum name
		return netguardv1beta1.ProtocolTCP, nil
	case "Networks_NetIP_UDP": // Handle old protobuf enum name
		return netguardv1beta1.ProtocolUDP, nil
	default:
		return "", fmt.Errorf("unknown transport protocol: %s", domainTransport)
	}
}

// convertTrafficToDomain converts k8s Traffic enum to domain Traffic enum
func (c *IEAgAgRuleConverter) convertTrafficToDomain(k8sTraffic netguardv1beta1.Traffic) (models.Traffic, error) {
	switch k8sTraffic {
	case netguardv1beta1.INGRESS:
		return models.INGRESS, nil
	case netguardv1beta1.EGRESS:
		return models.EGRESS, nil
	default:
		return "", fmt.Errorf("unknown traffic direction: %s", k8sTraffic)
	}
}

// convertTrafficFromDomain converts domain Traffic enum to k8s Traffic enum
func (c *IEAgAgRuleConverter) convertTrafficFromDomain(domainTraffic models.Traffic) (netguardv1beta1.Traffic, error) {
	switch domainTraffic {
	case models.INGRESS:
		return netguardv1beta1.INGRESS, nil
	case models.EGRESS:
		return netguardv1beta1.EGRESS, nil
	case "": // Handle empty traffic - default to Ingress
		return netguardv1beta1.INGRESS, nil
	case "Traffic_Ingress": // Handle old protobuf enum name
		return netguardv1beta1.INGRESS, nil
	case "Traffic_Egress": // Handle old protobuf enum name
		return netguardv1beta1.EGRESS, nil
	default:
		return "", fmt.Errorf("unknown traffic direction: %s", domainTraffic)
	}
}

// convertActionToDomain converts k8s Action string to domain RuleAction enum
func (c *IEAgAgRuleConverter) convertActionToDomain(k8sAction string) (models.RuleAction, error) {
	switch strings.ToUpper(k8sAction) {
	case "ACCEPT":
		return models.ActionAccept, nil
	case "DROP":
		return models.ActionDrop, nil
	default:
		return "", fmt.Errorf("unknown action: %s", k8sAction)
	}
}

// convertActionFromDomain converts domain RuleAction enum to k8s Action enum
func (c *IEAgAgRuleConverter) convertActionFromDomain(domainAction models.RuleAction) (netguardv1beta1.RuleAction, error) {
	switch domainAction {
	case models.ActionAccept:
		return netguardv1beta1.ActionAccept, nil
	case models.ActionDrop:
		return netguardv1beta1.ActionDrop, nil
	case "": // Handle empty action - default to ACCEPT
		return netguardv1beta1.ActionAccept, nil
	case "RuleAction_ACCEPT": // Handle old protobuf enum name
		return netguardv1beta1.ActionAccept, nil
	case "RuleAction_DROP": // Handle old protobuf enum name
		return netguardv1beta1.ActionDrop, nil
	default:
		return "", fmt.Errorf("unknown action: %s", domainAction)
	}
}

// NewIEAgAgRuleConverter creates a new IEAgAgRuleConverter instance
func NewIEAgAgRuleConverter() *IEAgAgRuleConverter {
	return &IEAgAgRuleConverter{}
}
