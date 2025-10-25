package resources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/application/utils"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/interfaces"
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

	// CLOUD-233: Removed syncHostWithExternal() - SGROUP sync now handled by OutboxWorker
	// Migration 026 triggers create outbox entries, OutboxWorker processes them asynchronously

	// ConditionManager needs the ACTUAL database state, not the in-memory object
	// Writer may have set Ready=False (PendingSGROUPSync), which must be preserved
	if s.conditionManager != nil {
		// Re-read the host from database to get Writer-applied conditions
		reader, err := s.repo.Reader(ctx)
		if err != nil {
			klog.Errorf("Failed to get reader for condition processing %s/%s: %v",
				host.Namespace, host.Name, err)
			return nil // Don't fail the operation
		}
		defer reader.Close()

		freshHost, err := reader.GetHostByID(ctx, models.ResourceIdentifier{
			Namespace: host.Namespace,
			Name:      host.Name,
		})
		if err != nil {
			klog.Errorf("Failed to re-read host for condition processing %s/%s: %v",
				host.Namespace, host.Name, err)
			return nil // Don't fail the operation
		}

		// Process conditions with the FRESH host from database
		if err := s.conditionManager.ProcessHostConditions(ctx, freshHost, nil); err != nil {
			klog.Errorf("Failed to process host conditions for %s/%s: %v",
				host.Namespace, host.Name, err)
			// Don't fail the operation if condition processing fails
		}
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

	// CLOUD-233: Removed syncHostWithExternal() - SGROUP sync now handled by OutboxWorker
	// Migration 026 triggers create outbox entries, OutboxWorker processes them asynchronously

	// ConditionManager needs the ACTUAL database state, not the in-memory object
	// Writer may have set Ready=False (PendingSGROUPSync), which must be preserved
	if s.conditionManager != nil {
		// Re-read the host from database to get Writer-applied conditions
		reader, err := s.repo.Reader(ctx)
		if err != nil {
			klog.Errorf("Failed to get reader for condition processing %s/%s: %v",
				host.Namespace, host.Name, err)
			return nil // Don't fail the operation
		}
		defer reader.Close()

		freshHost, err := reader.GetHostByID(ctx, models.ResourceIdentifier{
			Namespace: host.Namespace,
			Name:      host.Name,
		})
		if err != nil {
			klog.Errorf("Failed to re-read host for condition processing %s/%s: %v",
				host.Namespace, host.Name, err)
			return nil // Don't fail the operation
		}

		// Process conditions with the FRESH host from database
		if err := s.conditionManager.ProcessHostConditions(ctx, freshHost, nil); err != nil {
			klog.Errorf("Failed to process host conditions for %s/%s: %v",
				host.Namespace, host.Name, err)
			// Don't fail the operation if condition processing fails
		}
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

	// CLOUD-233: Removed AddressGroup sync preparation - no longer needed with OutboxWorker
	// HostBinding deletion and AG updates now handled by database triggers + OutboxWorker
	_ = hostBindingToDelete // Keep variable to avoid unused warning

	if err := writer.DeleteHostsByIDs(ctx, []models.ResourceIdentifier{id}); err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit cascading deletion: %w", err)
	}

	// CLOUD-233: Removed syncManager.SyncEntityForced() for AddressGroup
	// Migration 028 AddressGroup triggers handle aggregated_hosts updates
	// OutboxWorker processes AddressGroup sync asynchronously

	// CLOUD-233: Removed syncHostWithExternal() for DELETE
	// Migration 032 DELETE triggers create outbox entries
	// OutboxWorker processes Host deletion sync asynchronously

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
	// Get writer from registry
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Call writer.SyncHosts directly with the hosts and syncOp
	if err = writer.SyncHosts(ctx, hosts, scope, ports.WithSyncOp(syncOp)); err != nil {
		return fmt.Errorf("failed to sync hosts: %w", err)
	}

	// Commit transaction
	if err = writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// CLOUD-233: Removed syncHostWithExternal() for both UPSERT and DELETE
	// Migration 026 (UPSERT) and 032 (DELETE) triggers create outbox entries
	// OutboxWorker processes them asynchronously with retry/backoff

	// ConditionManager needs the ACTUAL database state, not the in-memory objects
	// Writer may have set Ready=False (PendingSGROUPSync), which must be preserved
	if s.conditionManager != nil && syncOp != models.SyncOpDelete {
		reader, err := s.repo.Reader(ctx)
		if err != nil {
			klog.Errorf("Failed to get reader for batch condition processing: %v", err)
			return nil // Don't fail the operation
		}
		defer reader.Close()

		// Re-read each host from database and process conditions
		for i := range hosts {
			freshHost, err := reader.GetHostByID(ctx, models.ResourceIdentifier{
				Namespace: hosts[i].Namespace,
				Name:      hosts[i].Name,
			})
			if err != nil {
				klog.Errorf("Failed to re-read host %s/%s for condition processing: %v",
					hosts[i].Namespace, hosts[i].Name, err)
				continue // Skip this host but continue with others
			}

			// Process conditions with the FRESH host from database
			if err := s.conditionManager.ProcessHostConditions(ctx, freshHost, nil); err != nil {
				klog.Errorf("Failed to process conditions for %s/%s: %v",
					freshHost.Namespace, freshHost.Name, err)
				// Continue with other hosts
			}
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

func (s *HostResourceService) getHostByID(ctx context.Context, id string) (*models.Host, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

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

// CLOUD-233: Removed syncHostWithExternal()
// This method performed synchronous SGROUP sync, which is now handled by OutboxWorker
// Migration 026 (Host UPSERT), 032 (Host DELETE) triggers create outbox entries
// OutboxWorker processes them asynchronously with exponential backoff and retry

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

	isBinding := bindingID.Name != "" && addressGroupID.Name != ""
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
		host.BindingRef = &v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "HostBinding",
				Name:       bindingID.Name,
			},
			Namespace: bindingID.Namespace,
		}
		host.AddressGroupRef = &v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       addressGroupID.Name,
			},
			Namespace: addressGroupID.Namespace,
		}
		host.IsBound = true
		if addressGroupID.Namespace != "" {
			host.AddressGroupName = fmt.Sprintf("%s/%s", addressGroupID.Namespace, addressGroupID.Name)
		} else {
			host.AddressGroupName = addressGroupID.Name
		}
	}

	// Update metadata
	host.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Sync the updated host
	hosts := []models.Host{*host}
	if err := writer.SyncHosts(ctx, hosts, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync host binding: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit host binding: %w", err)
	}

	// CLOUD-233: Removed syncManager.SyncEntity() - SGROUP sync now handled by OutboxWorker
	// Migration 026 triggers create outbox entries, OutboxWorker processes them asynchronously

	return nil
}

// CLOUD-233: Removed SyncHostWithExternal() - public wrapper for old sync method
// SGROUP sync now handled by OutboxWorker asynchronously

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
		if newAG.Namespace != "" {
			addressGroupName = fmt.Sprintf("%s/%s", newAG.Namespace, newAG.Name)
		} else {
			addressGroupName = newAG.Name
		}
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

	if isBound && !utils.IsReadyConditionTrue(host) {
		return fmt.Errorf("host %s/%s is not ready for binding via AddressGroup.spec - must be synchronized with SGROUP first (Ready condition must be True)", namespace, hostName)
	}

	// Update Host status
	host.IsBound = isBound
	if isBound {
		host.AddressGroupName = addressGroupName
		host.AddressGroupRef = &v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       addressGroupName,
			},
			Namespace: namespace,
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

	// CLOUD-233: Removed syncManager.SyncEntityForced() - SGROUP sync now handled by OutboxWorker
	// UpdateHost() above already triggers Migration 026, which creates outbox entries

	return nil
}

// updateHostStatus updates only the host status/conditions in the database without triggering sync
func (s *HostResourceService) updateHostStatus(ctx context.Context, host *models.Host) error {
	host.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	hosts := []models.Host{*host}
	if err := writer.SyncHosts(ctx, hosts, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync host status: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit host status update: %w", err)
	}

	return nil
}
