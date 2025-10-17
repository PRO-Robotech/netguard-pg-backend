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
	"netguard-pg-backend/internal/config"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/domain/registry"
	"netguard-pg-backend/internal/infrastructure/repositories/pg"
	"netguard-pg-backend/internal/sync"
	"netguard-pg-backend/internal/sync/adapters"
	"netguard-pg-backend/internal/sync/clients"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/manager"
	"netguard-pg-backend/internal/sync/syncers"
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

	// Setup sync manager
	syncManager := setupSyncManager(ctx, cfg, zapLogger)

	// Setup reverse sync system (SGROUP -> NETGUARD synchronization)
	reverseSyncSystem := setupReverseSyncSystem(ctx, cfg, pgRegistry, syncManager)

	var outboxWorker *worker.OutboxWorker
	if syncManager != nil {
		outboxWorker = setupOutboxWorker(ctx, cfg, pgRegistry, syncManager, zapLogger)
	}

	// Create condition manager (needed for facade)
	conditionManager := services.NewConditionManager(pgRegistry)

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
	httpServer, err := server.SetupServer(ctx, cfg.Settings.GRPCAddr, cfg.Settings.HTTPAddr, netguardFacade)
	if err != nil {
		log.Fatalf("Failed to setup server: %v", err)
	}

	if outboxWorker != nil {
		http.HandleFunc("/healthz/worker", outboxWorker.HealthHandler)
	}

	workerConfig := worker.LoadFromEnv()
	if workerConfig.MetricsEnabled {
		http.Handle("/metrics", promhttp.Handler())
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
func setupSyncManager(ctx context.Context, cfg *config.Config, logger *zap.Logger) interfaces.SyncManager {
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

	// Create SGroups client
	sgroupsClient, err := clients.NewSGroupsClient(syncConfig.SGroups)
	if err != nil {
		logger.Error("failed to create sgroups client", zap.Error(err))
		return nil
	}

	// Test connection to sgroups
	if err := sgroupsClient.Health(ctx); err != nil {
		logger.Error("sgroups health check failed", zap.Error(err))
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

	if err := syncManager.Start(ctx); err != nil {
		logger.Error("failed to start sync manager", zap.Error(err))
		return nil
	}

	return syncManager
}

// setupReverseSyncSystem creates and configures the reverse sync system for SGROUP -> NETGUARD synchronization
func setupReverseSyncSystem(ctx context.Context, cfg *config.Config, registry ports.Registry, syncManager interfaces.SyncManager) *sync.ReverseSyncSystem {

	// Skip setup if sync manager is not available (sync disabled)
	if syncManager == nil {
		return nil
	}

	// Validate reverse sync configuration
	if err := cfg.ReverseSync.Validate(); err != nil {
		return nil
	}

	// Create SGROUP gateway using existing sync configuration
	sgroupsClient, err := clients.NewSGroupsClient(cfg.Sync.SGroups)
	if err != nil {
		return nil
	}

	// Test connection to SGROUP
	if err := sgroupsClient.Health(ctx); err != nil {
		return nil
	}

	// Create PostgreSQL adapters
	hostReader := adapters.NewPostgreSQLHostReader(registry)
	hostWriter := adapters.NewPostgreSQLHostWriter(registry)

	// Create reverse sync system
	reverseSyncSystem, err := sync.NewReverseSyncSystem(
		sgroupsClient,
		hostReader,
		hostWriter,
		cfg.ReverseSync,
	)
	if err != nil {
		return nil
	}

	// Start reverse sync system
	go func() {
		if err := reverseSyncSystem.Start(ctx); err != nil {
			return
		}

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
							_ = reverseSyncSystem.GetStats()
						}
					}
				}
			}()
		}
	}()

	return reverseSyncSystem
}

// setupOutboxWorker creates and starts the OutboxWorker
func setupOutboxWorker(ctx context.Context, cfg *config.Config, pgRegistry *pg.Registry, syncManager interfaces.SyncManager, logger *zap.Logger) *worker.OutboxWorker {
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

	// Create SGroups client
	sgroupsClient, err := clients.NewSGroupsClient(cfg.Sync.SGroups)
	if err != nil {
		logger.Fatal("failed to create sgroups client for worker", zap.Error(err))
	}

	// Create logger for syncers (logr adapter)
	logrLogger := stdr.New(log.Default())

	// Create syncers for Worker
	hostSyncer := syncers.NewHostSyncer(sgroupsClient, logrLogger)
	addressGroupSyncer := syncers.NewAddressGroupSyncer(sgroupsClient, logrLogger)
	networkSyncer := syncers.NewNetworkSyncer(sgroupsClient, logrLogger)
	serviceSyncer := syncers.NewServiceSyncer(sgroupsClient, logrLogger)

	// Create OutboxWorker
	outboxWorker := worker.NewOutboxWorker(
		pool,
		pgRegistry,
		hostSyncer,
		addressGroupSyncer,
		networkSyncer,
		serviceSyncer,
		logger,
		workerConfig,
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
