package convert

import (
	"context"
	"fmt"
	"log"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

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
	if k8sObj == nil {
		return nil, fmt.Errorf("k8s IEAgAgRule object is nil")
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
		Transport: transport,
		Traffic:   traffic,
		AddressGroupLocal: models.AddressGroupRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Spec.AddressGroupLocal.Name,
				Namespace: k8sObj.Namespace, // AddressGroupLocal in k8s API is not namespaced, use rule namespace
			},
		},
		AddressGroup: models.AddressGroupRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Spec.AddressGroup.Name,
				Namespace: k8sObj.Namespace, // AddressGroup in k8s API is not namespaced, use rule namespace
			},
		},
		Action:   action,
		Logs:     false, // Not exposed in k8s API for now
		Priority: k8sObj.Spec.Priority,
		Meta: models.Meta{
			UID:                string(k8sObj.UID),
			ResourceVersion:    k8sObj.ResourceVersion,
			Generation:         k8sObj.Generation,
			CreationTS:         k8sObj.CreationTimestamp,
			ObservedGeneration: k8sObj.Status.ObservedGeneration,
			Conditions:         k8sObj.Status.Conditions,
		},
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

	// Copy metadata
	if k8sObj.Labels != nil {
		domainRule.Meta.Labels = make(map[string]string)
		for k, v := range k8sObj.Labels {
			domainRule.Meta.Labels[k] = v
		}
	}

	if k8sObj.Annotations != nil {
		domainRule.Meta.Annotations = make(map[string]string)
		for k, v := range k8sObj.Annotations {
			domainRule.Meta.Annotations[k] = v
		}
	}

	return domainRule, nil
}

// FromDomain converts a domain IEAgAgRule model to a Kubernetes IEAgAgRule object
func (c *IEAgAgRuleConverter) FromDomain(ctx context.Context, domainObj *models.IEAgAgRule) (*netguardv1beta1.IEAgAgRule, error) {
	if domainObj == nil {
		return nil, fmt.Errorf("domain IEAgAgRule object is nil")
	}

	// Add detailed logging for debugging
	log.Printf("🔧 DEBUG: Converting IEAgAgRule from domain: name=%s, namespace=%s, transport='%s', traffic='%s', action='%s'",
		domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace,
		domainObj.Transport, domainObj.Traffic, domainObj.Action)

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

	// Create k8s IEAgAgRule
	k8sRule := &netguardv1beta1.IEAgAgRule{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "IEAgAgRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              domainObj.ResourceIdentifier.Name,
			Namespace:         domainObj.ResourceIdentifier.Namespace,
			UID:               types.UID(domainObj.Meta.UID),
			ResourceVersion:   domainObj.Meta.ResourceVersion,
			Generation:        domainObj.Meta.Generation,
			CreationTimestamp: domainObj.Meta.CreationTS,
			Labels:            domainObj.Meta.Labels,
			Annotations:       domainObj.Meta.Annotations,
		},
		Spec: netguardv1beta1.IEAgAgRuleSpec{
			Description: fmt.Sprintf("IEAgAgRule: %s traffic from %s to %s",
				domainObj.Traffic, domainObj.AddressGroupLocal.Name, domainObj.AddressGroup.Name),
			Transport: transport,
			Traffic:   traffic,
			AddressGroupLocal: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       domainObj.AddressGroupLocal.Name,
			},
			AddressGroup: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       domainObj.AddressGroup.Name,
			},
			Action:   action,
			Priority: domainObj.Priority,
		},
		Status: netguardv1beta1.IEAgAgRuleStatus{
			ObservedGeneration: domainObj.Meta.ObservedGeneration,
			Conditions:         domainObj.Meta.Conditions,
		},
	}

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
							log.Printf("⚠️  WARNING: Failed to parse port range %s: %v", portStr, err)
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
							log.Printf("⚠️  WARNING: Failed to parse port %s: %v", portStr, err)
							continue
						}
						k8sPortSpec.Port = port
					}

					k8sRule.Spec.Ports = append(k8sRule.Spec.Ports, k8sPortSpec)
				}
			}
		}
	}

	// Copy metadata
	if domainObj.Meta.Labels != nil {
		k8sRule.Labels = make(map[string]string)
		for k, v := range domainObj.Meta.Labels {
			k8sRule.Labels[k] = v
		}
	}

	if domainObj.Meta.Annotations != nil {
		k8sRule.Annotations = make(map[string]string)
		for k, v := range domainObj.Meta.Annotations {
			k8sRule.Annotations[k] = v
		}
	}

	// Convert status - переносим условия из Meta в Status
	k8sRule.Status = netguardv1beta1.IEAgAgRuleStatus{
		ObservedGeneration: domainObj.Meta.ObservedGeneration,
		Conditions:         domainObj.Meta.Conditions, // Backend формирует условия
	}

	return k8sRule, nil
}

// ToList converts a slice of domain IEAgAgRule models to a Kubernetes IEAgAgRuleList object
func (c *IEAgAgRuleConverter) ToList(ctx context.Context, domainObjs []*models.IEAgAgRule) (runtime.Object, error) {
	ruleList := &netguardv1beta1.IEAgAgRuleList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "IEAgAgRuleList",
		},
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

// convertTransportToDomain converts k8s Transport string to domain TransportProtocol enum
func (c *IEAgAgRuleConverter) convertTransportToDomain(k8sTransport string) (models.TransportProtocol, error) {
	switch strings.ToUpper(k8sTransport) {
	case "TCP":
		return models.TCP, nil
	case "UDP":
		return models.UDP, nil
	default:
		return "", fmt.Errorf("unknown transport protocol: %s", k8sTransport)
	}
}

// convertTransportFromDomain converts domain TransportProtocol enum to k8s Transport string
func (c *IEAgAgRuleConverter) convertTransportFromDomain(domainTransport models.TransportProtocol) (string, error) {
	switch domainTransport {
	case models.TCP:
		return "TCP", nil
	case models.UDP:
		return "UDP", nil
	case "": // Handle empty transport - default to TCP
		log.Printf("⚠️  WARNING: IEAgAgRule has empty Transport field, defaulting to TCP")
		return "TCP", nil
	case "Networks_NetIP_TCP": // Handle old protobuf enum name
		log.Printf("⚠️  WARNING: IEAgAgRule has old protobuf enum name 'Networks_NetIP_TCP', converting to TCP")
		return "TCP", nil
	case "Networks_NetIP_UDP": // Handle old protobuf enum name
		log.Printf("⚠️  WARNING: IEAgAgRule has old protobuf enum name 'Networks_NetIP_UDP', converting to UDP")
		return "UDP", nil
	default:
		log.Printf("❌ ERROR: convertTransportFromDomain - unknown transport protocol: '%s' (length: %d)", domainTransport, len(string(domainTransport)))
		return "", fmt.Errorf("unknown transport protocol: %s", domainTransport)
	}
}

// convertTrafficToDomain converts k8s Traffic string to domain Traffic enum
func (c *IEAgAgRuleConverter) convertTrafficToDomain(k8sTraffic string) (models.Traffic, error) {
	switch strings.ToLower(k8sTraffic) {
	case "ingress":
		return models.INGRESS, nil
	case "egress":
		return models.EGRESS, nil
	default:
		return "", fmt.Errorf("unknown traffic direction: %s", k8sTraffic)
	}
}

// convertTrafficFromDomain converts domain Traffic enum to k8s Traffic string
func (c *IEAgAgRuleConverter) convertTrafficFromDomain(domainTraffic models.Traffic) (string, error) {
	switch domainTraffic {
	case models.INGRESS:
		return "Ingress", nil
	case models.EGRESS:
		return "Egress", nil
	case "": // Handle empty traffic - default to Ingress
		log.Printf("⚠️  WARNING: IEAgAgRule has empty Traffic field, defaulting to Ingress")
		return "Ingress", nil
	case "Traffic_Ingress": // Handle old protobuf enum name
		log.Printf("⚠️  WARNING: IEAgAgRule has old protobuf enum name 'Traffic_Ingress', converting to Ingress")
		return "Ingress", nil
	case "Traffic_Egress": // Handle old protobuf enum name
		log.Printf("⚠️  WARNING: IEAgAgRule has old protobuf enum name 'Traffic_Egress', converting to Egress")
		return "Egress", nil
	default:
		log.Printf("❌ ERROR: convertTrafficFromDomain - unknown traffic direction: '%s' (length: %d)", domainTraffic, len(string(domainTraffic)))
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
		log.Printf("⚠️  WARNING: IEAgAgRule has empty Action field, defaulting to ACCEPT")
		return netguardv1beta1.ActionAccept, nil
	case "RuleAction_ACCEPT": // Handle old protobuf enum name
		log.Printf("⚠️  WARNING: IEAgAgRule has old protobuf enum name 'RuleAction_ACCEPT', converting to ACCEPT")
		return netguardv1beta1.ActionAccept, nil
	case "RuleAction_DROP": // Handle old protobuf enum name
		log.Printf("⚠️  WARNING: IEAgAgRule has old protobuf enum name 'RuleAction_DROP', converting to DROP")
		return netguardv1beta1.ActionDrop, nil
	default:
		log.Printf("❌ ERROR: convertActionFromDomain - unknown action: '%s' (length: %d)", domainAction, len(string(domainAction)))
		return "", fmt.Errorf("unknown action: %s", domainAction)
	}
}

// NewIEAgAgRuleConverter creates a new IEAgAgRuleConverter instance
func NewIEAgAgRuleConverter() *IEAgAgRuleConverter {
	return &IEAgAgRuleConverter{}
}
