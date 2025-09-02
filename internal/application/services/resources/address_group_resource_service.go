package resources

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
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
	ruleS2SRegenerator RuleS2SRegenerator // Optional - for IEAgAg rule updates when bindings change
}

// RuleS2SRegenerator interface is now defined in interfaces.go to avoid circular dependencies

// NewAddressGroupResourceService creates a new AddressGroupResourceService
func NewAddressGroupResourceService(
	registry ports.Registry,
	syncManager interfaces.SyncManager,
	conditionManager AddressGroupConditionManagerInterface,
	validationService *ValidationService,
) *AddressGroupResourceService {
	return &AddressGroupResourceService{
		registry:           registry,
		syncManager:        syncManager,
		conditionManager:   conditionManager,
		validationService:  validationService,
		ruleS2SRegenerator: nil, // Will be set later via SetRuleS2SRegenerator
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

	// Sync with external systems after successful creation
	s.syncAddressGroupsWithSGroups(ctx, []models.AddressGroup{addressGroup}, types.SyncOperationUpsert)

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

	// Sync with external systems after successful update
	s.syncAddressGroupsWithSGroups(ctx, []models.AddressGroup{addressGroup}, types.SyncOperationUpsert)

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

	if err = s.syncAddressGroups(ctx, writer, addressGroups, syncOp); err != nil {
		return errors.Wrap(err, "failed to sync address groups")
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
		log.Printf("✅ ConditionManager: Processed conditions for %d AddressGroups (syncOp=%s)", len(addressGroups), syncOp)
	} else if syncOp == models.SyncOpDelete {
		log.Printf("🚫 ConditionManager: Skipping condition processing for DELETE operation to prevent recreation of deleted AddressGroups")
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
		log.Printf("🔄 SyncAddressGroups: External sync completed with operation %s", externalSyncOp)
	} else {
		log.Printf("🔄 SyncAddressGroups: Skipping external sync for DELETE operation (handled by DeleteAddressGroupsByIDs)")
	}

	return nil
}

// DeleteAddressGroupsByIDs deletes address groups by IDs with reference architecture compliance
// Follows k8s-controller pattern: cascade delete bindings first, then AddressGroups, with proper external sync
func (s *AddressGroupResourceService) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	if len(ids) == 0 {
		return nil
	}

	log.Printf("🔄 DeleteAddressGroupsByIDs: Starting reference-compliant deletion for %d AddressGroups", len(ids))

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
				log.Printf("⚠️ DeleteAddressGroupsByIDs: AddressGroup %s not found, skipping", id.Key())
				continue
			}
			return errors.Wrapf(err, "failed to fetch AddressGroup %s", id.Key())
		}
		addressGroupsToDelete = append(addressGroupsToDelete, *addressGroup)
		log.Printf("🔍 DeleteAddressGroupsByIDs: Found AddressGroup %s for deletion", id.Key())
	}

	if len(addressGroupsToDelete) == 0 {
		log.Printf("ℹ️ DeleteAddressGroupsByIDs: No AddressGroups found for deletion")
		return nil
	}

	// PHASE 2: Find and delete related AddressGroupBindings (CRITICAL - this triggers Universal Recalculation)
	log.Printf("🔍 DeleteAddressGroupsByIDs: Finding related AddressGroupBindings for cascading deletion")

	var bindingsToDelete []models.ResourceIdentifier
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		for _, agToDelete := range addressGroupsToDelete {
			if binding.AddressGroupRef.Name == agToDelete.SelfRef.Name &&
				binding.AddressGroupRef.Namespace == agToDelete.SelfRef.Namespace {
				bindingsToDelete = append(bindingsToDelete, binding.SelfRef.ResourceIdentifier)
				log.Printf("🔗 DeleteAddressGroupsByIDs: Found related binding %s → AddressGroup %s",
					binding.SelfRef.Key(), agToDelete.SelfRef.Key())
				break
			}
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to list AddressGroupBindings for cascading deletion")
	}

	// PHASE 3: Cascade delete bindings FIRST (triggers Universal Recalculation Engine via Service.AddressGroups changes)
	if len(bindingsToDelete) > 0 {
		log.Printf("🚨 CRITICAL: Cascade deleting %d AddressGroupBindings to trigger Universal Recalculation", len(bindingsToDelete))
		if err := s.DeleteAddressGroupBindingsByIDs(ctx, bindingsToDelete); err != nil {
			return errors.Wrap(err, "failed to cascade delete AddressGroupBindings")
		}
		log.Printf("✅ CASCADE_DELETE: Successfully deleted %d bindings, Universal Recalculation triggered", len(bindingsToDelete))
	} else {
		log.Printf("ℹ️ DeleteAddressGroupsByIDs: No related bindings found for cascade deletion")
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

	log.Printf("🗑️ DeleteAddressGroupsByIDs: Deleting %d AddressGroups from storage", len(ids))
	if err = writer.DeleteAddressGroupsByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete address groups from storage")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// PHASE 5: External sync - Delete AddressGroups from sgroups (CRITICAL FIX)
	log.Printf("🔄 EXTERNAL_SYNC: Syncing deletion of %d AddressGroups to external sgroups", len(addressGroupsToDelete))
	s.syncAddressGroupsWithSGroups(ctx, addressGroupsToDelete, types.SyncOperationDelete)
	log.Printf("✅ EXTERNAL_SYNC: Completed AddressGroup deletion sync to sgroups")

	// Close reader
	reader.Close()

	log.Printf("🎉 DeleteAddressGroupsByIDs: Successfully completed reference-compliant deletion of %d AddressGroups", len(addressGroupsToDelete))
	log.Printf("    ✅ Cascade deleted %d bindings (triggered Universal Recalculation)", len(bindingsToDelete))
	log.Printf("    ✅ Deleted %d AddressGroups from storage", len(addressGroupsToDelete))
	log.Printf("    ✅ Synced deletion to external sgroups")

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
	klog.Infof("🔄 CreateAddressGroupBinding: Creating associated AddressGroupPortMapping for %s/%s", binding.Namespace, binding.Name)
	if err := s.SyncAddressGroupPortMappings(ctx, binding); err != nil {
		klog.Errorf("Failed to create AddressGroupPortMapping for %s/%s: %v", binding.Namespace, binding.Name, err)
		// Don't fail the binding creation if port mapping creation fails
	}

	// Process conditions after successful commit
	klog.Infof("🔄 CreateAddressGroupBinding: Processing conditions for %s/%s, conditionManager=%v", binding.Namespace, binding.Name, s.conditionManager != nil)
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessAddressGroupBindingConditions(ctx, &binding); err != nil {
			klog.Errorf("Failed to process address group binding conditions for %s/%s: %v",
				binding.Namespace, binding.Name, err)
			// Don't fail the operation if condition processing fails
		}
	} else {
		klog.Warningf("⚠️ CreateAddressGroupBinding: conditionManager is nil, skipping condition processing for %s/%s", binding.Namespace, binding.Name)
	}

	// 🎯 ARCHITECTURAL FIX: Synchronize Service.AddressGroups field like reference controller
	serviceID := models.ResourceIdentifier{
		Name:      binding.ServiceRef.Name,
		Namespace: binding.ServiceRef.Namespace,
	}
	log.Printf("🔄 CreateAddressGroupBinding: Synchronizing Service.AddressGroups for service %s after binding creation", serviceID.Key())
	if err := s.synchronizeServiceAddressGroups(ctx, serviceID); err != nil {
		klog.Errorf("Failed to synchronize Service.AddressGroups for service %s after binding creation: %v", serviceID.Key(), err)
		// Don't fail the operation, but log the critical issue
	}
	if s.ruleS2SRegenerator != nil {
		log.Printf("🔔 CreateAddressGroupBinding: Notifying RuleS2S about Service.AddressGroups change for %s", serviceID.Key())
		if err := s.ruleS2SRegenerator.NotifyServiceAddressGroupsChanged(ctx, serviceID); err != nil {
			klog.Errorf("Failed to notify RuleS2S service about AddressGroupBinding %s: %v",
				binding.Key(), err)
			// Don't fail the operation, but log the issue
		} else {
			log.Printf("✅ CreateAddressGroupBinding: Successfully notified RuleS2S about binding %s", binding.Key())
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
			log.Printf("✅ CreateAddressGroupBinding: Successfully regenerated IEAgAg rules for new AddressGroupBinding %s", binding.Key())
		}
	} else {
		klog.Warningf("⚠️ CreateAddressGroupBinding: AddressGroupBinding %s created but no RuleS2S regenerator available", binding.Key())
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
			log.Printf("✅ UpdateAddressGroupBinding: Successfully regenerated IEAgAg rules for AddressGroupBinding %s", binding.Key())
		}
	} else {
		klog.Warningf("⚠️ UpdateAddressGroupBinding: AddressGroupBinding %s updated but no RuleS2S regenerator available", binding.Key())
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
	klog.Infof("🔄 SyncAddressGroupBindings: Updating AddressGroupPortMappings for %d bindings (syncOp=%s)", len(bindings), syncOp)
	for _, binding := range bindings {
		if err := s.SyncAddressGroupPortMappingsWithSyncOp(ctx, binding, syncOp); err != nil {
			klog.Errorf("Failed to update AddressGroupPortMapping for %s/%s: %v", binding.Namespace, binding.Name, err)
			// Don't fail the batch operation if port mapping update fails
		}
	}

	// Only process conditions for non-DELETE operations
	if syncOp != models.SyncOpDelete {
		// Process conditions after successful commit for each address group binding
		klog.Infof("🔄 SyncAddressGroupBindings: Processing conditions for %d bindings, conditionManager=%v", len(bindings), s.conditionManager != nil)
		if s.conditionManager != nil {
			for i := range bindings {
				klog.Infof("🔄 SyncAddressGroupBindings: Processing conditions for binding %s/%s", bindings[i].Namespace, bindings[i].Name)
				if err := s.conditionManager.ProcessAddressGroupBindingConditions(ctx, &bindings[i]); err != nil {
					klog.Errorf("Failed to process address group binding conditions for %s/%s: %v",
						bindings[i].Namespace, bindings[i].Name, err)
					// Don't fail the operation if condition processing fails
				}
			}
		} else {
			klog.Warningf("⚠️ SyncAddressGroupBindings: conditionManager is nil, skipping condition processing for %d bindings", len(bindings))
		}
	} else {
		klog.Infof("🗑️ SyncAddressGroupBindings: Skipping condition processing for DELETE operation (%d bindings) - but port mappings were updated", len(bindings))
	}

	// 🎯 ARCHITECTURAL FIX: Synchronize Service.AddressGroups field for all affected services
	// This matches the reference controller behavior for maintaining Service.AddressGroups integrity
	serviceIDs := make(map[string]models.ResourceIdentifier)
	for _, binding := range bindings {
		key := binding.ServiceRef.Namespace + "/" + binding.ServiceRef.Name
		serviceIDs[key] = models.ResourceIdentifier{Name: binding.ServiceRef.Name, Namespace: binding.ServiceRef.Namespace}
	}

	klog.Infof("🔄 SyncAddressGroupBindings: Synchronizing Service.AddressGroups for %d unique services (syncOp=%s)", len(serviceIDs), syncOp)
	for _, serviceID := range serviceIDs {
		log.Printf("🔄 SyncAddressGroupBindings: Synchronizing Service.AddressGroups for service %s after binding sync", serviceID.Key())
		if err := s.synchronizeServiceAddressGroups(ctx, serviceID); err != nil {
			klog.Errorf("Failed to synchronize Service.AddressGroups for service %s after binding sync: %v", serviceID.Key(), err)
			// Don't fail the operation, but log the critical issue
		}
	}

	// 🎯 NEW: Notify RuleS2S service about AddressGroup changes to enable reactive dependency chain
	// This is the key missing piece for API Aggregation Layer reactive flow
	// IMPORTANT: Include DELETE operations to trigger RuleS2S condition recalculation and IEAgAgRule cleanup
	klog.Infof("🔄 SyncAddressGroupBindings: Notifying RuleS2S regenerator for %d bindings (syncOp=%s)", len(bindings), syncOp)
	if s.ruleS2SRegenerator != nil {
		// Notify for each unique service (including DELETE operations for dependency cleanup)
		for key, serviceID := range serviceIDs {
			log.Printf("🔄 SyncAddressGroupBindings: Notifying RuleS2S regenerator for service %s (syncOp=%s)", key, syncOp)
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
	// 🗑️ COMPREHENSIVE DEBUG LOGGING FOR BINDING DELETION FLOW
	log.Printf("🗑️ BINDING_DELETION_START: Starting deletion of %d AddressGroupBindings", len(ids))
	for i, id := range ids {
		log.Printf("  📋 BINDING_TO_DELETE[%d]: %s", i, id.Key())
	}

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

		// 🔍 BINDING_DETAILS: Log comprehensive binding information
		log.Printf("  🔍 BINDING_DETAILS[%s]: Service %s/%s ↔ AddressGroup %s/%s",
			binding.Key(),
			binding.ServiceRef.Namespace, binding.ServiceRef.Name,
			binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name)

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
			log.Printf("  📦 SERVICE_PORTS_AFFECTED[%s]: Service %s has %d ports: [%s]",
				binding.Key(), service.Key(), len(service.IngressPorts), strings.Join(portStrs, ", "))
		} else {
			log.Printf("  ❌ SERVICE_LOOKUP_ERROR[%s]: Failed to get service details: %v", binding.Key(), serviceErr)
		}

		// Track the AddressGroup that will need port mapping regeneration
		agKey := fmt.Sprintf("%s/%s", binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name)
		affectedAddressGroups[agKey] = models.ResourceIdentifier{
			Name:      binding.AddressGroupRef.Name,
			Namespace: binding.AddressGroupRef.Namespace,
		}

		// 🎯 NEW: Track binding for Service.AddressGroups removal
		bindingsToRemove = append(bindingsToRemove, *binding)

		log.Printf("  ✅ BINDING_ANALYSIS[%s]: Will affect AddressGroup %s and trigger regeneration for Service %s/%s",
			binding.Key(), agKey, binding.ServiceRef.Namespace, binding.ServiceRef.Name)
	}

	// 🔧 SERIALIZATION_FIX: Use WriterForDeletes to reduce serialization conflicts during concurrent delete operations
	var writer ports.Writer
	if registryWithDeletes, ok := s.registry.(interface {
		WriterForDeletes(context.Context) (ports.Writer, error)
	}); ok {
		log.Printf("🔧 SERIALIZATION_FIX: Using WriterForDeletes with ReadCommitted isolation for %d bindings", len(ids))
		writer, err = registryWithDeletes.WriterForDeletes(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get delete writer with ReadCommitted isolation")
		}
	} else {
		log.Printf("🔧 SERIALIZATION_FIX: WriterForDeletes not available, using standard writer for %d bindings", len(ids))
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

	// 🔔 RULES2S_NOTIFICATION: Notify RuleS2S service about removed bindings for reactive dependency chain
	log.Printf("🔔 RULES2S_NOTIFICATION_START: Notifying RuleS2S regenerator for %d deleted bindings", len(bindingsToRemove))
	if s.ruleS2SRegenerator != nil {
		// Collect unique service IDs to avoid duplicate notifications
		serviceIDs := make(map[string]models.ResourceIdentifier)
		for _, binding := range bindingsToRemove {
			key := binding.ServiceRef.Namespace + "/" + binding.ServiceRef.Name
			serviceIDs[key] = models.ResourceIdentifier{Name: binding.ServiceRef.Name, Namespace: binding.ServiceRef.Namespace}
			log.Printf("  📝 SERVICE_TO_NOTIFY: Service %s (from binding %s)", key, binding.Key())
		}

		log.Printf("  🎯 UNIQUE_SERVICES_TO_NOTIFY: Found %d unique services to notify out of %d bindings", len(serviceIDs), len(bindingsToRemove))

		// 🎯 ARCHITECTURAL FIX: Synchronize Service.AddressGroups field for all affected services
		log.Printf("🔄 SYNC_SERVICE_ADDRESS_GROUPS_START: Synchronizing Service.AddressGroups for %d services after binding deletion", len(serviceIDs))
		for _, serviceID := range serviceIDs {
			log.Printf("🔄 DeleteAddressGroupBindingsByIDs: Synchronizing Service.AddressGroups for service %s after binding deletion", serviceID.Key())
			if err := s.synchronizeServiceAddressGroups(ctx, serviceID); err != nil {
				klog.Errorf("Failed to synchronize Service.AddressGroups for service %s after binding deletion: %v", serviceID.Key(), err)
				// Don't fail the operation, but log the critical issue
			}
		}

		// Notify for each unique service
		for key, serviceID := range serviceIDs {
			log.Printf("  🔔 NOTIFYING_SERVICE: Starting notification for service %s", key)
			if err := s.ruleS2SRegenerator.NotifyServiceAddressGroupsChanged(ctx, serviceID); err != nil {
				log.Printf("  ❌ NOTIFICATION_ERROR: Failed to notify RuleS2S regenerator for service %s: %v", key, err)
				// Don't fail the operation, but log the issue
			} else {
				log.Printf("  ✅ NOTIFICATION_SUCCESS: Successfully notified RuleS2S regenerator for service %s", key)
			}
		}
	} else {
		log.Printf("  ⚠️ NO_RULES2S_REGENERATOR: ruleS2SRegenerator is nil, cannot notify about binding changes!")
	}

	// After successful deletion, regenerate port mappings for affected AddressGroups
	// This will remove stale services that no longer have bindings
	log.Printf("🔄 DeleteAddressGroupBindingsByIDs: Regenerating port mappings for %d affected AddressGroups", len(affectedAddressGroups))
	for agKey, addressGroupRef := range affectedAddressGroups {
		log.Printf("🔄 DeleteAddressGroupBindingsByIDs: Regenerating port mapping for AddressGroup %s", agKey)

		// Get fresh reader after the deletion transaction
		freshReader, err := s.registry.Reader(ctx)
		if err != nil {
			log.Printf("⚠️ DeleteAddressGroupBindingsByIDs: Failed to get reader for port mapping regeneration: %v", err)
			continue // Don't fail the whole operation
		}
		defer freshReader.Close()

		// Generate the complete mapping with remaining bindings (deleted bindings will be excluded)
		addressGroupPortMapping, err := s.generateCompleteAddressGroupPortMapping(ctx, freshReader, addressGroupRef)
		if err != nil {
			log.Printf("⚠️ DeleteAddressGroupBindingsByIDs: Failed to regenerate port mapping for AddressGroup %s: %v", agKey, err)
			continue // Don't fail the whole operation
		}

		// Update the mapping in storage (or delete if no bindings remain)
		freshWriter, err := s.registry.Writer(ctx)
		if err != nil {
			log.Printf("⚠️ DeleteAddressGroupBindingsByIDs: Failed to get writer for port mapping update: %v", err)
			continue // Don't fail the whole operation
		}
		defer func() {
			if err != nil {
				freshWriter.Abort()
			}
		}()

		if addressGroupPortMapping == nil {
			// No bindings remain - port mapping should be empty/minimal
			log.Printf("🗑️ DeleteAddressGroupBindingsByIDs: No bindings remain for AddressGroup %s, creating empty port mapping", agKey)
			emptyMapping := &models.AddressGroupPortMapping{
				SelfRef: models.SelfRef{
					ResourceIdentifier: addressGroupRef,
				},
				AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
			}
			if err := s.syncAddressGroupPortMappings(ctx, freshWriter, []models.AddressGroupPortMapping{*emptyMapping}, models.SyncOpUpsert); err != nil {
				log.Printf("⚠️ DeleteAddressGroupBindingsByIDs: Failed to sync empty port mapping for AddressGroup %s: %v", agKey, err)
				freshWriter.Abort()
				continue
			}
		} else {
			// Some bindings remain - update with current services
			if err := s.syncAddressGroupPortMappings(ctx, freshWriter, []models.AddressGroupPortMapping{*addressGroupPortMapping}, models.SyncOpUpsert); err != nil {
				log.Printf("⚠️ DeleteAddressGroupBindingsByIDs: Failed to sync updated port mapping for AddressGroup %s: %v", agKey, err)
				freshWriter.Abort()
				continue
			}
		}

		if err := freshWriter.Commit(); err != nil {
			log.Printf("⚠️ DeleteAddressGroupBindingsByIDs: Failed to commit port mapping update for AddressGroup %s: %v", agKey, err)
			continue
		}

		log.Printf("✅ DeleteAddressGroupBindingsByIDs: Successfully regenerated port mapping for AddressGroup %s", agKey)
	}

	log.Printf("🎉 DeleteAddressGroupBindingsByIDs: Successfully deleted %d bindings and regenerated %d port mappings",
		len(ids), len(affectedAddressGroups))
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
		klog.V(4).Infof("🔄 SyncAddressGroupPortMappingsWithWriterAndReader: AddressGroupPortMapping %s/%s synced, conditions will be processed after commit",
			addressGroupPortMapping.Namespace, addressGroupPortMapping.Name)
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
			klog.Infof("🔄 SyncAddressGroupPortMappingsWithSyncOp: Processing conditions for AddressGroupPortMapping %s/%s after successful commit",
				addressGroupPortMapping.Namespace, addressGroupPortMapping.Name)

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
	log.Printf("🔄 regenerateCompletePortMappingForAddressGroup: Regenerating complete mapping for AddressGroup %s/%s", addressGroupNamespace, addressGroupName)

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
		log.Printf("🔄 regenerateCompletePortMappingForAddressGroup: Creating empty mapping for AddressGroup %s (no bindings remain)", addressGroupRef.Key())
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

	log.Printf("✅ regenerateCompletePortMappingForAddressGroup: Successfully updated port mapping for AddressGroup %s", addressGroupRef.Key())

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
	log.Printf("🔄 RegeneratePortMappingsForService: Starting regeneration for service %s (searching ALL namespaces for bindings)", serviceID.Key())

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
		log.Printf("🔄 RegeneratePortMappingsForService: Regenerating mapping for AddressGroup %s", agKey)

		// Generate the complete mapping with updated service ports
		addressGroupPortMapping, err := s.generateCompleteAddressGroupPortMapping(ctx, reader, addressGroupRef)
		if err != nil {
			return errors.Wrapf(err, "port conflict detected while regenerating mapping for AddressGroup %s", agKey)
		}
		if addressGroupPortMapping == nil {
			log.Printf("⚠️ RegeneratePortMappingsForService: No mapping generated for AddressGroup %s", agKey)
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
			log.Printf("🔄 RegeneratePortMappingsForService: Processing conditions for regenerated AddressGroupPortMapping %s", agKey)
			if err := s.conditionManager.ProcessAddressGroupPortMappingConditions(ctx, addressGroupPortMapping); err != nil {
				klog.Errorf("Failed to process conditions for regenerated AddressGroupPortMapping %s: %v", agKey, err)
				// Don't fail the operation if condition processing fails
			}
		}

		log.Printf("✅ RegeneratePortMappingsForService: Successfully regenerated mapping for AddressGroup %s", agKey)
	}

	log.Printf("🎉 RegeneratePortMappingsForService: Successfully regenerated %d AddressGroupPortMappings for service %s",
		len(addressGroupsToRegenerate), serviceID.Key())

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
		log.Printf("🔄 CRITICAL_FIX: Routing DELETE syncOp to service-level DeleteAddressGroupsByIDs for complete cascading deletion")
		return s.DeleteAddressGroupsByIDs(ctx, ids)
	}

	// Execute operation with specified option for non-deletion
	if err := writer.SyncAddressGroups(ctx, addressGroups, scope, ports.WithSyncOp(syncOp)); err != nil {
		log.Printf("❌ ERROR: syncAddressGroups - Failed to sync address groups to writer: %v", err)
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
			log.Printf("🔍 syncAddressGroupBindings: Skipping backend port conflict validation for single binding CREATE operation %s (avoiding circular dependency)", bindings[0].Key())
		} else {
			log.Printf("🔍 syncAddressGroupBindings: Performing backend port conflict validation for %d bindings (bulk operation)", len(bindings))

			reader, err := s.registry.ReaderFromWriter(ctx, writer)
			if err != nil {
				return errors.Wrap(err, "failed to get reader for port conflict validation")
			}
			defer reader.Close()

			// Validate each binding for port conflicts before allowing database sync
			for _, binding := range bindings {
				log.Printf("🔍 syncAddressGroupBindings: Validating port conflicts for binding %s before database sync", binding.Key())

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
						log.Printf("⚠️ syncAddressGroupBindings: Failed to get existing service %s, skipping conflict check: %v", existingServiceID.Key(), err)
						continue
					}

					// Check for port conflicts between services
					if err := s.checkPortConflictsBetweenServices(service, existingService); err != nil {
						return errors.Wrapf(err, "cannot create binding %s: port conflict with existing binding %s", binding.Key(), existingBinding.Key())
					}
				}

				log.Printf("✅ syncAddressGroupBindings: Port conflict validation passed for binding %s", binding.Key())
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
	log.Printf("🔍 checkPortConflictsBetweenServices: Checking conflicts between %s and %s", service1.Key(), service2.Key())

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
			log.Printf("⚠️ checkPortConflictsBetweenServices: Failed to parse port %s for service %s: %v", ingressPort.Port, service1.Name, err)
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
			log.Printf("⚠️ checkPortConflictsBetweenServices: Failed to parse port %s for service %s: %v", ingressPort.Port, service2.Name, err)
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

	log.Printf("✅ checkPortConflictsBetweenServices: No conflicts found between %s and %s", service1.Key(), service2.Key())
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
	klog.Infof("🔄 syncAddressGroupsWithSGroups: Starting efficient batch sync for %d AddressGroups, syncManager=%v", len(addressGroups), s.syncManager != nil)

	if s.syncManager == nil {
		klog.Warningf("⚠️ syncAddressGroupsWithSGroups: syncManager is nil, skipping external sync")
		return
	}

	if len(addressGroups) == 0 {
		klog.Infof("ℹ️ syncAddressGroupsWithSGroups: No AddressGroups to sync")
		return
	}

	// Convert addressGroups to SyncableEntity slice for batch sync
	var syncableEntities []interfaces.SyncableEntity
	var unsyncableKeys []string

	for _, addressGroup := range addressGroups {
		// Create a copy to avoid pointer issues
		agCopy := addressGroup
		klog.V(4).Infof("🔍 syncAddressGroupsWithSGroups: Checking if AddressGroup %s implements SyncableEntity", addressGroup.Key())
		if syncableEntity, ok := interface{}(&agCopy).(interfaces.SyncableEntity); ok {
			syncableEntities = append(syncableEntities, syncableEntity)
		} else {
			unsyncableKeys = append(unsyncableKeys, addressGroup.Key())
		}
	}

	// Log any unsyncable address groups
	if len(unsyncableKeys) > 0 {
		klog.Warningf("⚠️ syncAddressGroupsWithSGroups: Skipping sync for %d non-syncable AddressGroups: %v", len(unsyncableKeys), unsyncableKeys)
	}

	// Perform batch sync for all syncable address groups
	if len(syncableEntities) > 0 {
		klog.Infof("🚀 syncAddressGroupsWithSGroups: Batch syncing %d AddressGroups to sgroups with operation %s", len(syncableEntities), operation)
		if err := s.syncManager.SyncBatch(ctx, syncableEntities, operation); err != nil {
			klog.Errorf("❌ syncAddressGroupsWithSGroups: Warning - failed to batch sync %d AddressGroups to sgroups: %v", len(syncableEntities), err)
			// Don't fail the whole operation if sgroups sync fails
		} else {
			klog.Infof("✅ syncAddressGroupsWithSGroups: Successfully batch synced %d AddressGroups to SGROUPS", len(syncableEntities))
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
		log.Printf("⚠️ generateAddressGroupPortMapping (LEGACY): Port conflict detected, returning nil: %v", err)
		return nil // Legacy function can't return error, so return nil for port conflicts
	}
	return portMapping
}

// generateCompleteAddressGroupPortMapping generates port mapping for ALL services bound to an AddressGroup
// Returns error if port conflicts are detected to prevent binding creation
func (s *AddressGroupResourceService) generateCompleteAddressGroupPortMapping(ctx context.Context, reader ports.Reader, addressGroupRef models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	log.Printf("🔄 generateCompleteAddressGroupPortMapping: Generating complete port mapping for AddressGroup %s", addressGroupRef.Key())

	// Find all bindings for this AddressGroup
	var bindings []models.AddressGroupBinding
	err := reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		// Check if this binding targets our AddressGroup
		if binding.AddressGroupRef.Name == addressGroupRef.Name && binding.AddressGroupRef.Namespace == addressGroupRef.Namespace {
			bindings = append(bindings, binding)
			log.Printf("  📎 Found binding: %s (service: %s/%s → addressgroup: %s/%s)",
				binding.Name, binding.ServiceRef.Namespace, binding.ServiceRef.Name,
				binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name)
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
		log.Printf("  ✅ Added service %s/%s with %d ingress ports to mapping",
			service.Namespace, service.Name, len(service.IngressPorts))
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
		klog.Infof("✅ generateCompleteAddressGroupPortMapping: Port validation passed for AddressGroup %s",
			addressGroupRef.Key())
	} else {
		klog.Warningf("⚠️ generateCompleteAddressGroupPortMapping: ValidationService not available, skipping port conflict validation")
	}

	log.Printf("🎉 generateCompleteAddressGroupPortMapping: Successfully generated mapping for AddressGroup %s with %d services",
		addressGroupRef.Key(), len(addressGroupPortMapping.AccessPorts))
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
	log.Printf("🔄 SYNC_SERVICE_START: Synchronizing Service.AddressGroups for %s (reference architecture pattern)", serviceID.Key())

	// Step 1: Get current service
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for service sync")
	}
	defer reader.Close()

	service, err := reader.GetServiceByID(ctx, serviceID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			log.Printf("⚠️ SYNC_SERVICE_SKIP: Service %s not found (may have been deleted)", serviceID.Key())
			return nil // Service was deleted, nothing to sync
		}
		return errors.Wrapf(err, "failed to get service %s for AddressGroups sync", serviceID.Key())
	}

	log.Printf("🔍 SYNC_SERVICE_CURRENT: Service %s currently has %d AddressGroups loaded",
		serviceID.Key(), len(service.AddressGroups))
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
		log.Printf("🔧 SERIALIZATION_FIX: Using WriterForConditions with ReadCommitted isolation for service %s sync", serviceID.Key())
		writer, err = registryWithConditions.WriterForConditions(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get condition writer with ReadCommitted isolation for service sync")
		}
	} else {
		log.Printf("🔧 SERIALIZATION_FIX: WriterForConditions not available, using standard writer for service %s sync", serviceID.Key())
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

	log.Printf("✅ SYNC_SERVICE_COMPLETE: Successfully synchronized Service.AddressGroups for %s (%d AddressGroups)",
		serviceID.Key(), len(service.AddressGroups))

	return nil
}
