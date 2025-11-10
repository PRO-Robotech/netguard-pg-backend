package validation

import (
	"context"
	"net"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// NetworkValidator validates Network resources
type NetworkValidator struct {
	*BaseValidator
	reader ports.Reader
}

// NewNetworkValidator creates a new network validator
func NewNetworkValidator(reader ports.Reader) *NetworkValidator {
	return &NetworkValidator{
		BaseValidator: NewBaseValidator(reader, "Network", func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
			return reader.ListNetworks(ctx, func(network models.Network) error {
				return consume(&network)
			}, scope)
		}),
		reader: reader,
	}
}

// ValidateExists checks if a network exists
func (v *NetworkValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.Network).Key()
	})
}

// ValidateCIDR validates the CIDR format
func (v *NetworkValidator) ValidateCIDR(cidr string) error {
	if cidr == "" {
		return errors.New("CIDR cannot be empty")
	}

	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return errors.Wrapf(err, "invalid CIDR format: %s", cidr)
	}

	return nil
}

// ValidateCIDROverlap checks if the given CIDR overlaps with any existing network
// This includes exact matches, partial overlaps, and containment relationships
func (v *NetworkValidator) ValidateCIDROverlap(ctx context.Context, cidr string, excludeNetwork *models.ResourceIdentifier) error {
	overlappingNetworks, err := v.reader.GetNetworksOverlappingCIDR(ctx, cidr)
	if err != nil {
		return errors.Wrapf(err, "failed to check for overlapping networks")
	}

	for _, network := range overlappingNetworks {
		// Skip if this is the network we're updating
		if excludeNetwork != nil &&
			network.Namespace == excludeNetwork.Namespace &&
			network.Name == excludeNetwork.Name {
			continue
		}

		return &ports.CIDROverlapError{
			CIDR:            cidr,
			OverlappingCIDR: network.CIDR,
			NetworkName:     network.Key(),
			Err:             errors.New("CIDR overlap detected"),
		}
	}

	return nil
}

// ValidateForCreation validates a network for creation
func (v *NetworkValidator) ValidateForCreation(ctx context.Context, network models.Network) error {
	keyExtractor := func(entity interface{}) string {
		if n, ok := entity.(*models.Network); ok {
			return n.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, network.ResourceIdentifier, keyExtractor); err != nil {
		return err // Return the detailed EntityAlreadyExistsError with logging and context
	}

	if err := v.ValidateCIDR(network.CIDR); err != nil {
		return err
	}

	if err := v.ValidateCIDROverlap(ctx, network.CIDR, nil); err != nil {
		return err
	}

	return nil
}

// ValidateForUpdate validates a network for update
func (v *NetworkValidator) ValidateForUpdate(ctx context.Context, oldNetwork, newNetwork models.Network) error {
	// Validate CIDR format
	if err := v.ValidateCIDR(newNetwork.CIDR); err != nil {
		return err
	}

	networkID := &models.ResourceIdentifier{Name: newNetwork.Name, Namespace: newNetwork.Namespace}
	if err := v.ValidateCIDROverlap(ctx, newNetwork.CIDR, networkID); err != nil {
		return err
	}

	if err := v.ValidateExists(ctx, models.ResourceIdentifier{Name: oldNetwork.Name, Namespace: oldNetwork.Namespace}); err != nil {
		return err
	}

	if oldNetwork.Name != newNetwork.Name || oldNetwork.Namespace != newNetwork.Namespace {
		return errors.New("network name and namespace cannot be changed")
	}

	return nil
}

// CheckDependencies checks if the network can be deleted
func (v *NetworkValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Check if there are any NetworkBindings that reference this network
	hasBindings := false
	err := v.reader.ListNetworkBindings(ctx, func(binding models.NetworkBinding) error {
		if binding.NetworkRef.Name == id.Name && binding.Namespace == id.Namespace {
			hasBindings = true
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to check network bindings")
	}

	if hasBindings {
		return errors.Errorf("cannot delete network %s: it is referenced by network bindings", id.Key())
	}

	return nil
}
