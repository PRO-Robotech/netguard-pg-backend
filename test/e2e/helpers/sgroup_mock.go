package helpers

import (
	"context"
	"fmt"
	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"netguard-pg-backend/internal/sync/types"
	"sync"
	"time"
)

type MockSGroupGateway struct {
	failureMode  string
	failureCount int
	requestDelay time.Duration
	callCount    int
	syncedHosts  map[string]*pb.Host
	syncedGroups map[string]*pb.SecGroup
	syncRequests []*types.SyncRequest
	lastSyncTime time.Time
	mu           sync.Mutex
}

func NewMockSGroupGateway() *MockSGroupGateway {
	return &MockSGroupGateway{
		failureMode:  "none",
		failureCount: 0,
		requestDelay: 10 * time.Millisecond,
		syncedHosts:  make(map[string]*pb.Host),
		syncedGroups: make(map[string]*pb.SecGroup),
		syncRequests: make([]*types.SyncRequest, 0),
		lastSyncTime: time.Now(),
	}
}
func (m *MockSGroupGateway) SetFailureMode(mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureMode = mode
}
func (m *MockSGroupGateway) SetFailureCount(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureCount = count
}
func (m *MockSGroupGateway) SetRequestDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestDelay = delay
}
func (m *MockSGroupGateway) Sync(ctx context.Context, req *types.SyncRequest) error {
	m.mu.Lock()
	m.callCount++
	currentCount := m.callCount
	delay := m.requestDelay
	mode := m.failureMode
	failCount := m.failureCount
	m.syncRequests = append(m.syncRequests, req)
	m.mu.Unlock()
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	if currentCount <= failCount {
		switch mode {
		case "timeout":
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(30 * time.Second):
				return ctx.Err()
			}
		case "unavailable":
			return status.Error(codes.Unavailable, "SGROUP unavailable")
		case "validation":
			return status.Error(codes.InvalidArgument, "Invalid request")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch req.SubjectType {
	case types.SyncSubjectTypeHosts:
		if hosts, ok := req.Data.(*pb.SyncHosts); ok {
			for _, host := range hosts.Hosts {
				m.syncedHosts[host.Name] = host
			}
		}
	case types.SyncSubjectTypeGroups:
		if groups, ok := req.Data.(*pb.SyncSecurityGroups); ok {
			for _, group := range groups.Groups {
				m.syncedGroups[group.Name] = group
			}
		}
	}
	m.lastSyncTime = time.Now()
	return nil
}
func (m *MockSGroupGateway) Health(ctx context.Context) error {
	m.mu.Lock()
	mode := m.failureMode
	m.mu.Unlock()
	if mode == "unavailable" {
		return status.Error(codes.Unavailable, "SGROUP unavailable")
	}
	return nil
}
func (m *MockSGroupGateway) GetStatuses(ctx context.Context) (chan *timestamppb.Timestamp, error) {
	statusChan := make(chan *timestamppb.Timestamp, 10)
	go func() {
		defer close(statusChan)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.mu.Lock()
				timestamp := timestamppb.New(m.lastSyncTime)
				m.mu.Unlock()
				select {
				case statusChan <- timestamp:
				case <-ctx.Done():
					return
				default:
				}
			}
		}
	}()
	return statusChan, nil
}
func (m *MockSGroupGateway) GetHostsByUUIDs(ctx context.Context, uuids []string) ([]*pb.Host, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*pb.Host
	for _, host := range m.syncedHosts {
		result = append(result, host)
	}
	return result, nil
}
func (m *MockSGroupGateway) ListAllHosts(ctx context.Context) ([]*pb.Host, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*pb.Host
	for _, host := range m.syncedHosts {
		result = append(result, host)
	}
	return result, nil
}
func (m *MockSGroupGateway) GetHostsInSecurityGroup(ctx context.Context, sgNames []string) ([]*pb.Host, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*pb.Host
	for _, host := range m.syncedHosts {
		result = append(result, host)
	}
	return result, nil
}
func (m *MockSGroupGateway) Close() error {
	return nil
}
func (m *MockSGroupGateway) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}
func (m *MockSGroupGateway) GetLastRequest() *types.SyncRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.syncRequests) == 0 {
		return nil
	}
	return m.syncRequests[len(m.syncRequests)-1]
}
func (m *MockSGroupGateway) GetSyncedHost(name string) (*pb.Host, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	host, ok := m.syncedHosts[name]
	return host, ok
}
func (m *MockSGroupGateway) GetSyncedGroup(name string) (*pb.SecGroup, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	group, ok := m.syncedGroups[name]
	return group, ok
}
func (m *MockSGroupGateway) GetAllSyncedHosts() map[string]*pb.Host {
	m.mu.Lock()
	defer m.mu.Unlock()
	hosts := make(map[string]*pb.Host, len(m.syncedHosts))
	for k, v := range m.syncedHosts {
		hosts[k] = v
	}
	return hosts
}
func (m *MockSGroupGateway) GetAllSyncedGroups() map[string]*pb.SecGroup {
	m.mu.Lock()
	defer m.mu.Unlock()
	groups := make(map[string]*pb.SecGroup, len(m.syncedGroups))
	for k, v := range m.syncedGroups {
		groups[k] = v
	}
	return groups
}
func (m *MockSGroupGateway) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount = 0
	m.syncedHosts = make(map[string]*pb.Host)
	m.syncedGroups = make(map[string]*pb.SecGroup)
	m.syncRequests = make([]*types.SyncRequest, 0)
	m.lastSyncTime = time.Now()
}
func (m *MockSGroupGateway) VerifyHostSynced(name string, expectedBound bool, expectedSGName string) error {
	host, found := m.GetSyncedHost(name)
	if !found {
		return fmt.Errorf("host %s not found in synced hosts", name)
	}
	if host.Name != name {
		return fmt.Errorf("host name mismatch: expected %s, got %s", name, host.Name)
	}
	return nil
}
func (m *MockSGroupGateway) VerifyGroupSynced(name string) error {
	_, found := m.GetSyncedGroup(name)
	if !found {
		return fmt.Errorf("security group %s not found in synced groups", name)
	}
	return nil
}
