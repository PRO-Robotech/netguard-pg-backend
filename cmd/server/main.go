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
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
	"netguard-pg-backend/internal/infrastructure/repositories/pg"
	"netguard-pg-backend/internal/sync"
	"netguard-pg-backend/internal/sync/adapters"
	"netguard-pg-backend/internal/sync/clients"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/manager"
	"netguard-pg-backend/internal/sync/syncers"
	"netguard-pg-backend/internal/sync/types"

	"github.com/go-logr/stdr"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		_ = <-sigCh
		cancel()
	}()

	// Create registry
	var registry ports.Registry
	if *memoryDB {
		registry = mem.NewRegistry()
	} else if *pgURI != "" {

		// Migrations are now handled by separate Goose container (sgroups pattern)
		// The --migrate flag is deprecated but kept for compatibility
		if *migrateDB {
			log.Println("⚠️  DEPRECATED: --migrate flag is no longer needed. Migrations are handled by separate Goose container.")
			log.Println("✅ Assuming migrations were completed by the netguard-migrations Kubernetes Job")
		}

		// Create PostgreSQL registry (fixed after Docker image cache issue)
		log.Println("Creating PostgreSQL registry...")
		pgRegistry, err := pg.NewRegistryFromURI(ctx, *pgURI)
		if err != nil {
			log.Fatalf("Failed to create PostgreSQL registry: %v", err)
		}
		if pgRegistry == nil {
			log.Fatalf("PostgreSQL registry is nil!")
		}
		log.Println("PostgreSQL registry created successfully")
		registry = pgRegistry
	} else {
		log.Fatal("Either --memory or --pg-uri must be specified")
	}
	defer registry.Close()

	// Setup sync manager
	syncManager := setupSyncManager(ctx, cfg)

	// Setup reverse sync system (SGROUP -> NETGUARD synchronization)
	reverseSyncSystem := setupReverseSyncSystem(ctx, cfg, registry, syncManager)

	// Create condition manager (needed for facade)
	conditionManager := services.NewConditionManager(registry)

	// Create facade service (new architecture)
	netguardFacade := services.NewNetguardFacade(registry, conditionManager, syncManager)

	// Using immediate force sync approach instead of finalizers

	// Setup gRPC server
	grpcServer := grpc.NewServer()
	netguardServer := netguard.NewNetguardServiceServer(netguardFacade)
	netguardpb.RegisterNetguardServiceServer(grpcServer, netguardServer)

	// Register gRPC health check service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

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

	// Start HTTP server
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to serve HTTP: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Gracefully stop services

	// Stop reverse sync system first
	if reverseSyncSystem != nil {
		if err := reverseSyncSystem.Stop(); err != nil {
		}
	}

	// Stop gRPC and HTTP servers
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(context.Background()); err != nil {
	}
}

// setupSyncManager creates and configures the sync manager for sgroups integration
func setupSyncManager(ctx context.Context, cfg *config.Config) interfaces.SyncManager {
	// Use sync configuration from loaded config
	syncConfig := cfg.Sync

	// Validate configuration
	if err := syncConfig.Validate(); err != nil {
		return nil
	}

	// Skip sync setup if disabled
	if !syncConfig.Enabled {
		return nil
	}

	// Create SGroups client
	sgroupsClient, err := clients.NewSGroupsClient(syncConfig.SGroups)
	if err != nil {
		return nil
	}

	// Test connection to sgroups
	if err := sgroupsClient.Health(ctx); err != nil {
		return nil
	}

	// Create logger for sync manager
	logger := stdr.New(log.Default())

	// Create sync manager
	syncManager := manager.NewSyncManager(sgroupsClient, logger)

	// Register AddressGroup syncer
	addressGroupSyncer := syncers.NewAddressGroupSyncer(sgroupsClient, logger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeGroups, addressGroupSyncer); err != nil {
		return nil
	}

	// Register Network syncer
	networkSyncer := syncers.NewNetworkSyncer(sgroupsClient, logger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeNetworks, networkSyncer); err != nil {
		return nil
	}

	// Register Host syncer
	hostSyncer := syncers.NewHostSyncer(sgroupsClient, logger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeHosts, hostSyncer); err != nil {
		return nil
	}

	// Register IEAgAgRule syncer
	ieagagRuleSyncer := syncers.NewIEAgAgRuleSyncer(sgroupsClient, logger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeIEAgAgRules, ieagagRuleSyncer); err != nil {
		return nil
	}

	// Start sync manager
	if err := syncManager.Start(ctx); err != nil {
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
