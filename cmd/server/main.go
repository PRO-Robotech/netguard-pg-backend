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

	"netguard-pg-backend/internal/api/netguard"
	"netguard-pg-backend/internal/app/server"
	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/config"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
	"netguard-pg-backend/internal/sync/clients"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/manager"
	"netguard-pg-backend/internal/sync/syncers"
	"netguard-pg-backend/internal/sync/types"

	"github.com/go-logr/stdr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	//"netguard-pg-backend/internal/infrastructure/repositories/pg"
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
		sig := <-sigCh
		log.Printf("Received signal: %v", sig)
		cancel()
	}()

	// Create registry
	var registry ports.Registry
	if *memoryDB {
		log.Println("Using in-memory database")
		registry = mem.NewRegistry()
	} else if *pgURI != "" {
		log.Println("Using PostgreSQL database")

		//// Run migrations if requested
		//if *migrateDB {
		//	log.Println("Running database migrations")
		//	if err := pg.RunMigrations(*pgURI); err != nil {
		//		log.Fatalf("Failed to run migrations: %v", err)
		//	}
		//	log.Println("Migrations completed successfully")
		//}
		//
		//// Create PostgreSQL registry
		//pgRegistry, err := pg.NewRegistry(ctx, *pgURI)
		//if err != nil {
		//	log.Fatalf("Failed to create PostgreSQL registry: %v", err)
		//}
		//registry = pgRegistry
	} else {
		log.Fatal("Either --memory or --pg-uri must be specified")
	}
	defer registry.Close()

	// Setup sync manager
	syncManager := setupSyncManager(ctx, cfg)

	// Create service
	netguardService := services.NewNetguardService(registry, syncManager)

	// Setup gRPC server
	grpcServer := grpc.NewServer()
	netguardServer := netguard.NewNetguardServiceServer(netguardService)
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
		log.Printf("Starting gRPC server on %s", cfg.Settings.GRPCAddr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// Setup HTTP server with gRPC-Gateway
	httpServer, err := server.SetupServer(ctx, cfg.Settings.GRPCAddr, cfg.Settings.HTTPAddr, netguardService)
	if err != nil {
		log.Fatalf("Failed to setup server: %v", err)
	}

	// Start HTTP server
	go func() {
		log.Printf("Starting HTTP server on %s", cfg.Settings.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to serve HTTP: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	log.Println("Shutting down...")

	// Gracefully stop servers
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(context.Background()); err != nil {
		log.Printf("Failed to shutdown HTTP server: %v", err)
	}
}

// setupSyncManager creates and configures the sync manager for sgroups integration
func setupSyncManager(ctx context.Context, cfg *config.Config) interfaces.SyncManager {
	log.Println("🔧 DEBUG: Starting SyncManager setup")

	// Use sync configuration from loaded config
	syncConfig := cfg.Sync
	log.Printf("🔧 DEBUG: Sync config loaded - Enabled: %v, SGroups address: %s",
		syncConfig.Enabled, syncConfig.SGroups.GRPCAddress)

	// Validate configuration
	log.Println("🔧 DEBUG: Validating sync configuration")
	if err := syncConfig.Validate(); err != nil {
		log.Printf("❌ ERROR: Invalid sync configuration: %v", err)
		return nil
	}
	log.Println("✅ DEBUG: Sync configuration is valid")

	// Skip sync setup if disabled
	if !syncConfig.Enabled {
		log.Println("⚠️  DEBUG: Sync is disabled, skipping SyncManager setup")
		return nil
	}

	// Create SGroups client
	log.Printf("🔧 DEBUG: Creating SGroups client for address: %s", syncConfig.SGroups.GRPCAddress)
	sgroupsClient, err := clients.NewSGroupsClient(syncConfig.SGroups)
	if err != nil {
		log.Printf("❌ ERROR: Failed to create sgroups client: %v", err)
		return nil
	}
	log.Println("✅ DEBUG: SGroups client created successfully")

	// Test connection to sgroups
	log.Println("🔧 DEBUG: Testing connection to sgroups service")
	if err := sgroupsClient.Health(ctx); err != nil {
		log.Printf("❌ ERROR: Failed to connect to sgroups service: %v", err)
		return nil
	}
	log.Println("✅ DEBUG: Successfully connected to sgroups service")

	// Create logger for sync manager
	log.Println("🔧 DEBUG: Creating logger for sync manager")
	logger := stdr.New(log.Default())

	// Create sync manager
	log.Println("🔧 DEBUG: Creating sync manager")
	syncManager := manager.NewSyncManager(sgroupsClient, logger)
	log.Println("✅ DEBUG: Sync manager created successfully")

	// Register AddressGroup syncer
	log.Println("🔧 DEBUG: Creating and registering AddressGroup syncer")
	addressGroupSyncer := syncers.NewAddressGroupSyncer(sgroupsClient, logger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeGroups, addressGroupSyncer); err != nil {
		log.Printf("❌ ERROR: Failed to register AddressGroup syncer: %v", err)
		return nil
	}
	log.Println("✅ DEBUG: AddressGroup syncer registered successfully")

	// Start sync manager
	log.Println("🔧 DEBUG: Starting sync manager")
	if err := syncManager.Start(ctx); err != nil {
		log.Printf("❌ ERROR: Failed to start sync manager: %v", err)
		return nil
	}
	log.Println("✅ DEBUG: Sync manager started successfully")

	log.Println("🎉 SyncManager initialized successfully")
	return syncManager
}
