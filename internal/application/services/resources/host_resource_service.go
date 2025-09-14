package resources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"netguard-pg-backend/internal/application/utils"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// HostConditionManagerInterface provides condition processing for hosts
type HostConditionManagerInterface interface {
	ProcessHostConditions(ctx context.Context, host *models.Host, syncResult error) error
}

// HostResourceService provides business logic for Host resources
type HostResourceService struct {
	repo             ports.Registry
	syncTracker      *utils.SyncTracker
	retryConfig      utils.RetryConfig
	syncManager      interfaces.SyncManager
	conditionManager HostConditionManagerInterface
}

// NewHostResourceService creates a new HostResourceService
func NewHostResourceService(
	repo ports.Registry,
	syncManager interfaces.SyncManager,
	conditionManager HostConditionManagerInterface,
) *HostResourceService {
	return &HostResourceService{
		repo:             repo,
		syncTracker:      utils.NewSyncTracker(1 * time.Second),
		retryConfig:      utils.DefaultRetryConfig(),
		syncManager:      syncManager,
		conditionManager: conditionManager,
	}
}

// CreateHost creates a new Host with business logic validation
func (s *HostResourceService) CreateHost(ctx context.Context, host *models.Host) error {
	// Validate host
	if err := s.validateHost(host); err != nil {
		return fmt.Errorf("invalid host: %w", err)
	}

	// Check if Host already exists
	existing, err := s.getHostByID(ctx, host.Key())
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("failed to check existing host: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("host already exists: %s", host.Key())
	}

	// Initialize metadata
	host.GetMeta().TouchOnCreate()

	// Create the host
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Convert to slice for sync
	hosts := []models.Host{*host}
	if err := writer.SyncHosts(ctx, hosts, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync hosts: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit host creation: %w", err)
	}

	// Sync with external systems
	syncErr := s.syncHostWithExternal(ctx, host, types.SyncOperationUpsert)
	if syncErr != nil {
		log.Printf("❌ Failed to sync Host %s with external systems: %v", host.Key(), syncErr)
		// Continue with condition processing even if sync fails
	} else {
		log.Printf("✅ Successfully synced Host %s with external systems", host.Key())
	}

	// Process conditions after sync (so sync result can be included in conditions)
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessHostConditions(ctx, host, syncErr); err != nil {
			log.Printf("⚠️ DEBUG: Failed to process conditions for host %s: %v", host.Key(), err)
			// Don't fail the creation due to condition processing errors
		}
	}

	return syncErr // Return sync error if it occurred
}

// UpdateHost updates an existing Host
func (s *HostResourceService) UpdateHost(ctx context.Context, host *models.Host) error {
	// Validate host
	if err := s.validateHost(host); err != nil {
		return fmt.Errorf("invalid host: %w", err)
	}

	// Update metadata
	host.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Update the host
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Convert to slice for sync
	hosts := []models.Host{*host}
	if err := writer.SyncHosts(ctx, hosts, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync hosts: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit host update: %w", err)
	}

	// Sync with external systems
	syncErr := s.syncHostWithExternal(ctx, host, types.SyncOperationUpsert)
	if syncErr != nil {
		log.Printf("❌ Failed to sync Host %s with external systems: %v", host.Key(), syncErr)
		// Continue with condition processing even if sync fails
	} else {
		log.Printf("✅ Successfully synced Host %s with external systems", host.Key())
	}

	// Process conditions after sync
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessHostConditions(ctx, host, syncErr); err != nil {
			log.Printf("⚠️ DEBUG: Failed to process conditions for host %s: %v", host.Key(), err)
		}
	}

	return syncErr // Return sync error if it occurred
}

// DeleteHost deletes a Host by resource identifier with cascading deletion of HostBinding
func (s *HostResourceService) DeleteHost(ctx context.Context, id models.ResourceIdentifier) error {
	log.Printf("🔥 DEBUG: HostResourceService.DeleteHost called for %s", id.Key())

	// Check if Host exists
	log.Printf("🔍 DEBUG: Checking if Host %s exists", id.Key())
	existing, err := s.getHostByID(ctx, id.Key())
	log.Printf("🔍 DEBUG: getHostByID returned: existing=%v, err=%v", existing != nil, err)
	if existing != nil {
		log.Printf("🔍 DEBUG: Found Host %s: IsBound=%v", id.Key(), existing.IsBound)
	}
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		log.Printf("❌ DEBUG: Failed to get host %s: %v", id.Key(), err)
		return fmt.Errorf("failed to get host: %w", err)
	}
	if existing == nil || errors.Is(err, ports.ErrNotFound) {
		// Host doesn't exist - delete is idempotent, so this is success
		log.Printf("ℹ️ DEBUG: Host %s not found (existing=%v, err=%v), treating as success (idempotent delete)", id.Key(), existing != nil, err)
		return nil
	}

	log.Printf("✅ DEBUG: Found Host %s for deletion", id.Key())

	// Check if there's a HostBinding that needs to be deleted first
	log.Printf("🔍 DEBUG: Looking for HostBinding bound to Host %s", id.Key())
	hostBinding, err := s.findHostBindingByHostID(ctx, id)
	var hostBindingToDelete *models.HostBinding
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		log.Printf("❌ DEBUG: Failed to search for HostBinding for Host %s: %v", id.Key(), err)
		return fmt.Errorf("failed to search for host binding: %w", err)
	}
	if err == nil && hostBinding != nil {
		log.Printf("🚨 DEBUG: Found HostBinding %s bound to Host %s - will delete it first", hostBinding.Key(), id.Key())
		hostBindingToDelete = hostBinding
	} else {
		log.Printf("ℹ️ DEBUG: No HostBinding found for Host %s, proceeding with host deletion only", id.Key())
	}

	// Start transaction for cascading deletion
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		log.Printf("❌ DEBUG: Failed to get writer for Host %s: %v", id.Key(), err)
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// If there's a HostBinding to delete, delete it first
	if hostBindingToDelete != nil {
		log.Printf("🗑️ DEBUG: Deleting HostBinding %s before deleting Host %s", hostBindingToDelete.Key(), id.Key())
		hostBindingID := models.NewResourceIdentifier(hostBindingToDelete.Name, models.WithNamespace(hostBindingToDelete.Namespace))
		if err := writer.DeleteHostBindingsByIDs(ctx, []models.ResourceIdentifier{hostBindingID}); err != nil {
			log.Printf("❌ DEBUG: writer.DeleteHostBindingsByIDs failed for %s: %v", hostBindingToDelete.Key(), err)
			return fmt.Errorf("failed to delete host binding %s: %w", hostBindingToDelete.Key(), err)
		}
		log.Printf("✅ DEBUG: HostBinding %s successfully deleted from storage", hostBindingToDelete.Key())
	}

	// Delete the host
	log.Printf("🗑️ DEBUG: Deleting Host %s", id.Key())
	if err := writer.DeleteHostsByIDs(ctx, []models.ResourceIdentifier{id}); err != nil {
		log.Printf("❌ DEBUG: writer.DeleteHostsByIDs failed for %s: %v", id.Key(), err)
		return fmt.Errorf("failed to delete host: %w", err)
	}

	log.Printf("💾 DEBUG: Committing cascading deletion (HostBinding + Host) for %s", id.Key())
	if err := writer.Commit(); err != nil {
		log.Printf("❌ DEBUG: Failed to commit cascading deletion for %s: %v", id.Key(), err)
		return fmt.Errorf("failed to commit cascading deletion: %w", err)
	}

	log.Printf("✅ DEBUG: Host %s (and associated HostBinding) successfully deleted from storage", id.Key())

	// Sync deletions with external systems - HostBinding first, then Host
	if hostBindingToDelete != nil {
		log.Printf("🔗 DEBUG: Syncing HostBinding %s deletion with external systems", hostBindingToDelete.Key())
		err = s.syncHostBindingWithExternal(ctx, hostBindingToDelete, types.SyncOperationDelete)
		if err != nil {
			log.Printf("❌ DEBUG: External sync failed for HostBinding %s: %v", hostBindingToDelete.Key(), err)
			return fmt.Errorf("failed to sync host binding deletion: %w", err)
		}
		log.Printf("✅ DEBUG: HostBinding %s deletion synced successfully", hostBindingToDelete.Key())
	}

	log.Printf("🔗 DEBUG: Syncing Host %s deletion with external systems", id.Key())
	err = s.syncHostWithExternal(ctx, existing, types.SyncOperationDelete)
	if err != nil {
		log.Printf("❌ DEBUG: External sync failed for Host %s: %v", id.Key(), err)
		return fmt.Errorf("failed to sync host deletion: %w", err)
	}

	log.Printf("🎉 DEBUG: Host %s cascading deletion completed successfully (storage + external sync)", id.Key())
	return nil
}

// GetHost retrieves a Host by resource identifier
func (s *HostResourceService) GetHost(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error) {
	log.Printf("🔥 DEBUG: HostResourceService.GetHost called for %s", id.Key())
	return s.getHostByID(ctx, id.Key())
}

// ListHosts retrieves all Hosts within a scope
func (s *HostResourceService) ListHosts(ctx context.Context, scope ports.Scope) ([]models.Host, error) {
	log.Printf("🔥 DEBUG: HostResourceService.ListHosts called with scope %v", scope)

	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	var hosts []models.Host
	err = reader.ListHosts(ctx, func(host models.Host) error {
		hosts = append(hosts, host)
		return nil
	}, scope)
	if err != nil {
		log.Printf("❌ DEBUG: Failed to list hosts from registry: %v", err)
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}

	log.Printf("✅ DEBUG: Listed %d hosts successfully", len(hosts))
	return hosts, nil
}

// SyncHosts synchronizes multiple hosts with the specified operation
func (s *HostResourceService) SyncHosts(ctx context.Context, hosts []models.Host, scope ports.Scope, syncOp models.SyncOp) error {
	log.Printf("🔥 DEBUG: HostResourceService.SyncHosts called with %d hosts, syncOp=%v", len(hosts), syncOp)

	switch syncOp {
	case models.SyncOpFullSync:
		return s.fullSyncHosts(ctx, hosts, scope)
	case models.SyncOpUpsert:
		return s.upsertHosts(ctx, hosts)
	case models.SyncOpDelete:
		return s.deleteHosts(ctx, hosts)
	default:
		return fmt.Errorf("unsupported sync operation: %v", syncOp)
	}
}

// fullSyncHosts performs a full synchronization of hosts
func (s *HostResourceService) fullSyncHosts(ctx context.Context, hosts []models.Host, scope ports.Scope) error {
	log.Printf("🔥 DEBUG: Starting full sync of %d hosts", len(hosts))

	// Get current hosts from registry
	existingHosts, err := s.ListHosts(ctx, scope)
	if err != nil {
		return fmt.Errorf("failed to get existing hosts: %w", err)
	}

	// Create maps for efficient lookup
	incomingHosts := make(map[string]models.Host)
	for _, host := range hosts {
		incomingHosts[host.Key()] = host
	}

	existingHostsMap := make(map[string]models.Host)
	for _, host := range existingHosts {
		existingHostsMap[host.Key()] = host
	}

	// Process incoming hosts (create or update)
	for _, host := range hosts {
		if _, exists := existingHostsMap[host.Key()]; exists {
			if err := s.UpdateHost(ctx, &host); err != nil {
				log.Printf("❌ DEBUG: Failed to update host %s: %v", host.Key(), err)
				return fmt.Errorf("failed to update host %s: %w", host.Key(), err)
			}
		} else {
			if err := s.CreateHost(ctx, &host); err != nil {
				log.Printf("❌ DEBUG: Failed to create host %s: %v", host.Key(), err)
				return fmt.Errorf("failed to create host %s: %w", host.Key(), err)
			}
		}
	}

	// Delete hosts that are no longer in the incoming set
	for _, existingHost := range existingHosts {
		if _, stillExists := incomingHosts[existingHost.Key()]; !stillExists {
			if err := s.DeleteHost(ctx, existingHost.SelfRef.ResourceIdentifier); err != nil {
				log.Printf("❌ DEBUG: Failed to delete host %s: %v", existingHost.Key(), err)
				return fmt.Errorf("failed to delete host %s: %w", existingHost.Key(), err)
			}
		}
	}

	log.Printf("✅ DEBUG: Full sync of hosts completed successfully")
	return nil
}

// upsertHosts creates or updates multiple hosts
func (s *HostResourceService) upsertHosts(ctx context.Context, hosts []models.Host) error {
	log.Printf("🔥 DEBUG: Upserting %d hosts", len(hosts))

	for _, host := range hosts {
		// Check if host already exists
		existing, err := s.getHostByID(ctx, host.Key())
		if err != nil && !errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("failed to check existing host %s: %w", host.Key(), err)
		}

		if existing != nil {
			// Update existing host
			if err := s.UpdateHost(ctx, &host); err != nil {
				return fmt.Errorf("failed to update host %s: %w", host.Key(), err)
			}
		} else {
			// Create new host
			if err := s.CreateHost(ctx, &host); err != nil {
				return fmt.Errorf("failed to create host %s: %w", host.Key(), err)
			}
		}
	}

	log.Printf("✅ DEBUG: Upserted %d hosts successfully", len(hosts))
	return nil
}

// deleteHosts deletes multiple hosts
func (s *HostResourceService) deleteHosts(ctx context.Context, hosts []models.Host) error {
	log.Printf("🔥 DEBUG: Deleting %d hosts", len(hosts))

	for _, host := range hosts {
		if err := s.DeleteHost(ctx, host.SelfRef.ResourceIdentifier); err != nil {
			return fmt.Errorf("failed to delete host %s: %w", host.Key(), err)
		}
	}

	log.Printf("✅ DEBUG: Deleted %d hosts successfully", len(hosts))
	return nil
}

// validateHost performs business logic validation on a host
func (s *HostResourceService) validateHost(host *models.Host) error {
	if host == nil {
		return errors.New("host cannot be nil")
	}

	// Validate resource identifier
	if err := s.validateResourceIdentifier(host.SelfRef.ResourceIdentifier); err != nil {
		return fmt.Errorf("invalid resource identifier: %w", err)
	}

	// Host validation - hostname field removed, no additional validation needed

	// Additional business logic validation can be added here

	return nil
}

// validateResourceIdentifier validates a resource identifier
func (s *HostResourceService) validateResourceIdentifier(id models.ResourceIdentifier) error {
	if id.Name == "" {
		return errors.New("resource name cannot be empty")
	}

	if id.Namespace == "" {
		return errors.New("resource namespace cannot be empty")
	}

	return nil
}

// SyncStatusUpdate handles sync status updates for hosts
func (s *HostResourceService) SyncStatusUpdate(ctx context.Context, resourceType string, status interface{}) error {
	if resourceType != "Host" {
		return nil
	}

	log.Printf("🔥 DEBUG: HostResourceService received sync status update: %v", status)

	// TODO: Implement sync status update when interface is clarified
	log.Printf("⚠️ DEBUG: Sync status update not yet implemented for hosts")

	return nil
}

// findHostBindingByHostID finds a HostBinding that binds the specified Host
func (s *HostResourceService) findHostBindingByHostID(ctx context.Context, hostID models.ResourceIdentifier) (*models.HostBinding, error) {
	log.Printf("🔍 DEBUG: findHostBindingByHostID called for host %s", hostID.Key())

	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	// Use ListHostBindings to find the binding for this host
	var foundBinding *models.HostBinding
	err = reader.ListHostBindings(ctx, func(hostBinding models.HostBinding) error {
		// Check if this binding binds our target host
		if hostBinding.HostRef.Namespace == hostID.Namespace && hostBinding.HostRef.Name == hostID.Name {
			log.Printf("✅ DEBUG: Found HostBinding %s for host %s", hostBinding.Key(), hostID.Key())
			foundBinding = &hostBinding
			return nil // Found it, continue to collect (though there should only be one due to UNIQUE constraint)
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		log.Printf("❌ DEBUG: Failed to list host bindings: %v", err)
		return nil, fmt.Errorf("failed to list host bindings: %w", err)
	}

	if foundBinding == nil {
		log.Printf("ℹ️ DEBUG: No HostBinding found for host %s", hostID.Key())
		return nil, ports.ErrNotFound
	}

	log.Printf("✅ DEBUG: Successfully found HostBinding %s for host %s", foundBinding.Key(), hostID.Key())
	return foundBinding, nil
}

// syncHostBindingWithExternal synchronizes a HostBinding with external systems
func (s *HostResourceService) syncHostBindingWithExternal(ctx context.Context, hostBinding *models.HostBinding, operation types.SyncOperation) error {
	log.Printf("🔗 DEBUG: syncHostBindingWithExternal called for HostBinding %s, operation=%v", hostBinding.Key(), operation)

	if s.syncManager == nil {
		log.Printf("⚠️ DEBUG: No sync manager available for HostBinding %s", hostBinding.Key())
		return nil
	}

	// Use SyncManager to synchronize with external systems
	err := s.syncManager.SyncEntity(ctx, hostBinding, operation)
	if err != nil {
		log.Printf("❌ DEBUG: Failed to sync HostBinding %s with external systems: %v", hostBinding.Key(), err)
		return err
	}

	log.Printf("✅ DEBUG: Successfully synced HostBinding %s with external systems", hostBinding.Key())
	return nil
}

// getHostByID retrieves a host by its key using Reader pattern
func (s *HostResourceService) getHostByID(ctx context.Context, id string) (*models.Host, error) {
	log.Printf("🔍 DEBUG: getHostByID called with id=%s", id)
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		log.Printf("❌ DEBUG: getHostByID failed to get reader: %v", err)
		return nil, err
	}
	defer reader.Close()

	// Parse namespace/name from id (format: "namespace/name")
	parts := strings.Split(id, "/")
	var resourceID models.ResourceIdentifier
	if len(parts) == 2 {
		resourceID = models.ResourceIdentifier{Namespace: parts[0], Name: parts[1]}
		log.Printf("🔍 DEBUG: getHostByID parsed resourceID: namespace=%s, name=%s", parts[0], parts[1])
	} else {
		resourceID = models.ResourceIdentifier{Name: id}
		log.Printf("🔍 DEBUG: getHostByID using single name: %s", id)
	}

	host, err := reader.GetHostByID(ctx, resourceID)
	log.Printf("🔍 DEBUG: reader.GetHostByID returned: host=%v, err=%v", host != nil, err)
	return host, err
}

// syncHostWithExternal syncs a Host with external systems
func (s *HostResourceService) syncHostWithExternal(ctx context.Context, host *models.Host, operation types.SyncOperation) error {
	syncKey := fmt.Sprintf("%s-%s", operation, host.Key())

	// Check debouncing
	if !s.syncTracker.ShouldSync(syncKey) {
		return nil // Skip sync due to debouncing
	}

	// Execute sync with retry
	err := utils.ExecuteWithRetry(ctx, s.retryConfig, func() error {
		// Sync Host with SGROUP
		if s.syncManager != nil {
			log.Printf("🔄 Syncing Host %s with SGROUP (operation: %s)", host.Key(), operation)
			if syncErr := s.syncManager.SyncEntity(ctx, host, operation); syncErr != nil {
				log.Printf("❌ Failed to sync Host %s with SGROUP: %v", host.Key(), syncErr)
				return syncErr
			}
			log.Printf("✅ Successfully synced Host %s with SGROUP (operation: %s)", host.Key(), operation)
		}
		return nil
	})

	if err != nil {
		s.syncTracker.RecordFailure(syncKey, err)
		utils.SetSyncFailedCondition(host, err)
		return fmt.Errorf("failed to sync with external system: %w", err)
	}

	s.syncTracker.RecordSuccess(syncKey)
	utils.SetSyncSuccessCondition(host)
	return nil
}

// UpdateHostBinding updates Host status when a binding is created
func (s *HostResourceService) UpdateHostBinding(ctx context.Context, hostID models.ResourceIdentifier, bindingID models.ResourceIdentifier, addressGroupID models.ResourceIdentifier) error {
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

	// Get the host using the same session reader
	host, err := reader.GetHostByID(ctx, hostID)
	if err != nil {
		return fmt.Errorf("failed to get host: %w", err)
	}
	if host == nil {
		return fmt.Errorf("host not found: %s", hostID.Key())
	}

	// Update binding references
	host.BindingRef = &v1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "HostBinding",
		Name:       bindingID.Name, // Store only the name part for repository consistency
	}
	host.AddressGroupRef = &v1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "AddressGroup",
		Name:       addressGroupID.Name,
	}
	host.IsBound = true
	host.AddressGroupName = addressGroupID.Name

	// Update metadata
	host.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Set success condition
	utils.SetSyncSuccessCondition(host)

	// Sync the updated host
	hosts := []models.Host{*host}
	if err := writer.SyncHosts(ctx, hosts, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync host binding: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit host binding: %w", err)
	}

	// Sync with SGROUP
	if s.syncManager != nil {
		log.Printf("🔄 Syncing Host %s with SGROUP after binding update", host.Key())
		if syncErr := s.syncManager.SyncEntity(ctx, host, types.SyncOperationUpsert); syncErr != nil {
			log.Printf("❌ Failed to sync Host %s with SGROUP: %v", host.Key(), syncErr)
			// Don't fail the operation, sync can be retried later
		} else {
			log.Printf("✅ Successfully synced Host %s with SGROUP", host.Key())
		}
	}

	return nil
}

// SyncHostWithExternal syncs a Host with external systems (public wrapper)
func (s *HostResourceService) SyncHostWithExternal(ctx context.Context, host *models.Host, operation types.SyncOperation) error {
	return s.syncHostWithExternal(ctx, host, operation)
}

// UpdateHostBindingStatus updates Host.isBound status based on AddressGroup hosts changes
func (s *HostResourceService) UpdateHostBindingStatus(ctx context.Context, oldAG, newAG *models.AddressGroup) error {
	log.Printf("🔄 UpdateHostBindingStatus called for AddressGroup changes")

	// Get lists of hosts from old and new AddressGroups
	var oldHosts, newHosts []v1beta1.ObjectReference

	if oldAG != nil {
		oldHosts = oldAG.Hosts
		log.Printf("🔍 Old AddressGroup %s has %d hosts", oldAG.Key(), len(oldHosts))
	}
	if newAG != nil {
		newHosts = newAG.Hosts
		log.Printf("🔍 New AddressGroup %s has %d hosts", newAG.Key(), len(newHosts))
	}

	// Convert to maps for easier comparison
	oldHostsMap := make(map[string]bool)
	for _, host := range oldHosts {
		oldHostsMap[host.Name] = true
	}

	newHostsMap := make(map[string]bool)
	for _, host := range newHosts {
		newHostsMap[host.Name] = true
	}

	// Get namespace (from newAG or oldAG)
	namespace := ""
	addressGroupName := ""
	if newAG != nil {
		namespace = newAG.Namespace
		addressGroupName = newAG.Name
	} else if oldAG != nil {
		namespace = oldAG.Namespace
	}

	// Update hosts that were removed (set isBound = false)
	for hostName := range oldHostsMap {
		if !newHostsMap[hostName] {
			log.Printf("🔓 Unbinding host %s/%s from AddressGroup", namespace, hostName)
			if err := s.updateHostBindingStatusForHost(ctx, hostName, namespace, false, ""); err != nil {
				log.Printf("❌ Failed to unbind host %s/%s: %v", namespace, hostName, err)
			}
		}
	}

	// Update hosts that were added (set isBound = true)
	for hostName := range newHostsMap {
		if !oldHostsMap[hostName] {
			log.Printf("🔒 Binding host %s/%s to AddressGroup %s", namespace, hostName, addressGroupName)
			if err := s.updateHostBindingStatusForHost(ctx, hostName, namespace, true, addressGroupName); err != nil {
				log.Printf("❌ Failed to bind host %s/%s to AddressGroup %s: %v", namespace, hostName, addressGroupName, err)
			}
		}
	}

	return nil
}

// updateHostBindingStatusForHost updates a specific Host's binding status
func (s *HostResourceService) updateHostBindingStatusForHost(ctx context.Context, hostName, namespace string, isBound bool, addressGroupName string) error {
	hostID := models.ResourceIdentifier{
		Name:      hostName,
		Namespace: namespace,
	}

	// Get the Host
	host, err := s.getHostByID(ctx, hostID.Key())
	if err != nil {
		return fmt.Errorf("failed to get host %s/%s: %w", namespace, hostName, err)
	}

	// Update Host status
	host.IsBound = isBound
	if isBound {
		host.AddressGroupName = addressGroupName
		host.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       addressGroupName,
		}
	} else {
		host.AddressGroupName = ""
		host.AddressGroupRef = nil
		host.BindingRef = nil
	}

	// Update the Host in registry
	if err := s.UpdateHost(ctx, host); err != nil {
		return fmt.Errorf("failed to update host status: %w", err)
	}

	log.Printf("✅ Successfully updated Host %s/%s: isBound=%v, addressGroupName=%s",
		namespace, hostName, isBound, addressGroupName)

	return nil
}
