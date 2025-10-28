package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"netguard-pg-backend/internal/api/netguard"
	"netguard-pg-backend/internal/app/server"
	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/application/services/conditions"
	"netguard-pg-backend/internal/config"
	"netguard-pg-backend/internal/domain/registry"
	"netguard-pg-backend/internal/infrastructure/repositories/pg"
	"netguard-pg-backend/internal/sync/adapters"
	"netguard-pg-backend/internal/sync/clients"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/manager"
	"netguard-pg-backend/internal/sync/monitor"
	"netguard-pg-backend/internal/sync/processors"
	"netguard-pg-backend/internal/sync/syncers"
	"netguard-pg-backend/internal/sync/synchronizer"
	"netguard-pg-backend/internal/sync/types"
	"netguard-pg-backend/internal/sync/worker"

	"github.com/go-logr/stdr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

var (
	memoryDB   = flag.Bool("memory", false, "Use in-memory database")
	pgURI      = flag.String("pg-uri", "", "PostgreSQL connection URI")
	migrateDB  = flag.Bool("migrate", false, "Run database migrations")
	configPath = flag.String("config", "config/config.yaml", "Path to configuration file")
	grpcAddr   = flag.String("grpc-addr", "", "gRPC server address (overrides config)")
	httpAddr   = flag.String("http-addr", "", "HTTP server address (overrides config)")
)

func main() {
	flag.Parse()

	// Load configuration
	cfg, err := config.NewConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	// Override config values with command line flags if provided
	if *grpcAddr != "" {
		cfg.Settings.GRPCAddr = *grpcAddr
	}
	if *httpAddr != "" {
		cfg.Settings.HTTPAddr = *httpAddr
	}

	// Initialize structured logger
	zapLogger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer zapLogger.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create registry
	var pgRegistry *pg.Registry
	if *memoryDB {
		log.Fatal("OutboxWorker requires PostgreSQL, cannot use memory database")
	} else if *pgURI != "" {
		pgRegistry, err = pg.NewRegistryFromURI(ctx, *pgURI)
		if err != nil {
			log.Fatalf("Failed to create PostgreSQL registry: %v", err)
		}
		if pgRegistry == nil {
			log.Fatalf("PostgreSQL registry is nil!")
		}
	} else {
		log.Fatal("Either --memory or --pg-uri must be specified")
	}
	defer pgRegistry.Close()

	if err := registry.ValidateRegistry(); err != nil {
		zapLogger.Fatal("invalid resource registry", zap.Error(err))
	}

	// Create SGroups client and ConnectionMonitor (CENTRALIZED)
	var sgroupsClient interfaces.SGroupGateway
	var connMonitor *monitor.SGroupConnectionMonitor

	if cfg.Sync.Enabled {
		// Create SGroups client once
		client, err := clients.NewSGroupsClient(cfg.Sync.SGroups)
		if err != nil {
			log.Fatalf("Failed to create sgroups client: %v", err)
		}
		sgroupsClient = client
		defer sgroupsClient.Close()

		// Create and start ConnectionMonitor
		connMonitor = monitor.NewSGroupConnectionMonitor(
			sgroupsClient,
			monitor.ConnectionMonitorConfig{
				ReconnectInterval: 5 * time.Second,
				MaxReconnectDelay: 60 * time.Second,
				BackoffMultiplier: 2.0,
			},
			zapLogger,
		)
		connMonitor.Start()
		defer connMonitor.Stop()

		// Wait for connection if required
		if cfg.Sync.Required {
			zapLogger.Info("Waiting for SGROUP connection (required mode)...")
			if err := connMonitor.WaitForConnection(10 * time.Second); err != nil {
				log.Fatalf("SGROUP connection required but not established: %v", err)
			}
			zapLogger.Info("SGROUP connection established")
		} else {
			// Not required - just log current state
			if connMonitor.IsConnected() {
				zapLogger.Info("SGROUP connection established")
			} else {
				zapLogger.Warn("SGROUP not connected, will retry in background")
			}
		}
	}

	// Setup sync manager
	syncManager := setupSyncManager(ctx, cfg, sgroupsClient, zapLogger)

	// Setup reverse sync system (SGROUP -> NETGUARD synchronization)
	// REFACTORED: Now uses full processor registration with HostProcessor
	reverseSyncSystem := setupReverseSyncSystem(ctx, cfg, pgRegistry, sgroupsClient, connMonitor, zapLogger)

	var outboxWorker *worker.OutboxWorker
	if syncManager != nil {
		outboxWorker = setupOutboxWorker(ctx, cfg, pgRegistry, syncManager, connMonitor, zapLogger)
	}

	// Create condition manager (needed for facade)
	conditionManager := conditions.NewConditionManager(pgRegistry)

	// Create facade service (new architecture)
	netguardFacade := services.NewNetguardFacade(pgRegistry, conditionManager, syncManager)

	// Using immediate force sync approach instead of finalizers

	// Setup gRPC server
	grpcServer := grpc.NewServer()
	netguardServer := netguard.NewServiceServer(netguardFacade)
	netguardpb.RegisterNetguardServiceServer(grpcServer, netguardServer)

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("liveness", grpc_health_v1.HealthCheckResponse_SERVING)

	// Start gRPC server
	lis, err := net.Listen("tcp", cfg.Settings.GRPCAddr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// Setup HTTP server with gRPC-Gateway
	httpServer, httpMux, err := server.SetupServer(ctx, cfg.Settings.GRPCAddr, cfg.Settings.HTTPAddr, netguardFacade)
	if err != nil {
		log.Fatalf("Failed to setup server: %v", err)
	}

	// Register health endpoint for SGROUP sync
	if connMonitor != nil {
		healthListener := server.NewHealthEndpointListener()
		connMonitor.Subscribe(healthListener)
		httpMux.HandleFunc("/healthz/sync", healthListener.ServeHTTP)
	}

	// Register worker health endpoint in the correct mux
	if outboxWorker != nil {
		httpMux.HandleFunc("/healthz/worker", outboxWorker.HealthHandler)
	}

	// Register metrics endpoint if enabled
	workerConfig := worker.LoadFromEnv()
	if workerConfig.MetricsEnabled {
		httpMux.Handle("/metrics", promhttp.Handler())
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to serve HTTP: %v", err)
		}
	}()

	<-sigCh
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	healthServer.SetServingStatus("liveness", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if outboxWorker != nil {
		outboxWorker.Stop()
		select {
		case <-outboxWorker.Done():
		case <-shutdownCtx.Done():
		}
	}

	if reverseSyncSystem != nil {
		if err := reverseSyncSystem.Stop(); err != nil {
			zapLogger.Error("failed to stop reverse sync system", zap.Error(err))
		}
	}

	grpcServer.GracefulStop()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		zapLogger.Error("HTTP server shutdown failed", zap.Error(err))
	}
}

// setupSyncManager creates and configures the sync manager for sgroups integration
func setupSyncManager(ctx context.Context, cfg *config.Config, sgroupsClient interfaces.SGroupGateway, logger *zap.Logger) interfaces.SyncManager {
	// Skip if client not provided (sync disabled)
	if sgroupsClient == nil {
		return nil
	}

	// Use sync configuration from loaded config
	syncConfig := cfg.Sync

	// Validate configuration
	if err := syncConfig.Validate(); err != nil {
		logger.Error("sync config validation failed", zap.Error(err))
		return nil
	}

	if !syncConfig.Enabled {
		return nil
	}

	// Create logger for sync manager (logr adapter)
	logrLogger := stdr.New(log.Default())

	// Create sync manager
	syncManager := manager.NewSyncManager(sgroupsClient, logrLogger)

	// Register AddressGroup syncer
	addressGroupSyncer := syncers.NewAddressGroupSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeGroups, addressGroupSyncer); err != nil {
		logger.Error("failed to register AddressGroup syncer", zap.Error(err))
		return nil
	}

	// Register Network syncer
	networkSyncer := syncers.NewNetworkSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeNetworks, networkSyncer); err != nil {
		logger.Error("failed to register Network syncer", zap.Error(err))
		return nil
	}

	// Register Host syncer
	hostSyncer := syncers.NewHostSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeHosts, hostSyncer); err != nil {
		logger.Error("failed to register Host syncer", zap.Error(err))
		return nil
	}

	// Register IEAgAgRule syncer
	ieagagRuleSyncer := syncers.NewIEAgAgRuleSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeIEAgAgRules, ieagagRuleSyncer); err != nil {
		logger.Error("failed to register IEAgAgRule syncer", zap.Error(err))
		return nil
	}

	// Register Service syncer
	serviceSyncer := syncers.NewServiceSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeServices, serviceSyncer); err != nil {
		logger.Error("failed to register Service syncer", zap.Error(err))
		return nil
	}

	// Register SvcSvcRule syncer
	svcSvcRuleSyncer := syncers.NewSvcSvcRuleSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeSvcSvcRules, svcSvcRuleSyncer); err != nil {
		logger.Error("failed to register SvcSvcRule syncer", zap.Error(err))
		return nil
	}

	if err := syncManager.Start(ctx); err != nil {
		logger.Error("failed to start sync manager", zap.Error(err))
		return nil
	}

	return syncManager
}

// setupReverseSyncSystem creates and configures the reverse sync system for SGROUP -> NETGUARD synchronization
func setupReverseSyncSystem(
	ctx context.Context,
	cfg *config.Config,
	pgRegistry *pg.Registry,
	sgroupsClient interfaces.SGroupGateway,
	connMonitor *monitor.SGroupConnectionMonitor,
	logger *zap.Logger,
) *manager.ReverseSyncManager {
	// Skip setup if client or monitor not available (sync disabled)
	if sgroupsClient == nil || connMonitor == nil {
		logger.Info("Reverse sync disabled: sgroups client or connection monitor not available")
		return nil
	}

	// Validate reverse sync configuration
	if err := cfg.ReverseSync.Validate(); err != nil {
		logger.Error("Invalid reverse sync configuration", zap.Error(err))
		return nil
	}

	// Create reverse sync manager with correct config structure
	reverseSyncSystem := manager.NewReverseSyncManager(
		connMonitor,
		cfg.ReverseSync.Manager,
	)

	// ========================================
	// FULL PROCESSOR SETUP (Host Synchronization)
	// ========================================

	// Create PostgreSQL adapters for Host synchronization
	hostReader := adapters.NewPostgreSQLHostReader(pgRegistry)
	hostWriter := adapters.NewPostgreSQLHostWriter(pgRegistry)

	// SGroupGateway already implements SGROUPHostReader interface
	// (GetHostsByUUIDs, ListAllHosts, GetHostsInSecurityGroup methods)
	sgroupHostReader := sgroupsClient

	// Create HostSynchronizer configuration with defaults
	// Default values: BatchSize=50, MaxConcurrency=5, SyncTimeout=30s
	hostSyncConfig := synchronizer.DefaultHostSyncConfig()

	// Create HostSynchronizer
	hostSynchronizer := synchronizer.NewHostSynchronizer(
		hostReader,
		hostWriter,
		sgroupHostReader,
		hostSyncConfig,
	)

	// Create HostProcessor configuration
	hostProcessorConfig := processors.DefaultHostProcessorConfig()
	// Enable full sync on every change (comprehensive approach)
	hostProcessorConfig.EnableFullSyncOnChange = true

	// Create HostProcessor
	hostProcessor := processors.NewHostProcessor(
		hostSynchronizer,
		hostProcessorConfig,
	)

	// Register HostProcessor with ReverseSyncManager
	if err := reverseSyncSystem.RegisterProcessor(hostProcessor); err != nil {
		logger.Error("Failed to register HostProcessor", zap.Error(err))
		return nil
	}

	logger.Info("HostProcessor registered successfully for reverse sync")

	// TODO: Register additional processors for AddressGroup, Network, etc. as needed
	// Example:
	// - AddressGroupProcessor (SGROUP Groups → NETGUARD AddressGroups)
	// - NetworkProcessor (SGROUP Networks → NETGUARD Networks)

	// ========================================
	// START REVERSE SYNC SYSTEM
	// ========================================

	// Start reverse sync system
	go func() {
		if err := reverseSyncSystem.Start(ctx); err != nil {
			logger.Error("Failed to start reverse sync system", zap.Error(err))
			return
		}

		logger.Info("Reverse sync system started successfully")

		// Log system statistics periodically
		if cfg.ReverseSync.System.EnableMetrics {
			go func() {
				ticker := time.NewTicker(60 * time.Second)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if reverseSyncSystem.IsRunning() {
							stats := reverseSyncSystem.GetStats()
							processorCount := reverseSyncSystem.GetProcessorCount()
							logger.Info("Reverse sync statistics",
								zap.Int("processors", processorCount),
								zap.Int64("total_events", stats.TotalEvents),
								zap.Int64("processed_events", stats.ProcessedEvents),
								zap.Int64("failed_events", stats.FailedEvents))
						}
					}
				}
			}()
		}
	}()

	return reverseSyncSystem
}

// setupOutboxWorker creates and starts the OutboxWorker
func setupOutboxWorker(
	ctx context.Context,
	cfg *config.Config,
	pgRegistry *pg.Registry,
	syncManager interfaces.SyncManager,
	connMonitor *monitor.SGroupConnectionMonitor,
	logger *zap.Logger,
) *worker.OutboxWorker {
	// Load worker configuration from environment
	workerConfig := worker.LoadFromEnv()

	// Validate configuration
	if err := workerConfig.Validate(); err != nil {
		logger.Fatal("invalid worker config", zap.Error(err))
	}

	if !workerConfig.Enabled {
		return nil
	}

	// Get database pool from registry
	pool := pgRegistry.Pool()
	if pool == nil {
		logger.Fatal("cannot get database pool from registry")
	}

	// Create logger for syncers (logr adapter)
	logrLogger := stdr.New(log.Default())

	// Get SGroups client from config
	sgroupsClient, err := clients.NewSGroupsClient(cfg.Sync.SGroups)
	if err != nil {
		logger.Fatal("failed to create sgroups client for worker", zap.Error(err))
	}

	// Create syncers for Worker
	hostSyncer := syncers.NewHostSyncer(sgroupsClient, logrLogger)
	addressGroupSyncer := syncers.NewAddressGroupSyncer(sgroupsClient, logrLogger)
	networkSyncer := syncers.NewNetworkSyncer(sgroupsClient, logrLogger)
	serviceSyncer := syncers.NewServiceSyncer(sgroupsClient, logrLogger)
	svcSvcRuleSyncer := syncers.NewSvcSvcRuleSyncer(sgroupsClient, logrLogger)

	// Create OutboxWorker with ConnectionMonitor
	outboxWorker := worker.NewOutboxWorker(
		pool,
		pgRegistry,
		hostSyncer,
		addressGroupSyncer,
		networkSyncer,
		serviceSyncer,
		svcSvcRuleSyncer,
		logger,
		workerConfig,
		connMonitor,
	)

	go func() {
		if err := outboxWorker.Start(ctx); err != nil {
			if err != context.Canceled {
				logger.Error("OutboxWorker failed", zap.Error(err))
			}
		}
	}()

	return outboxWorker
}
