package resources

import (
	"context"
	"errors"
	"fmt"
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
		// Continue with condition processing even if sync fails
	}

	// Process conditions after sync (so sync result can be included in conditions)
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessHostConditions(ctx, host, syncErr); err != nil {
		}

		// Update the host status with conditions in the database
		if updateErr := s.updateHostStatus(ctx, host); updateErr != nil {
		}
	}

	if syncErr != nil {
		return fmt.Errorf("host created but SGROUP sync failed: %w", syncErr)
	}

	return nil
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
		// Continue with condition processing even if sync fails
	} else {
	}

	// Process conditions after sync
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessHostConditions(ctx, host, syncErr); err != nil {
		}

		if updateErr := s.updateHostStatus(ctx, host); updateErr != nil {
		}
	}

	if syncErr != nil {
	}

	return nil
}

// DeleteHost deletes a Host by resource identifier with cascading deletion of HostBinding
func (s *HostResourceService) DeleteHost(ctx context.Context, id models.ResourceIdentifier) error {

	// Check if Host exists
	existing, err := s.getHostByID(ctx, id.Key())
	if existing != nil {
	}
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("failed to get host: %w", err)
	}
	if existing == nil || errors.Is(err, ports.ErrNotFound) {
		return nil
	}

	if existing.IsBound && existing.AddressGroupRef != nil && existing.BindingRef == nil {

		reader, err := s.repo.Reader(ctx)
		if err != nil {
			return fmt.Errorf("failed to get reader: %w", err)
		}
		defer reader.Close()

		agID := models.ResourceIdentifier{
			Name:      existing.AddressGroupRef.Name,
			Namespace: existing.Namespace,
		}

		ag, err := reader.GetAddressGroupByID(ctx, agID)
		if err != nil && !errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("failed to get address group %s: %w", agID.Key(), err)
		}

		if ag != nil {
			// Remove host from AddressGroup.spec.hosts
			var updatedHosts []v1beta1.ObjectReference
			for _, hostRef := range ag.Hosts {
				if hostRef.Name != existing.Name {
					updatedHosts = append(updatedHosts, hostRef)
				}
			}

			if len(updatedHosts) != len(ag.Hosts) {
				ag.Hosts = updatedHosts

				writer, err := s.repo.Writer(ctx)
				if err != nil {
					return fmt.Errorf("failed to get writer for AddressGroup update: %w", err)
				}
				defer writer.Abort()

				ags := []models.AddressGroup{*ag}
				if err := writer.SyncAddressGroups(ctx, ags, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
					return fmt.Errorf("failed to update address group: %w", err)
				}

				if err := writer.Commit(); err != nil {
					return fmt.Errorf("failed to commit address group update: %w", err)
				}
			}
		}

		// Refresh host to get updated binding status
		existing, err = s.getHostByID(ctx, id.Key())
		if err != nil {
			return fmt.Errorf("failed to refresh host after unbinding: %w", err)
		}
	}

	hostBinding, err := s.findHostBindingByHostID(ctx, id)
	var hostBindingToDelete *models.HostBinding
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("failed to search for host binding: %w", err)
	}
	if err == nil && hostBinding != nil {
		hostBindingToDelete = hostBinding
	}

	// Start transaction for cascading deletion
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// If there's a HostBinding to delete, delete it first
	var addressGroupToSync *models.AddressGroup
	if hostBindingToDelete != nil {
		agID := models.ResourceIdentifier{
			Name:      hostBindingToDelete.AddressGroupRef.Name,
			Namespace: hostBindingToDelete.Namespace, // HostBinding is in same namespace as AddressGroup
		}

		reader, err := s.repo.ReaderFromWriter(ctx, writer)
		if err != nil {
			return fmt.Errorf("failed to get reader from writer: %w", err)
		}
		defer reader.Close()

		if ag, err := reader.GetAddressGroupByID(ctx, agID); err == nil && ag != nil {
			addressGroupToSync = ag
		}

		hostBindingID := models.NewResourceIdentifier(hostBindingToDelete.Name, models.WithNamespace(hostBindingToDelete.Namespace))
		if err := writer.DeleteHostBindingsByIDs(ctx, []models.ResourceIdentifier{hostBindingID}); err != nil {
			return fmt.Errorf("failed to delete host binding %s: %w", hostBindingToDelete.Key(), err)
		}
	}

	if err := writer.DeleteHostsByIDs(ctx, []models.ResourceIdentifier{id}); err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit cascading deletion: %w", err)
	}

	if addressGroupToSync != nil && s.syncManager != nil {
		reader, err := s.repo.Reader(ctx)
		if err == nil {
			if updatedAG, err := reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{
				Name:      addressGroupToSync.Name,
				Namespace: addressGroupToSync.Namespace,
			}); err == nil && updatedAG != nil {
				if syncErr := s.syncManager.SyncEntityForced(ctx, updatedAG, types.SyncOperationUpsert); syncErr != nil {
				}
			}
			reader.Close()
		}
	}

	err = s.syncHostWithExternal(ctx, existing, types.SyncOperationDelete)
	if err != nil {
	}
	return nil
}

// GetHost retrieves a Host by resource identifier
func (s *HostResourceService) GetHost(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error) {
	return s.getHostByID(ctx, id.Key())
}

// ListHosts retrieves all Hosts within a scope
func (s *HostResourceService) ListHosts(ctx context.Context, scope ports.Scope) ([]models.Host, error) {

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
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}

	return hosts, nil
}

// SyncHosts synchronizes multiple hosts with the specified operation
func (s *HostResourceService) SyncHosts(ctx context.Context, hosts []models.Host, scope ports.Scope, syncOp models.SyncOp) error {
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
				return fmt.Errorf("failed to update host %s: %w", host.Key(), err)
			}
		} else {
			if err := s.CreateHost(ctx, &host); err != nil {
				return fmt.Errorf("failed to create host %s: %w", host.Key(), err)
			}
		}
	}

	// Delete hosts that are no longer in the incoming set
	for _, existingHost := range existingHosts {
		if _, stillExists := incomingHosts[existingHost.Key()]; !stillExists {
			if err := s.DeleteHost(ctx, existingHost.SelfRef.ResourceIdentifier); err != nil {
				return fmt.Errorf("failed to delete host %s: %w", existingHost.Key(), err)
			}
		}
	}

	return nil
}

// upsertHosts creates or updates multiple hosts
func (s *HostResourceService) upsertHosts(ctx context.Context, hosts []models.Host) error {

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

	return nil
}

// deleteHosts deletes multiple hosts
func (s *HostResourceService) deleteHosts(ctx context.Context, hosts []models.Host) error {

	for _, host := range hosts {
		if err := s.DeleteHost(ctx, host.SelfRef.ResourceIdentifier); err != nil {
			return fmt.Errorf("failed to delete host %s: %w", host.Key(), err)
		}
	}

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

	return nil
}

// findHostBindingByHostID finds a HostBinding that binds the specified Host
func (s *HostResourceService) findHostBindingByHostID(ctx context.Context, hostID models.ResourceIdentifier) (*models.HostBinding, error) {

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
			foundBinding = &hostBinding
			return nil
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return nil, fmt.Errorf("failed to list host bindings: %w", err)
	}

	if foundBinding == nil {
		return nil, ports.ErrNotFound
	}

	return foundBinding, nil
}

// HostBinding is a NetGuard-only resource, no external sync needed
// syncHostBindingWithExternal is removed - HostBinding doesn't sync with external systems

// getHostByID retrieves a host by its key using Reader pattern
func (s *HostResourceService) getHostByID(ctx context.Context, id string) (*models.Host, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Parse namespace/name from id (format: "namespace/name")
	parts := strings.Split(id, "/")
	var resourceID models.ResourceIdentifier
	if len(parts) == 2 {
		resourceID = models.ResourceIdentifier{Namespace: parts[0], Name: parts[1]}
	} else {
		resourceID = models.ResourceIdentifier{Name: id}
	}

	host, err := reader.GetHostByID(ctx, resourceID)
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
			if syncErr := s.syncManager.SyncEntity(ctx, host, operation); syncErr != nil {
				return syncErr
			}
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

	// Check if this is a binding operation (not unbinding)
	isBinding := bindingID.Name != "" && addressGroupID.Name != ""

	// CRITICAL: Only allow binding if host is ready (synchronized with SGROUP)
	if isBinding && !utils.IsReadyConditionTrue(host) {
		return fmt.Errorf("host %s is not ready for binding - must be synchronized with SGROUP first (Ready condition must be True)", hostID.Key())
	}

	if bindingID.Name == "" && addressGroupID.Name == "" {
		host.BindingRef = nil
		host.AddressGroupRef = nil
		host.IsBound = false
		host.AddressGroupName = ""
	} else {
		// Binding case - set all binding references
		host.BindingRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "HostBinding",
			Name:       bindingID.Name,
		}
		host.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       addressGroupID.Name,
		}
		host.IsBound = true
		host.AddressGroupName = addressGroupID.Name
	}

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
		if syncErr := s.syncManager.SyncEntity(ctx, host, types.SyncOperationUpsert); syncErr != nil {
			// Don't fail the operation, sync can be retried later
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

	// Get lists of hosts from old and new AddressGroups
	var oldHosts, newHosts []v1beta1.ObjectReference

	if oldAG != nil {
		oldHosts = oldAG.Hosts
	}
	if newAG != nil {
		newHosts = newAG.Hosts
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
			if err := s.updateHostBindingStatusForHost(ctx, hostName, namespace, false, ""); err != nil {
			}
		}
	}

	// Update hosts that were added (set isBound = true)
	for hostName := range newHostsMap {
		if !oldHostsMap[hostName] {
			if err := s.updateHostBindingStatusForHost(ctx, hostName, namespace, true, addressGroupName); err != nil {
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

	// CRITICAL: Only allow binding via AG.spec if host is ready (synchronized with SGROUP)
	if isBound && !utils.IsReadyConditionTrue(host) {
		return fmt.Errorf("host %s/%s is not ready for binding via AddressGroup.spec - must be synchronized with SGROUP first (Ready condition must be True)", namespace, hostName)
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

	if s.syncManager != nil {
		if syncErr := s.syncManager.SyncEntityForced(ctx, host, types.SyncOperationUpsert); syncErr != nil {
		}
	}

	return nil
}

// updateHostStatus updates only the host status/conditions in the database without triggering sync
func (s *HostResourceService) updateHostStatus(ctx context.Context, host *models.Host) error {
	host.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Update only the status in the database
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Convert to slice for sync - this only updates status, no external sync
	hosts := []models.Host{*host}
	if err := writer.SyncHosts(ctx, hosts, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync host status: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit host status update: %w", err)
	}

	return nil
}
