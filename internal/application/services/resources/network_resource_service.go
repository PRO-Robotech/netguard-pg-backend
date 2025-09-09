package resources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/application/utils"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// NetworkConditionManagerInterface provides condition processing for networks
type NetworkConditionManagerInterface interface {
	ProcessNetworkConditions(ctx context.Context, network *models.Network, syncResult error) error
}

// NetworkResourceService provides business logic for Network resources
type NetworkResourceService struct {
	repo             ports.Registry
	syncTracker      *utils.SyncTracker
	retryConfig      utils.RetryConfig
	syncManager      interfaces.SyncManager
	conditionManager NetworkConditionManagerInterface
}

// NewNetworkResourceService creates a new NetworkResourceService
func NewNetworkResourceService(
	repo ports.Registry,
	syncManager interfaces.SyncManager,
	conditionManager NetworkConditionManagerInterface,
) *NetworkResourceService {
	return &NetworkResourceService{
		repo:             repo,
		syncTracker:      utils.NewSyncTracker(1 * time.Second),
		retryConfig:      utils.DefaultRetryConfig(),
		syncManager:      syncManager,
		conditionManager: conditionManager,
	}
}

// CreateNetwork creates a new Network with business logic validation
func (s *NetworkResourceService) CreateNetwork(ctx context.Context, network *models.Network) error {
	// Validate CIDR format
	if err := s.validateCIDR(network.CIDR); err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}

	// Check if Network already exists
	existing, err := s.getNetworkByID(ctx, network.Key())
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("failed to check existing network: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("network already exists: %s", network.Key())
	}

	// Initialize metadata
	network.GetMeta().TouchOnCreate()

	// Create the network
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Convert to slice for sync
	networks := []models.Network{*network}
	if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync networks: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network creation: %w", err)
	}

	// Sync with external systems
	syncErr := s.syncNetworkWithExternal(ctx, network, types.SyncOperationUpsert)

	// Process conditions after sync (so sync result can be included in conditions)
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessNetworkConditions(ctx, network, syncErr); err != nil {
			klog.Errorf("Failed to process network conditions for %s/%s: %v",
				network.Namespace, network.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	return syncErr
}

// UpdateNetwork updates an existing Network with business logic validation
func (s *NetworkResourceService) UpdateNetwork(ctx context.Context, network *models.Network) error {
	// Validate CIDR format
	if err := s.validateCIDR(network.CIDR); err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}

	// Check if Network exists
	existing, err := s.getNetworkByID(ctx, network.GetID())
	if err != nil {
		return fmt.Errorf("failed to get existing network: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("network not found: %s", network.GetID())
	}

	// Check if Network is bound and prevent certain changes
	if existing.IsBound {
		// Prevent changing CIDR when bound
		if existing.CIDR != network.CIDR {
			return fmt.Errorf("cannot change CIDR when network is bound")
		}
	}

	// Update metadata
	network.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Update the network
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Convert to slice for sync
	networks := []models.Network{*network}
	if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync networks: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network update: %w", err)
	}

	// Sync with external systems
	syncErr := s.syncNetworkWithExternal(ctx, network, types.SyncOperationUpsert)

	// Process conditions after sync (so sync result can be included in conditions)
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessNetworkConditions(ctx, network, syncErr); err != nil {
			klog.Errorf("Failed to process network conditions for %s/%s: %v",
				network.Namespace, network.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	return syncErr
}

// DeleteNetwork deletes a Network with cleanup logic
func (s *NetworkResourceService) DeleteNetwork(ctx context.Context, id models.ResourceIdentifier) error {
	log.Printf("🔥 DEBUG: NetworkResourceService.DeleteNetwork called for %s", id.Key())

	// Check if Network exists
	log.Printf("🔍 DEBUG: Checking if Network %s exists", id.Key())
	existing, err := s.getNetworkByID(ctx, id.Key())
	log.Printf("🔍 DEBUG: getNetworkByID returned: existing=%v, err=%v", existing != nil, err)
	if existing != nil {
		log.Printf("🔍 DEBUG: Found Network %s: IsBound=%v, CIDR=%s", id.Key(), existing.IsBound, existing.CIDR)
	}
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		log.Printf("❌ DEBUG: Failed to get network %s: %v", id.Key(), err)
		return fmt.Errorf("failed to get network: %w", err)
	}
	if existing == nil || errors.Is(err, ports.ErrNotFound) {
		// Network doesn't exist - delete is idempotent, so this is success
		log.Printf("ℹ️ DEBUG: Network %s not found (existing=%v, err=%v), treating as success (idempotent delete)", id.Key(), existing != nil, err)
		return nil
	}

	log.Printf("✅ DEBUG: Found Network %s for deletion", id.Key())

	// Check if Network is bound and handle cleanup
	if existing.IsBound {
		// Capture AddressGroupRef before clearing it for cleanup
		var addressGroupRef models.ResourceIdentifier
		if existing.AddressGroupRef != nil {
			addressGroupRef = models.ResourceIdentifier{
				Name:      existing.AddressGroupRef.Name,
				Namespace: existing.Namespace, // AddressGroup is in same namespace as Network
			}
			log.Printf("🔍 DEBUG: Captured AddressGroupRef for cleanup: %s", addressGroupRef.Key())
		}

		// Remove Network from AddressGroup.Networks before deleting
		if existing.AddressGroupRef != nil {
			networkRef := models.ResourceIdentifier{Name: existing.Name, Namespace: existing.Namespace}
			log.Printf("🔧 DEBUG: Removing Network %s from AddressGroup %s", networkRef.Key(), addressGroupRef.Key())
			if err := s.removeNetworkFromAddressGroup(ctx, addressGroupRef, networkRef); err != nil {
				log.Printf("❌ DEBUG: Failed to remove network from AddressGroup: %v", err)
				return fmt.Errorf("failed to remove network from address group: %w", err)
			}
			log.Printf("✅ DEBUG: Successfully removed Network from AddressGroup")
		}

		// Clear binding references
		existing.BindingRef = nil
		existing.AddressGroupRef = nil
		existing.IsBound = false

		// Update the network to clear bindings
		writer, err := s.repo.Writer(ctx)
		if err != nil {
			return fmt.Errorf("failed to get writer: %w", err)
		}
		defer writer.Abort()

		networks := []models.Network{*existing}
		if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
			return fmt.Errorf("failed to sync network cleanup: %w", err)
		}

		if err := writer.Commit(); err != nil {
			return fmt.Errorf("failed to commit network cleanup: %w", err)
		}
	}

	// Delete the network
	log.Printf("🗑️ DEBUG: Starting Network deletion from storage for %s", id.Key())
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		log.Printf("❌ DEBUG: Failed to get writer for Network %s: %v", id.Key(), err)
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	log.Printf("🔥 DEBUG: Calling writer.DeleteNetworksByIDs for %s", id.Key())
	if err := writer.DeleteNetworksByIDs(ctx, []models.ResourceIdentifier{id}); err != nil {
		log.Printf("❌ DEBUG: writer.DeleteNetworksByIDs failed for %s: %v", id.Key(), err)
		return fmt.Errorf("failed to delete network: %w", err)
	}

	log.Printf("💾 DEBUG: Committing Network deletion for %s", id.Key())
	if err := writer.Commit(); err != nil {
		log.Printf("❌ DEBUG: Failed to commit Network deletion for %s: %v", id.Key(), err)
		return fmt.Errorf("failed to commit network deletion: %w", err)
	}

	log.Printf("✅ DEBUG: Network %s successfully deleted from storage", id.Key())

	// Sync deletion with external systems
	log.Printf("🔗 DEBUG: Starting external sync for Network %s deletion", id.Key())
	err = s.syncNetworkWithExternal(ctx, existing, types.SyncOperationDelete)
	if err != nil {
		log.Printf("❌ DEBUG: External sync failed for Network %s: %v", id.Key(), err)
		return err
	}

	log.Printf("🎉 DEBUG: Network %s deletion completed successfully (storage + external sync)", id.Key())
	return nil
}

// GetNetwork retrieves a Network by ID
func (s *NetworkResourceService) GetNetwork(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	log.Printf("🚀 NetworkService.GetNetwork: request for %s", id.Key())
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		log.Printf("❌ NetworkService.GetNetwork: failed to get reader for %s: %v", id.Key(), err)
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	log.Printf("🔍 NetworkService.GetNetwork: reader type: %T", reader)
	network, err := reader.GetNetworkByID(ctx, id)
	if err != nil {
		log.Printf("❌ NetworkService.GetNetwork: reader.GetNetworkByID failed for %s: %v", id.Key(), err)
		return nil, fmt.Errorf("failed to get network: %w", err)
	}

	if network != nil {
		log.Printf("🔍 NetworkService.GetNetwork: Network[%s] returned with IsBound=%t", id.Key(), network.IsBound)
		if network.BindingRef != nil {
			log.Printf("  🔍 NetworkService.GetNetwork: network[%s].BindingRef=%s", id.Key(), network.BindingRef.Name)
		} else {
			log.Printf("  🔍 NetworkService.GetNetwork: network[%s].BindingRef=nil", id.Key())
		}
	}

	return network, nil
}

// GetAddressGroup retrieves an AddressGroup by ID
func (s *NetworkResourceService) GetAddressGroup(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	addressGroup, err := reader.GetAddressGroupByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get address group: %w", err)
	}

	return addressGroup, nil
}

// UpdateAddressGroup updates an AddressGroup
func (s *NetworkResourceService) UpdateAddressGroup(ctx context.Context, addressGroup *models.AddressGroup) error {
	log.Printf("🚀 UpdateAddressGroup: Starting update for %s with %d networks", addressGroup.Key(), len(addressGroup.Networks))
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Convert to slice for sync
	addressGroups := []models.AddressGroup{*addressGroup}
	if err := writer.SyncAddressGroups(ctx, addressGroups, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync address groups: %w", err)
	}

	log.Printf("🔧 UpdateAddressGroup: About to commit transaction for %s", addressGroup.Key())
	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit address group update: %w", err)
	}
	log.Printf("✅ UpdateAddressGroup: Transaction committed successfully for %s", addressGroup.Key())

	// REMOVE DUPLICATE SGROUPS SYNC - this will be handled by the calling function
	log.Printf("ℹ️  UpdateAddressGroup: Skipping sgroups sync (will be handled by caller)")
	return nil
}

// ListNetworks retrieves all Networks
func (s *NetworkResourceService) ListNetworks(ctx context.Context, scope ports.Scope) ([]models.Network, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	var networks []models.Network
	err = reader.ListNetworks(ctx, func(network models.Network) error {
		networks = append(networks, network)
		return nil
	}, scope)

	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	return networks, nil
}

// ValidateNetworkBinding validates that a NetworkBinding can be created for this Network
func (s *NetworkResourceService) ValidateNetworkBinding(ctx context.Context, networkID models.ResourceIdentifier, bindingID models.ResourceIdentifier) error {
	log.Printf("🔍 ValidateNetworkBinding called: networkID=%s, bindingID=%s", networkID.Key(), bindingID.Key())

	// Check if Network exists
	network, err := s.getNetworkByID(ctx, networkID.Key())
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", networkID.Key())
	}

	// Check if Network is already bound to a different binding
	if network.IsBound {
		// If bound to the same binding, that's valid
		if network.BindingRef != nil {
			// BindingRef.Name now contains only the name part, so compare with binding name
			expectedName := bindingID.Name
			actualName := network.BindingRef.Name
			log.Printf("🔍 NetworkBinding validation (regular): comparing expectedName='%s' with actualName='%s'", expectedName, actualName)

			if actualName == expectedName {
				log.Printf("✅ NetworkBinding validation (regular): Network is bound to the same binding - VALID")
				return nil // Network is bound to the same binding - valid
			}
			log.Printf("❌ NetworkBinding validation (regular): Network is bound to DIFFERENT binding - expectedName='%s' vs actualName='%s'", expectedName, actualName)
		}
		return fmt.Errorf("network is already bound to another binding (expected: %s, actual: %s)", bindingID.Name, network.BindingRef.Name)
	}

	log.Printf("✅ NetworkBinding validation (regular): Network is not bound - VALID")
	return nil
}

// ValidateNetworkBindingWithReader validates that a NetworkBinding can be created using provided reader (same session)
func (s *NetworkResourceService) ValidateNetworkBindingWithReader(ctx context.Context, reader ports.Reader, networkID models.ResourceIdentifier, bindingID models.ResourceIdentifier) error {
	// Check if Network exists using provided reader
	network, err := reader.GetNetworkByID(ctx, networkID)
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", networkID.Key())
	}

	// Check if Network is already bound to a different binding
	if network.IsBound {
		// If bound to the same binding, that's valid
		if network.BindingRef != nil {
			// BindingRef.Name now contains only the name part, so compare with binding name
			expectedName := bindingID.Name
			actualName := network.BindingRef.Name
			log.Printf("🔍 NetworkBinding validation: comparing expectedName='%s' with actualName='%s'", expectedName, actualName)

			if actualName == expectedName {
				log.Printf("✅ NetworkBinding validation: Network is bound to the same binding - VALID")
				return nil // Network is bound to the same binding - valid
			}
			log.Printf("❌ NetworkBinding validation: Network is bound to DIFFERENT binding - expectedName='%s' vs actualName='%s'", expectedName, actualName)
		}
		return fmt.Errorf("network is already bound to another binding (expected: %s, actual: %s)", bindingID.Name, network.BindingRef.Name)
	}

	log.Printf("✅ NetworkBinding validation: Network is not bound - VALID")
	return nil
}

// UpdateNetworkBinding updates Network status when a binding is created
func (s *NetworkResourceService) UpdateNetworkBinding(ctx context.Context, networkID models.ResourceIdentifier, bindingID models.ResourceIdentifier, addressGroupID models.ResourceIdentifier) error {
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Get reader from writer to ensure same session/transaction visibility
	reader, err := s.repo.ReaderFromWriter(ctx, writer)
	if err != nil {
		return fmt.Errorf("failed to get reader from writer: %w", err)
	}
	defer reader.Close()

	// Get the network using the same session reader
	network, err := reader.GetNetworkByID(ctx, networkID)
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", networkID.Key())
	}

	// Update binding references
	network.BindingRef = &v1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "NetworkBinding",
		Name:       bindingID.Name, // Store only the name part for repository consistency
	}
	network.AddressGroupRef = &v1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "AddressGroup",
		Name:       addressGroupID.Name,
	}
	network.IsBound = true

	// Update metadata
	network.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Set success condition
	utils.SetSyncSuccessCondition(network)

	// Sync the updated network
	networks := []models.Network{*network}
	if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync network binding: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network binding: %w", err)
	}

	// Sync with SGROUP
	if s.syncManager != nil {
		log.Printf("🔄 Syncing Network %s with SGROUP after binding update", network.Key())
		if syncErr := s.syncManager.SyncEntity(ctx, network, types.SyncOperationUpsert); syncErr != nil {
			log.Printf("❌ Failed to sync Network %s with SGROUP: %v", network.Key(), syncErr)
			// Don't fail the operation, sync can be retried later
		} else {
			log.Printf("✅ Successfully synced Network %s with SGROUP", network.Key())
		}
	}

	return nil
}

// RemoveNetworkBinding removes binding references from Network
func (s *NetworkResourceService) RemoveNetworkBinding(ctx context.Context, networkID models.ResourceIdentifier) error {
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Get reader from writer to ensure same session/transaction visibility
	reader, err := s.repo.ReaderFromWriter(ctx, writer)
	if err != nil {
		return fmt.Errorf("failed to get reader from writer: %w", err)
	}
	defer reader.Close()

	// Get the network using the same session reader
	network, err := reader.GetNetworkByID(ctx, networkID)
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", networkID.Key())
	}

	// Clear binding references
	network.BindingRef = nil
	network.AddressGroupRef = nil
	network.IsBound = false

	// Update metadata
	network.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Set success condition
	utils.SetSyncSuccessCondition(network)

	// Sync the updated network
	networks := []models.Network{*network}
	if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync network unbinding: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network unbinding: %w", err)
	}

	// Sync with SGROUP
	if s.syncManager != nil {
		log.Printf("🔄 Syncing Network %s with SGROUP after binding removal", network.Key())
		if syncErr := s.syncManager.SyncEntity(ctx, network, types.SyncOperationUpsert); syncErr != nil {
			log.Printf("❌ Failed to sync Network %s with SGROUP: %v", network.Key(), syncErr)
			// Don't fail the operation, sync can be retried later
		} else {
			log.Printf("✅ Successfully synced Network %s with SGROUP", network.Key())
		}
	}

	return nil
}

// Helper methods

// removeNetworkFromAddressGroup removes a network from AddressGroup.Networks field
func (s *NetworkResourceService) removeNetworkFromAddressGroup(ctx context.Context, addressGroupRef, networkRef models.ResourceIdentifier) error {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	// Get the AddressGroup
	addressGroup, err := reader.GetAddressGroupByID(ctx, addressGroupRef)
	if err != nil {
		return fmt.Errorf("failed to get address group: %w", err)
	}
	if addressGroup == nil {
		log.Printf("ℹ️  AddressGroup %s not found, skipping network removal", addressGroupRef.Key())
		return nil // AddressGroup doesn't exist, nothing to clean up
	}

	// Generate network name (namespace/name format) to match the pattern used in NetworkBinding
	networkName := fmt.Sprintf("%s/%s", networkRef.Namespace, networkRef.Name)

	// Remove network from AddressGroup.Networks.Items
	log.Printf("🔗 Removing network %s from AddressGroup %s", networkName, addressGroupRef.Key())

	var updatedNetworks []models.NetworkItem
	found := false
	for _, existingNetwork := range addressGroup.Networks {
		if existingNetwork.Name != networkName {
			updatedNetworks = append(updatedNetworks, existingNetwork)
		} else {
			found = true
		}
	}

	if found {
		addressGroup.Networks = updatedNetworks
		log.Printf("✅ Removed network %s from AddressGroup %s", networkName, addressGroupRef.Key())

		// Update metadata
		addressGroup.Meta.TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

		// Sync the updated AddressGroup (commits to database)
		log.Printf("🔧 removeNetworkFromAddressGroup: About to call UpdateAddressGroup for %s", addressGroupRef.Key())
		if err := s.UpdateAddressGroup(ctx, addressGroup); err != nil {
			return fmt.Errorf("failed to update address group: %w", err)
		}
		log.Printf("✅ removeNetworkFromAddressGroup: UpdateAddressGroup completed for %s", addressGroupRef.Key())
	} else {
		log.Printf("ℹ️  Network %s not found in AddressGroup %s", networkName, addressGroupRef.Key())
	}

	log.Printf("✅ removeNetworkFromAddressGroup: AddressGroup Networks field updated successfully")
	return nil
}

func (s *NetworkResourceService) getNetworkByID(ctx context.Context, id string) (*models.Network, error) {
	log.Printf("🔍 DEBUG: getNetworkByID called with id=%s", id)
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		log.Printf("❌ DEBUG: getNetworkByID failed to get reader: %v", err)
		return nil, err
	}
	defer reader.Close()

	// Parse namespace/name from id (format: "namespace/name")
	parts := strings.Split(id, "/")
	var resourceID models.ResourceIdentifier
	if len(parts) == 2 {
		resourceID = models.ResourceIdentifier{Namespace: parts[0], Name: parts[1]}
		log.Printf("🔍 DEBUG: getNetworkByID parsed resourceID: namespace=%s, name=%s", parts[0], parts[1])
	} else {
		resourceID = models.ResourceIdentifier{Name: id}
		log.Printf("🔍 DEBUG: getNetworkByID using single name: %s", id)
	}

	network, err := reader.GetNetworkByID(ctx, resourceID)
	log.Printf("🔍 DEBUG: reader.GetNetworkByID returned: network=%v, err=%v", network != nil, err)
	return network, err
}

func (s *NetworkResourceService) validateCIDR(cidr string) error {
	// Basic CIDR validation - in a real implementation, you'd use a proper IP/CIDR library
	if cidr == "" {
		return fmt.Errorf("CIDR cannot be empty")
	}

	// Simple format check
	if !strings.Contains(cidr, "/") {
		return fmt.Errorf("invalid CIDR format: %s", cidr)
	}

	return nil
}

// syncNetworkWithExternal syncs a Network with external systems
func (s *NetworkResourceService) syncNetworkWithExternal(ctx context.Context, network *models.Network, operation types.SyncOperation) error {
	syncKey := fmt.Sprintf("%s-%s", operation, network.Key())

	// Check debouncing
	if !s.syncTracker.ShouldSync(syncKey) {
		return nil // Skip sync due to debouncing
	}

	// Execute sync with retry
	err := utils.ExecuteWithRetry(ctx, s.retryConfig, func() error {
		// Sync Network with SGROUP
		if s.syncManager != nil {
			log.Printf("🔄 Syncing Network %s with SGROUP (operation: %s)", network.Key(), operation)
			if syncErr := s.syncManager.SyncEntity(ctx, network, operation); syncErr != nil {
				log.Printf("❌ Failed to sync Network %s with SGROUP: %v", network.Key(), syncErr)
				return syncErr
			}
			log.Printf("✅ Successfully synced Network %s with SGROUP (operation: %s)", network.Key(), operation)
		}
		return nil
	})

	if err != nil {
		s.syncTracker.RecordFailure(syncKey, err)
		utils.SetSyncFailedCondition(network, err)
		return fmt.Errorf("failed to sync with external system: %w", err)
	}

	s.syncTracker.RecordSuccess(syncKey)
	utils.SetSyncSuccessCondition(network)
	return nil
}

// syncAddressGroupWithExternal syncs an AddressGroup with external systems
func (s *NetworkResourceService) syncAddressGroupWithExternal(ctx context.Context, addressGroup *models.AddressGroup, operation string) error {
	syncKey := fmt.Sprintf("%s-%s", operation, addressGroup.Key())

	// Check debouncing
	if !s.syncTracker.ShouldSync(syncKey) {
		return nil // Skip sync due to debouncing
	}

	// Execute sync with retry
	err := utils.ExecuteWithRetry(ctx, s.retryConfig, func() error {
		// Sync AddressGroup with SGROUP
		if s.syncManager != nil {
			log.Printf("🔄 Syncing AddressGroup %s with SGROUP", addressGroup.Key())
			if syncErr := s.syncManager.SyncEntity(ctx, addressGroup, types.SyncOperationUpsert); syncErr != nil {
				log.Printf("❌ Failed to sync AddressGroup %s with SGROUP: %v", addressGroup.Key(), syncErr)
				return syncErr
			}
			log.Printf("✅ Successfully synced AddressGroup %s with SGROUP", addressGroup.Key())
		}
		return nil
	})

	if err != nil {
		s.syncTracker.RecordFailure(syncKey, err)
		utils.SetSyncFailedCondition(addressGroup, err)
		return fmt.Errorf("failed to sync with external system: %w", err)
	}

	s.syncTracker.RecordSuccess(syncKey)
	utils.SetSyncSuccessCondition(addressGroup)
	return nil
}
