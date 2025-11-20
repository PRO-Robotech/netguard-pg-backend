package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"net"
	"net/http"
	"netguard-pg-backend/internal/api/netguard"
	"netguard-pg-backend/internal/app/server"
	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/application/services/conditions"
	"netguard-pg-backend/internal/config"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/domain/registry"
	"netguard-pg-backend/internal/infrastructure/repositories"
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
	"netguard-pg-backend/internal/watch"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/stdr"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
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
	cfg, err := config.NewConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}
	if *grpcAddr != "" {
		cfg.Settings.GRPCAddr = *grpcAddr
	}
	if *httpAddr != "" {
		cfg.Settings.HTTPAddr = *httpAddr
	}
	zapLogger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer zapLogger.Sync()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	var (
		pgRegistry   *pg.Registry
		sqlDB        *sql.DB
		watchManager *watch.Manager
	)
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
	if *pgURI != "" {
		sqlDB, err = sql.Open("postgres", *pgURI)
		if err != nil {
			log.Fatalf("Failed to open SQL connection: %v", err)
		}
		defer sqlDB.Close()
	}
	if err := registry.ValidateRegistry(); err != nil {
		zapLogger.Fatal("invalid resource registry", zap.Error(err))
	}
	var sgroupsClient interfaces.SGroupGateway
	var connMonitor *monitor.SGroupConnectionMonitor
	if cfg.Sync.Enabled {
		client, err := clients.NewSGroupsClient(cfg.Sync.SGroups)
		if err != nil {
			log.Fatalf("Failed to create sgroups client: %v", err)
		}
		sgroupsClient = client
		defer sgroupsClient.Close()
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
		if cfg.Sync.Required {
			zapLogger.Info("Waiting for SGROUP connection (required mode)...")
			if err := connMonitor.WaitForConnection(10 * time.Second); err != nil {
				log.Fatalf("SGROUP connection required but not established: %v", err)
			}
			zapLogger.Info("SGROUP connection established")
		} else {
			if connMonitor.IsConnected() {
				zapLogger.Info("SGROUP connection established")
			} else {
				zapLogger.Warn("SGROUP not connected, will retry in background")
			}
		}
	}
	syncManager := setupSyncManager(ctx, cfg, sgroupsClient, zapLogger)
	reverseSyncSystem := setupReverseSyncSystem(ctx, cfg, pgRegistry, sgroupsClient, connMonitor, zapLogger)
	outboxRepo := repositories.NewOutboxRepository(pgRegistry.Pool())
	conditionManager := conditions.NewConditionManager(pgRegistry, outboxRepo)
	netguardFacade := services.NewNetguardFacade(pgRegistry, conditionManager, syncManager)
	if sqlDB != nil {
		watchManager, err = watch.NewManagerWithConfig(ctx, sqlDB, *pgURI, netguardFacade, cfg.Watch.CacheSize, cfg.Watch.PGChannel)
		if err != nil {
			log.Fatalf("Failed to initialize watch manager: %v", err)
		}
		defer watchManager.Stop()
	}
	var outboxWorker *worker.OutboxWorker
	if syncManager != nil {
		outboxWorker = setupOutboxWorker(ctx, cfg, pgRegistry, syncManager, conditionManager, connMonitor, netguardFacade.AddressGroupResourceService(), zapLogger)
	}
	grpcServer := grpc.NewServer()
	netguardServer := netguard.NewServiceServer(netguardFacade, cfg.Watch)
	if watchManager != nil {
		netguardServer.SetWatchManager(watchManager)
	}
	netguardpb.RegisterNetguardServiceServer(grpcServer, netguardServer)
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("liveness", grpc_health_v1.HealthCheckResponse_SERVING)
	lis, err := net.Listen("tcp", cfg.Settings.GRPCAddr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()
	httpServer, httpMux, err := server.SetupServer(ctx, cfg.Settings.GRPCAddr, cfg.Settings.HTTPAddr, netguardFacade, cfg.Watch)
	if err != nil {
		log.Fatalf("Failed to setup server: %v", err)
	}
	if watchManager != nil {
		httpMux.Handle("/healthz/watch", watch.NewHealthHandler(watchManager))
	}
	if connMonitor != nil {
		healthListener := server.NewHealthEndpointListener()
		connMonitor.Subscribe(healthListener)
		httpMux.HandleFunc("/healthz/sync", healthListener.ServeHTTP)
	}
	if outboxWorker != nil {
		httpMux.HandleFunc("/healthz/worker", outboxWorker.HealthHandler)
	}
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
func setupSyncManager(ctx context.Context, cfg *config.Config, sgroupsClient interfaces.SGroupGateway, logger *zap.Logger) interfaces.SyncManager {
	if sgroupsClient == nil {
		return nil
	}
	syncConfig := cfg.Sync
	if err := syncConfig.Validate(); err != nil {
		logger.Error("sync config validation failed", zap.Error(err))
		return nil
	}
	if !syncConfig.Enabled {
		return nil
	}
	logrLogger := stdr.New(log.Default())
	syncManager := manager.NewSyncManager(sgroupsClient, logrLogger)
	addressGroupSyncer := syncers.NewAddressGroupSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeGroups, addressGroupSyncer); err != nil {
		logger.Error("failed to register AddressGroup syncer", zap.Error(err))
		return nil
	}
	networkSyncer := syncers.NewNetworkSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeNetworks, networkSyncer); err != nil {
		logger.Error("failed to register Network syncer", zap.Error(err))
		return nil
	}
	hostSyncer := syncers.NewHostSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeHosts, hostSyncer); err != nil {
		logger.Error("failed to register Host syncer", zap.Error(err))
		return nil
	}
	ieagagRuleSyncer := syncers.NewIEAgAgRuleSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeIEAgAgRules, ieagagRuleSyncer); err != nil {
		logger.Error("failed to register IEAgAgRule syncer", zap.Error(err))
		return nil
	}
	serviceSyncer := syncers.NewServiceSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeServices, serviceSyncer); err != nil {
		logger.Error("failed to register Service syncer", zap.Error(err))
		return nil
	}
	svcSvcRuleSyncer := syncers.NewSvcSvcRuleSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeSvcSvcRules, svcSvcRuleSyncer); err != nil {
		logger.Error("failed to register SvcSvcRule syncer", zap.Error(err))
		return nil
	}
	svcFqdnRuleSyncer := syncers.NewSvcFqdnRuleSyncer(sgroupsClient, logrLogger)
	if err := syncManager.RegisterSyncer(types.SyncSubjectTypeSvcFqdnRules, svcFqdnRuleSyncer); err != nil {
		logger.Error("failed to register SvcFqdnRule syncer", zap.Error(err))
		return nil
	}
	if err := syncManager.Start(ctx); err != nil {
		logger.Error("failed to start sync manager", zap.Error(err))
		return nil
	}
	return syncManager
}
func setupReverseSyncSystem(
	ctx context.Context,
	cfg *config.Config,
	pgRegistry *pg.Registry,
	sgroupsClient interfaces.SGroupGateway,
	connMonitor *monitor.SGroupConnectionMonitor,
	logger *zap.Logger,
) *manager.ReverseSyncManager {
	if sgroupsClient == nil || connMonitor == nil {
		logger.Info("Reverse sync disabled: sgroups client or connection monitor not available")
		return nil
	}
	if err := cfg.ReverseSync.Validate(); err != nil {
		logger.Error("Invalid reverse sync configuration", zap.Error(err))
		return nil
	}
	reverseSyncSystem := manager.NewReverseSyncManager(
		connMonitor,
		cfg.ReverseSync.Manager,
	)
	hostReader := adapters.NewPostgreSQLHostReader(pgRegistry)
	hostWriter := adapters.NewPostgreSQLHostWriter(pgRegistry)
	sgroupHostReader := sgroupsClient
	hostSyncConfig := synchronizer.DefaultHostSyncConfig()
	hostSynchronizer := synchronizer.NewHostSynchronizer(
		hostReader,
		hostWriter,
		sgroupHostReader,
		hostSyncConfig,
	)
	hostProcessorConfig := processors.DefaultHostProcessorConfig()
	hostProcessorConfig.EnableFullSyncOnChange = true
	hostProcessor := processors.NewHostProcessor(
		hostSynchronizer,
		hostProcessorConfig,
	)
	if err := reverseSyncSystem.RegisterProcessor(hostProcessor); err != nil {
		logger.Error("Failed to register HostProcessor", zap.Error(err))
		return nil
	}
	logger.Info("HostProcessor registered successfully for reverse sync")
	go func() {
		if err := reverseSyncSystem.Start(ctx); err != nil {
			logger.Error("Failed to start reverse sync system", zap.Error(err))
			return
		}
		logger.Info("Reverse sync system started successfully")
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
func setupOutboxWorker(
	ctx context.Context,
	cfg *config.Config,
	pgRegistry *pg.Registry,
	syncManager interfaces.SyncManager,
	conditionManager ports.ConditionManager,
	connMonitor *monitor.SGroupConnectionMonitor,
	portMappingRegenerator worker.PortMappingRegenerator,
	logger *zap.Logger,
) *worker.OutboxWorker {
	workerConfig := worker.LoadFromEnv()
	if err := workerConfig.Validate(); err != nil {
		logger.Fatal("invalid worker config", zap.Error(err))
	}
	if !workerConfig.Enabled {
		return nil
	}
	pool := pgRegistry.Pool()
	if pool == nil {
		logger.Fatal("cannot get database pool from registry")
	}
	logrLogger := stdr.New(log.Default())
	sgroupsClient, err := clients.NewSGroupsClient(cfg.Sync.SGroups)
	if err != nil {
		logger.Fatal("failed to create sgroups client for worker", zap.Error(err))
	}
	hostSyncer := syncers.NewHostSyncer(sgroupsClient, logrLogger)
	addressGroupSyncer := syncers.NewAddressGroupSyncer(sgroupsClient, logrLogger)
	networkSyncer := syncers.NewNetworkSyncer(sgroupsClient, logrLogger)
	serviceSyncer := syncers.NewServiceSyncer(sgroupsClient, logrLogger)
	svcSvcRuleSyncer := syncers.NewSvcSvcRuleSyncer(sgroupsClient, logrLogger)
	svcFqdnRuleSyncer := syncers.NewSvcFqdnRuleSyncer(sgroupsClient, logrLogger)
	outboxWorker := worker.NewOutboxWorker(
		pool,
		pgRegistry,
		hostSyncer,
		addressGroupSyncer,
		networkSyncer,
		serviceSyncer,
		svcSvcRuleSyncer,
		svcFqdnRuleSyncer,
		conditionManager,
		logger,
		workerConfig,
		connMonitor,
		portMappingRegenerator,
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
