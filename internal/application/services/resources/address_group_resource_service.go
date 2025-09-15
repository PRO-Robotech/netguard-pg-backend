package resources

import (
	"context"
	"fmt"
	"log"
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
	ruleS2SRegenerator RuleS2SRegenerator   // Optional - for IEAgAg rule updates when bindings change
	hostService        *HostResourceService // For updating Host.isBound status
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
	log.Printf("CreateAddressGroup: Creating AddressGroup %s", addressGroup.Key())

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Validate address group for creation
	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	if err := addressGroupValidator.ValidateForCreation(ctx, addressGroup); err != nil {
		log.Printf("CreateAddressGroup: Validation failed for AddressGroup %s: %v", addressGroup.Key(), err)
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

	// Validate SGROUP synchronization for new hosts before commit (CreateAddressGroup)
	log.Printf("🔍 CreateAddressGroup: SGROUP validation check for %s (syncManager_nil=%v)", addressGroup.Key(), s.syncManager == nil)
	if s.syncManager != nil && len(addressGroup.Hosts) > 0 {
		log.Printf("🔍 CreateAddressGroup: Calling validateHostsSGroupSync for %d hosts", len(addressGroup.Hosts))
		if err = s.validateHostsSGroupSync(ctx, addressGroup.Hosts, addressGroup.ResourceIdentifier); err != nil {
			log.Printf("❌ CreateAddressGroup: SGROUP validation failed: %v", err)
			return errors.Wrap(err, "SGROUP synchronization validation failed")
		}
		log.Printf("✅ CreateAddressGroup: SGROUP validation passed")
	} else {
		log.Printf("⚠️ CreateAddressGroup: SGROUP validation skipped (syncManager_nil=%v, hosts_count=%d)", s.syncManager == nil, len(addressGroup.Hosts))
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupConditions(ctx, &addressGroup); err != nil {
			klog.Errorf("Failed to process address group conditions for %s/%s: %v",
				addressGroup.Namespace, addressGroup.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	// Note: Host aggregation is now handled automatically by PostgreSQL triggers

	// Sync with external systems after successful creation
	s.syncAddressGroupsWithSGroups(ctx, []models.AddressGroup{addressGroup}, types.SyncOperationUpsert)

	// Update Host.isBound status for hosts in this AddressGroup
	if s.hostService != nil && len(addressGroup.Hosts) > 0 {
		log.Printf("CreateAddressGroup: Updating Host binding status for %d spec hosts", len(addressGroup.Hosts))
		if err := s.hostService.UpdateHostBindingStatus(ctx, nil, &addressGroup); err != nil {
			log.Printf("❌ Failed to update Host binding status after AddressGroup creation: %v", err)
			// Don't fail the operation if Host status update fails
		}
	}

	log.Printf("CreateAddressGroup: Successfully created AddressGroup %s", addressGroup.Key())
	return nil
}

// UpdateAddressGroup updates an existing address group
func (s *AddressGroupResourceService) UpdateAddressGroup(ctx context.Context, addressGroup models.AddressGroup) error {
	log.Printf("UpdateAddressGroup: Updating AddressGroup %s", addressGroup.Key())

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
		log.Printf("UpdateAddressGroup: Validation failed for AddressGroup %s: %v", addressGroup.Key(), err)
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

	// Validate SGROUP synchronization for host changes before commit (UpdateAddressGroup)
	log.Printf("🔍 UpdateAddressGroup: SGROUP validation check for %s (syncManager_nil=%v)", addressGroup.Key(), s.syncManager == nil)
	if s.syncManager != nil {
		log.Printf("🔍 UpdateAddressGroup: Calling validateSGroupSyncForChangedHosts")
		oldAddressGroups := map[string]*models.AddressGroup{
			existingAddressGroup.Key(): existingAddressGroup,
		}
		if err = s.validateSGroupSyncForChangedHosts(ctx, []models.AddressGroup{addressGroup}, oldAddressGroups); err != nil {
			log.Printf("❌ UpdateAddressGroup: SGROUP validation failed: %v", err)
			return errors.Wrap(err, "SGROUP synchronization validation failed")
		}
		log.Printf("✅ UpdateAddressGroup: SGROUP validation passed")
	} else {
		log.Printf("⚠️ UpdateAddressGroup: SGROUP validation skipped (syncManager is nil)")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupConditions(ctx, &addressGroup); err != nil {
			klog.Errorf("Failed to process address group conditions for %s/%s: %v",
				addressGroup.Namespace, addressGroup.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	// Note: Host aggregation is now handled automatically by PostgreSQL triggers

	// Sync with external systems after successful update
	s.syncAddressGroupsWithSGroups(ctx, []models.AddressGroup{addressGroup}, types.SyncOperationUpsert)

	// Update Host.isBound status for hosts changes in this AddressGroup
	if s.hostService != nil {
		log.Printf("UpdateAddressGroup: Updating Host binding status for hosts changes")
		if err := s.hostService.UpdateHostBindingStatus(ctx, existingAddressGroup, &addressGroup); err != nil {
			log.Printf("❌ Failed to update Host binding status after AddressGroup update: %v", err)
			// Don't fail the operation if Host status update fails
		}
	}

	log.Printf("UpdateAddressGroup: Successfully updated AddressGroup %s", addressGroup.Key())
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

	// Validate SGROUP synchronization for changed hosts before commit
	log.Printf("🔍 SGROUP_VALIDATION_CHECK: syncOp=%v, syncManager_nil=%v, addressGroups_count=%d", syncOp, s.syncManager == nil, len(addressGroups))
	if syncOp == models.SyncOpUpsert && s.syncManager != nil {
		log.Printf("🔍 SGROUP_VALIDATION_START: Calling validateSGroupSyncForChangedHosts with %d AddressGroups", len(addressGroups))
		if err = s.validateSGroupSyncForChangedHosts(ctx, addressGroups, oldAddressGroups); err != nil {
			log.Printf("❌ SGROUP_VALIDATION_FAILED: %v", err)
			return errors.Wrap(err, "SGROUP synchronization validation failed")
		}
		log.Printf("✅ SGROUP_VALIDATION_SUCCESS: All hosts passed SGROUP validation")
	} else {
		log.Printf("⚠️ SGROUP_VALIDATION_SKIPPED: syncOp=%v, syncManager_nil=%v", syncOp, s.syncManager == nil)
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit for each address group (skip for DELETE operations)
	if s.conditionManager != nil && syncOp != models.SyncOpDelete {
		for i := range addressGroups {
			if err := s.conditionManager.ProcessAddressGroupConditions(ctx, &addressGroups[i]); err != nil {
				klog.Errorf("Failed to process address group conditions for %s/%s: %v",
					addressGroups[i].Namespace, addressGroups[i].Name, err)
				// Don't fail the operation if condition processing fails
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

		// Update Host.isBound status for hosts in synced AddressGroups
		s.updateHostBindingStatusForSyncedAddressGroups(ctx, addressGroups, syncOp)
	} else {
		// For DELETE operations, unbind all hosts from deleted AddressGroups
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

	// PHASE 1: Validate dependencies for each AddressGroup
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for validation")
	}

	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	for _, id := range ids {
		log.Printf("DeleteAddressGroupsByIDs: Validating dependencies for AddressGroup %s", id.Key())
		if err := addressGroupValidator.CheckDependencies(ctx, id); err != nil {
			log.Printf("DeleteAddressGroupsByIDs: Cannot delete AddressGroup %s due to dependencies: %v", id.Key(), err)
			return errors.Wrapf(err, "cannot delete AddressGroup %s", id.Key())
		}
	}

	log.Printf("DeleteAddressGroupsByIDs: All %d AddressGroups validated for deletion", len(ids))

	// PHASE 2: Fetch AddressGroups before deletion (needed for external sync)

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

	// PHASE 2: Find and delete related AddressGroupBindings (CRITICAL - this triggers Universal Recalculation)

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

	// PHASE 2.5: Find and delete related NetworkBindings (CRITICAL for Network.IsBound update)

	var networkBindingsToDelete []models.ResourceIdentifier
	var networksToUpdate []models.ResourceIdentifier
	err = reader.ListNetworkBindings(ctx, func(binding models.NetworkBinding) error {
		for _, agToDelete := range addressGroupsToDelete {
			if binding.AddressGroupRef.Name == agToDelete.SelfRef.Name &&
				binding.SelfRef.Namespace == agToDelete.SelfRef.Namespace {
				networkBindingsToDelete = append(networkBindingsToDelete, binding.SelfRef.ResourceIdentifier)
				// Также помечаем Network для обновления IsBound=false
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

	// PHASE 3: Cascade delete bindings FIRST (triggers Universal Recalculation Engine via Service.AddressGroups changes)
	if len(bindingsToDelete) > 0 {
		if err := s.DeleteAddressGroupBindingsByIDs(ctx, bindingsToDelete); err != nil {
			return errors.Wrap(err, "failed to cascade delete AddressGroupBindings")
		}
	}

	// PHASE 3.5: Cascade delete NetworkBindings using writer (CRITICAL for Network.IsBound update)
	if len(networkBindingsToDelete) > 0 {

		// Get writer for NetworkBinding deletion
		networkBindingWriter, err := s.registry.Writer(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get writer for NetworkBinding deletion")
		}
		defer func() {
			if err != nil {
				networkBindingWriter.Abort()
			}
		}()

		// Delete NetworkBindings
		if err := networkBindingWriter.DeleteNetworkBindingsByIDs(ctx, networkBindingsToDelete); err != nil {
			return errors.Wrap(err, "failed to cascade delete NetworkBindings")
		}

		if err := networkBindingWriter.Commit(); err != nil {
			return errors.Wrap(err, "failed to commit NetworkBinding deletion")
		}

		// PHASE 3.6: Update Networks to set IsBound=false after NetworkBinding deletion
		if len(networksToUpdate) > 0 {

			// Get a new writer for Network updates
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

				// Update Network to clear binding
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

	// PHASE 4: Delete AddressGroups from storage
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

	// PHASE 5: External sync - Delete AddressGroups from sgroups (CRITICAL FIX)
	s.syncAddressGroupsWithSGroups(ctx, addressGroupsToDelete, types.SyncOperationDelete)

	// Update Host.isBound status for hosts in deleted AddressGroups
	if s.hostService != nil {
		for _, deletedAG := range addressGroupsToDelete {
			if len(deletedAG.Hosts) > 0 {
				log.Printf("DeleteAddressGroups: Unbinding %d hosts from deleted AddressGroup %s",
					len(deletedAG.Hosts), deletedAG.Key())
				if err := s.hostService.UpdateHostBindingStatus(ctx, &deletedAG, nil); err != nil {
					log.Printf("❌ Failed to unbind hosts after AddressGroup deletion: %v", err)
					// Don't fail the operation if Host status update fails
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
	log.Printf("CreateAddressGroupBinding: Creating AddressGroupBinding %s", binding.Key())

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

	// Validate binding for creation
	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetAddressGroupBindingValidator()

	if err := bindingValidator.ValidateForCreation(ctx, &binding); err != nil {
		log.Printf("CreateAddressGroupBinding: Validation failed for AddressGroupBinding %s: %v", binding.Key(), err)
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

	// 🎯 ARCHITECTURAL FIX: Synchronize Service.AddressGroups field like reference controller
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

	// Regenerate IEAgAg rules that depend on this new AddressGroupBinding
	log.Printf("CreateAddressGroupBinding: AddressGroupBinding %s created, triggering IEAgAg rules regeneration", binding.Key())

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

	log.Printf("CreateAddressGroupBinding: Successfully created AddressGroupBinding %s", binding.Key())
	return nil
}

// UpdateAddressGroupBinding updates an existing address group binding
func (s *AddressGroupResourceService) UpdateAddressGroupBinding(ctx context.Context, binding models.AddressGroupBinding) error {
	log.Printf("UpdateAddressGroupBinding: Updating AddressGroupBinding %s", binding.Key())

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
		log.Printf("UpdateAddressGroupBinding: Validation failed for AddressGroupBinding %s: %v", binding.Key(), err)
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

	// 🎯 NEW: Notify RuleS2S service about AddressGroup changes for reactive dependency chain
	log.Printf("UpdateAddressGroupBinding: Triggering RuleS2S condition recalculation for updated binding %s", binding.Key())
	serviceID := models.ResourceIdentifier{Name: binding.ServiceRef.Name, Namespace: binding.ServiceRef.Namespace}
	if s.ruleS2SRegenerator != nil {
		if err := s.ruleS2SRegenerator.NotifyServiceAddressGroupsChanged(ctx, serviceID); err != nil {
			klog.Errorf("Failed to notify RuleS2S regenerator for updated AddressGroupBinding %s: %v",
				binding.Key(), err)
			// Don't fail the operation, but log the issue
		}
	}

	// Regenerate IEAgAg rules that depend on this AddressGroupBinding
	log.Printf("UpdateAddressGroupBinding: AddressGroupBinding %s updated, triggering IEAgAg rules regeneration", binding.Key())

	if s.ruleS2SRegenerator != nil {
		bindingID := models.ResourceIdentifier{Name: binding.Name, Namespace: binding.Namespace}
		if err := s.ruleS2SRegenerator.RegenerateIEAgAgRulesForAddressGroupBinding(ctx, bindingID); err != nil {
			klog.Errorf("Failed to regenerate IEAgAg rules for AddressGroupBinding %s: %v",
				binding.Key(), err)
			// Don't fail the operation if IEAgAg rule regeneration fails
		} else {
		}
	} else {
	}

	log.Printf("UpdateAddressGroupBinding: Successfully updated AddressGroupBinding %s", binding.Key())
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

	// Only process conditions for non-DELETE operations
	if syncOp != models.SyncOpDelete {
		// Process conditions after successful commit for each address group binding
		if s.conditionManager != nil {
			for i := range bindings {
				if err := s.conditionManager.ProcessAddressGroupBindingConditions(ctx, &bindings[i]); err != nil {
					klog.Errorf("Failed to process address group binding conditions for %s/%s: %v",
						bindings[i].Namespace, bindings[i].Name, err)
					// Don't fail the operation if condition processing fails
				}
			}
		} else {
		}
	} else {
	}

	// 🎯 ARCHITECTURAL FIX: Synchronize Service.AddressGroups field for all affected services
	// This matches the reference controller behavior for maintaining Service.AddressGroups integrity
	serviceIDs := make(map[string]models.ResourceIdentifier)
	for _, binding := range bindings {
		key := binding.ServiceRef.Namespace + "/" + binding.ServiceRef.Name
		serviceIDs[key] = models.ResourceIdentifier{Name: binding.ServiceRef.Name, Namespace: binding.ServiceRef.Namespace}
	}

	for _, serviceID := range serviceIDs {
		if err := s.synchronizeServiceAddressGroups(ctx, serviceID); err != nil {
			klog.Errorf("Failed to synchronize Service.AddressGroups for service %s after binding sync: %v", serviceID.Key(), err)
			// Don't fail the operation, but log the critical issue
		}
	}

	// 🎯 NEW: Notify RuleS2S service about AddressGroup changes to enable reactive dependency chain
	// This is the key missing piece for API Aggregation Layer reactive flow
	// IMPORTANT: Include DELETE operations to trigger RuleS2S condition recalculation and IEAgAgRule cleanup
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

	// Collect AddressGroups that will be affected by binding deletions
	// AND collect bindings for Service.AddressGroups updates
	affectedAddressGroups := make(map[string]models.ResourceIdentifier)
	bindingsToRemove := make([]models.AddressGroupBinding, 0) // 🎯 NEW: Track bindings for Service updates

	for _, id := range ids {
		binding, err := reader.GetAddressGroupBindingByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				log.Printf("DeleteAddressGroupBindingsByIDs: Binding %s not found (already deleted?)", id.Key())
				continue // Skip if binding already deleted
			}
			return errors.Wrapf(err, "failed to get binding %s for port mapping regeneration", id.Key())
		}

		// Get service details to log ports being affected
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

		// 🎯 NEW: Track binding for Service.AddressGroups removal
		bindingsToRemove = append(bindingsToRemove, *binding)

	}

	// 🔧 SERIALIZATION_FIX: Use WriterForDeletes to reduce serialization conflicts during concurrent delete operations
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
				// Don't fail the operation, but log the critical issue
			}
		}

		// Notify for each unique service
		for _, serviceID := range serviceIDs {
			if err := s.ruleS2SRegenerator.NotifyServiceAddressGroupsChanged(ctx, serviceID); err != nil {
				// Don't fail the operation, but log the issue
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
	log.Printf("CreateAddressGroupPortMapping: Creating AddressGroupPortMapping %s", mapping.Key())

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Validate mapping for creation
	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	if err := mappingValidator.ValidateForCreation(ctx, mapping); err != nil {
		log.Printf("CreateAddressGroupPortMapping: Validation failed for AddressGroupPortMapping %s: %v", mapping.Key(), err)
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

	log.Printf("CreateAddressGroupPortMapping: Successfully created AddressGroupPortMapping %s", mapping.Key())
	return nil
}

// UpdateAddressGroupPortMapping updates an existing address group port mapping
func (s *AddressGroupResourceService) UpdateAddressGroupPortMapping(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	log.Printf("UpdateAddressGroupPortMapping: Updating AddressGroupPortMapping %s", mapping.Key())

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
		log.Printf("UpdateAddressGroupPortMapping: Validation failed for AddressGroupPortMapping %s: %v", mapping.Key(), err)
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

	log.Printf("UpdateAddressGroupPortMapping: Successfully updated AddressGroupPortMapping %s", mapping.Key())
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
	log.Printf("CreateAddressGroupBindingPolicy: Creating AddressGroupBindingPolicy %s", policy.Key())

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
		log.Printf("CreateAddressGroupBindingPolicy: Validation failed for AddressGroupBindingPolicy %s: %v", policy.Key(), err)
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

	log.Printf("CreateAddressGroupBindingPolicy: Successfully created AddressGroupBindingPolicy %s", policy.Key())
	return nil
}

// UpdateAddressGroupBindingPolicy updates an existing address group binding policy
func (s *AddressGroupResourceService) UpdateAddressGroupBindingPolicy(ctx context.Context, policy models.AddressGroupBindingPolicy) error {
	log.Printf("UpdateAddressGroupBindingPolicy: Updating AddressGroupBindingPolicy %s", policy.Key())

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
		log.Printf("UpdateAddressGroupBindingPolicy: Validation failed for AddressGroupBindingPolicy %s: %v", policy.Key(), err)
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

	log.Printf("UpdateAddressGroupBindingPolicy: Successfully updated AddressGroupBindingPolicy %s", policy.Key())
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

		// IMPORTANT: Do NOT process conditions here during shared transaction!
		// Conditions will be processed after the main transaction commits
		// to avoid transaction conflicts and status overwrites
	}

	return nil
}

// SyncAddressGroupPortMappings syncs port mappings for a binding
func (s *AddressGroupResourceService) SyncAddressGroupPortMappings(ctx context.Context, binding models.AddressGroupBinding) error {
	return s.SyncAddressGroupPortMappingsWithSyncOp(ctx, binding, models.SyncOpUpsert)
}

// SyncAddressGroupPortMappingsWithSyncOp syncs port mappings with specific sync operation
func (s *AddressGroupResourceService) SyncAddressGroupPortMappingsWithSyncOp(ctx context.Context, binding models.AddressGroupBinding, syncOp models.SyncOp) error {
	// For DELETE operations, we need to regenerate the complete mapping after the binding change
	// For other operations, we can use the optimized single-binding approach
	if syncOp == models.SyncOpDelete {
		return s.regenerateCompletePortMappingForAddressGroup(ctx, binding.AddressGroupRef.Name, binding.AddressGroupRef.Namespace)
	}

	// For non-DELETE operations, use the existing optimized approach
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

	// Process conditions AFTER successful commit to avoid transaction conflicts
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
			// Port conflict detected - this should fail the entire operation
			return errors.Wrapf(err, "failed to generate port mapping for AddressGroup %s", addressGroupRef.Key())
		}
		if addressGroupPortMapping != nil {

			if err := s.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, addressGroupPortMapping); err != nil {
				klog.Errorf("Failed to process AddressGroupPortMapping conditions for %s/%s: %v",
					addressGroupPortMapping.Namespace, addressGroupPortMapping.Name, err)
				// Don't fail the operation if condition processing fails
			}
			// Note: ProcessAddressGroupPortMappingConditions handles its own save via saveAddressGroupPortMappingConditions
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

	// Find ALL AddressGroupBindings that reference this service (including cross-namespace bindings via policies)
	// We search in ALL namespaces because bindings can exist in different namespaces than their target service
	var affectedBindings []models.AddressGroupBinding
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		// Check if this binding references our service (name and namespace must match exactly)
		if binding.ServiceRef.Name == serviceID.Name && binding.ServiceRef.Namespace == serviceID.Namespace {
			affectedBindings = append(affectedBindings, binding)

			// Highlight cross-namespace bindings for visibility
			crossNamespace := binding.Namespace != serviceID.Namespace || binding.AddressGroupRef.Namespace != serviceID.Namespace
			crossMarker := ""
			if crossNamespace {
				crossMarker = " 🌐"
			}

			log.Printf("  📎 Found binding: %s/%s (service: %s/%s → addressgroup: %s/%s)%s",
				binding.Namespace, binding.Name, binding.ServiceRef.Namespace, binding.ServiceRef.Name,
				binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name, crossMarker)
		}
		return nil
	}, ports.EmptyScope{}) // EmptyScope = search ALL namespaces

	if err != nil {
		return errors.Wrap(err, "failed to list address group bindings")
	}

	if len(affectedBindings) == 0 {
		log.Printf("RegeneratePortMappingsForService: No bindings found for service %s", serviceID.Key())
		return nil
	}

	// Group bindings by their target AddressGroup to avoid duplicate regeneration
	addressGroupsToRegenerate := make(map[string]models.ResourceIdentifier)
	for _, binding := range affectedBindings {
		agKey := fmt.Sprintf("%s/%s", binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name)
		addressGroupsToRegenerate[agKey] = models.ResourceIdentifier{
			Name:      binding.AddressGroupRef.Name,
			Namespace: binding.AddressGroupRef.Namespace,
		}
	}

	log.Printf("RegeneratePortMappingsForService: Service %s affects %d AddressGroups",
		serviceID.Key(), len(addressGroupsToRegenerate))

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

// =============================================================================
// Private Helper Methods (extracted from original NetguardService)
// =============================================================================

// syncAddressGroups handles the actual address group synchronization logic
func (s *AddressGroupResourceService) syncAddressGroups(ctx context.Context, writer ports.Writer, addressGroups []models.AddressGroup, syncOp models.SyncOp) error {
	log.Printf("syncAddressGroups: Syncing %d address groups with operation %s", len(addressGroups), syncOp)

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
		// For FullSync operation, use empty scope to delete all address groups, then add only new ones
		scope = ports.EmptyScope{}
	} else if len(addressGroups) > 0 {
		var ids []models.ResourceIdentifier
		for _, addressGroup := range addressGroups {
			ids = append(ids, addressGroup.ResourceIdentifier)
		}
		scope = ports.NewResourceIdentifierScope(ids...)
	} else {
		scope = ports.EmptyScope{}
	}

	// If this is deletion, use DeleteAddressGroupsByIDs for correct cascading deletion
	if syncOp == models.SyncOpDelete {
		// Collect address group IDs
		var ids []models.ResourceIdentifier
		for _, addressGroup := range addressGroups {
			ids = append(ids, addressGroup.ResourceIdentifier)
		}

		// 🚨 CRITICAL FIX: Call service-level DeleteAddressGroupsByIDs for complete reference architecture compliance
		// This includes: cascading binding deletion + Universal Recalculation + external sync + storage deletion
		return s.DeleteAddressGroupsByIDs(ctx, ids)
	}

	// Execute operation with specified option for non-deletion
	if err := writer.SyncAddressGroups(ctx, addressGroups, scope, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address groups")
	}

	log.Printf("syncAddressGroups: Successfully synced %d address groups", len(addressGroups))
	return nil
}

// syncAddressGroupBindings handles the actual address group binding synchronization logic
func (s *AddressGroupResourceService) syncAddressGroupBindings(ctx context.Context, writer ports.Writer, bindings []models.AddressGroupBinding, syncOp models.SyncOp) error {
	log.Printf("syncAddressGroupBindings: Syncing %d address group bindings with operation %s", len(bindings), syncOp)

	// For UPSERT/FULLSYNC operations, validate port conflicts BEFORE database sync to prevent invalid bindings
	// SKIP backend validation for individual CREATE operations to avoid circular dependency
	if syncOp == models.SyncOpUpsert || syncOp == models.SyncOpFullSync {
		// Skip backend service lookup validation for CREATE operations (single bindings from K8s API)
		// This avoids circular dependency where binding CREATE tries to read service from backend
		// before the service has been persisted to backend database
		if len(bindings) == 1 {
		} else {

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
					// Include bindings that target the same AddressGroup but are different bindings
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

				// Check for port conflicts between the new service and existing bound services
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
	}

	if err := writer.SyncAddressGroupBindings(ctx, bindings, ports.EmptyScope{}, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address group bindings in storage")
	}

	log.Printf("syncAddressGroupBindings: Successfully synced %d address group bindings", len(bindings))
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
	log.Printf("syncAddressGroupPortMappings: Syncing %d address group port mappings with operation %s", len(mappings), syncOp)

	if err := writer.SyncAddressGroupPortMappings(ctx, mappings, ports.EmptyScope{}, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address group port mappings in storage")
	}

	log.Printf("syncAddressGroupPortMappings: Successfully synced %d address group port mappings", len(mappings))
	return nil
}

// syncAddressGroupBindingPolicies handles the actual address group binding policy synchronization logic
func (s *AddressGroupResourceService) syncAddressGroupBindingPolicies(ctx context.Context, writer ports.Writer, policies []models.AddressGroupBindingPolicy, syncOp models.SyncOp) error {
	log.Printf("syncAddressGroupBindingPolicies: Syncing %d address group binding policies with operation %s", len(policies), syncOp)

	if err := writer.SyncAddressGroupBindingPolicies(ctx, policies, ports.EmptyScope{}, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address group binding policies in storage")
	}

	log.Printf("syncAddressGroupBindingPolicies: Successfully synced %d address group binding policies", len(policies))
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

	for _, addressGroup := range addressGroups {
		// Create a copy to avoid pointer issues
		agCopy := addressGroup
		if syncableEntity, ok := interface{}(&agCopy).(interfaces.SyncableEntity); ok {
			syncableEntities = append(syncableEntities, syncableEntity)
		} else {
			unsyncableKeys = append(unsyncableKeys, addressGroup.Key())
		}
	}

	// Log any unsyncable address groups
	if len(unsyncableKeys) > 0 {
	}

	// Perform batch sync for all syncable address groups
	if len(syncableEntities) > 0 {
		if err := s.syncManager.SyncBatch(ctx, syncableEntities, operation); err != nil {
			// Don't fail the whole operation if sgroups sync fails
		} else {
		}
	}
}

// generateAddressGroupPortMapping generates port mapping from binding (LEGACY - kept for compatibility)
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

// generateCompleteAddressGroupPortMapping generates port mapping for ALL services bound to an AddressGroup
// Returns error if port conflicts are detected to prevent binding creation
func (s *AddressGroupResourceService) generateCompleteAddressGroupPortMapping(ctx context.Context, reader ports.Reader, addressGroupRef models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {

	// Find all bindings for this AddressGroup
	var bindings []models.AddressGroupBinding
	err := reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		// Check if this binding targets our AddressGroup
		if binding.AddressGroupRef.Name == addressGroupRef.Name && binding.AddressGroupRef.Namespace == addressGroupRef.Namespace {
			bindings = append(bindings, binding)
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		log.Printf("generateCompleteAddressGroupPortMapping: Failed to list bindings for AddressGroup %s: %v", addressGroupRef.Key(), err)
		return nil, errors.Wrapf(err, "failed to list bindings for AddressGroup %s", addressGroupRef.Key())
	}

	if len(bindings) == 0 {
		log.Printf("generateCompleteAddressGroupPortMapping: No bindings found for AddressGroup %s, creating empty mapping", addressGroupRef.Key())
		emptyMapping := &models.AddressGroupPortMapping{
			SelfRef: models.SelfRef{
				ResourceIdentifier: addressGroupRef,
			},
			AccessPorts: make(map[models.ServiceRef]models.ServicePorts), // Empty services map
		}
		return emptyMapping, nil
	}

	// Create the port mapping
	addressGroupPortMapping := &models.AddressGroupPortMapping{
		SelfRef: models.SelfRef{
			ResourceIdentifier: addressGroupRef,
		},
		AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
	}

	// Collect services from all bindings
	for _, binding := range bindings {
		// Get the service referenced in this binding
		service, err := reader.GetServiceByID(ctx, models.ResourceIdentifier{
			Name:      binding.ServiceRef.Name,
			Namespace: binding.ServiceRef.Namespace,
		})
		if err != nil {
			log.Printf("generateCompleteAddressGroupPortMapping: Failed to get service %s/%s: %v",
				binding.ServiceRef.Namespace, binding.ServiceRef.Name, err)
			continue // Skip this service but continue with others
		}

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
				log.Printf("generateCompleteAddressGroupPortMapping: Failed to parse port %s for service %s: %v",
					ingressPort.Port, service.Name, err)
				continue // Skip invalid ports
			}

			// Add all parsed port ranges to service ports
			for _, portRange := range portRanges {
				servicePorts.Ports[transport] = append(servicePorts.Ports[transport], portRange)
			}
		}

		addressGroupPortMapping.AccessPorts[serviceRef] = servicePorts
	}

	// Validate port mapping for conflicts using ValidationService
	if s.validationService != nil {
		mappingValidator := validation.NewAddressGroupPortMappingValidator(reader)
		if err := mappingValidator.CheckInternalPortOverlaps(*addressGroupPortMapping); err != nil {
			log.Printf("generateCompleteAddressGroupPortMapping: Port conflict detected for AddressGroup %s: %v",
				addressGroupRef.Key(), err)
			// Return error to prevent creation of conflicting mapping and fail the binding operation
			return nil, errors.Wrapf(err, "port conflict detected for AddressGroup %s", addressGroupRef.Key())
		}
	} else {
	}

	return addressGroupPortMapping, nil
}

// 🎯 REMOVED: updateServiceAddressGroups method
// REASON: Service.AddressGroups is a computed field from AddressGroupBindings, not a stored field
// The PostgreSQL schema has no address_groups column - it's computed by the Service reader
// We use direct notification instead of manual Service updates

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

// synchronizeServiceAddressGroups implements the reference architecture pattern:
// Updates Service.AddressGroups field directly by reading current bindings and syncing the service
// This matches the behavior of AddressGroupBinding controller in netguard-k8s-controller
func (s *AddressGroupResourceService) synchronizeServiceAddressGroups(ctx context.Context, serviceID models.ResourceIdentifier) error {

	// Step 1: Get current service
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for service sync")
	}
	defer reader.Close()

	service, err := reader.GetServiceByID(ctx, serviceID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil // Service was deleted, nothing to sync
		}
		return errors.Wrapf(err, "failed to get service %s for AddressGroups sync", serviceID.Key())
	}

	for i, ag := range service.AddressGroups {
		log.Printf("  [%d] %s/%s", i, ag.Namespace, ag.Name)
	}

	// Step 2: The service already has refreshed AddressGroups from loadServiceAddressGroups
	// We just need to sync it back to storage to maintain consistency
	// 🔧 SERIALIZATION_FIX: Use WriterForConditions to avoid serialization conflicts during cascade operations
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

	// Step 3: Sync the service with its current AddressGroups state
	// This ensures the Service record reflects the current binding relationships
	if err := writer.SyncServices(ctx, []models.Service{*service}, ports.EmptyScope{}); err != nil {
		writer.Abort()
		return errors.Wrapf(err, "failed to sync service %s with updated AddressGroups", serviceID.Key())
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

	log.Printf("🔄 updateHostBindingStatusForSyncedAddressGroups: Processing %d AddressGroups with syncOp=%v", len(addressGroups), syncOp)

	for _, ag := range addressGroups {
		log.Printf("🔧 DEBUG updateHostBindingStatusForSyncedAddressGroups: AddressGroup %s - syncOp=%v (%T), ag.AggregatedHosts=%d, ag.Hosts=%d", ag.Key(), syncOp, syncOp, len(ag.AggregatedHosts), len(ag.Hosts))

		// IMPORTANT: AddressGroup objects from Sync API don't contain AggregatedHosts field
		// We need to load the fresh data from database to get the accurate host information
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			log.Printf("❌ Failed to get reader for loading fresh AddressGroup data: %v", err)
			continue
		}

		freshAG, err := reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{Name: ag.Name, Namespace: ag.Namespace})
		reader.Close()
		if err != nil {
			log.Printf("❌ Failed to load fresh AddressGroup %s: %v", ag.Key(), err)
			continue
		}

		// Use fresh data from database that contains AggregatedHosts
		ag = *freshAG
		log.Printf("🔄 FIXED: Fresh AddressGroup %s - AggregatedHosts=%d, Hosts=%d", ag.Key(), len(ag.AggregatedHosts), len(ag.Hosts))

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
			log.Printf("🔓 Unbinding %d hosts from deleted AddressGroup %s (AggregatedHosts=%d, Hosts=%d)", totalHosts, ag.Key(), len(ag.AggregatedHosts), len(ag.Hosts))
			if err := s.hostService.UpdateHostBindingStatus(ctx, &ag, nil); err != nil {
				log.Printf("❌ Failed to unbind hosts from deleted AddressGroup %s: %v", ag.Key(), err)
			}

		case models.SyncOpUpsert, models.SyncOpFullSync:
			// Complete host binding management: handle both adding and removing hosts from AddressGroup
			log.Printf("🔄 Processing host binding changes for AddressGroup %s", ag.Key())

			reader, err := s.registry.Reader(ctx)
			if err != nil {
				log.Printf("❌ Failed to get reader for host binding management: %v", err)
				continue
			}

			// Step 1: Find all hosts currently bound to this AddressGroup (old bindings)
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
				log.Printf("❌ Failed to list hosts for binding comparison: %v", err)
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

			log.Printf("📊 Binding analysis for %s: %d currently bound, %d should be bound",
				ag.Key(), len(currentlyBoundHosts), len(shouldBeBoundHosts))

			var hostsToUpdate []models.Host

			// Step 3: Find hosts to bind (in shouldBe but not in current)
			for hostKey, hostID := range shouldBeBoundHosts {
				if _, alreadyBound := currentlyBoundHosts[hostKey]; !alreadyBound {
					host, err := reader.GetHostByID(ctx, hostID)
					if err != nil {
						log.Printf("❌ Failed to get host %s for binding: %v", hostID.Key(), err)
						continue
					}

					// Bind this host
					host.IsBound = true
					host.AddressGroupRef = &netguardv1beta1.ObjectReference{
						Name: ag.Name,
					}
					hostsToUpdate = append(hostsToUpdate, *host)
					log.Printf("➕ Host %s queued for binding to %s", hostID.Key(), ag.Key())
				}
			}

			// Step 4: Find hosts to unbind (in current but not in shouldBe)
			for hostKey, host := range currentlyBoundHosts {
				if _, shouldStayBound := shouldBeBoundHosts[hostKey]; !shouldStayBound {
					// Unbind this host
					host.IsBound = false
					host.AddressGroupRef = nil
					hostsToUpdate = append(hostsToUpdate, *host)
					log.Printf("➖ Host %s queued for unbinding from %s", host.Key(), ag.Key())
				}
			}

			reader.Close()

			// Step 5: Batch update all hosts that need changes
			if len(hostsToUpdate) > 0 {
				writer, err := s.registry.Writer(ctx)
				if err != nil {
					log.Printf("❌ Failed to get writer for host binding updates: %v", err)
					continue
				}

				if err := writer.SyncHosts(ctx, hostsToUpdate, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
					log.Printf("❌ Failed to sync host binding updates: %v", err)
					writer.Abort()
					continue
				}

				if err := writer.Commit(); err != nil {
					log.Printf("❌ Failed to commit host binding updates: %v", err)
					writer.Abort()
					continue
				}

				// 🔄 NEW: Sync updated hosts with SGroup after binding status changes
				log.Printf("🔗 Syncing %d hosts with SGroup after binding changes for AddressGroup %s", len(hostsToUpdate), ag.Key())
				for _, host := range hostsToUpdate {
					if s.hostService != nil {
						hostCopy := host // Create a copy for the pointer
						if syncErr := s.hostService.SyncHostWithExternal(ctx, &hostCopy, types.SyncOperationUpsert); syncErr != nil {
							log.Printf("❌ Failed to sync Host %s with SGroup: %v", host.Key(), syncErr)
							// Continue with other hosts even if one fails
						} else {
							log.Printf("✅ Successfully synced Host %s with SGroup (isBound=%v)", host.Key(), host.IsBound)
						}
					}
				}

				log.Printf("✅ Updated %d hosts binding status for AddressGroup %s", len(hostsToUpdate), ag.Key())
			} else {
				log.Printf("ℹ️ No host binding changes needed for AddressGroup %s", ag.Key())
			}

		default:
			log.Printf("⚠️ Unknown syncOp %v for AddressGroup %s", syncOp, ag.Key())
		}
	}
}

// validateSGroupSyncForChangedHosts validates SGROUP synchronization for hosts that changed in AddressGroups
// This method detects hosts that were added/removed and validates them with SGROUP before committing the transaction
func (s *AddressGroupResourceService) validateSGroupSyncForChangedHosts(ctx context.Context, newAddressGroups []models.AddressGroup, oldAddressGroups map[string]*models.AddressGroup) error {
	log.Printf("🔍 SGROUP validation: Processing %d AddressGroups for host changes (oldAddressGroups_count=%d)", len(newAddressGroups), len(oldAddressGroups))

	for _, newAG := range newAddressGroups {
		oldAG := oldAddressGroups[newAG.Key()]

		// Get changed hosts
		addedHosts, removedHosts := s.getHostChanges(newAG, oldAG)

		if len(addedHosts) == 0 && len(removedHosts) == 0 {
			continue // No host changes for this AddressGroup
		}

		log.Printf("🔄 SGROUP validation: AddressGroup %s - %d added hosts, %d removed hosts",
			newAG.Key(), len(addedHosts), len(removedHosts))

		// Validate SGROUP synchronization for added hosts
		if len(addedHosts) > 0 {
			if err := s.validateHostsSGroupSync(ctx, addedHosts, newAG.ResourceIdentifier); err != nil {
				return errors.Wrapf(err, "SGROUP validation failed for added hosts in AddressGroup %s", newAG.Key())
			}
		}

		// Note: We don't need to validate removed hosts with SGROUP since they're being removed
		// The SGROUP sync for removal will happen post-commit in updateHostBindingStatusForSyncedAddressGroups
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

	for i, hostRef := range hosts {
		// Load the actual host entity
		hostID := models.ResourceIdentifier{
			Name:      hostRef.Name,
			Namespace: agID.Namespace, // Host must be in same namespace as AddressGroup
		}

		host, err := reader.GetHostByID(ctx, hostID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				return errors.Errorf("host reference %d: host '%s' does not exist in namespace '%s'", i, hostRef.Name, agID.Namespace)
			}
			return errors.Wrapf(err, "host reference %d: failed to load host '%s' for SGROUP validation", i, hostRef.Name)
		}

		// Test SGROUP synchronization
		err = s.syncManager.SyncEntity(ctx, host, types.SyncOperationUpsert)
		if err != nil {
			return errors.Errorf("SGROUP synchronization failed for host '%s' in namespace '%s': %v - the host cannot be added to AddressGroup %s due to SGROUP constraints",
				hostRef.Name, agID.Namespace, err, agID.Key())
		}

		log.Printf("✅ SGROUP validation passed for host %s/%s in AddressGroup %s", agID.Namespace, hostRef.Name, agID.Key())
	}

	return nil
}
