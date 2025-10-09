package validation

import (
	"context"
	"fmt"
	"net"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"

	"github.com/pkg/errors"
)

// ValidateExists checks if an address group exists
func (v *AddressGroupValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.AddressGroup).Key()
	})
}

// ValidateReferences checks if all references in an address group are valid
func (v *AddressGroupValidator) ValidateReferences(ctx context.Context, group models.AddressGroup) error {
	return nil
}

// ValidateForCreation validates an address group before creation
func (v *AddressGroupValidator) ValidateForCreation(ctx context.Context, group models.AddressGroup) error {
	keyExtractor := func(entity interface{}) string {
		if ag, ok := entity.(*models.AddressGroup); ok {
			return ag.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, group.ResourceIdentifier, keyExtractor); err != nil {
		return err // Return the detailed EntityAlreadyExistsError with logging and context
	}

	// Networks can't be modified or added during creation
	if group.Networks != nil && len(group.Networks) > 0 {
		return fmt.Errorf("networks can't be modified or added during creation")
	}

	if err := v.validateHostReferences(ctx, group.Hosts, group.ResourceIdentifier); err != nil {
		return err
	}

	if err := v.validateHostExclusivity(ctx, group.Hosts, group.ResourceIdentifier); err != nil {
		return err
	}

	return v.ValidateReferences(ctx, group)
}

// ValidateForPostCommit validates an address group after it has been committed to database
// This skips duplicate checking since the entity already exists in the database
func (v *AddressGroupValidator) ValidateForPostCommit(ctx context.Context, group models.AddressGroup) error {
	return v.ValidateReferences(ctx, group)
}

// ValidateForUpdate validates an address group before update
func (v *AddressGroupValidator) ValidateForUpdate(ctx context.Context, oldGroup, newGroup models.AddressGroup) error {
	if err := v.validateNetworks(newGroup.Networks); err != nil {
		return err
	}

	if err := v.validateHostReferences(ctx, newGroup.Hosts, newGroup.ResourceIdentifier); err != nil {
		return err
	}

	if err := v.validateHostExclusivity(ctx, newGroup.Hosts, newGroup.ResourceIdentifier); err != nil {
		return err
	}

	return v.ValidateReferences(ctx, newGroup)
}

// validateNetworks validates the Networks field of an AddressGroup
func (v *AddressGroupValidator) validateNetworks(networks []models.NetworkItem) error {
	for i, network := range networks {
		// Validate required fields
		if network.Name == "" {
			return fmt.Errorf("network item %d: name is required", i)
		}

		if network.CIDR == "" {
			return fmt.Errorf("network item %d (%s): CIDR is required", i, network.Name)
		}

		// Validate CIDR format
		if _, _, err := net.ParseCIDR(network.CIDR); err != nil {
			return fmt.Errorf("network item %d (%s): invalid CIDR format '%s': %v", i, network.Name, network.CIDR, err)
		}

		if network.Kind == "" {
			return fmt.Errorf("network item %d (%s): kind is required", i, network.Name)
		}
	}

	return nil
}

// validateHostReferences validates host references in AddressGroup (existence and format)
func (v *AddressGroupValidator) validateHostReferences(ctx context.Context, hosts []netguardv1beta1.ObjectReference, currentAG models.ResourceIdentifier) error {
	if len(hosts) == 0 {
		return nil // No hosts to validate
	}

	for i, host := range hosts {
		if host.Name == "" {
			return fmt.Errorf("host reference %d: name is required", i)
		}

		if host.Kind != "Host" {
			return fmt.Errorf("host reference %d (%s): invalid kind '%s' - must be 'Host' (not 'Hosts')", i, host.Name, host.Kind)
		}

		expectedAPIVersion := "netguard.sgroups.io/v1beta1"
		if host.APIVersion != expectedAPIVersion {
			return fmt.Errorf("host reference %d (%s): invalid apiVersion '%s' - must be '%s'", i, host.Name, host.APIVersion, expectedAPIVersion)
		}

		hostID := models.ResourceIdentifier{
			Name:      host.Name,
			Namespace: currentAG.Namespace, // Hosts must be in same namespace as AddressGroup
		}

		_, err := v.reader.GetHostByID(ctx, hostID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				return fmt.Errorf("host reference %d: host '%s' does not exist in namespace '%s'", i, host.Name, currentAG.Namespace)
			}
			return fmt.Errorf("host reference %d: failed to validate host '%s' existence: %v", i, host.Name, err)
		}
	}

	return nil
}

// validateHostExclusivity validates that hosts in the AddressGroup don't belong to other AddressGroups
// Each host can belong to only one AddressGroup (exclusivity constraint)
func (v *AddressGroupValidator) validateHostExclusivity(ctx context.Context, hosts []netguardv1beta1.ObjectReference, currentAG models.ResourceIdentifier) error {
	if len(hosts) == 0 {
		return nil // No hosts to validate
	}

	// Check each host against all existing AddressGroups
	for _, host := range hosts {
		// List all AddressGroups to check for host conflicts
		err := v.reader.ListAddressGroups(ctx, func(ag models.AddressGroup) error {
			if ag.Key() == currentAG.Key() {
				return nil
			}
			for _, existingHost := range ag.Hosts {
				if host.Name == existingHost.Name && currentAG.Namespace == ag.Namespace {
					return fmt.Errorf("host %s/%s already belongs to AddressGroup %s - each host can belong to only one AddressGroup",
						currentAG.Namespace, host.Name, ag.Key())
				}
			}
			return nil
		}, ports.EmptyScope{})

		if err != nil {
			return err
		}
	}

	return nil
}

// ValidateSgroupSyncForHosts validates that hosts can be synchronized with SGROUP
// This is a pre-validation step that tests SGROUP synchronization before saving to database
func (v *AddressGroupValidator) ValidateSgroupSyncForHosts(ctx context.Context, hosts []netguardv1beta1.ObjectReference, currentAG models.ResourceIdentifier, syncManager interfaces.SyncManager) error {
	if len(hosts) == 0 {
		return nil // No hosts to validate
	}

	if syncManager == nil {
		return nil
	}

	for i, hostRef := range hosts {
		// Get the actual host entity from database
		hostID := models.ResourceIdentifier{
			Name:      hostRef.Name,
			Namespace: currentAG.Namespace, // Host must be in same namespace as AddressGroup
		}

		host, err := v.reader.GetHostByID(ctx, hostID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				return fmt.Errorf("host reference %d: host '%s' does not exist in namespace '%s'", i, hostRef.Name, currentAG.Namespace)
			}
			return fmt.Errorf("host reference %d: failed to load host '%s' for SGROUP validation: %v", i, hostRef.Name, err)
		}

		err = syncManager.SyncEntity(ctx, host, types.SyncOperationUpsert)
		if err != nil {
			return fmt.Errorf("SGROUP synchronization failed for host '%s' in namespace '%s': %v - the host cannot be added to this AddressGroup due to SGROUP constraints",
				hostRef.Name, currentAG.Namespace, err)
		}
	}

	return nil
}

// CheckDependencies checks if there are dependencies before deleting an address group
func (v *AddressGroupValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Check Services referencing the address group to be deleted
	hasServices := false
	err := v.reader.ListServices(ctx, func(service models.Service) error {
		for _, agRef := range service.AddressGroups {
			if models.AddressGroupRefKey(agRef) == id.Key() {
				hasServices = true
				break
			}
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to check services")
	}

	if hasServices {
		return NewDependencyExistsError("address_group", id.Key(), "service")
	}

	// Check AddressGroupBindings referencing the address group to be deleted
	hasBindings := false
	err = v.reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		if binding.AddressGroupRefKey() == id.Key() {
			hasBindings = true
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to check address group bindings")
	}

	if hasBindings {
		return NewDependencyExistsError("address_group", id.Key(), "address_group_binding")
	}

	return nil
}
