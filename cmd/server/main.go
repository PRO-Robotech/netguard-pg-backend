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
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	//"netguard-pg-backend/internal/infrastructure/repositories/pg"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

var (
	memoryDB  = flag.Bool("memory", false, "Use in-memory database")
	pgURI     = flag.String("pg-uri", "", "PostgreSQL connection URI")
	migrateDB = flag.Bool("migrate", false, "Run database migrations")
	grpcAddr  = flag.String("grpc-addr", ":9090", "gRPC server address")
	httpAddr  = flag.String("http-addr", ":8080", "HTTP server address")
)

func main() {
	flag.Parse()

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

	// Create service
	netguardService := services.NewNetguardService(registry)

	// Setup gRPC server
	grpcServer := grpc.NewServer()
	netguardServer := netguard.NewNetguardServiceServer(netguardService)
	netguardpb.RegisterNetguardServiceServer(grpcServer, netguardServer)

	// Register gRPC health check service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Start gRPC server
	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	go func() {
		log.Printf("Starting gRPC server on %s", *grpcAddr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// Setup HTTP server with gRPC-Gateway
	httpServer, err := server.SetupServer(ctx, *grpcAddr, *httpAddr, netguardService)
	if err != nil {
		log.Fatalf("Failed to setup server: %v", err)
	}

	// Start HTTP server
	go func() {
		log.Printf("Starting HTTP server on %s", *httpAddr)
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
