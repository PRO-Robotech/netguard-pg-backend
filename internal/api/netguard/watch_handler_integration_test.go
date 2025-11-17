package netguard

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/config"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/watch"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

const bufSize = 1024 * 1024

func TestWatchServicesStream(t *testing.T) {
	cache := watch.NewWatchCache("services", 16)
	obj := &netguardv1beta1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "demo",
			Namespace:       "default",
			ResourceVersion: "1",
		},
	}
	require.NoError(t, cache.Add(obj, 1))

	provider := &fakeCacheProvider{
		caches: map[string]*watch.WatchCache{
			"services": cache,
		},
	}

	cfg := config.DefaultWatchConfig()
	cfg.PollInterval = time.Millisecond
	cfg.BookmarkInterval = time.Hour

	server := &ServiceServer{
		watchConfig: cfg,
	}
	server.SetWatchManager(provider)

	listener := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	netguardpb.RegisterNetguardServiceServer(grpcServer, server)
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := netguardpb.NewNetguardServiceClient(conn)

	stream, err := client.WatchServices(ctx, &netguardpb.WatchRequest{ResourceVersion: "0"})
	require.NoError(t, err)

	event, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, netguardpb.WatchEventType_WATCH_EVENT_TYPE_ADDED, event.GetType())
	require.Equal(t, "demo", event.GetService().GetSelfRef().GetName())
}
