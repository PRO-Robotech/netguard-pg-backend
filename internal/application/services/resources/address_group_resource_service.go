package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// AddressGroupConditionManagerInterface provides condition processing for address groups
type AddressGroupConditionManagerInterface interface {
	ProcessAddressGroupConditions(ctx context.Context, addressGroup *models.AddressGroup) error
	ProcessAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error
	ProcessAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error
	ProcessAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error

	// Save methods for condition persistence
	SaveAddressGroupConditions(ctx context.Context, addressGroup *models.AddressGroup) error
	SaveAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error
	SaveAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error
	SaveAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error
}

// AddressGroupResourceService handles AddressGroup, AddressGroupBinding, AddressGroupPortMapping, and AddressGroupBindingPolicy operations
type AddressGroupResourceService struct {
	registry           ports.Registry
	syncManager        interfaces.SyncManager
	conditionManager   AddressGroupConditionManagerInterface
	validationService  *ValidationService
	ruleS2SRegenerator RuleS2SRegenerator
	hostService        *HostResourceService
}

// RuleS2SRegenerator interface is now defined in interfaces.go to avoid circular dependencies

// NewAddressGroupResourceService creates a new AddressGroupResourceService
func NewAddressGroupResourceService(
	registry ports.Registry,
	syncManager interfaces.SyncManager,
	conditionManager AddressGroupConditionManagerInterface,
	validationService *ValidationService,
	hostService *HostResourceService,
) *AddressGroupResourceService {
	return &AddressGroupResourceService{
		registry:           registry,
		syncManager:        syncManager,
		conditionManager:   conditionManager,
		validationService:  validationService,
		ruleS2SRegenerator: nil, // Will be set later via SetRuleS2SRegenerator
		hostService:        hostService,
	}
}

// SetRuleS2SRegenerator sets the RuleS2S regenerator (used to avoid circular dependencies)
func (s *AddressGroupResourceService) SetRuleS2SRegenerator(regenerator RuleS2SRegenerator) {
	s.ruleS2SRegenerator = regenerator
}

// =============================================================================
// AddressGroup Operations
// =============================================================================

// GetAddressGroups returns all address groups within scope
func (s *AddressGroupResourceService) GetAddressGroups(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var addressGroups []models.AddressGroup
	err = reader.ListAddressGroups(ctx, func(addressGroup models.AddressGroup) error {
		addressGroups = append(addressGroups, addressGroup)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list address groups")
	}
	return addressGroups, nil
}

// GetAddressGroupByID returns address group by ID
func (s *AddressGroupResourceService) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return reader.GetAddressGroupByID(ctx, id)
}

// GetAddressGroupsByIDs returns multiple address groups by IDs
func (s *AddressGroupResourceService) GetAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroup, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var addressGroups []models.AddressGroup
	for _, id := range ids {
		addressGroup, err := reader.GetAddressGroupByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Skip not found address groups
			}
			return nil, errors.Wrapf(err, "failed to get address group %s", id.Key())
		}
		addressGroups = append(addressGroups, *addressGroup)
	}
	return addressGroups, nil
}

// CreateAddressGroup creates a new address group
func (s *AddressGroupResourceService) CreateAddressGroup(ctx context.Context, addressGroup models.AddressGroup) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Validate address group for creation
	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	if err := addressGroupValidator.ValidateForCreation(ctx, addressGroup); err != nil {
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Set AddressGroupName before syncing
	if addressGroup.Namespace != "" {
		addressGroup.AddressGroupName = fmt.Sprintf("%s/%s", addressGroup.Namespace, addressGroup.Name)
	} else {
		addressGroup.AddressGroupName = addressGroup.Name
	}

	// Sync address group (this will create it)
	if err = s.syncAddressGroups(ctx, writer, []models.AddressGroup{addressGroup}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to create address group")
	}

	if s.syncManager != nil && len(addressGroup.Hosts) > 0 {
		if err = s.validateHostsSGroupSync(ctx, addressGroup.Hosts, addressGroup.ResourceIdentifier); err != nil {
			return errors.Wrap(err, "SGROUP synchronization validation failed")
		}
	} else {
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupConditions(ctx, &addressGroup); err != nil {
			klog.Errorf("Failed to process address group conditions for %s/%s: %v",
				addressGroup.Namespace, addressGroup.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	readerForNetworks, err := s.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("Failed to get reader for re-reading AddressGroup %s/%s: %v", addressGroup.Namespace, addressGroup.Name, err)
	} else {
		defer readerForNetworks.Close()
		createdAG, err := readerForNetworks.GetAddressGroupByID(ctx, addressGroup.ResourceIdentifier)
		if err != nil {
			klog.Errorf("Failed to re-read AddressGroup %s/%s after creation: %v", addressGroup.Namespace, addressGroup.Name, err)
		} else {
			addressGroup = *createdAG
		}
	}

	// Sync with external systems after successful creation
	s.syncAddressGroupsWithSGroups(ctx, []models.AddressGroup{addressGroup}, types.SyncOperationUpsert)

	// Update Host.isBound status for hosts in this AddressGroup
	if s.hostService != nil && len(addressGroup.Hosts) > 0 {
		if err := s.hostService.UpdateHostBindingStatus(ctx, nil, &addressGroup); err != nil {
		} else {
			if err := s.syncSpecHostsWithSGroups(ctx, addressGroup.Hosts, addressGroup.ResourceIdentifier); err != nil {
			}
		}
	}

	return nil
}

// UpdateAddressGroup updates an existing address group
func (s *AddressGroupResourceService) UpdateAddressGroup(ctx context.Context, addressGroup models.AddressGroup) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Get existing address group for validation
	existingAddressGroup, err := reader.GetAddressGroupByID(ctx, addressGroup.ResourceIdentifier)
	if err != nil {
		return errors.Wrap(err, "failed to get existing address group")
	}

	// Validate address group for update
	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	if err := addressGroupValidator.ValidateForUpdate(ctx, *existingAddressGroup, addressGroup); err != nil {
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Set AddressGroupName before syncing
	if addressGroup.Namespace != "" {
		addressGroup.AddressGroupName = fmt.Sprintf("%s/%s", addressGroup.Namespace, addressGroup.Name)
	} else {
		addressGroup.AddressGroupName = addressGroup.Name
	}

	// Sync address group (this will update it)
	if err = s.syncAddressGroups(ctx, writer, []models.AddressGroup{addressGroup}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to update address group")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	if s.syncManager != nil {
		if err := s.syncHostChangesWithSGroup(ctx, existingAddressGroup, &addressGroup); err != nil {
		}
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupConditions(ctx, &addressGroup); err != nil {
			klog.Errorf("Failed to process address group conditions for %s/%s: %v",
				addressGroup.Namespace, addressGroup.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	readerForNetworks, err := s.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("Failed to get reader for re-reading AddressGroup %s/%s: %v", addressGroup.Namespace, addressGroup.Name, err)
	} else {
		defer readerForNetworks.Close()
		updatedAG, err := readerForNetworks.GetAddressGroupByID(ctx, addressGroup.ResourceIdentifier)
		if err != nil {
			klog.Errorf("Failed to re-read AddressGroup %s/%s after update: %v", addressGroup.Namespace, addressGroup.Name, err)
		} else {
			addressGroup = *updatedAG
		}
	}

	// Sync with external systems after successful update
	s.syncAddressGroupsWithSGroups(ctx, []models.AddressGroup{addressGroup}, types.SyncOperationUpsert)

	return nil
}

// SyncAddressGroups synchronizes multiple address groups
func (s *AddressGroupResourceService) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope, syncOp models.SyncOp) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Load old AddressGroup states before syncing for SGROUP validation
	var oldAddressGroups map[string]*models.AddressGroup
	if syncOp == models.SyncOpUpsert && s.syncManager != nil {
		oldAddressGroups = make(map[string]*models.AddressGroup)
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get reader for old state loading")
		}
		defer reader.Close()

		for _, ag := range addressGroups {
			oldAG, err := reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{
				Name:      ag.Name,
				Namespace: ag.Namespace,
			})
			if err != nil && !errors.Is(err, ports.ErrNotFound) {
				return errors.Wrapf(err, "failed to load old state for AddressGroup %s", ag.Key())
			}
			if err == nil {
				oldAddressGroups[ag.Key()] = oldAG
			}
		}
	}

	if err = s.syncAddressGroups(ctx, writer, addressGroups, syncOp); err != nil {
		return errors.Wrap(err, "failed to sync address groups")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	if syncOp != models.SyncOpDelete {
		readerForNetworks, err := s.registry.Reader(ctx)
		if err != nil {
			klog.Errorf("Failed to get reader for re-reading AddressGroups: %v", err)
		} else {
			defer readerForNetworks.Close()
			for i := range addressGroups {
				updatedAG, err := readerForNetworks.GetAddressGroupByID(ctx, addressGroups[i].ResourceIdentifier)
				if err != nil {
					klog.Errorf("Failed to re-read AddressGroup %s/%s after sync: %v",
						addressGroups[i].Namespace, addressGroups[i].Name, err)
				} else {
					addressGroups[i] = *updatedAG
				}
			}
		}
	}

	if syncOp == models.SyncOpUpsert && s.syncManager != nil {
		if len(oldAddressGroups) > 0 {
			for _, newAG := range addressGroups {
				oldAG := oldAddressGroups[newAG.Key()]
				if oldAG != nil {
					if err := s.syncHostChangesWithSGroup(ctx, oldAG, &newAG); err != nil {
					}
				}
			}
		}
	}

	// Process conditions after successful commit for each address group (skip for DELETE operations)
	if s.conditionManager != nil && syncOp != models.SyncOpDelete {
		for i := range addressGroups {
			if err := s.conditionManager.ProcessAddressGroupConditions(ctx, &addressGroups[i]); err != nil {
				klog.Errorf("Failed to process address group conditions for %s/%s: %v",
					addressGroups[i].Namespace, addressGroups[i].Name, err)
			}
		}
	} else if syncOp == models.SyncOpDelete {
	}

	// Sync with external systems after successful batch sync (skip for DELETE - handled by DeleteAddressGroupsByIDs)
	if syncOp != models.SyncOpDelete {
		var externalSyncOp types.SyncOperation
		switch syncOp {
		case models.SyncOpUpsert:
			externalSyncOp = types.SyncOperationUpsert
		case models.SyncOpFullSync:
			externalSyncOp = types.SyncOperationUpsert // FullSync uses upsert for external systems
		default:
			externalSyncOp = types.SyncOperationUpsert
		}
		s.syncAddressGroupsWithSGroups(ctx, addressGroups, externalSyncOp)
		s.updateHostBindingStatusForSyncedAddressGroups(ctx, addressGroups, syncOp)
	} else {
		s.updateHostBindingStatusForSyncedAddressGroups(ctx, addressGroups, syncOp)
	}

	return nil
}

// DeleteAddressGroupsByIDs deletes address groups by IDs with reference architecture compliance
// Follows k8s-controller pattern: cascade delete bindings first, then AddressGroups, with proper external sync
func (s *AddressGroupResourceService) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	if len(ids) == 0 {
		return nil
	}

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for validation")
	}

	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	for _, id := range ids {
		if err := addressGroupValidator.CheckDependencies(ctx, id); err != nil {
			return errors.Wrapf(err, "cannot delete AddressGroup %s", id.Key())
		}
	}

	var addressGroupsToDelete []models.AddressGroup
	for _, id := range ids {
		addressGroup, err := reader.GetAddressGroupByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue
			}
			return errors.Wrapf(err, "failed to fetch AddressGroup %s", id.Key())
		}
		addressGroupsToDelete = append(addressGroupsToDelete, *addressGroup)
	}

	if len(addressGroupsToDelete) == 0 {
		return nil
	}

	var bindingsToDelete []models.ResourceIdentifier
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		for _, agToDelete := range addressGroupsToDelete {
			if binding.AddressGroupRef.Name == agToDelete.SelfRef.Name &&
				binding.AddressGroupRef.Namespace == agToDelete.SelfRef.Namespace {
				bindingsToDelete = append(bindingsToDelete, binding.SelfRef.ResourceIdentifier)
				break
			}
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to list AddressGroupBindings for cascading deletion")
	}

	var networkBindingsToDelete []models.ResourceIdentifier
	var networksToUpdate []models.ResourceIdentifier
	err = reader.ListNetworkBindings(ctx, func(binding models.NetworkBinding) error {
		for _, agToDelete := range addressGroupsToDelete {
			if binding.AddressGroupRef.Name == agToDelete.SelfRef.Name &&
				binding.SelfRef.Namespace == agToDelete.SelfRef.Namespace {
				networkBindingsToDelete = append(networkBindingsToDelete, binding.SelfRef.ResourceIdentifier)
				networksToUpdate = append(networksToUpdate, models.ResourceIdentifier{
					Name:      binding.NetworkRef.Name,
					Namespace: binding.SelfRef.Namespace,
				})
				break
			}
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to list NetworkBindings for cascading deletion")
	}

	if len(bindingsToDelete) > 0 {
		if err := s.DeleteAddressGroupBindingsByIDs(ctx, bindingsToDelete); err != nil {
			return errors.Wrap(err, "failed to cascade delete AddressGroupBindings")
		}
	}

	if len(networkBindingsToDelete) > 0 {
		networkBindingWriter, err := s.registry.Writer(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get writer for NetworkBinding deletion")
		}
		defer func() {
			if err != nil {
				networkBindingWriter.Abort()
			}
		}()

		if err := networkBindingWriter.DeleteNetworkBindingsByIDs(ctx, networkBindingsToDelete); err != nil {
			return errors.Wrap(err, "failed to cascade delete NetworkBindings")
		}

		if err := networkBindingWriter.Commit(); err != nil {
			return errors.Wrap(err, "failed to commit NetworkBinding deletion")
		}

		if len(networksToUpdate) > 0 {
			networkWriter, err := s.registry.Writer(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get writer for Network updates")
			}
			defer func() {
				if err != nil {
					networkWriter.Abort()
				}
			}()

			// Update each Network
			reader2, err := s.registry.Reader(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get reader for Network updates")
			}
			defer reader2.Close()

			for _, networkID := range networksToUpdate {
				network, err := reader2.GetNetworkByID(ctx, networkID)
				if err != nil {
					continue
				}

				network.IsBound = false
				network.BindingRef = nil
				network.AddressGroupRef = nil
				network.GetMeta().TouchOnWrite(fmt.Sprintf("binding-deleted-%d", time.Now().UnixNano()))

				if err := networkWriter.SyncNetworks(ctx, []models.Network{*network}, ports.EmptyScope{}); err != nil {
					continue
				}
			}

			if err := networkWriter.Commit(); err != nil {
				return errors.Wrap(err, "failed to commit Network updates")
			}

		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteAddressGroupsByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete address groups from storage")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	s.syncAddressGroupsWithSGroups(ctx, addressGroupsToDelete, types.SyncOperationDelete)

	if s.hostService != nil {
		for _, deletedAG := range addressGroupsToDelete {
			if len(deletedAG.Hosts) > 0 {
				if err := s.hostService.UpdateHostBindingStatus(ctx, &deletedAG, nil); err != nil {
				}
			}
		}
	}

	// Close reader
	reader.Close()

	return nil
}

// =============================================================================
// AddressGroupBinding Operations
// =============================================================================

// GetAddressGroupBindings returns all address group bindings within scope
func (s *AddressGroupResourceService) GetAddressGroupBindings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBinding, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var bindings []models.AddressGroupBinding
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		bindings = append(bindings, binding)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list address group bindings")
	}
	return bindings, nil
}

// GetAddressGroupBindingByID returns address group binding by ID
func (s *AddressGroupResourceService) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return reader.GetAddressGroupBindingByID(ctx, id)
}

// GetAddressGroupBindingsByIDs returns multiple address group bindings by IDs
func (s *AddressGroupResourceService) GetAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroupBinding, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var bindings []models.AddressGroupBinding
	for _, id := range ids {
		binding, err := reader.GetAddressGroupBindingByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Skip not found bindings
			}
			return nil, errors.Wrapf(err, "failed to get address group binding %s", id.Key())
		}
		bindings = append(bindings, *binding)
	}
	return bindings, nil
}

// CreateAddressGroupBinding creates a new address group binding
func (s *AddressGroupResourceService) CreateAddressGroupBinding(ctx context.Context, binding models.AddressGroupBinding) error {

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	reader, err := s.registry.ReaderFromWriter(ctx, writer)
	if err != nil {
		return errors.Wrap(err, "failed to get reader from writer")
	}
	defer reader.Close()

	// Validate binding for creation
	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetAddressGroupBindingValidator()

	if err := bindingValidator.ValidateForCreation(ctx, &binding); err != nil {
		return err
	}

	// Sync binding (this will create it)
	if err = s.syncAddressGroupBindings(ctx, writer, []models.AddressGroupBinding{binding}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to create address group binding")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Create associated AddressGroupPortMapping
	if err := s.SyncAddressGroupPortMappings(ctx, binding); err != nil {
		klog.Errorf("Failed to create AddressGroupPortMapping for %s/%s: %v", binding.Namespace, binding.Name, err)
		// Don't fail the binding creation if port mapping creation fails
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupBindingConditions(ctx, &binding); err != nil {
			klog.Errorf("Failed to process address group binding conditions for %s/%s: %v",
				binding.Namespace, binding.Name, err)
			// Don't fail the operation if condition processing fails
		}
	} else {
	}

	serviceID := models.ResourceIdentifier{
		Name:      binding.ServiceRef.Name,
		Namespace: binding.ServiceRef.Namespace,
	}
	if err := s.synchronizeServiceAddressGroups(ctx, serviceID); err != nil {
		klog.Errorf("Failed to synchronize Service.AddressGroups for service %s after binding creation: %v", serviceID.Key(), err)
		// Don't fail the operation, but log the critical issue
	}
	if s.ruleS2SRegenerator != nil {
		if err := s.ruleS2SRegenerator.NotifyServiceAddressGroupsChanged(ctx, serviceID); err != nil {
			klog.Errorf("Failed to notify RuleS2S service about AddressGroupBinding %s: %v",
				binding.Key(), err)
			// Don't fail the operation, but log the issue
		} else {
		}
	}

	if s.ruleS2SRegenerator != nil {
		bindingID := models.ResourceIdentifier{Name: binding.Name, Namespace: binding.Namespace}
		if err := s.ruleS2SRegenerator.RegenerateIEAgAgRulesForAddressGroupBinding(ctx, bindingID); err != nil {
			klog.Errorf("Failed to regenerate IEAgAg rules for new AddressGroupBinding %s: %v",
				binding.Key(), err)
			// Don't fail the operation if IEAgAg rule regeneration fails
		} else {
		}
	} else {
	}

	return nil
}

// UpdateAddressGroupBinding updates an existing address group binding
func (s *AddressGroupResourceService) UpdateAddressGroupBinding(ctx context.Context, binding models.AddressGroupBinding) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Get existing binding for validation
	existingBinding, err := reader.GetAddressGroupBindingByID(ctx, binding.ResourceIdentifier)
	if err != nil {
		return errors.Wrap(err, "failed to get existing address group binding")
	}

	// Validate binding for update
	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetAddressGroupBindingValidator()

	if err := bindingValidator.ValidateForUpdate(ctx, *existingBinding, &binding); err != nil {
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Sync binding (this will update it)
	if err = s.syncAddressGroupBindings(ctx, writer, []models.AddressGroupBinding{binding}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to update address group binding")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupBindingConditions(ctx, &binding); err != nil {
			klog.Errorf("Failed to process address group binding conditions for %s/%s: %v",
				binding.Namespace, binding.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	serviceID := models.ResourceIdentifier{Name: binding.ServiceRef.Name, Namespace: binding.ServiceRef.Namespace}
	if s.ruleS2SRegenerator != nil {
		if err := s.ruleS2SRegenerator.NotifyServiceAddressGroupsChanged(ctx, serviceID); err != nil {
			klog.Errorf("Failed to notify RuleS2S regenerator for updated AddressGroupBinding %s: %v",
				binding.Key(), err)
			// Don't fail the operation, but log the issue
		}
	}

	if s.ruleS2SRegenerator != nil {
		bindingID := models.ResourceIdentifier{Name: binding.Name, Namespace: binding.Namespace}
		if err := s.ruleS2SRegenerator.RegenerateIEAgAgRulesForAddressGroupBinding(ctx, bindingID); err != nil {
			klog.Errorf("Failed to regenerate IEAgAg rules for AddressGroupBinding %s: %v",
				binding.Key(), err)
		}
	}

	return nil
}

// SyncAddressGroupBindings synchronizes multiple address group bindings
func (s *AddressGroupResourceService) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, syncOp models.SyncOp) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.syncAddressGroupBindings(ctx, writer, bindings, syncOp); err != nil {
		return errors.Wrap(err, "failed to sync address group bindings")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Always update AddressGroupPortMappings to reflect binding changes, but skip condition processing for DELETE
	for _, binding := range bindings {
		if err := s.SyncAddressGroupPortMappingsWithSyncOp(ctx, binding, syncOp); err != nil {
			klog.Errorf("Failed to update AddressGroupPortMapping for %s/%s: %v", binding.Namespace, binding.Name, err)
			// Don't fail the batch operation if port mapping update fails
		}
	}

	if syncOp != models.SyncOpDelete {
		// Process conditions after successful commit for each address group binding
		if s.conditionManager != nil {
			for i := range bindings {
				if err := s.conditionManager.ProcessAddressGroupBindingConditions(ctx, &bindings[i]); err != nil {
					klog.Errorf("Failed to process address group binding conditions for %s/%s: %v",
						bindings[i].Namespace, bindings[i].Name, err)
				}
			}
		}
	}

	serviceIDs := make(map[string]models.ResourceIdentifier)
	for _, binding := range bindings {
		key := binding.ServiceRef.Namespace + "/" + binding.ServiceRef.Name
		serviceIDs[key] = models.ResourceIdentifier{Name: binding.ServiceRef.Name, Namespace: binding.ServiceRef.Namespace}
	}

	for _, serviceID := range serviceIDs {
		if err := s.synchronizeServiceAddressGroups(ctx, serviceID); err != nil {
			klog.Errorf("Failed to synchronize Service.AddressGroups for service %s after binding sync: %v", serviceID.Key(), err)
		}
	}

	if s.ruleS2SRegenerator != nil {
		// Notify for each unique service (including DELETE operations for dependency cleanup)
		for key, serviceID := range serviceIDs {
			if err := s.ruleS2SRegenerator.NotifyServiceAddressGroupsChanged(ctx, serviceID); err != nil {
				klog.Errorf("Failed to notify RuleS2S regenerator for service %s: %v", key, err)
				// Don't fail the operation, but log the issue
			}
		}
	}

	return nil
}

// DeleteAddressGroupBindingsByIDs deletes address group bindings by IDs
func (s *AddressGroupResourceService) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	// First, get the bindings before deletion to know which AddressGroups need port mapping regeneration
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	affectedAddressGroups := make(map[string]models.ResourceIdentifier)
	bindingsToRemove := make([]models.AddressGroupBinding, 0) // 🎯 NEW: Track bindings for Service updates

	for _, id := range ids {
		binding, err := reader.GetAddressGroupBindingByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Skip if binding already deleted
			}
			return errors.Wrapf(err, "failed to get binding %s for port mapping regeneration", id.Key())
		}

		serviceID := models.ResourceIdentifier{
			Name:      binding.ServiceRef.Name,
			Namespace: binding.ServiceRef.Namespace,
		}
		service, serviceErr := reader.GetServiceByID(ctx, serviceID)
		if serviceErr == nil {
			portStrs := make([]string, len(service.IngressPorts))
			for i, port := range service.IngressPorts {
				portStrs[i] = fmt.Sprintf("%s/%s", port.Port, port.Protocol)
			}
		} else {
		}

		// Track the AddressGroup that will need port mapping regeneration
		agKey := fmt.Sprintf("%s/%s", binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name)
		affectedAddressGroups[agKey] = models.ResourceIdentifier{
			Name:      binding.AddressGroupRef.Name,
			Namespace: binding.AddressGroupRef.Namespace,
		}

		bindingsToRemove = append(bindingsToRemove, *binding)

	}

	var writer ports.Writer
	if registryWithDeletes, ok := s.registry.(interface {
		WriterForDeletes(context.Context) (ports.Writer, error)
	}); ok {
		writer, err = registryWithDeletes.WriterForDeletes(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get delete writer with ReadCommitted isolation")
		}
	} else {
		writer, err = s.registry.Writer(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get writer")
		}
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteAddressGroupBindingsByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete address group bindings")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	if s.ruleS2SRegenerator != nil {
		// Collect unique service IDs to avoid duplicate notifications
		serviceIDs := make(map[string]models.ResourceIdentifier)
		for _, binding := range bindingsToRemove {
			key := binding.ServiceRef.Namespace + "/" + binding.ServiceRef.Name
			serviceIDs[key] = models.ResourceIdentifier{Name: binding.ServiceRef.Name, Namespace: binding.ServiceRef.Namespace}
		}

		for _, serviceID := range serviceIDs {
			if err := s.synchronizeServiceAddressGroups(ctx, serviceID); err != nil {
				klog.Errorf("Failed to synchronize Service.AddressGroups for service %s after binding deletion: %v", serviceID.Key(), err)
			}
		}

		for _, serviceID := range serviceIDs {
			if err := s.ruleS2SRegenerator.NotifyServiceAddressGroupsChanged(ctx, serviceID); err != nil {
			}
		}
	}

	// After successful deletion, regenerate port mappings for affected AddressGroups
	// This will remove stale services that no longer have bindings
	for _, addressGroupRef := range affectedAddressGroups {
		// Get fresh reader after the deletion transaction
		freshReader, err := s.registry.Reader(ctx)
		if err != nil {
			continue // Don't fail the whole operation
		}
		defer freshReader.Close()

		// Generate the complete mapping with remaining bindings (deleted bindings will be excluded)
		addressGroupPortMapping, err := s.generateCompleteAddressGroupPortMapping(ctx, freshReader, addressGroupRef)
		if err != nil {
			continue // Don't fail the whole operation
		}

		// Update the mapping in storage (or delete if no bindings remain)
		freshWriter, err := s.registry.Writer(ctx)
		if err != nil {
			continue // Don't fail the whole operation
		}
		defer func() {
			if err != nil {
				freshWriter.Abort()
			}
		}()

		if addressGroupPortMapping == nil {
			// No bindings remain - port mapping should be empty/minimal
			emptyMapping := &models.AddressGroupPortMapping{
				SelfRef: models.SelfRef{
					ResourceIdentifier: addressGroupRef,
				},
				AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
			}
			if err := s.syncAddressGroupPortMappings(ctx, freshWriter, []models.AddressGroupPortMapping{*emptyMapping}, models.SyncOpUpsert); err != nil {
				freshWriter.Abort()
				continue
			}
		} else {
			// Some bindings remain - update with current services
			if err := s.syncAddressGroupPortMappings(ctx, freshWriter, []models.AddressGroupPortMapping{*addressGroupPortMapping}, models.SyncOpUpsert); err != nil {
				freshWriter.Abort()
				continue
			}
		}

		if err := freshWriter.Commit(); err != nil {
			continue
		}

	}

	return nil
}

// =============================================================================
// AddressGroupPortMapping Operations
// =============================================================================

// GetAddressGroupPortMappings returns all address group port mappings within scope
func (s *AddressGroupResourceService) GetAddressGroupPortMappings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupPortMapping, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var mappings []models.AddressGroupPortMapping
	err = reader.ListAddressGroupPortMappings(ctx, func(mapping models.AddressGroupPortMapping) error {
		mappings = append(mappings, mapping)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list address group port mappings")
	}
	return mappings, nil
}

// GetAddressGroupPortMappingByID returns address group port mapping by ID
func (s *AddressGroupResourceService) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return reader.GetAddressGroupPortMappingByID(ctx, id)
}

// GetAddressGroupPortMappingsByIDs returns multiple address group port mappings by IDs
func (s *AddressGroupResourceService) GetAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroupPortMapping, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var mappings []models.AddressGroupPortMapping
	for _, id := range ids {
		mapping, err := reader.GetAddressGroupPortMappingByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Skip not found mappings
			}
			return nil, errors.Wrapf(err, "failed to get address group port mapping %s", id.Key())
		}
		mappings = append(mappings, *mapping)
	}
	return mappings, nil
}

// CreateAddressGroupPortMapping creates a new address group port mapping
func (s *AddressGroupResourceService) CreateAddressGroupPortMapping(ctx context.Context, mapping models.AddressGroupPortMapping) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Validate mapping for creation
	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	if err := mappingValidator.ValidateForCreation(ctx, mapping); err != nil {
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Sync mapping (this will create it)
	if err = s.syncAddressGroupPortMappings(ctx, writer, []models.AddressGroupPortMapping{mapping}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to create address group port mapping")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, &mapping); err != nil {
			klog.Errorf("Failed to process address group port mapping conditions for %s/%s: %v",
				mapping.Namespace, mapping.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	return nil
}

// UpdateAddressGroupPortMapping updates an existing address group port mapping
func (s *AddressGroupResourceService) UpdateAddressGroupPortMapping(ctx context.Context, mapping models.AddressGroupPortMapping) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Get existing mapping for validation
	existingMapping, err := reader.GetAddressGroupPortMappingByID(ctx, mapping.ResourceIdentifier)
	if err != nil {
		return errors.Wrap(err, "failed to get existing address group port mapping")
	}

	// Validate mapping for update
	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	if err := mappingValidator.ValidateForUpdate(ctx, *existingMapping, mapping); err != nil {
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Sync mapping (this will update it)
	if err = s.syncAddressGroupPortMappings(ctx, writer, []models.AddressGroupPortMapping{mapping}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to update address group port mapping")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, &mapping); err != nil {
			klog.Errorf("Failed to process address group port mapping conditions for %s/%s: %v",
				mapping.Namespace, mapping.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	return nil
}

// SyncMultipleAddressGroupPortMappings synchronizes multiple address group port mappings
func (s *AddressGroupResourceService) SyncMultipleAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, syncOp models.SyncOp) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.syncAddressGroupPortMappings(ctx, writer, mappings, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to sync address group port mappings")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit for each address group port mapping
	if s.conditionManager != nil {
		for i := range mappings {
			if err := s.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, &mappings[i]); err != nil {
				klog.Errorf("Failed to process address group port mapping conditions for %s/%s: %v",
					mappings[i].Namespace, mappings[i].Name, err)
				// Don't fail the operation if condition processing fails
			} else {
				// Save the processed conditions back to storage
				if err := s.conditionManager.SaveAddressGroupPortMappingConditions(ctx, &mappings[i]); err != nil {
					klog.Errorf("Failed to save address group port mapping conditions for %s/%s: %v",
						mappings[i].Namespace, mappings[i].Name, err)
				}
			}
		}
	}

	return nil
}

// DeleteAddressGroupPortMappingsByIDs deletes address group port mappings by IDs
func (s *AddressGroupResourceService) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteAddressGroupPortMappingsByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete address group port mappings")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}

// =============================================================================
// AddressGroupBindingPolicy Operations
// =============================================================================

// GetAddressGroupBindingPolicies returns all address group binding policies within scope
func (s *AddressGroupResourceService) GetAddressGroupBindingPolicies(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBindingPolicy, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var policies []models.AddressGroupBindingPolicy
	err = reader.ListAddressGroupBindingPolicies(ctx, func(policy models.AddressGroupBindingPolicy) error {
		policies = append(policies, policy)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list address group binding policies")
	}
	return policies, nil
}

// GetAddressGroupBindingPolicyByID returns address group binding policy by ID
func (s *AddressGroupResourceService) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return reader.GetAddressGroupBindingPolicyByID(ctx, id)
}

// GetAddressGroupBindingPoliciesByIDs returns multiple address group binding policies by IDs
func (s *AddressGroupResourceService) GetAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroupBindingPolicy, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var policies []models.AddressGroupBindingPolicy
	for _, id := range ids {
		policy, err := reader.GetAddressGroupBindingPolicyByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Skip not found policies
			}
			return nil, errors.Wrapf(err, "failed to get address group binding policy %s", id.Key())
		}
		policies = append(policies, *policy)
	}
	return policies, nil
}

// CreateAddressGroupBindingPolicy creates a new address group binding policy
func (s *AddressGroupResourceService) CreateAddressGroupBindingPolicy(ctx context.Context, policy models.AddressGroupBindingPolicy) error {

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Get reader from writer to ensure same session/transaction visibility
	reader, err := s.registry.ReaderFromWriter(ctx, writer)
	if err != nil {
		return errors.Wrap(err, "failed to get reader from writer")
	}
	defer reader.Close()

	// Validate policy for creation
	validator := validation.NewDependencyValidator(reader)
	policyValidator := validator.GetAddressGroupBindingPolicyValidator()

	if err := policyValidator.ValidateForCreation(ctx, &policy); err != nil {
		return err
	}

	// Sync policy (this will create it)
	if err = s.syncAddressGroupBindingPolicies(ctx, writer, []models.AddressGroupBindingPolicy{policy}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to create address group binding policy")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupBindingPolicyConditions(ctx, &policy); err != nil {
			klog.Errorf("Failed to process address group binding policy conditions for %s/%s: %v",
				policy.Namespace, policy.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	return nil
}

// UpdateAddressGroupBindingPolicy updates an existing address group binding policy
func (s *AddressGroupResourceService) UpdateAddressGroupBindingPolicy(ctx context.Context, policy models.AddressGroupBindingPolicy) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Get existing policy for validation
	existingPolicy, err := reader.GetAddressGroupBindingPolicyByID(ctx, policy.ResourceIdentifier)
	if err != nil {
		return errors.Wrap(err, "failed to get existing address group binding policy")
	}

	// Validate policy for update
	validator := validation.NewDependencyValidator(reader)
	policyValidator := validator.GetAddressGroupBindingPolicyValidator()

	if err := policyValidator.ValidateForUpdate(ctx, *existingPolicy, &policy); err != nil {
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Sync policy (this will update it)
	if err = s.syncAddressGroupBindingPolicies(ctx, writer, []models.AddressGroupBindingPolicy{policy}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to update address group binding policy")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupBindingPolicyConditions(ctx, &policy); err != nil {
			klog.Errorf("Failed to process address group binding policy conditions for %s/%s: %v",
				policy.Namespace, policy.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	return nil
}

// SyncAddressGroupBindingPolicies synchronizes multiple address group binding policies
func (s *AddressGroupResourceService) SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope ports.Scope, syncOp models.SyncOp) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.syncAddressGroupBindingPolicies(ctx, writer, policies, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to sync address group binding policies")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit for each address group binding policy
	if s.conditionManager != nil {
		for i := range policies {
			if err := s.conditionManager.ProcessAddressGroupBindingPolicyConditions(ctx, &policies[i]); err != nil {
				klog.Errorf("Failed to process address group binding policy conditions for %s/%s: %v",
					policies[i].Namespace, policies[i].Name, err)
				// Don't fail the operation if condition processing fails
			}
		}
	}

	return nil
}

// DeleteAddressGroupBindingPoliciesByIDs deletes address group binding policies by IDs
func (s *AddressGroupResourceService) DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteAddressGroupBindingPoliciesByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete address group binding policies")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}

// =============================================================================
// Helper Methods for Port Mapping Sync Operations
// =============================================================================

// SyncAddressGroupPortMappingsWithWriter syncs port mappings for a binding using existing writer
func (s *AddressGroupResourceService) SyncAddressGroupPortMappingsWithWriter(ctx context.Context, writer ports.Writer, binding models.AddressGroupBinding, syncOp models.SyncOp) error {
	reader, err := s.registry.ReaderFromWriter(ctx, writer)
	if err != nil {
		return errors.Wrap(err, "failed to get reader from writer")
	}
	defer reader.Close()

	return s.SyncAddressGroupPortMappingsWithWriterAndReader(ctx, writer, reader, binding, syncOp)
}

// SyncAddressGroupPortMappingsWithWriterAndReader syncs port mappings using existing writer and reader
func (s *AddressGroupResourceService) SyncAddressGroupPortMappingsWithWriterAndReader(ctx context.Context, writer ports.Writer, reader ports.Reader, binding models.AddressGroupBinding, syncOp models.SyncOp) error {
	// Generate address group port mapping
	addressGroupPortMapping := s.generateAddressGroupPortMapping(ctx, reader, binding)

	if addressGroupPortMapping != nil {
		if err := s.syncAddressGroupPortMappings(ctx, writer, []models.AddressGroupPortMapping{*addressGroupPortMapping}, syncOp); err != nil {
			return errors.Wrap(err, "failed to sync address group port mapping")
		}
	}

	return nil
}

// SyncAddressGroupPortMappings syncs port mappings for a binding
func (s *AddressGroupResourceService) SyncAddressGroupPortMappings(ctx context.Context, binding models.AddressGroupBinding) error {
	return s.SyncAddressGroupPortMappingsWithSyncOp(ctx, binding, models.SyncOpUpsert)
}

// SyncAddressGroupPortMappingsWithSyncOp syncs port mappings with specific sync operation
func (s *AddressGroupResourceService) SyncAddressGroupPortMappingsWithSyncOp(ctx context.Context, binding models.AddressGroupBinding, syncOp models.SyncOp) error {
	if syncOp == models.SyncOpDelete {
		return s.regenerateCompletePortMappingForAddressGroup(ctx, binding.AddressGroupRef.Name, binding.AddressGroupRef.Namespace)
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.SyncAddressGroupPortMappingsWithWriter(ctx, writer, binding, syncOp); err != nil {
		return errors.Wrap(err, "failed to sync address group port mappings")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	if s.conditionManager != nil {
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			klog.Errorf("Failed to get reader for condition processing: %v", err)
			return nil // Don't fail the operation
		}
		defer reader.Close()

		// Generate the complete mapping with all services for this address group
		addressGroupRef := models.ResourceIdentifier{
			Name:      binding.AddressGroupRef.Name,
			Namespace: binding.AddressGroupRef.Namespace,
		}

		addressGroupPortMapping, err := s.generateCompleteAddressGroupPortMapping(ctx, reader, addressGroupRef)
		if err != nil {
			return errors.Wrapf(err, "failed to generate port mapping for AddressGroup %s", addressGroupRef.Key())
		}
		if addressGroupPortMapping != nil {
			if err := s.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, addressGroupPortMapping); err != nil {
				klog.Errorf("Failed to process AddressGroupPortMapping conditions for %s/%s: %v",
					addressGroupPortMapping.Namespace, addressGroupPortMapping.Name, err)
				// Don't fail the operation if condition processing fails
			}
		}
	}

	return nil
}

// regenerateCompletePortMappingForAddressGroup regenerates the complete port mapping for an AddressGroup
// This is used after binding deletions to ensure the mapping reflects only remaining bindings
func (s *AddressGroupResourceService) regenerateCompletePortMappingForAddressGroup(ctx context.Context, addressGroupName, addressGroupNamespace string) error {

	// Get fresh data after binding deletion
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	addressGroupRef := models.ResourceIdentifier{
		Name:      addressGroupName,
		Namespace: addressGroupNamespace,
	}

	// Generate complete mapping based on current state (after binding changes)
	addressGroupPortMapping, err := s.generateCompleteAddressGroupPortMapping(ctx, reader, addressGroupRef)
	if err != nil {
		return errors.Wrapf(err, "failed to generate complete port mapping for AddressGroup %s", addressGroupRef.Key())
	}

	// Always create a mapping (empty if no bindings remain) to ensure Kubernetes resource is updated
	if addressGroupPortMapping == nil {
		addressGroupPortMapping = &models.AddressGroupPortMapping{
			SelfRef: models.SelfRef{
				ResourceIdentifier: addressGroupRef,
			},
			AccessPorts: make(map[models.ServiceRef]models.ServicePorts), // Empty services map
		}
	}

	// Persist the regenerated mapping to storage
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.syncAddressGroupPortMappings(ctx, writer, []models.AddressGroupPortMapping{*addressGroupPortMapping}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to sync regenerated port mapping")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit regenerated port mapping")
	}

	// Process conditions after successful storage update
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, addressGroupPortMapping); err != nil {
			klog.Errorf("Failed to process conditions for regenerated port mapping %s: %v", addressGroupRef.Key(), err)
			// Don't fail the operation if condition processing fails
		}
	}

	return nil
}

// RegeneratePortMappingsForService regenerates all AddressGroupPortMappings that reference a specific service
// This is called when a service's ingress ports are updated to ensure mappings reflect the current ports
func (s *AddressGroupResourceService) RegeneratePortMappingsForService(ctx context.Context, serviceID models.ResourceIdentifier) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Collect AddressGroups from both bindings AND spec.addressGroups
	addressGroupsToRegenerate := make(map[string]models.ResourceIdentifier)

	// 1. Collect from AddressGroupBindings
	var affectedBindings []models.AddressGroupBinding
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		// Check if this binding references our service (name and namespace must match exactly)
		if binding.ServiceRef.Name == serviceID.Name && binding.ServiceRef.Namespace == serviceID.Namespace {
			affectedBindings = append(affectedBindings, binding)
			agKey := fmt.Sprintf("%s/%s", binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name)
			addressGroupsToRegenerate[agKey] = models.ResourceIdentifier{
				Name:      binding.AddressGroupRef.Name,
				Namespace: binding.AddressGroupRef.Namespace,
			}
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to list address group bindings")
	}

	// 2. Collect from Service.Spec.AddressGroups
	service, err := reader.GetServiceByID(ctx, serviceID)
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return errors.Wrap(err, "failed to get service for spec.addressGroups")
	}
	if service != nil && len(service.AddressGroups) > 0 {
		for _, agRef := range service.AddressGroups {
			agKey := fmt.Sprintf("%s/%s", agRef.Namespace, agRef.Name)
			addressGroupsToRegenerate[agKey] = models.ResourceIdentifier{
				Name:      agRef.Name,
				Namespace: agRef.Namespace,
			}
		}
	}

	if len(addressGroupsToRegenerate) == 0 {
		return nil
	}

	// Regenerate each affected AddressGroupPortMapping
	for agKey, addressGroupRef := range addressGroupsToRegenerate {

		// Generate the complete mapping with updated service ports
		addressGroupPortMapping, err := s.generateCompleteAddressGroupPortMapping(ctx, reader, addressGroupRef)
		if err != nil {
			return errors.Wrapf(err, "port conflict detected while regenerating mapping for AddressGroup %s", agKey)
		}
		if addressGroupPortMapping == nil {
			continue
		}

		// Update the mapping in storage
		writer, err := s.registry.Writer(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get writer")
		}
		defer func() {
			if err != nil {
				writer.Abort()
			}
		}()

		if err := s.syncAddressGroupPortMappings(ctx, writer, []models.AddressGroupPortMapping{*addressGroupPortMapping}, models.SyncOpUpsert); err != nil {
			writer.Abort()
			return errors.Wrapf(err, "failed to sync regenerated mapping for AddressGroup %s", agKey)
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			return errors.Wrapf(err, "failed to commit regenerated mapping for AddressGroup %s", agKey)
		}

		// Process conditions for the regenerated mapping
		if s.conditionManager != nil {
			if err := s.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, addressGroupPortMapping); err != nil {
				klog.Errorf("Failed to process conditions for regenerated AddressGroupPortMapping %s: %v", agKey, err)
				// Don't fail the operation if condition processing fails
			}
		}

	}

	return nil
}

// RegeneratePortMappingsForAddressGroup regenerates AddressGroupPortMapping for a specific AddressGroup
// This is called when a Service with spec.addressGroups is created/updated/deleted
func (s *AddressGroupResourceService) RegeneratePortMappingsForAddressGroup(ctx context.Context, addressGroupID models.ResourceIdentifier) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Generate the complete mapping with all services (both from bindings and spec)
	addressGroupPortMapping, err := s.generateCompleteAddressGroupPortMapping(ctx, reader, addressGroupID)
	if err != nil {
		return errors.Wrapf(err, "port conflict detected while regenerating mapping for AddressGroup %s", addressGroupID.Key())
	}

	// Always create a mapping (empty if no services) to ensure resource exists
	if addressGroupPortMapping == nil {
		addressGroupPortMapping = &models.AddressGroupPortMapping{
			SelfRef: models.SelfRef{
				ResourceIdentifier: addressGroupID,
			},
			AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
		}
	}

	// Update the mapping in storage
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err := s.syncAddressGroupPortMappings(ctx, writer, []models.AddressGroupPortMapping{*addressGroupPortMapping}, models.SyncOpUpsert); err != nil {
		writer.Abort()
		return errors.Wrapf(err, "failed to sync regenerated mapping for AddressGroup %s", addressGroupID.Key())
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return errors.Wrapf(err, "failed to commit regenerated mapping for AddressGroup %s", addressGroupID.Key())
	}

	// Process conditions for the regenerated mapping
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, addressGroupPortMapping); err != nil {
			klog.Errorf("Failed to process conditions for regenerated AddressGroupPortMapping %s: %v", addressGroupID.Key(), err)
			// Don't fail the operation if condition processing fails
		}
	}

	return nil
}

func (s *AddressGroupResourceService) syncAddressGroups(ctx context.Context, writer ports.Writer, addressGroups []models.AddressGroup, syncOp models.SyncOp) error {

	// Set AddressGroupName for all address groups before syncing
	for i := range addressGroups {
		if addressGroups[i].Namespace != "" {
			addressGroups[i].AddressGroupName = fmt.Sprintf("%s/%s", addressGroups[i].Namespace, addressGroups[i].Name)
		} else {
			addressGroups[i].AddressGroupName = addressGroups[i].Name
		}
	}

	// Validation based on operation
	if syncOp != models.SyncOpDelete {
		reader, err := s.registry.ReaderFromWriter(ctx, writer)
		if err != nil {
			return errors.Wrap(err, "failed to get reader from writer")
		}
		defer reader.Close()

		validator := validation.NewDependencyValidator(reader)
		addressGroupValidator := validator.GetAddressGroupValidator()

		for _, addressGroup := range addressGroups {
			existingAddressGroup, err := reader.GetAddressGroupByID(ctx, addressGroup.ResourceIdentifier)
			if err == nil && syncOp != models.SyncOpDelete {
				// Address group exists - use ValidateForUpdate
				if err := addressGroupValidator.ValidateForUpdate(ctx, *existingAddressGroup, addressGroup); err != nil {
					return err
				}
			} else if errors.Is(err, ports.ErrNotFound) && syncOp != models.SyncOpDelete {
				// Address group is new - use ValidateForCreation
				if err := addressGroupValidator.ValidateForCreation(ctx, addressGroup); err != nil {
					return err
				}
			} else if err != nil && !errors.Is(err, ports.ErrNotFound) {
				// Other error occurred
				return errors.Wrap(err, "failed to get address group")
			}
		}
	}

	// Determine scope
	var scope ports.Scope
	if syncOp == models.SyncOpFullSync {
		scope = ports.EmptyScope{}
	} else {
		scope = ports.EmptyScope{}
	}

	if syncOp == models.SyncOpDelete {
		// Collect address group IDs
		var ids []models.ResourceIdentifier
		for _, addressGroup := range addressGroups {
			ids = append(ids, addressGroup.ResourceIdentifier)
		}
		return s.DeleteAddressGroupsByIDs(ctx, ids)
	}

	if err := writer.SyncAddressGroups(ctx, addressGroups, scope, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address groups")
	}

	return nil
}

func (s *AddressGroupResourceService) syncAddressGroupBindings(ctx context.Context, writer ports.Writer, bindings []models.AddressGroupBinding, syncOp models.SyncOp) error {
	if syncOp == models.SyncOpUpsert || syncOp == models.SyncOpFullSync {
		reader, err := s.registry.ReaderFromWriter(ctx, writer)
		if err != nil {
			return errors.Wrap(err, "failed to get reader for port conflict validation")
		}
		defer reader.Close()

		// Validate each binding for port conflicts before allowing database sync
		for _, binding := range bindings {

			// Get the service that this binding references
			serviceID := models.ResourceIdentifier{
				Name:      binding.ServiceRef.Name,
				Namespace: binding.ServiceRef.Namespace,
			}
			service, err := reader.GetServiceByID(ctx, serviceID)
			if err != nil {
				return errors.Wrapf(err, "cannot validate binding %s: service %s not found", binding.Key(), serviceID.Key())
			}

			// Get existing bindings for the same AddressGroup (excluding the current binding)
			addressGroupRef := models.ResourceIdentifier{
				Name:      binding.AddressGroupRef.Name,
				Namespace: binding.AddressGroupRef.Namespace,
			}

			var existingBindings []models.AddressGroupBinding
			err = reader.ListAddressGroupBindings(ctx, func(existingBinding models.AddressGroupBinding) error {
				if existingBinding.AddressGroupRef.Name == addressGroupRef.Name &&
					existingBinding.AddressGroupRef.Namespace == addressGroupRef.Namespace &&
					!(existingBinding.Name == binding.Name && existingBinding.Namespace == binding.Namespace) {
					existingBindings = append(existingBindings, existingBinding)
				}
				return nil
			}, ports.EmptyScope{})

			if err != nil {
				return errors.Wrapf(err, "failed to list existing bindings for AddressGroup %s", addressGroupRef.Key())
			}

			for _, existingBinding := range existingBindings {
				existingServiceID := models.ResourceIdentifier{
					Name:      existingBinding.ServiceRef.Name,
					Namespace: existingBinding.ServiceRef.Namespace,
				}
				existingService, err := reader.GetServiceByID(ctx, existingServiceID)
				if err != nil {
					continue
				}

				// Check for port conflicts between services
				if err := s.checkPortConflictsBetweenServices(service, existingService); err != nil {
					return errors.Wrapf(err, "cannot create binding %s: port conflict with existing binding %s", binding.Key(), existingBinding.Key())
				}
			}
		}
	}

	if err := writer.SyncAddressGroupBindings(ctx, bindings, ports.EmptyScope{}, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address group bindings in storage")
	}

	return nil
}

// checkPortConflictsBetweenServices checks for port conflicts between two services
func (s *AddressGroupResourceService) checkPortConflictsBetweenServices(service1, service2 *models.Service) error {

	// Convert service1 ingress ports to port ranges
	service1Ports := make(map[models.TransportProtocol][]models.PortRange)
	for _, ingressPort := range service1.IngressPorts {
		transport := ingressPort.Protocol
		if service1Ports[transport] == nil {
			service1Ports[transport] = make([]models.PortRange, 0)
		}

		// Parse port ranges from ingress port string
		portRanges, err := validation.ParsePortRanges(ingressPort.Port)
		if err != nil {
			continue
		}

		service1Ports[transport] = append(service1Ports[transport], portRanges...)
	}

	// Convert service2 ingress ports to port ranges
	service2Ports := make(map[models.TransportProtocol][]models.PortRange)
	for _, ingressPort := range service2.IngressPorts {
		transport := ingressPort.Protocol
		if service2Ports[transport] == nil {
			service2Ports[transport] = make([]models.PortRange, 0)
		}

		portRanges, err := validation.ParsePortRanges(ingressPort.Port)
		if err != nil {
			continue
		}

		service2Ports[transport] = append(service2Ports[transport], portRanges...)
	}

	// Check for overlaps between port ranges for each protocol
	for protocol1, ranges1 := range service1Ports {
		ranges2, exists := service2Ports[protocol1]
		if !exists {
			continue // No conflict if services don't share the same protocol
		}

		// Check each port range in service1 against each port range in service2
		for _, range1 := range ranges1 {
			for _, range2 := range ranges2 {
				if validation.DoPortRangesOverlap(range1, range2) {
					return errors.Errorf("%s port range %d-%d for service %s overlaps with existing port range %d-%d for service %s",
						protocol1, range1.Start, range1.End, service1.Key(),
						range2.Start, range2.End, service2.Key())
				}
			}
		}
	}

	return nil
}

// syncAddressGroupPortMappings handles the actual address group port mapping synchronization logic
func (s *AddressGroupResourceService) syncAddressGroupPortMappings(ctx context.Context, writer ports.Writer, mappings []models.AddressGroupPortMapping, syncOp models.SyncOp) error {
	if err := writer.SyncAddressGroupPortMappings(ctx, mappings, ports.EmptyScope{}, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address group port mappings in storage")
	}

	return nil
}

// syncAddressGroupBindingPolicies handles the actual address group binding policy synchronization logic
func (s *AddressGroupResourceService) syncAddressGroupBindingPolicies(ctx context.Context, writer ports.Writer, policies []models.AddressGroupBindingPolicy, syncOp models.SyncOp) error {
	if err := writer.SyncAddressGroupBindingPolicies(ctx, policies, ports.EmptyScope{}, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address group binding policies in storage")
	}
	return nil
}

// syncAddressGroupsWithSGroups syncs address groups with external sgroups system
func (s *AddressGroupResourceService) syncAddressGroupsWithSGroups(ctx context.Context, addressGroups []models.AddressGroup, operation types.SyncOperation) {

	if s.syncManager == nil {
		return
	}

	if len(addressGroups) == 0 {
		return
	}

	// Convert addressGroups to SyncableEntity slice for batch sync
	var syncableEntities []interfaces.SyncableEntity
	var unsyncableKeys []string
	var allHostReferences []models.HostReference // Collect all host references that need sync

	for _, addressGroup := range addressGroups {
		// Create a copy to avoid pointer issues
		agCopy := addressGroup
		if syncableEntity, ok := interface{}(&agCopy).(interfaces.SyncableEntity); ok {
			syncableEntities = append(syncableEntities, syncableEntity)
		} else {
			unsyncableKeys = append(unsyncableKeys, addressGroup.Key())
		}

		for _, hostRef := range addressGroup.AggregatedHosts {
			allHostReferences = append(allHostReferences, hostRef)
		}
		if len(addressGroup.AggregatedHosts) == 0 {
			for _, hostObjRef := range addressGroup.Hosts {
				hostRef := models.HostReference{
					ObjectReference: hostObjRef,
					UUID:            "",
					Source:          models.HostSourceSpec,
				}
				allHostReferences = append(allHostReferences, hostRef)
			}
		}
	}

	// Perform batch sync for all syncable address groups
	if len(syncableEntities) > 0 {
		if err := s.syncManager.SyncBatch(ctx, syncableEntities, operation); err != nil {
		}
	}

	if len(allHostReferences) > 0 {
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			return
		}
		defer reader.Close()

		for _, hostRef := range allHostReferences {
			var hostNamespace string
			if len(syncableEntities) > 0 {
				if ag, ok := syncableEntities[0].(*models.AddressGroup); ok {
					hostNamespace = ag.GetNamespace()
				}
			}
			hostID := models.ResourceIdentifier{
				Namespace: hostNamespace,
				Name:      hostRef.ObjectReference.Name,
			}

			// Load full host data from database
			host, err := reader.GetHostByID(ctx, hostID)
			if err != nil {
				continue
			}

			// Sync the full host with SGROUP
			if err := s.syncManager.SyncEntity(ctx, host, operation); err != nil {
			}
		}
	}
}

func (s *AddressGroupResourceService) generateAddressGroupPortMapping(ctx context.Context, reader ports.Reader, binding models.AddressGroupBinding) *models.AddressGroupPortMapping {
	// Use the new aggregated version instead
	addressGroupRef := models.ResourceIdentifier{
		Name:      binding.AddressGroupRef.Name,
		Namespace: binding.AddressGroupRef.Namespace,
	}
	portMapping, err := s.generateCompleteAddressGroupPortMapping(ctx, reader, addressGroupRef)
	if err != nil {
		return nil // Legacy function can't return error, so return nil for port conflicts
	}
	return portMapping
}

func (s *AddressGroupResourceService) generateCompleteAddressGroupPortMapping(ctx context.Context, reader ports.Reader, addressGroupRef models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	addressGroupPortMapping := &models.AddressGroupPortMapping{
		SelfRef: models.SelfRef{
			ResourceIdentifier: addressGroupRef,
		},
		AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
	}

	servicesToProcess := make(map[string]*models.Service)
	var bindings []models.AddressGroupBinding
	err := reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		// Check if this binding targets our AddressGroup
		if binding.AddressGroupRef.Name == addressGroupRef.Name && binding.AddressGroupRef.Namespace == addressGroupRef.Namespace {
			bindings = append(bindings, binding)
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return nil, errors.Wrapf(err, "failed to list bindings for AddressGroup %s", addressGroupRef.Key())
	}

	for _, binding := range bindings {
		service, err := reader.GetServiceByID(ctx, models.ResourceIdentifier{
			Name:      binding.ServiceRef.Name,
			Namespace: binding.ServiceRef.Namespace,
		})
		if err != nil {
			continue
		}
		servicesToProcess[service.Key()] = service
	}

	err = reader.ListServices(ctx, func(service models.Service) error {
		for _, agRef := range service.AddressGroups {
			if agRef.Name == addressGroupRef.Name && agRef.Namespace == addressGroupRef.Namespace {
				servicesToProcess[service.Key()] = &service
				break
			}
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return nil, errors.Wrapf(err, "failed to list services for AddressGroup %s", addressGroupRef.Key())
	}

	// Process all collected services
	for _, service := range servicesToProcess {
		// Add service ports to the mapping
		serviceRef := models.NewServiceRef(service.Name, models.WithNamespace(service.Namespace))
		servicePorts := models.ServicePorts{
			Ports: make(map[models.TransportProtocol][]models.PortRange),
		}

		// Convert ingress ports to service ports
		for _, ingressPort := range service.IngressPorts {
			transport := ingressPort.Protocol
			if servicePorts.Ports[transport] == nil {
				servicePorts.Ports[transport] = make([]models.PortRange, 0)
			}

			// Parse port ranges from ingress port string (supports "80", "8080-9090", "80,443")
			portRanges, err := validation.ParsePortRanges(ingressPort.Port)
			if err != nil {
				continue // Skip invalid ports
			}

			// Add all parsed port ranges to service ports
			for _, portRange := range portRanges {
				servicePorts.Ports[transport] = append(servicePorts.Ports[transport], portRange)
			}
		}

		addressGroupPortMapping.AccessPorts[serviceRef] = servicePorts
	}

	if s.validationService != nil {
		mappingValidator := validation.NewAddressGroupPortMappingValidator(reader)
		if err := mappingValidator.CheckInternalPortOverlaps(*addressGroupPortMapping); err != nil {
			// Return error to prevent creation of conflicting mapping and fail the binding operation
			return nil, errors.Wrapf(err, "port conflict detected for AddressGroup %s", addressGroupRef.Key())
		}
	} else {
	}

	return addressGroupPortMapping, nil
}

// FindServicesForAddressGroups finds all services that are bound to given address groups
func (s *AddressGroupResourceService) FindServicesForAddressGroups(ctx context.Context, addressGroupIDs []models.ResourceIdentifier) ([]models.Service, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var relatedServices []models.Service
	serviceIDs := make(map[string]models.ResourceIdentifier)

	// Find all address group bindings for these address groups
	for _, agID := range addressGroupIDs {
		var bindings []models.AddressGroupBinding
		err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
			if binding.AddressGroupRef.Name == agID.Name && binding.AddressGroupRef.Namespace == agID.Namespace {
				bindings = append(bindings, binding)
			}
			return nil
		}, ports.EmptyScope{})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to find bindings for address group %s", agID.Key())
		}

		// Collect unique service IDs
		for _, binding := range bindings {
			key := binding.ServiceRef.Namespace + "/" + binding.ServiceRef.Name
			serviceIDs[key] = models.ResourceIdentifier{
				Name:      binding.ServiceRef.Name,
				Namespace: binding.ServiceRef.Namespace,
			}
		}
	}

	// Fetch all related services
	for _, serviceID := range serviceIDs {
		service, err := reader.GetServiceByID(ctx, serviceID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Service might have been deleted
			}
			return nil, errors.Wrapf(err, "failed to get service %s", serviceID.Key())
		}
		relatedServices = append(relatedServices, *service)
	}

	return relatedServices, nil
}

func (s *AddressGroupResourceService) synchronizeServiceAddressGroups(ctx context.Context, serviceID models.ResourceIdentifier) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for service sync")
	}
	defer reader.Close()

	service, err := reader.GetServiceByID(ctx, serviceID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil
		}
		return errors.Wrapf(err, "failed to get service %s for AddressGroups sync", serviceID.Key())
	}

	var writer ports.Writer
	if registryWithConditions, ok := s.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err = registryWithConditions.WriterForConditions(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get condition writer with ReadCommitted isolation for service sync")
		}
	} else {
		writer, err = s.registry.Writer(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get writer for service sync")
		}
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err := writer.SyncServices(ctx, []models.Service{*service}, ports.EmptyScope{}); err != nil {
		writer.Abort()
		return errors.Wrapf(err, "failed to sync service %s with updated AddressGroups", serviceID.Key())
	}

	if s.syncManager != nil {
		if err := s.syncManager.SyncEntity(ctx, service, types.SyncOperationUpsert); err != nil {
			writer.Abort()
			return fmt.Errorf("SGROUP sync failed for service %s after AddressGroup binding update, transaction aborted: %w", serviceID.Key(), err)
		}
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return errors.Wrapf(err, "failed to commit service %s AddressGroups sync", serviceID.Key())
	}

	return nil
}

// updateHostBindingStatusForSyncedAddressGroups updates Host.isBound status for AddressGroups processed via Sync API
func (s *AddressGroupResourceService) updateHostBindingStatusForSyncedAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, syncOp models.SyncOp) {
	if s.hostService == nil {
		return
	}

	for _, ag := range addressGroups {
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			continue
		}

		freshAG, err := reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{Name: ag.Name, Namespace: ag.Namespace})
		reader.Close()
		if err != nil {
			continue
		}

		ag = *freshAG
		if len(ag.AggregatedHosts) == 0 && len(ag.Hosts) == 0 {
			continue // No hosts to process
		}

		switch syncOp {
		case models.SyncOpDelete:
			// For delete operations, unbind all hosts
			totalHosts := len(ag.AggregatedHosts)
			if totalHosts == 0 {
				totalHosts = len(ag.Hosts) // fallback if AggregatedHosts is empty
			}
			if err := s.hostService.UpdateHostBindingStatus(ctx, &ag, nil); err != nil {
			}

		case models.SyncOpUpsert, models.SyncOpFullSync:
			reader, err := s.registry.Reader(ctx)
			if err != nil {
				continue
			}

			currentlyBoundHosts := make(map[string]*models.Host)
			err = reader.ListHosts(ctx, func(host models.Host) error {
				if host.IsBound && host.AddressGroupRef != nil &&
					host.AddressGroupRef.Name == ag.Name &&
					host.Namespace == ag.Namespace {
					currentlyBoundHosts[host.Key()] = &host
				}
				return nil
			}, ports.EmptyScope{})

			if err != nil {
				reader.Close()
				continue
			}

			// Step 2: Create set of hosts that should be bound (new bindings from aggregated hosts or spec hosts)
			shouldBeBoundHosts := make(map[string]models.ResourceIdentifier)
			if len(ag.AggregatedHosts) > 0 {
				// Use AggregatedHosts if available (preferred)
				for _, hostRef := range ag.AggregatedHosts {
					hostID := models.ResourceIdentifier{Name: hostRef.GetName(), Namespace: ag.Namespace}
					shouldBeBoundHosts[hostID.Key()] = hostID
				}
			} else if len(ag.Hosts) > 0 {
				// Fallback to spec.hosts if AggregatedHosts is empty
				for _, hostRef := range ag.Hosts {
					hostID := models.ResourceIdentifier{Name: hostRef.Name, Namespace: ag.Namespace}
					shouldBeBoundHosts[hostID.Key()] = hostID
				}
			}

			var hostsToUpdate []models.Host
			for hostKey, hostID := range shouldBeBoundHosts {
				if _, alreadyBound := currentlyBoundHosts[hostKey]; !alreadyBound {
					host, err := reader.GetHostByID(ctx, hostID)
					if err != nil {
						continue
					}

					// Bind this host
					host.IsBound = true
					host.AddressGroupRef = &netguardv1beta1.ObjectReference{
						Name: ag.Name,
					}
					hostsToUpdate = append(hostsToUpdate, *host)
				}
			}

			// Step 4: Find hosts to unbind (in current but not in shouldBe)
			for hostKey, host := range currentlyBoundHosts {
				if _, shouldStayBound := shouldBeBoundHosts[hostKey]; !shouldStayBound {
					// Unbind this host
					host.IsBound = false
					host.AddressGroupRef = nil
					hostsToUpdate = append(hostsToUpdate, *host)
				}
			}

			reader.Close()

			// Step 5: Batch update all hosts that need changes
			if len(hostsToUpdate) > 0 {
				writer, err := s.registry.Writer(ctx)
				if err != nil {
					continue
				}

				if err := writer.SyncHosts(ctx, hostsToUpdate, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
					writer.Abort()
					continue
				}

				if err := writer.Commit(); err != nil {
					writer.Abort()
					continue
				}

				for _, host := range hostsToUpdate {
					if s.syncManager != nil {
						hostCopy := host // Create a copy for the pointer
						if syncErr := s.syncManager.SyncEntityForced(ctx, &hostCopy, types.SyncOperationUpsert); syncErr != nil {
						}
					}
				}
			}
		default:
		}
	}
}

// validateSGroupSyncForChangedHosts validates SGROUP synchronization for hosts that changed in AddressGroups
// This method detects hosts that were added/removed and validates them with SGROUP before committing the transaction
func (s *AddressGroupResourceService) validateSGroupSyncForChangedHosts(ctx context.Context, newAddressGroups []models.AddressGroup, oldAddressGroups map[string]*models.AddressGroup) error {

	for _, newAG := range newAddressGroups {
		oldAG := oldAddressGroups[newAG.Key()]
		addedHosts, removedHosts := s.getHostChanges(newAG, oldAG)
		if len(addedHosts) == 0 && len(removedHosts) == 0 {
			continue // No host changes for this AddressGroup
		}

		if len(addedHosts) > 0 {
			if err := s.validateHostsSGroupSync(ctx, addedHosts, newAG.ResourceIdentifier); err != nil {
				return errors.Wrapf(err, "SGROUP validation failed for added hosts in AddressGroup %s", newAG.Key())
			}
		}

		if len(removedHosts) > 0 {
			if err := s.forceSyncRemovedHostsWithSGroup(ctx, removedHosts, newAG.ResourceIdentifier); err != nil {
				return errors.Wrapf(err, "SGROUP sync failed for removed hosts in AddressGroup %s", newAG.Key())
			}
		}
	}

	return nil
}

// getHostChanges compares old and new AddressGroup hosts and returns added/removed hosts
func (s *AddressGroupResourceService) getHostChanges(newAG models.AddressGroup, oldAG *models.AddressGroup) (addedHosts, removedHosts []netguardv1beta1.ObjectReference) {
	newHosts := make(map[string]netguardv1beta1.ObjectReference)
	oldHosts := make(map[string]netguardv1beta1.ObjectReference)

	// Build map of new hosts
	for _, host := range newAG.Hosts {
		newHosts[host.Name] = host
	}

	// Build map of old hosts (if oldAG exists)
	if oldAG != nil {
		for _, host := range oldAG.Hosts {
			oldHosts[host.Name] = host
		}
	}

	// Find added hosts (in new but not in old)
	for hostName, host := range newHosts {
		if _, exists := oldHosts[hostName]; !exists {
			addedHosts = append(addedHosts, host)
		}
	}

	// Find removed hosts (in old but not in new)
	for hostName, host := range oldHosts {
		if _, exists := newHosts[hostName]; !exists {
			removedHosts = append(removedHosts, host)
		}
	}

	return addedHosts, removedHosts
}

// validateHostsSGroupSync validates a list of hosts with SGROUP
func (s *AddressGroupResourceService) validateHostsSGroupSync(ctx context.Context, hosts []netguardv1beta1.ObjectReference, agID models.ResourceIdentifier) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for host validation")
	}
	defer reader.Close()

	for _, hostRef := range hosts {
		// Load the actual host entity
		hostID := models.ResourceIdentifier{
			Name:      hostRef.Name,
			Namespace: agID.Namespace, // Host must be in same namespace as AddressGroup
		}

		host, err := reader.GetHostByID(ctx, hostID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				return errors.Errorf("host '%s' does not exist in namespace '%s'", hostRef.Name, agID.Namespace)
			}
			return errors.Wrapf(err, "failed to load host '%s' for SGROUP validation", hostRef.Name)
		}

		err = s.syncManager.SyncEntity(ctx, host, types.SyncOperationUpsert)
		if err != nil {
			return errors.Errorf("SGROUP synchronization failed for host '%s' in namespace '%s': %v - the host cannot be added to AddressGroup %s due to SGROUP constraints",
				hostRef.Name, agID.Namespace, err, agID.Key())
		}

	}

	return nil
}

func (s *AddressGroupResourceService) forceSyncRemovedHostsWithSGroup(ctx context.Context, removedHosts []netguardv1beta1.ObjectReference, agID models.ResourceIdentifier) error {
	if s.syncManager == nil {
		return nil
	}

	if len(removedHosts) == 0 {
		return nil
	}

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil
	}
	defer reader.Close()

	// Load and force sync each removed host
	for _, hostRef := range removedHosts {
		hostID := models.ResourceIdentifier{
			Namespace: agID.Namespace, // Host must be in same namespace as AddressGroup
			Name:      hostRef.Name,
		}

		host, err := reader.GetHostByID(ctx, hostID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Skip non-existent hosts
			}
			continue // Don't fail entire operation for one host
		}

		if err := s.syncManager.SyncEntityForced(ctx, host, types.SyncOperationUpsert); err != nil {
			// Continue with other hosts even if one fails
		}
	}

	return nil
}

func (s *AddressGroupResourceService) syncSpecHostsWithSGroups(ctx context.Context, hostRefs []netguardv1beta1.ObjectReference, agID models.ResourceIdentifier) error {
	if s.syncManager == nil {
		return nil
	}

	if len(hostRefs) == 0 {
		return nil
	}

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil
	}
	defer reader.Close()

	// Load and sync each host
	for _, hostRef := range hostRefs {
		hostID := models.ResourceIdentifier{
			Namespace: agID.Namespace, // Hosts are in the same namespace as AddressGroup
			Name:      hostRef.Name,
		}

		// Load full host data from database
		host, err := reader.GetHostByID(ctx, hostID)
		if err != nil {
			continue // Skip this host but continue with others
		}

		if err := s.syncManager.SyncEntityForced(ctx, host, types.SyncOperationUpsert); err != nil {
		}
	}

	return nil
}

func (s *AddressGroupResourceService) syncHostChangesWithSGroup(ctx context.Context, oldAG, newAG *models.AddressGroup) error {
	if s.syncManager == nil {
		return nil
	}

	// Calculate host changes
	addedHosts, removedHosts := s.getHostChanges(*newAG, oldAG)

	if len(addedHosts) == 0 && len(removedHosts) == 0 {
		return nil
	}

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil
	}
	defer reader.Close()

	// Sync added hosts (newly bound)
	for _, hostRef := range addedHosts {
		hostID := models.ResourceIdentifier{
			Namespace: newAG.Namespace,
			Name:      hostRef.Name,
		}

		host, err := reader.GetHostByID(ctx, hostID)
		if err != nil {
			continue
		}

		_ = s.syncManager.SyncEntityForced(ctx, host, types.SyncOperationUpsert) // Ignore sync errors
	}

	// Sync removed hosts (now unbound)
	for _, hostRef := range removedHosts {
		hostID := models.ResourceIdentifier{
			Namespace: newAG.Namespace,
			Name:      hostRef.Name,
		}

		host, err := reader.GetHostByID(ctx, hostID)
		if err != nil {
			continue
		}

		_ = s.syncManager.SyncEntityForced(ctx, host, types.SyncOperationUpsert) // Ignore sync errors
	}

	return nil
}
