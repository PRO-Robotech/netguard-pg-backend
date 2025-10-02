package sync

import (
	"context"

	"netguard-pg-backend/internal/sync/config"
	"netguard-pg-backend/internal/sync/detector"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/manager"
	"netguard-pg-backend/internal/sync/processors"
	"netguard-pg-backend/internal/sync/synchronizer"
)

// ReverseSyncSystem demonstrates how to integrate all components
// This is an example of how to wire up the complete reverse synchronization system
type ReverseSyncSystem struct {
	manager        *manager.ReverseSyncManager
	changeDetector detector.ChangeDetector
	config         config.ReverseSyncSystemConfig
}

// NewReverseSyncSystem creates a complete reverse synchronization system
func NewReverseSyncSystem(
	sgroupGateway interfaces.SGroupGateway, // Interface to SGROUP system
	hostReader synchronizer.HostReader, // Interface to read hosts from NETGUARD
	hostWriter synchronizer.HostWriter, // Interface to write hosts to NETGUARD
	systemConfig config.ReverseSyncSystemConfig,
) (*ReverseSyncSystem, error) {
	// 1. Create SGROUP change detector
	changeDetector := detector.NewSGROUPChangeDetector(sgroupGateway, systemConfig.SGROUPDetector)

	// 2. Create host synchronizer
	hostSynchronizer := synchronizer.NewHostSynchronizer(
		hostReader,
		hostWriter,
		sgroupGateway, // Also implements SGROUPHostReader
		systemConfig.HostSynchronizer,
	)

	// 3. Create host processor
	hostProcessor := processors.NewHostProcessor(hostSynchronizer, systemConfig.HostProcessor)

	// 4. Create reverse sync manager
	reverseSyncManager := manager.NewReverseSyncManager(changeDetector, systemConfig.Manager)

	// 5. Register processors
	err := reverseSyncManager.RegisterProcessor(hostProcessor)
	if err != nil {
		return nil, err
	}

	return &ReverseSyncSystem{
		manager:        reverseSyncManager,
		changeDetector: changeDetector,
		config:         systemConfig,
	}, nil
}

// Start starts the reverse synchronization system
func (s *ReverseSyncSystem) Start(ctx context.Context) error {

	err := s.manager.Start(ctx)
	if err != nil {
		return err
	}

	return nil
}

// Stop stops the reverse synchronization system
func (s *ReverseSyncSystem) Stop() error {

	err := s.manager.Stop()
	if err != nil {
		return err
	}

	return nil
}

// GetStats returns system statistics
func (s *ReverseSyncSystem) GetStats() manager.ReverseSyncStats {
	return s.manager.GetStats()
}

// IsRunning returns true if the system is running
func (s *ReverseSyncSystem) IsRunning() bool {
	return s.manager.IsRunning()
}

// Note: SGROUPGatewayInterface has been replaced by interfaces.SGroupGateway
// which now includes all necessary host methods for reverse synchronization

// ExampleUsage shows how to use the reverse sync system
func ExampleUsage() {
	// 1. Create configuration (choose based on environment)
	config := config.DevelopmentConfig() // or ProductionConfig(), TestConfig()

	// 2. Validate configuration
	err := config.Validate()
	if err != nil {
		// log.Fatalf("Invalid configuration: %v", err)
	}

	// 3. Create your implementations of required interfaces
	// sgroupGateway := yourSGROUPGatewayImplementation()
	// hostReader := yourHostReaderImplementation()
	// hostWriter := yourHostWriterImplementation()

	// 4. Create reverse sync system
	// reverseSyncSystem, err := NewReverseSyncSystem(sgroupGateway, hostReader, hostWriter, config)
	// if err != nil {
	//     // log.Fatalf("Failed to create reverse sync system: %v", err)
	// }

	// 5. Start the system
	// ctx := context.Background()
	// err = reverseSyncSystem.Start(ctx)
	// if err != nil {
	//     // log.Fatalf("Failed to start reverse sync system: %v", err)
	// }

	// 6. System will now automatically:
	//    - Monitor SGROUP for changes via SyncStatuses stream
	//    - When changes detected, find hosts without IPSet in NETGUARD
	//    - Query SGROUP for those hosts' IP information
	//    - Update NETGUARD hosts with the IP information
	//    - Handle connection failures with automatic reconnection
	//    - Provide statistics and health monitoring

	// 7. Stop the system when done
	// defer func() {
	//     err := reverseSyncSystem.Stop()
	//     if err != nil {
	//     }
	// }()

	// log.Println("Reverse synchronization system example completed")
}
