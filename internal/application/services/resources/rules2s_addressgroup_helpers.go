package resources

import (
	"context"
	"log"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// extractAddressGroupRefs extracts AddressGroupRef slice from AggregatedAddressGroups
// This helper converts AddressGroupReference (with Source field) to simple AddressGroupRef
func extractAddressGroupRefs(aggregated []models.AddressGroupReference) []models.AddressGroupRef {
	if len(aggregated) == 0 {
		return nil
	}

	refs := make([]models.AddressGroupRef, len(aggregated))
	for i, agRef := range aggregated {
		refs[i] = models.AddressGroupRef{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: agRef.Ref.APIVersion,
				Kind:       agRef.Ref.Kind,
				Name:       agRef.Ref.Name,
			},
			Namespace: agRef.Ref.Namespace,
		}
	}
	return refs
}

// populateServiceAddressGroups populates Service.AddressGroups from AddressGroupBindings
// This is the critical function for Phase 5 - Service → AddressGroup dual registration
func (s *RuleS2SResourceService) populateServiceAddressGroups(
	ctx context.Context,
	reader ports.Reader,
	service *models.Service,
) (*models.Service, error) {
	klog.V(2).Infof("🔧 POPULATE_ADDRESSGROUPS: Starting AddressGroup population for service %s", service.Key())

	// Create a copy of the service to avoid modifying the original
	serviceCopy := *service
	serviceCopy.AddressGroups = []models.AddressGroupRef{} // Reset to empty slice

	// Find all AddressGroupBindings that reference this service
	err := reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		// Check if this binding references our service
		if binding.ServiceRef.Name == service.Name && binding.ServiceRef.Namespace == service.Namespace {
			klog.V(2).Infof("  🔗 FOUND_BINDING: %s → AddressGroup %s/%s",
				binding.Key(), binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name)

			// Create AddressGroupRef from the binding
			agRef := models.AddressGroupRef{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: binding.AddressGroupRef.APIVersion,
					Kind:       binding.AddressGroupRef.Kind,
					Name:       binding.AddressGroupRef.Name,
				},
				Namespace: binding.AddressGroupRef.Namespace,
			}

			// Add to service's AddressGroups
			serviceCopy.AddressGroups = append(serviceCopy.AddressGroups, agRef)
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return nil, errors.Wrapf(err, "failed to list AddressGroupBindings for service %s", service.Key())
	}

	klog.V(2).Infof("  ✅ POPULATE_ADDRESSGROUPS: Service %s now has %d AddressGroups populated",
		service.Key(), len(serviceCopy.AddressGroups))

	// Log the AddressGroups for debugging
	for i, ag := range serviceCopy.AddressGroups {
		klog.V(2).Infof("    📍 AG[%d]: %s/%s", i, ag.Namespace, ag.Name)
	}

	return &serviceCopy, nil
}

// extractAggregationGroupsFromRuleS2S extracts aggregation groups from a RuleS2S
// This function analyzes which AddressGroup combinations a RuleS2S creates
func (s *RuleS2SResourceService) extractAggregationGroupsFromRuleS2S(ctx context.Context, reader ports.Reader, rule models.RuleS2S) ([]AggregationGroup, error) {
	// Get service IDs directly from RuleS2S references (no ServiceAlias lookup needed)
	localServiceID := models.ResourceIdentifier{
		Name:      rule.ServiceLocalRef.Name,
		Namespace: rule.ServiceLocalRef.Namespace,
	}
	targetServiceID := models.ResourceIdentifier{
		Name:      rule.ServiceRef.Name,
		Namespace: rule.ServiceRef.Namespace,
	}

	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get local service %s", localServiceID.Key())
	}

	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get target service %s", targetServiceID.Key())
	}

	// Extract ports based on traffic direction
	var portsSource *models.Service
	if rule.Traffic == models.INGRESS {
		portsSource = localService
	} else {
		portsSource = targetService
	}

	var groups []AggregationGroup

	// 🎯 STORY-001: Use AggregatedAddressGroups (spec + bindings) instead of AddressGroups (spec only)
	localAGs := extractAddressGroupRefs(localService.AggregatedAddressGroups)
	targetAGs := extractAddressGroupRefs(targetService.AggregatedAddressGroups)

	// Generate aggregation groups for all AG combinations and protocols
	for _, localAG := range localAGs {
		for _, targetAG := range targetAGs {
			// Check what protocols this service supports
			protocolsSupported := make(map[models.TransportProtocol]bool)
			for _, port := range portsSource.IngressPorts {
				protocolsSupported[port.Protocol] = true
			}

			// Create aggregation groups for each protocol
			for protocol := range protocolsSupported {
				group := AggregationGroup{
					Traffic:   rule.Traffic,
					LocalAG:   localAG,
					TargetAG:  targetAG,
					Protocol:  protocol,
					Namespace: rule.Namespace,
				}
				groups = append(groups, group)
			}
		}
	}

	return groups, nil
}

// findRuleS2SByAddressGroupInteraction finds all RuleS2S where the specified AddressGroup appears in aggregation
func (s *RuleS2SResourceService) findRuleS2SByAddressGroupInteraction(ctx context.Context, reader ports.Reader, addressGroup models.AddressGroupRef) ([]models.RuleS2S, error) {
	var rules []models.RuleS2S

	err := reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		// Extract aggregation groups from this rule to see if it interacts with the AddressGroup
		groups, err := s.extractAggregationGroupsFromRuleS2S(ctx, reader, rule)
		if err != nil {
			// Skip this rule if we can't analyze it
			log.Printf("⚠️ findRuleS2SByAddressGroupInteraction: Failed to extract aggregation groups from rule %s: %v", rule.Key(), err)
			return nil
		}

		// Check if any aggregation group involves the specified AddressGroup
		for _, group := range groups {
			if (group.LocalAG.Name == addressGroup.Name && group.LocalAG.Namespace == addressGroup.Namespace) ||
				(group.TargetAG.Name == addressGroup.Name && group.TargetAG.Namespace == addressGroup.Namespace) {
				rules = append(rules, rule)
				log.Printf("🔍 findRuleS2SByAddressGroupInteraction: Rule %s interacts with AddressGroup %s/%s via aggregation", rule.Key(), addressGroup.Namespace, addressGroup.Name)
				return nil // Found interaction, no need to check more groups for this rule
			}
		}

		return nil
	}, ports.EmptyScope{})

	return rules, err
}
