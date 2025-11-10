package worker

import (
	"context"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"
	"google.golang.org/protobuf/types/known/timestamppb"

	"netguard-pg-backend/internal/sync/types"
)

// mockSGroupGateway - mock для тестирования
type mockSGroupGateway struct{}

func (m *mockSGroupGateway) Sync(ctx context.Context, req *types.SyncRequest) error {
	return nil
}

func (m *mockSGroupGateway) Health(ctx context.Context) error {
	return nil
}

func (m *mockSGroupGateway) GetStatuses(ctx context.Context) (chan *timestamppb.Timestamp, error) {
	ch := make(chan *timestamppb.Timestamp)
	close(ch) // Empty channel for mock
	return ch, nil
}

func (m *mockSGroupGateway) Close() error {
	return nil
}

func (m *mockSGroupGateway) GetHostsByUUIDs(ctx context.Context, uuids []string) ([]*pb.Host, error) {
	return []*pb.Host{}, nil
}

func (m *mockSGroupGateway) ListAllHosts(ctx context.Context) ([]*pb.Host, error) {
	return []*pb.Host{}, nil
}

func (m *mockSGroupGateway) GetHostsInSecurityGroup(ctx context.Context, sgNames []string) ([]*pb.Host, error) {
	return []*pb.Host{}, nil
}
