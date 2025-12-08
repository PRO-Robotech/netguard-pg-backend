package adapters

import (
	"context"
	"fmt"
	"strings"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/synchronizer"
	"netguard-pg-backend/internal/sync/types"
)

// PostgreSQLHostReader implements synchronizer.HostReader using PostgreSQL registry
type PostgreSQLHostReader struct {
	registry ports.Registry
}

// NewPostgreSQLHostReader creates a new PostgreSQL-based HostReader
func NewPostgreSQLHostReader(registry ports.Registry) synchronizer.HostReader {
	return &PostgreSQLHostReader{
		registry: registry,
	}
}

// GetHostsWithoutIPSet returns hosts that don't have IPSet filled
func (r *PostgreSQLHostReader) GetHostsWithoutIPSet(ctx context.Context, namespace string) ([]models.Host, error) {
	reader, err := r.registry.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	var hosts []models.Host
	var scope ports.Scope = ports.EmptyScope{}

	err = reader.ListHosts(ctx, func(host models.Host) error {
		// Apply namespace filter if specified
		if namespace != "" && host.Namespace != namespace {
			return nil
		}
		// Filter hosts without IPSet (empty or nil)
		if len(host.IpList) == 0 || host.MetaInfo == nil {
			hosts = append(hosts, host)
		}
		return nil
	}, scope)

	if err != nil {
		return nil, fmt.Errorf("failed to list hosts without IPSet: %w", err)
	}

	return hosts, nil
}

// GetHostByUUID returns a host by its UUID
func (r *PostgreSQLHostReader) GetHostByUUID(ctx context.Context, uuid string) (*models.Host, error) {
	reader, err := r.registry.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	var foundHost *models.Host
	err = reader.ListHosts(ctx, func(host models.Host) error {
		if host.UUID == uuid {
			foundHost = &host
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return nil, fmt.Errorf("failed to find host by UUID %s: %w", uuid, err)
	}

	if foundHost == nil {
		return nil, fmt.Errorf("host with UUID %s not found", uuid)
	}

	return foundHost, nil
}

// ListHosts lists hosts by identifiers (namespace, name pairs)
func (r *PostgreSQLHostReader) ListHosts(ctx context.Context, identifiers []synchronizer.HostIdentifier) ([]models.Host, error) {
	reader, err := r.registry.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	var hosts []models.Host
	var resourceIds []models.ResourceIdentifier

	for _, id := range identifiers {
		resourceIds = append(resourceIds, models.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		})
	}

	scope := ports.NewResourceIdentifierScope(resourceIds...)

	err = reader.ListHosts(ctx, func(host models.Host) error {
		hosts = append(hosts, host)
		return nil
	}, scope)

	if err != nil {
		return nil, fmt.Errorf("failed to list hosts by identifiers: %w", err)
	}

	return hosts, nil
}

// PostgreSQLHostWriter implements synchronizer.HostWriter using PostgreSQL registry
type PostgreSQLHostWriter struct {
	registry ports.Registry
}

// NewPostgreSQLHostWriter creates a new PostgreSQL-based HostWriter
func NewPostgreSQLHostWriter(registry ports.Registry) synchronizer.HostWriter {
	return &PostgreSQLHostWriter{
		registry: registry,
	}
}

// UpdateHostIPSet updates the IPSet for a specific host
func (w *PostgreSQLHostWriter) UpdateHostIPSet(ctx context.Context, hostID string, ipSet []string) error {
	return w.updateHostData(ctx, hostID, ipSet, nil)
}

// updateHostData updates the IPSet and optionally MetaInfo for a specific host
func (w *PostgreSQLHostWriter) updateHostData(ctx context.Context, hostID string, ipSet []string, metaInfo *types.HostMetaInfoUpdate) error {
	// Parse hostID to get namespace and name
	namespace, name, err := parseHostID(hostID)
	if err != nil {
		return fmt.Errorf("invalid host ID %s: %w", hostID, err)
	}

	// Create a reader to get the existing host
	reader, err := w.registry.Reader(ctx)
	if err != nil {
		return fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	// Get the existing host
	resourceId := models.ResourceIdentifier{
		Namespace: namespace,
		Name:      name,
	}

	existingHost, err := reader.GetHostByID(ctx, resourceId)
	if err != nil {
		return fmt.Errorf("failed to get host %s: %w", hostID, err)
	}

	// Convert string slice to IPItem slice
	var ipItems []models.IPItem
	for _, ip := range ipSet {
		ipItems = append(ipItems, models.IPItem{IP: ip})
	}

	existingHost.IpList = ipItems

	if metaInfo != nil {
		existingHost.MetaInfo = &models.HostMetaInfo{
			HostName:        metaInfo.HostName,
			Os:              metaInfo.Os,
			Platform:        metaInfo.Platform,
			PlatformFamily:  metaInfo.PlatformFamily,
			PlatformVersion: metaInfo.PlatformVersion,
			KernelVersion:   metaInfo.KernelVersion,
		}
	}

	// Create a writer to update the host
	writer, err := w.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Sync the updated host
	err = writer.SyncHosts(ctx, []models.Host{*existingHost}, ports.EmptyScope{})
	if err != nil {
		return fmt.Errorf("failed to update host %s: %w", hostID, err)
	}

	// Commit the transaction
	err = writer.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit host %s update: %w", hostID, err)
	}

	return nil
}

// UpdateHostsIPSet updates IPSet and MetaInfo for multiple hosts in batch
func (w *PostgreSQLHostWriter) UpdateHostsIPSet(ctx context.Context, updates []types.HostIPSetUpdate) error {
	for _, update := range updates {
		err := w.updateHostData(ctx, update.HostID, update.IPSet, update.MetaInfo)
		if err != nil {
			return fmt.Errorf("failed to update host %s in batch: %w", update.HostID, err)
		}
	}
	return nil
}

// parseHostID parses a host ID in format "namespace/name" into components
func parseHostID(hostID string) (namespace, name string, err error) {
	// Host ID format is based on Key() method: "namespace/name" or just "name" for default namespace
	if hostID == "" {
		return "", "", fmt.Errorf("host ID cannot be empty")
	}

	// Check if hostID contains namespace separator
	if idx := strings.LastIndex(hostID, "/"); idx > 0 {
		namespace = hostID[:idx]
		name = hostID[idx+1:]
		if name == "" {
			return "", "", fmt.Errorf("invalid host ID format, empty name: %s", hostID)
		}
	} else {
		// No namespace separator, use default namespace
		namespace = "default"
		name = hostID
	}

	return namespace, name, nil
}
