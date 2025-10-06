package synchronizer

import (
	"context"
	"fmt"
	"net"
	"time"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/sync/types"
)

// hostSynchronizer implements HostSynchronizer interface
type hostSynchronizer struct {
	hostReader   HostReader
	hostWriter   HostWriter
	sgroupReader SGROUPHostReader
	config       HostSyncConfig
}

// NewHostSynchronizer creates a new host synchronizer
func NewHostSynchronizer(
	hostReader HostReader,
	hostWriter HostWriter,
	sgroupReader SGROUPHostReader,
	config HostSyncConfig,
) HostSynchronizer {
	return &hostSynchronizer{
		hostReader:   hostReader,
		hostWriter:   hostWriter,
		sgroupReader: sgroupReader,
		config:       config,
	}
}

// SyncHosts synchronizes hosts for a specific namespace
func (s *hostSynchronizer) SyncHosts(ctx context.Context, namespace string) (*types.HostSyncResult, error) {

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(s.config.SyncTimeout)*time.Second)
	defer cancel()

	result := types.NewHostSyncResult()
	result.SetDetail("namespace", namespace)
	result.SetDetail("sync_start_time", time.Now())

	// Find hosts without IPSet
	hosts, err := s.findHostsWithoutIPSet(timeoutCtx, namespace)
	if err != nil {
		return result, fmt.Errorf("failed to find hosts without IPSet: %w", err)
	}

	if len(hosts) == 0 {
		return result, nil
	}

	result.SetTotalRequested(len(hosts))

	// Extract UUIDs for SGROUP query
	uuids := make([]string, len(hosts))
	hostMap := make(map[string]models.Host)
	for i, host := range hosts {
		uuids[i] = host.UUID
		hostMap[host.UUID] = host
	}

	// Process in batches
	batches := s.createBatches(uuids, s.config.BatchSize)
	result.SetDetail("batch_count", len(batches))

	for _, batch := range batches {

		batchResult, err := s.syncHostBatch(timeoutCtx, batch, hostMap)
		if err != nil {
			// Continue with other batches even if one fails
		}

		// Merge batch results
		s.mergeBatchResult(result, batchResult)
	}

	result.SetDetail("sync_end_time", time.Now())

	return result, nil
}

// SyncHostsByUUIDs synchronizes specific hosts by their UUIDs
func (s *hostSynchronizer) SyncHostsByUUIDs(ctx context.Context, uuids []string) (*types.HostSyncResult, error) {

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(s.config.SyncTimeout)*time.Second)
	defer cancel()

	result := types.NewHostSyncResult()
	result.SetTotalRequested(len(uuids))
	result.SetDetail("sync_type", "by_uuids")
	result.SetDetail("sync_start_time", time.Now())

	if len(uuids) == 0 {
		return result, nil
	}

	// Get host information from NETGUARD
	hostMap, err := s.getHostMapByUUIDs(timeoutCtx, uuids)
	if err != nil {
		return result, fmt.Errorf("failed to get host information: %w", err)
	}

	// Process in batches
	batches := s.createBatches(uuids, s.config.BatchSize)
	result.SetDetail("batch_count", len(batches))

	for _, batch := range batches {

		batchResult, err := s.syncHostBatch(timeoutCtx, batch, hostMap)
		if err != nil {
		}

		s.mergeBatchResult(result, batchResult)
	}

	result.SetDetail("sync_end_time", time.Now())
	return result, nil
}

// SyncAllHosts performs full synchronization of all hosts
func (s *hostSynchronizer) SyncAllHosts(ctx context.Context) (*types.HostSyncResult, error) {

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(s.config.SyncTimeout)*time.Second)
	defer cancel()

	result := types.NewHostSyncResult()
	result.SetDetail("sync_type", "full_sync")
	result.SetDetail("sync_start_time", time.Now())

	// Get all hosts without IPSet from all namespaces
	hosts, err := s.findHostsWithoutIPSet(timeoutCtx, "") // empty namespace = all namespaces
	if err != nil {
		return result, fmt.Errorf("failed to find hosts without IPSet: %w", err)
	}

	if len(hosts) == 0 {
		return result, nil
	}

	result.SetTotalRequested(len(hosts))

	// Convert to UUIDs and create host map
	uuids := make([]string, len(hosts))
	hostMap := make(map[string]models.Host)
	for i, host := range hosts {
		uuids[i] = host.UUID
		hostMap[host.UUID] = host
	}

	// Process in batches
	batches := s.createBatches(uuids, s.config.BatchSize)
	result.SetDetail("batch_count", len(batches))

	for _, batch := range batches {

		batchResult, err := s.syncHostBatch(timeoutCtx, batch, hostMap)
		if err != nil {
		}

		s.mergeBatchResult(result, batchResult)
	}

	result.SetDetail("sync_end_time", time.Now())

	return result, nil
}

// findHostsWithoutIPSet finds hosts that don't have IPSet filled
func (s *hostSynchronizer) findHostsWithoutIPSet(ctx context.Context, namespace string) ([]models.Host, error) {
	hosts, err := s.hostReader.GetHostsWithoutIPSet(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get hosts without IPSet: %w", err)
	}

	return hosts, nil
}

// getHostMapByUUIDs creates a map of UUID -> Host for given UUIDs
func (s *hostSynchronizer) getHostMapByUUIDs(ctx context.Context, uuids []string) (map[string]models.Host, error) {
	hostMap := make(map[string]models.Host)

	// Get host information for each UUID
	// In a real implementation, you might want to batch this operation
	for _, uuid := range uuids {
		host, err := s.hostReader.GetHostByUUID(ctx, uuid)
		if err != nil {
			continue
		}
		if host != nil {
			hostMap[uuid] = *host
		}
	}

	return hostMap, nil
}

// syncHostBatch synchronizes a batch of hosts
func (s *hostSynchronizer) syncHostBatch(ctx context.Context, uuids []string, hostMap map[string]models.Host) (*types.HostSyncResult, error) {
	result := types.NewHostSyncResult()
	result.SetTotalRequested(len(uuids))

	// Get host data from SGROUP
	sgroupHosts, err := s.sgroupReader.GetHostsByUUIDs(ctx, uuids)
	if err != nil {
		// Mark all as failed
		for _, uuid := range uuids {
			result.AddFailedHost(uuid, fmt.Sprintf("SGROUP query failed: %v", err))
		}
		return result, err
	}

	// Create map of UUID -> SGROUP Host
	sgroupHostMap := make(map[string]*pb.Host)
	for _, sgroupHost := range sgroupHosts {
		if sgroupHost != nil && sgroupHost.Uuid != "" {
			sgroupHostMap[sgroupHost.Uuid] = sgroupHost
		}
	}

	// Prepare IPSet updates
	var updates []types.HostIPSetUpdate

	for _, uuid := range uuids {
		netguardHost, hasNetguardHost := hostMap[uuid]
		sgroupHost, hasSGroupHost := sgroupHostMap[uuid]

		if !hasNetguardHost {
			result.AddFailedHost(uuid, "host not found in NETGUARD")
			continue
		}

		if !hasSGroupHost {
			result.AddFailedHost(uuid, "host not found in SGROUP")
			continue
		}

		// Extract IPSet from SGROUP host
		var ipSet []string
		if sgroupHost.IpList != nil && len(sgroupHost.IpList.IPs) > 0 {
			ipSet = sgroupHost.IpList.IPs

			// Validate IP addresses if enabled
			if s.config.EnableIPSetValidation {
				validIPs := make([]string, 0, len(ipSet))
				for _, ip := range ipSet {
					if s.isValidIP(ip) {
						validIPs = append(validIPs, ip)
					} else {
					}
				}
				ipSet = validIPs
			}
		}

		if len(ipSet) > 0 {
			updates = append(updates, types.HostIPSetUpdate{
				HostUUID:  uuid,
				HostID:    netguardHost.GetID(),
				Namespace: netguardHost.Namespace,
				Name:      netguardHost.Name,
				IPSet:     ipSet,
				SGName:    sgroupHost.SgName,
			})
		} else {
			result.AddFailedHost(uuid, "no valid IP addresses found in SGROUP")
		}
	}

	// Apply updates
	if len(updates) > 0 {

		err = s.hostWriter.UpdateHostsIPSet(ctx, updates)
		if err != nil {
			// Mark all as failed
			for _, update := range updates {
				result.AddFailedHost(update.HostUUID, fmt.Sprintf("update failed: %v", err))
			}
			return result, err
		}

		// Mark all as successful
		for _, update := range updates {
			result.AddSyncedHost(update.HostUUID)
		}
	}

	return result, nil
}

// createBatches creates batches of UUIDs for processing
func (s *hostSynchronizer) createBatches(uuids []string, batchSize int) [][]string {
	if batchSize <= 0 {
		batchSize = 50
	}

	var batches [][]string
	for i := 0; i < len(uuids); i += batchSize {
		end := i + batchSize
		if end > len(uuids) {
			end = len(uuids)
		}
		batches = append(batches, uuids[i:end])
	}

	return batches
}

// mergeBatchResult merges batch result into main result
func (s *hostSynchronizer) mergeBatchResult(mainResult, batchResult *types.HostSyncResult) {
	// Add synced hosts
	for _, uuid := range batchResult.SyncedHostUUIDs {
		mainResult.AddSyncedHost(uuid)
	}

	// Add failed hosts
	for _, uuid := range batchResult.FailedUUIDs {
		errorMsg := batchResult.GetError(uuid)
		mainResult.AddFailedHost(uuid, errorMsg)
	}
}

// isValidIP validates if the given string is a valid IP address
func (s *hostSynchronizer) isValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}
