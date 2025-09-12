package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"netguard-pg-backend/internal/api/netguard"
	"netguard-pg-backend/internal/application/services"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// SetupServer sets up the HTTP server with gRPC-Gateway and Swagger UI
func SetupServer(ctx context.Context, grpcAddr string, httpAddr string, service *services.NetguardFacade) (*http.Server, error) {
	// Create gRPC server
	grpcServer := grpc.NewServer()
	netguardServer := netguard.NewNetguardServiceServer(service)
	netguardpb.RegisterNetguardServiceServer(grpcServer, netguardServer)

	gwmux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				EmitUnpopulated: true,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: false,
			},
		}),
	)

	// Register handlers for gRPC-Gateway
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	err := netguardpb.RegisterNetguardServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to register netguard gateway")
	}

	httpMux := http.NewServeMux()

	swaggerDir := http.Dir("./swagger-ui")
	fileServer := http.FileServer(swaggerDir)
	httpMux.Handle("/swagger/", http.StripPrefix("/swagger/", fileServer))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/swagger/") {
			httpMux.ServeHTTP(w, r)
			return
		}

		gwmux.ServeHTTP(w, r)
	})

	httpServer := &http.Server{
		Addr:    httpAddr,
		Handler: handler,
	}

	return httpServer, nil
}
