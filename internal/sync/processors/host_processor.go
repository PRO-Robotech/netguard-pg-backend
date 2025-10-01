package processors

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/sync/detector"
	"netguard-pg-backend/internal/sync/synchronizer"
	"netguard-pg-backend/internal/sync/types"
)

// hostProcessor implements EntityProcessor for host synchronization
type hostProcessor struct {
	synchronizer synchronizer.HostSynchronizer
	config       HostProcessorConfig
}

// HostProcessorConfig holds configuration for host processing
type HostProcessorConfig struct {
	// EnableNamespaceFiltering enables filtering by specific namespaces
	EnableNamespaceFiltering bool
	// AllowedNamespaces is list of namespaces to process (empty = all namespaces)
	AllowedNamespaces []string
	// EnableFullSyncOnChange triggers full sync instead of namespace-specific sync
	EnableFullSyncOnChange bool
	// MaxRetryAttempts for processing failures
	MaxRetryAttempts int
}

// DefaultHostProcessorConfig returns default configuration for host processor
func DefaultHostProcessorConfig() HostProcessorConfig {
	return HostProcessorConfig{
		EnableNamespaceFiltering: false,
		AllowedNamespaces:        []string{},
		EnableFullSyncOnChange:   false,
		MaxRetryAttempts:         3,
	}
}

// NewHostProcessor creates a new host processor
func NewHostProcessor(
	synchronizer synchronizer.HostSynchronizer,
	config HostProcessorConfig,
) EntityProcessor {
	return &hostProcessor{
		synchronizer: synchronizer,
		config:       config,
	}
}

// GetEntityType returns the entity type this processor handles
func (p *hostProcessor) GetEntityType() string {
	return "host"
}

// ProcessChanges processes change events for hosts
func (p *hostProcessor) ProcessChanges(ctx context.Context, event detector.ChangeEvent) error {

	var result *types.HostSyncResult
	var err error

	if p.config.EnableFullSyncOnChange {
		// Perform full synchronization
		result, err = p.performFullSync(ctx)
	} else {
		// Perform namespace-based synchronization
		result, err = p.performNamespaceSync(ctx)
	}

	if err != nil {
		return p.handleSyncError(err, event)
	}

	// Log sync results
	p.logSyncResults(result, event)

	return nil
}

// performFullSync performs full synchronization of all hosts
func (p *hostProcessor) performFullSync(ctx context.Context) (*types.HostSyncResult, error) {
	return p.synchronizer.SyncAllHosts(ctx)
}

// performNamespaceSync performs synchronization for allowed namespaces
func (p *hostProcessor) performNamespaceSync(ctx context.Context) (*types.HostSyncResult, error) {
	if !p.config.EnableNamespaceFiltering || len(p.config.AllowedNamespaces) == 0 {
		// No filtering - sync all namespaces
		return p.synchronizer.SyncAllHosts(ctx)
	}

	// Aggregate results from all allowed namespaces
	aggregateResult := types.NewHostSyncResult()
	aggregateResult.SetDetail("sync_type", "namespace_filtered")
	aggregateResult.SetDetail("namespaces", p.config.AllowedNamespaces)

	for _, namespace := range p.config.AllowedNamespaces {

		result, err := p.synchronizer.SyncHosts(ctx, namespace)
		if err != nil {
			// Continue with other namespaces even if one fails
			continue
		}

		// Merge results
		p.mergeResults(aggregateResult, result)
	}

	return aggregateResult, nil
}

// mergeResults merges individual sync results into aggregate result
func (p *hostProcessor) mergeResults(aggregate, individual *types.HostSyncResult) {
	// Add synced hosts
	for _, uuid := range individual.SyncedHostUUIDs {
		aggregate.AddSyncedHost(uuid)
	}

	// Add failed hosts
	for _, uuid := range individual.FailedUUIDs {
		errorMsg := individual.GetError(uuid)
		aggregate.AddFailedHost(uuid, errorMsg)
	}

	// Update totals
	aggregate.TotalRequested += individual.TotalRequested
}

// handleSyncError handles synchronization errors with retry logic
func (p *hostProcessor) handleSyncError(err error, event detector.ChangeEvent) error {

	// For now, just log and return the error
	// In future, could implement retry logic based on MaxRetryAttempts
	return fmt.Errorf("host sync failed for event from %s: %w", event.Source, err)
}

// logSyncResults logs the synchronization results
func (p *hostProcessor) logSyncResults(result *types.HostSyncResult, event detector.ChangeEvent) {
	if result.IsEmpty() {
		return
	}

	if result.HasErrors() {
	} else {
	}

	// Log details if available
	if result.Details != nil && len(result.Details) > 0 {
	}
}
