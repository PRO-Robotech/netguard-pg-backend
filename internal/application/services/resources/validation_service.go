package resources

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// ValidationService centralizes validation logic for all resource types
type ValidationService struct {
	registry ports.Registry
}

// NewValidationService creates a new ValidationService
func NewValidationService(registry ports.Registry) *ValidationService {
	return &ValidationService{
		registry: registry,
	}
}

// =============================================================================
// Service Validation
// =============================================================================

// ValidateServiceForCreation validates a service for creation
func (s *ValidationService) ValidateServiceForCreation(ctx context.Context, service models.Service) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	if err := serviceValidator.ValidateForCreation(ctx, service); err != nil {
		return errors.Wrapf(err, "service validation failed for creation: %s", service.Key())
	}

	return nil
}

// ValidateServiceForUpdate validates a service for update
func (s *ValidationService) ValidateServiceForUpdate(ctx context.Context, oldService, newService models.Service) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	if err := serviceValidator.ValidateForUpdate(ctx, oldService, newService); err != nil {
		return errors.Wrapf(err, "service validation failed for update: %s", newService.Key())
	}

	return nil
}

// ValidateServiceForDeletion validates a service for deletion
func (s *ValidationService) ValidateServiceForDeletion(ctx context.Context, service models.Service) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	// Basic existence check for deletion - service should exist
	if err := serviceValidator.ValidateExists(ctx, service.SelfRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "service validation failed for deletion: %s", service.Key())
	}

	return nil
}

// =============================================================================
// AddressGroup Validation
// =============================================================================

// ValidateAddressGroupForCreation validates an address group for creation
func (s *ValidationService) ValidateAddressGroupForCreation(ctx context.Context, addressGroup models.AddressGroup) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	if err := addressGroupValidator.ValidateForCreation(ctx, addressGroup); err != nil {
		return errors.Wrapf(err, "address group validation failed for creation: %s", addressGroup.Key())
	}

	return nil
}

// ValidateAddressGroupForUpdate validates an address group for update
func (s *ValidationService) ValidateAddressGroupForUpdate(ctx context.Context, oldAddressGroup, newAddressGroup models.AddressGroup) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	if err := addressGroupValidator.ValidateForUpdate(ctx, oldAddressGroup, newAddressGroup); err != nil {
		return errors.Wrapf(err, "address group validation failed for update: %s", newAddressGroup.Key())
	}

	return nil
}

// ValidateAddressGroupForDeletion validates an address group for deletion
func (s *ValidationService) ValidateAddressGroupForDeletion(ctx context.Context, addressGroup models.AddressGroup) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	if err := addressGroupValidator.ValidateExists(ctx, addressGroup.SelfRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "address group validation failed for deletion: %s", addressGroup.Key())
	}

	return nil
}

// =============================================================================
// AddressGroupBinding Validation
// =============================================================================

// ValidateAddressGroupBindingForCreation validates an address group binding for creation
func (s *ValidationService) ValidateAddressGroupBindingForCreation(ctx context.Context, binding models.AddressGroupBinding) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetAddressGroupBindingValidator()

	if err := bindingValidator.ValidateForCreation(ctx, &binding); err != nil {
		return errors.Wrapf(err, "address group binding validation failed for creation: %s", binding.Key())
	}

	return nil
}

// ValidateAddressGroupBindingForUpdate validates an address group binding for update
func (s *ValidationService) ValidateAddressGroupBindingForUpdate(ctx context.Context, oldBinding, newBinding models.AddressGroupBinding) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetAddressGroupBindingValidator()

	if err := bindingValidator.ValidateForUpdate(ctx, oldBinding, &newBinding); err != nil {
		return errors.Wrapf(err, "address group binding validation failed for update: %s", newBinding.Key())
	}

	return nil
}

// ValidateAddressGroupBindingForDeletion validates an address group binding for deletion
func (s *ValidationService) ValidateAddressGroupBindingForDeletion(ctx context.Context, binding models.AddressGroupBinding) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetAddressGroupBindingValidator()

	// Basic existence check for deletion - binding should exist
	if err := bindingValidator.ValidateExists(ctx, binding.SelfRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "address group binding validation failed for deletion: %s", binding.Key())
	}

	return nil
}

// =============================================================================
// AddressGroupPortMapping Validation
// =============================================================================

// ValidateAddressGroupPortMappingForCreation validates an address group port mapping for creation
func (s *ValidationService) ValidateAddressGroupPortMappingForCreation(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	if err := mappingValidator.ValidateForCreation(ctx, mapping); err != nil {
		return errors.Wrapf(err, "address group port mapping validation failed for creation: %s", mapping.Key())
	}

	return nil
}

// ValidateAddressGroupPortMappingForUpdate validates an address group port mapping for update
func (s *ValidationService) ValidateAddressGroupPortMappingForUpdate(ctx context.Context, oldMapping, newMapping models.AddressGroupPortMapping) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	if err := mappingValidator.ValidateForUpdate(ctx, oldMapping, newMapping); err != nil {
		return errors.Wrapf(err, "address group port mapping validation failed for update: %s", newMapping.Key())
	}

	return nil
}

// ValidateAddressGroupPortMappingForDeletion validates an address group port mapping for deletion
func (s *ValidationService) ValidateAddressGroupPortMappingForDeletion(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	// Basic existence check for deletion - mapping should exist
	if err := mappingValidator.ValidateExists(ctx, mapping.SelfRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "address group port mapping validation failed for deletion: %s", mapping.Key())
	}

	return nil
}

// =============================================================================
// RuleS2S Validation
// =============================================================================

// ValidateRuleS2SForCreation validates a RuleS2S for creation
func (s *ValidationService) ValidateRuleS2SForCreation(ctx context.Context, rule models.RuleS2S) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	if err := ruleValidator.ValidateForCreation(ctx, rule); err != nil {
		return errors.Wrapf(err, "RuleS2S validation failed for creation: %s", rule.Key())
	}

	return nil
}

// ValidateRuleS2SForUpdate validates a RuleS2S for update
func (s *ValidationService) ValidateRuleS2SForUpdate(ctx context.Context, oldRule, newRule models.RuleS2S) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	if err := ruleValidator.ValidateForUpdate(ctx, oldRule, newRule); err != nil {
		return errors.Wrapf(err, "RuleS2S validation failed for update: %s", newRule.Key())
	}

	return nil
}

// ValidateRuleS2SForDeletion validates a RuleS2S for deletion
func (s *ValidationService) ValidateRuleS2SForDeletion(ctx context.Context, rule models.RuleS2S) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	// Basic existence check for deletion - rule should exist
	if err := ruleValidator.ValidateExists(ctx, rule.SelfRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "RuleS2S validation failed for deletion: %s", rule.Key())
	}

	return nil
}

// =============================================================================
// ServiceAlias Validation
// =============================================================================

// ValidateServiceAliasForCreation validates a service alias for creation
func (s *ValidationService) ValidateServiceAliasForCreation(ctx context.Context, alias models.ServiceAlias) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	aliasValidator := validator.GetServiceAliasValidator()

	if err := aliasValidator.ValidateForCreation(ctx, &alias); err != nil {
		return errors.Wrapf(err, "service alias validation failed for creation: %s", alias.Key())
	}

	return nil
}

// ValidateServiceAliasForUpdate validates a service alias for update
func (s *ValidationService) ValidateServiceAliasForUpdate(ctx context.Context, oldAlias, newAlias models.ServiceAlias) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	aliasValidator := validator.GetServiceAliasValidator()

	if err := aliasValidator.ValidateForUpdate(ctx, oldAlias, newAlias); err != nil {
		return errors.Wrapf(err, "service alias validation failed for update: %s", newAlias.Key())
	}

	return nil
}

// ValidateServiceAliasForDeletion validates a service alias for deletion
func (s *ValidationService) ValidateServiceAliasForDeletion(ctx context.Context, alias models.ServiceAlias) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	aliasValidator := validator.GetServiceAliasValidator()

	// Basic existence check for deletion - alias should exist
	if err := aliasValidator.ValidateExists(ctx, alias.SelfRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "service alias validation failed for deletion: %s", alias.Key())
	}

	return nil
}

// =============================================================================
// AddressGroupBindingPolicy Validation
// =============================================================================

// ValidateAddressGroupBindingPolicyForCreation validates an address group binding policy for creation
func (s *ValidationService) ValidateAddressGroupBindingPolicyForCreation(ctx context.Context, policy models.AddressGroupBindingPolicy) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	policyValidator := validator.GetAddressGroupBindingPolicyValidator()

	if err := policyValidator.ValidateForCreation(ctx, &policy); err != nil {
		return errors.Wrapf(err, "address group binding policy validation failed for creation: %s", policy.Key())
	}

	return nil
}

// ValidateAddressGroupBindingPolicyForUpdate validates an address group binding policy for update
func (s *ValidationService) ValidateAddressGroupBindingPolicyForUpdate(ctx context.Context, oldPolicy, newPolicy models.AddressGroupBindingPolicy) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	policyValidator := validator.GetAddressGroupBindingPolicyValidator()

	if err := policyValidator.ValidateForUpdate(ctx, oldPolicy, &newPolicy); err != nil {
		return errors.Wrapf(err, "address group binding policy validation failed for update: %s", newPolicy.Key())
	}

	return nil
}

// ValidateAddressGroupBindingPolicyForDeletion validates an address group binding policy for deletion
func (s *ValidationService) ValidateAddressGroupBindingPolicyForDeletion(ctx context.Context, policy models.AddressGroupBindingPolicy) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	policyValidator := validator.GetAddressGroupBindingPolicyValidator()

	// Basic existence check for deletion - policy should exist
	if err := policyValidator.ValidateExists(ctx, policy.SelfRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "address group binding policy validation failed for deletion: %s", policy.Key())
	}

	return nil
}

// =============================================================================
// Network Validation
// =============================================================================

// ValidateNetworkForCreation validates a network for creation
func (s *ValidationService) ValidateNetworkForCreation(ctx context.Context, network models.Network) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	networkValidator := validator.GetNetworkValidator()

	if err := networkValidator.ValidateForCreation(ctx, network); err != nil {
		return errors.Wrapf(err, "network validation failed for creation: %s", network.Key())
	}

	return nil
}

// ValidateNetworkForUpdate validates a network for update
func (s *ValidationService) ValidateNetworkForUpdate(ctx context.Context, oldNetwork, newNetwork models.Network) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	networkValidator := validator.GetNetworkValidator()

	if err := networkValidator.ValidateForUpdate(ctx, oldNetwork, newNetwork); err != nil {
		return errors.Wrapf(err, "network validation failed for update: %s", newNetwork.Key())
	}

	return nil
}

// ValidateNetworkForDeletion validates a network for deletion
func (s *ValidationService) ValidateNetworkForDeletion(ctx context.Context, network models.Network) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	networkValidator := validator.GetNetworkValidator()

	// Basic existence check for deletion - network should exist
	if err := networkValidator.ValidateExists(ctx, network.SelfRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "network validation failed for deletion: %s", network.Key())
	}

	return nil
}

// =============================================================================
// NetworkBinding Validation
// =============================================================================

// ValidateNetworkBindingForCreation validates a network binding for creation
func (s *ValidationService) ValidateNetworkBindingForCreation(ctx context.Context, binding models.NetworkBinding) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetNetworkBindingValidator()

	if err := bindingValidator.ValidateForCreation(ctx, binding); err != nil {
		return errors.Wrapf(err, "network binding validation failed for creation: %s", binding.Key())
	}

	return nil
}

// ValidateNetworkBindingForUpdate validates a network binding for update
func (s *ValidationService) ValidateNetworkBindingForUpdate(ctx context.Context, oldBinding, newBinding models.NetworkBinding) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetNetworkBindingValidator()

	if err := bindingValidator.ValidateForUpdate(ctx, oldBinding, newBinding); err != nil {
		return errors.Wrapf(err, "network binding validation failed for update: %s", newBinding.Key())
	}

	return nil
}

// ValidateNetworkBindingForDeletion validates a network binding for deletion
func (s *ValidationService) ValidateNetworkBindingForDeletion(ctx context.Context, binding models.NetworkBinding) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetNetworkBindingValidator()

	// Basic existence check for deletion - binding should exist
	if err := bindingValidator.ValidateExists(ctx, binding.SelfRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "network binding validation failed for deletion: %s", binding.Key())
	}

	return nil
}

// =============================================================================
// Bulk Validation Methods
// =============================================================================

// ValidateMultipleResourcesForOperation validates multiple resources for a specific operation
func (s *ValidationService) ValidateMultipleResourcesForOperation(ctx context.Context, resources []interface{}, operation string) error {
	var validationErrors []error

	for i, resource := range resources {
		var err error

		switch r := resource.(type) {
		case models.Service:
			switch operation {
			case "create":
				err = s.ValidateServiceForCreation(ctx, r)
			case "update":
				// For bulk operations, we assume the old version is fetched separately
				err = fmt.Errorf("bulk update validation requires old version for service %s", r.Key())
			case "delete":
				err = s.ValidateServiceForDeletion(ctx, r)
			}

		case models.AddressGroup:
			switch operation {
			case "create":
				err = s.ValidateAddressGroupForCreation(ctx, r)
			case "update":
				err = fmt.Errorf("bulk update validation requires old version for address group %s", r.Key())
			case "delete":
				err = s.ValidateAddressGroupForDeletion(ctx, r)
			}

		case models.RuleS2S:
			switch operation {
			case "create":
				err = s.ValidateRuleS2SForCreation(ctx, r)
			case "update":
				err = fmt.Errorf("bulk update validation requires old version for RuleS2S %s", r.Key())
			case "delete":
				err = s.ValidateRuleS2SForDeletion(ctx, r)
			}

		default:
			err = fmt.Errorf("unsupported resource type for validation: %T", r)
		}

		if err != nil {
			validationErrors = append(validationErrors, errors.Wrapf(err, "validation failed for resource %d", i))
		}
	}

	if len(validationErrors) > 0 {
		var errorMessage string
		for _, err := range validationErrors {
			errorMessage += err.Error() + "; "
		}
		return fmt.Errorf("bulk validation failed: %s", errorMessage)
	}

	return nil
}

// =============================================================================
// Validation With Existing Reader
// =============================================================================

// ValidateWithReader provides validation using an existing reader (for performance optimization)
type ValidateWithReaderOptions struct {
	Reader ports.Reader
}

// ValidateServiceForCreationWithReader validates a service for creation using existing reader
func (s *ValidationService) ValidateServiceForCreationWithReader(ctx context.Context, service models.Service, reader ports.Reader) error {
	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	if err := serviceValidator.ValidateForCreation(ctx, service); err != nil {
		return errors.Wrapf(err, "service validation failed for creation: %s", service.Key())
	}

	return nil
}

// ValidateAddressGroupForCreationWithReader validates an address group for creation using existing reader
func (s *ValidationService) ValidateAddressGroupForCreationWithReader(ctx context.Context, addressGroup models.AddressGroup, reader ports.Reader) error {
	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	if err := addressGroupValidator.ValidateForCreation(ctx, addressGroup); err != nil {
		return errors.Wrapf(err, "address group validation failed for creation: %s", addressGroup.Key())
	}

	return nil
}

// ValidateRuleS2SForCreationWithReader validates a RuleS2S for creation using existing reader
func (s *ValidationService) ValidateRuleS2SForCreationWithReader(ctx context.Context, rule models.RuleS2S, reader ports.Reader) error {
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	if err := ruleValidator.ValidateForCreation(ctx, rule); err != nil {
		return errors.Wrapf(err, "RuleS2S validation failed for creation: %s", rule.Key())
	}

	return nil
}

// =============================================================================
// Cross-Resource Validation
// =============================================================================

// ValidateResourceDependencies validates dependencies between resources
func (s *ValidationService) ValidateResourceDependencies(ctx context.Context, resource interface{}) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	switch r := resource.(type) {
	case models.AddressGroupBinding:
		// Validate that both service and address group exist
		_, err := reader.GetServiceByID(ctx, models.ResourceIdentifier{
			Name:      r.ServiceRef.Name,
			Namespace: r.ServiceRef.Namespace,
		})
		if err != nil {
			return errors.Wrapf(err, "service dependency validation failed for binding %s", r.Key())
		}

		_, err = reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{
			Name:      r.AddressGroupRef.Name,
			Namespace: r.AddressGroupRef.Namespace,
		})
		if err != nil {
			return errors.Wrapf(err, "address group dependency validation failed for binding %s", r.Key())
		}

	case models.RuleS2S:
		// Validate that both service aliases exist
		_, err := reader.GetServiceAliasByID(ctx, models.ResourceIdentifier{
			Name:      r.ServiceRef.Name,
			Namespace: r.ServiceRef.Namespace,
		})
		if err != nil {
			return errors.Wrapf(err, "target service alias dependency validation failed for rule %s", r.Key())
		}

		_, err = reader.GetServiceAliasByID(ctx, models.ResourceIdentifier{
			Name:      r.ServiceLocalRef.Name,
			Namespace: r.ServiceLocalRef.Namespace,
		})
		if err != nil {
			return errors.Wrapf(err, "local service alias dependency validation failed for rule %s", r.Key())
		}

	case models.ServiceAlias:
		// Validate that the referenced service exists
		_, err := reader.GetServiceByID(ctx, models.ResourceIdentifier{
			Name:      r.ServiceRef.Name,
			Namespace: r.ServiceRef.Namespace,
		})
		if err != nil {
			return errors.Wrapf(err, "service dependency validation failed for alias %s", r.Key())
		}
	}

	return nil
}
