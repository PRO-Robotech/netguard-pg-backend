package convert

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

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
	if err := ValidateNilObject(k8sObj, "k8s RuleS2S"); err != nil {
		return nil, err
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
		Traffic:         traffic,
		ServiceLocalRef: k8sObj.Spec.ServiceLocalRef,
		ServiceRef:      k8sObj.Spec.ServiceRef,
		Trace:           k8sObj.Spec.Trace, // Copy trace field from spec
		Meta:            ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	// Convert IEAgAgRuleRefs from status
	if len(k8sObj.Status.IEAgAgRuleRefs) > 0 {
		domainRule.IEAgAgRuleRefs = make([]netguardv1beta1.NamespacedObjectReference, len(k8sObj.Status.IEAgAgRuleRefs))
		for i, ref := range k8sObj.Status.IEAgAgRuleRefs {
			domainRule.IEAgAgRuleRefs[i] = ref
		}
	}

	// Metadata already converted by ConvertMetadataToDomain helper

	return domainRule, nil
}

// FromDomain converts a domain RuleS2S model to a Kubernetes RuleS2S object
func (c *RuleS2SConverter) FromDomain(ctx context.Context, domainObj *models.RuleS2S) (*netguardv1beta1.RuleS2S, error) {
	if err := ValidateNilObject(domainObj, "domain RuleS2S"); err != nil {
		return nil, err
	}

	// Convert Traffic enum
	traffic, err := c.convertTrafficFromDomain(domainObj.Traffic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert traffic: %w", err)
	}

	// Create k8s rule s2s with standard metadata conversion
	k8sRule := &netguardv1beta1.RuleS2S{
		TypeMeta:   CreateStandardTypeMetaForResource("RuleS2S"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.RuleS2SSpec{
			Traffic:         traffic,
			ServiceLocalRef: EnsureNamespacedObjectReferenceFields(domainObj.ServiceLocalRef, "Service"),
			ServiceRef:      EnsureNamespacedObjectReferenceFields(domainObj.ServiceRef, "Service"),
			Trace:           domainObj.Trace, // Copy trace field from domain
		},
	}

	// Metadata already converted by ConvertMetadataFromDomain helper

	// Convert status using standard helper
	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sRule.Status = netguardv1beta1.RuleS2SStatus{
		ObservedGeneration: observedGeneration,
		Conditions:         conditions,
	}

	// Convert IEAgAgRuleRefs to status
	if len(domainObj.IEAgAgRuleRefs) > 0 {
		k8sRule.Status.IEAgAgRuleRefs = make([]netguardv1beta1.NamespacedObjectReference, len(domainObj.IEAgAgRuleRefs))
		for i, ref := range domainObj.IEAgAgRuleRefs {
			k8sRule.Status.IEAgAgRuleRefs[i] = EnsureNamespacedObjectReferenceFields(ref, "IEAgAgRule")
		}
	}

	return k8sRule, nil
}

// ToList converts a slice of domain RuleS2S models to a Kubernetes RuleS2SList object
func (c *RuleS2SConverter) ToList(ctx context.Context, domainObjs []*models.RuleS2S) (runtime.Object, error) {
	ruleList := &netguardv1beta1.RuleS2SList{
		TypeMeta: CreateStandardTypeMetaForList("RuleS2SList"),
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

// convertTrafficToDomain converts k8s Traffic enum to domain Traffic enum
func (c *RuleS2SConverter) convertTrafficToDomain(k8sTraffic netguardv1beta1.Traffic) (models.Traffic, error) {
	switch k8sTraffic {
	case netguardv1beta1.INGRESS:
		return models.INGRESS, nil
	case netguardv1beta1.EGRESS:
		return models.EGRESS, nil
	default:
		return "", fmt.Errorf("unknown traffic type: %s", k8sTraffic)
	}
}

// convertTrafficFromDomain converts domain Traffic enum to k8s Traffic enum
func (c *RuleS2SConverter) convertTrafficFromDomain(domainTraffic models.Traffic) (netguardv1beta1.Traffic, error) {
	switch domainTraffic {
	case models.INGRESS:
		return netguardv1beta1.INGRESS, nil
	case models.EGRESS:
		return netguardv1beta1.EGRESS, nil
	default:
		return "", fmt.Errorf("unknown traffic type: %s", domainTraffic)
	}
}

// NewRuleS2SConverter creates a new RuleS2SConverter instance
func NewRuleS2SConverter() *RuleS2SConverter {
	return &RuleS2SConverter{}
}
