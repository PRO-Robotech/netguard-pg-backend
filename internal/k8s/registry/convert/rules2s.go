package convert

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// RuleS2SConverter implements conversion between k8s RuleS2S and domain RuleS2S
type RuleS2SConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.RuleS2S, *models.RuleS2S] = &RuleS2SConverter{}

// ToDomain converts a Kubernetes RuleS2S object to a domain RuleS2S model
func (c *RuleS2SConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.RuleS2S) (*models.RuleS2S, error) {
	if k8sObj == nil {
		return nil, fmt.Errorf("k8s RuleS2S object is nil")
	}

	// Convert Traffic enum
	traffic, err := c.convertTrafficToDomain(k8sObj.Spec.Traffic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert traffic: %w", err)
	}

	// Create domain rule s2s
	domainRule := &models.RuleS2S{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		Traffic: traffic,
		ServiceLocalRef: models.ServiceAliasRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Spec.ServiceLocalRef.Name,
				Namespace: k8sObj.Spec.ServiceLocalRef.Namespace,
			},
		},
		ServiceRef: models.ServiceAliasRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Spec.ServiceRef.Name,
				Namespace: k8sObj.Spec.ServiceRef.Namespace,
			},
		},
		Meta: models.Meta{
			UID:                string(k8sObj.UID),
			ResourceVersion:    k8sObj.ResourceVersion,
			Generation:         k8sObj.Generation,
			CreationTS:         k8sObj.CreationTimestamp,
			ObservedGeneration: k8sObj.Status.ObservedGeneration,
			Conditions:         k8sObj.Status.Conditions,
		},
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

// FromDomain converts a domain RuleS2S model to a Kubernetes RuleS2S object
func (c *RuleS2SConverter) FromDomain(ctx context.Context, domainObj *models.RuleS2S) (*netguardv1beta1.RuleS2S, error) {
	if domainObj == nil {
		return nil, fmt.Errorf("domain RuleS2S object is nil")
	}

	// Convert Traffic enum
	traffic, err := c.convertTrafficFromDomain(domainObj.Traffic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert traffic: %w", err)
	}

	// Create k8s rule s2s
	k8sRule := &netguardv1beta1.RuleS2S{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "RuleS2S",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              domainObj.ResourceIdentifier.Name,
			Namespace:         domainObj.ResourceIdentifier.Namespace,
			UID:               types.UID(domainObj.Meta.UID),
			ResourceVersion:   domainObj.Meta.ResourceVersion,
			Generation:        domainObj.Meta.Generation,
			CreationTimestamp: domainObj.Meta.CreationTS,
		},
		Spec: netguardv1beta1.RuleS2SSpec{
			Traffic: traffic,
			ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       domainObj.ServiceLocalRef.Name,
				},
				Namespace: domainObj.ServiceLocalRef.Namespace,
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       domainObj.ServiceRef.Name,
				},
				Namespace: domainObj.ServiceRef.Namespace,
			},
		},
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
	k8sRule.Status = netguardv1beta1.RuleS2SStatus{
		ObservedGeneration: domainObj.Meta.ObservedGeneration,
		Conditions:         domainObj.Meta.Conditions, // Backend формирует условия
	}

	return k8sRule, nil
}

// ToList converts a slice of domain RuleS2S models to a Kubernetes RuleS2SList object
func (c *RuleS2SConverter) ToList(ctx context.Context, domainObjs []*models.RuleS2S) (runtime.Object, error) {
	ruleList := &netguardv1beta1.RuleS2SList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "RuleS2SList",
		},
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.RuleS2S, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain rule s2s %d to k8s: %w", i, err)
		}
		ruleList.Items[i] = *k8sObj
	}

	return ruleList, nil
}

// Helper methods for traffic conversion

// convertTrafficToDomain converts k8s Traffic string to domain Traffic enum
func (c *RuleS2SConverter) convertTrafficToDomain(k8sTraffic string) (models.Traffic, error) {
	switch strings.ToLower(k8sTraffic) {
	case "ingress":
		return models.INGRESS, nil
	case "egress":
		return models.EGRESS, nil
	default:
		return "", fmt.Errorf("unknown traffic type: %s", k8sTraffic)
	}
}

// convertTrafficFromDomain converts domain Traffic enum to k8s Traffic string
func (c *RuleS2SConverter) convertTrafficFromDomain(domainTraffic models.Traffic) (string, error) {
	switch domainTraffic {
	case models.INGRESS:
		return "ingress", nil
	case models.EGRESS:
		return "egress", nil
	default:
		return "", fmt.Errorf("unknown traffic type: %s", domainTraffic)
	}
}

// NewRuleS2SConverter creates a new RuleS2SConverter instance
func NewRuleS2SConverter() *RuleS2SConverter {
	return &RuleS2SConverter{}
}
