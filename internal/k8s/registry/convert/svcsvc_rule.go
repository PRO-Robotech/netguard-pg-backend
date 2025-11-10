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

// SvcSvcRuleConverter implements conversion between k8s SvcSvcRule and domain SvcSvcRule
type SvcSvcRuleConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.SvcSvcRule, *models.SvcSvcRule] = &SvcSvcRuleConverter{}

// ToDomain converts a Kubernetes SvcSvcRule object to a domain SvcSvcRule model
func (c *SvcSvcRuleConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.SvcSvcRule) (*models.SvcSvcRule, error) {
	if err := ValidateNilObject(k8sObj, "k8s SvcSvcRule"); err != nil {
		return nil, err
	}

	// Convert Action
	action, err := c.convertActionToDomain(string(k8sObj.Spec.Action))
	if err != nil {
		return nil, fmt.Errorf("failed to convert action: %w", err)
	}

	// Create domain SvcSvcRule
	domainRule := &models.SvcSvcRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		ServiceFromRef: EnsureNamespacedObjectReferenceFields(k8sObj.Spec.ServiceFrom, "Service"),
		ServiceToRef:   EnsureNamespacedObjectReferenceFields(k8sObj.Spec.ServiceTo, "Service"),
		Action:         action,
		Priority:       k8sObj.Spec.Priority,
		Logs:           k8sObj.Spec.Logs,
		Trace:          k8sObj.Spec.Trace,
		Meta:           ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	return domainRule, nil
}

// FromDomain converts a domain SvcSvcRule model to a Kubernetes SvcSvcRule object
func (c *SvcSvcRuleConverter) FromDomain(ctx context.Context, domainObj *models.SvcSvcRule) (*netguardv1beta1.SvcSvcRule, error) {
	if err := ValidateNilObject(domainObj, "domain SvcSvcRule"); err != nil {
		return nil, err
	}

	// Convert Action
	action, err := c.convertActionFromDomain(domainObj.Action)
	if err != nil {
		return nil, fmt.Errorf("failed to convert action: %w", err)
	}

	// Create k8s SvcSvcRule with standard metadata conversion
	k8sRule := &netguardv1beta1.SvcSvcRule{
		TypeMeta:   CreateStandardTypeMetaForResource("SvcSvcRule"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.SvcSvcRuleSpec{
			ServiceFrom: EnsureNamespacedObjectReferenceFields(domainObj.ServiceFromRef, "Service"),
			ServiceTo:   EnsureNamespacedObjectReferenceFields(domainObj.ServiceToRef, "Service"),
			Action:      action,
			Priority:    domainObj.Priority,
			Logs:        domainObj.Logs,
			Trace:       domainObj.Trace,
		},
	}

	// Convert status using standard helper
	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sRule.Status = netguardv1beta1.SvcSvcRuleStatus{
		ObservedGeneration: observedGeneration,
		Conditions:         conditions,
		SyncReady:          false, // Will be set by condition manager
	}

	return k8sRule, nil
}

// ToList converts a slice of domain SvcSvcRule models to a Kubernetes SvcSvcRuleList object
func (c *SvcSvcRuleConverter) ToList(ctx context.Context, domainObjs []*models.SvcSvcRule) (runtime.Object, error) {
	ruleList := &netguardv1beta1.SvcSvcRuleList{
		TypeMeta: CreateStandardTypeMetaForList("SvcSvcRuleList"),
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.SvcSvcRule, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain SvcSvcRule %d to k8s: %w", i, err)
		}
		ruleList.Items[i] = *k8sObj
	}

	return ruleList, nil
}

// Helper methods for enum conversions

// convertActionToDomain converts k8s Action enum to domain RuleAction enum
func (c *SvcSvcRuleConverter) convertActionToDomain(k8sAction string) (models.RuleAction, error) {
	switch k8sAction {
	case string(netguardv1beta1.ActionAccept):
		return models.ActionAccept, nil
	case string(netguardv1beta1.ActionDrop):
		return models.ActionDrop, nil
	default:
		return "", fmt.Errorf("unknown action: %s", k8sAction)
	}
}

// convertActionFromDomain converts domain RuleAction enum to k8s Action enum
func (c *SvcSvcRuleConverter) convertActionFromDomain(domainAction models.RuleAction) (netguardv1beta1.RuleAction, error) {
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

// NewSvcSvcRuleConverter creates a new SvcSvcRuleConverter instance
func NewSvcSvcRuleConverter() *SvcSvcRuleConverter {
	return &SvcSvcRuleConverter{}
}
