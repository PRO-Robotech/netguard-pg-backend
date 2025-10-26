package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// NetworkBindingValidator validates NetworkBinding resources
type NetworkBindingValidator struct {
	*BaseValidator
}

// NewNetworkBindingValidator creates a new network binding validator
func NewNetworkBindingValidator(reader ports.Reader) *NetworkBindingValidator {
	return &NetworkBindingValidator{
		BaseValidator: NewBaseValidator(reader, "NetworkBinding", func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
			return reader.ListNetworkBindings(ctx, func(binding models.NetworkBinding) error {
				return consume(&binding)
			}, scope)
		}),
	}
}

// ValidateExists checks if a network binding exists
func (v *NetworkBindingValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.NetworkBinding).Key()
	})
}

// ValidateReferences validates that referenced Network and AddressGroup exist
func (v *NetworkBindingValidator) ValidateReferences(ctx context.Context, binding models.NetworkBinding) error {
	// Validate Network reference
	networkID := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
	network, err := v.reader.GetNetworkByID(ctx, networkID)
	if err != nil {
		return errors.Wrapf(err, "invalid network reference in binding %s", binding.Key())
	}
	if network == nil {
		return errors.Errorf("network %s not found", networkID.Key())
	}

	if !isNetworkReady(network) {
		return errors.Errorf("network %s is not ready yet (Ready=False) - binding can only be created between Ready resources", networkID.Key())
	}

	// Validate AddressGroup reference
	addressGroupID := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}
	addressGroup, err := v.reader.GetAddressGroupByID(ctx, addressGroupID)
	if err != nil {
		return errors.Wrapf(err, "invalid address group reference in binding %s", binding.Key())
	}
	if addressGroup == nil {
		return errors.Errorf("address group %s not found", addressGroupID.Key())
	}

	if !isAddressGroupReady(addressGroup) {
		return errors.Errorf("address group %s is not ready yet (Ready=False) - binding can only be created between Ready resources", addressGroupID.Key())
	}

	return nil
}

// ValidateForCreation validates a network binding for creation
func (v *NetworkBindingValidator) ValidateForCreation(ctx context.Context, binding models.NetworkBinding) error {
	// This prevents creation of entities with the same namespace/name combination
	keyExtractor := func(entity interface{}) string {
		if nb, ok := entity.(*models.NetworkBinding); ok {
			return nb.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, binding.ResourceIdentifier, keyExtractor); err != nil {
		return err // Return the detailed EntityAlreadyExistsError with logging and context
	}

	if err := v.ValidateReferences(ctx, binding); err != nil {
		return err
	}

	networkID := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
	network, err := v.reader.GetNetworkByID(ctx, networkID)
	if err != nil {
		return errors.Wrap(err, "failed to get network")
	}
	if network != nil && network.IsBound {
		// Network is already bound - get the binding details for error message
		agRefInfo := "unknown"
		if network.AddressGroupRef != nil {
			agRefInfo = fmt.Sprintf("%s/%s", binding.Namespace, network.AddressGroupRef.Name)
		}
		return errors.Errorf("network %s is already bound to address group %s (each network can only be bound to one address group)",
			networkID.Key(), agRefInfo)
	}

	return nil
}

// ValidateForPostCommit validates a network binding after it has been committed to database
// This skips duplicate checking since the entity already exists in the database
func (v *NetworkBindingValidator) ValidateForPostCommit(ctx context.Context, binding models.NetworkBinding) error {
	// This method is called AFTER the entity is saved to database, so existence is expected

	if err := v.ValidateReferences(ctx, binding); err != nil {
		return err
	}

	return nil
}

// ValidateForUpdate validates a network binding for update
func (v *NetworkBindingValidator) ValidateForUpdate(ctx context.Context, oldBinding, newBinding models.NetworkBinding) error {
	// Validate references
	if err := v.ValidateReferences(ctx, newBinding); err != nil {
		return err
	}

	// Check if binding exists
	if err := v.ValidateExists(ctx, models.ResourceIdentifier{Name: oldBinding.Name, Namespace: oldBinding.Namespace}); err != nil {
		return err
	}

	// Check if name or namespace changed (should not be allowed)
	if oldBinding.Name != newBinding.Name || oldBinding.Namespace != newBinding.Namespace {
		return errors.New("network binding name and namespace cannot be changed")
	}

	// If network reference changed, validate that the new network is not already bound
	if oldBinding.NetworkRef.Name != newBinding.NetworkRef.Name {
		networkID := models.ResourceIdentifier{Name: newBinding.NetworkRef.Name, Namespace: newBinding.Namespace}
		network, err := v.reader.GetNetworkByID(ctx, networkID)
		if err != nil {
			return errors.Wrap(err, "failed to get network")
		}

		if network != nil && network.IsBound {
			return errors.Errorf("network %s is already bound to another address group", networkID.Key())
		}
	}

	return nil
}

// CheckDependencies checks if the network binding can be deleted
func (v *NetworkBindingValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	return nil
}

// isNetworkReady checks if Network has Ready=True condition
func isNetworkReady(network *models.Network) bool {
	if network == nil || network.Meta.Conditions == nil {
		return false
	}
	for _, cond := range network.Meta.Conditions {
		if cond.Type == "Ready" && cond.Status == "True" {
			return true
		}
	}
	return false
}

// isAddressGroupReady checks if AddressGroup has Ready=True condition
func isAddressGroupReady(ag *models.AddressGroup) bool {
	if ag == nil || ag.Meta.Conditions == nil {
		return false
	}
	for _, cond := range ag.Meta.Conditions {
		if cond.Type == "Ready" && cond.Status == "True" {
			return true
		}
	}
	return false
}
