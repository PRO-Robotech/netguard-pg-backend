package integration

import (
	"context"
	"fmt"
	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"
	"google.golang.org/protobuf/types/known/timestamppb"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
	"sync"
)

type MockSGROUPClient struct {
	mu                              sync.Mutex
	SyncCalls                       []SyncCall
	GetHostsByUUIDsCalls            []GetHostsByUUIDsCall
	ListAllHostsCalls               int
	GetHostsInSecurityGroupCalls    []GetHostsInSGCall
	SyncResponse                    func(req *types.SyncRequest) error
	GetHostsByUUIDsResponse         func(uuids []string) ([]*pb.Host, error)
	ListAllHostsResponse            func() ([]*pb.Host, error)
	GetHostsInSecurityGroupResponse func(sgNames []string) ([]*pb.Host, error)
	NextErrorToReturn               error
	HealthStatus                    error
	StatusesChan                    chan *timestamppb.Timestamp
}
type SyncCall struct {
	Request   *types.SyncRequest
	Operation types.SyncOperation
	Subject   types.SyncSubjectType
	Error     error
}
type GetHostsByUUIDsCall struct {
	UUIDs []string
	Error error
}
type GetHostsInSGCall struct {
	SGNames []string
	Error   error
}

func NewMockSGROUPClient() *MockSGROUPClient {
	return &MockSGROUPClient{
		SyncCalls:                    []SyncCall{},
		GetHostsByUUIDsCalls:         []GetHostsByUUIDsCall{},
		GetHostsInSecurityGroupCalls: []GetHostsInSGCall{},
		ListAllHostsCalls:            0,
		StatusesChan:                 make(chan *timestamppb.Timestamp, 100),
	}
}
func (m *MockSGROUPClient) Sync(ctx context.Context, req *types.SyncRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	call := SyncCall{
		Request:   req,
		Operation: req.Operation,
		Subject:   req.SubjectType,
	}
	if m.NextErrorToReturn != nil {
		call.Error = m.NextErrorToReturn
		m.SyncCalls = append(m.SyncCalls, call)
		err := m.NextErrorToReturn
		m.NextErrorToReturn = nil
		return err
	}
	if m.SyncResponse != nil {
		call.Error = m.SyncResponse(req)
		m.SyncCalls = append(m.SyncCalls, call)
		return call.Error
	}
	m.SyncCalls = append(m.SyncCalls, call)
	return nil
}
func (m *MockSGROUPClient) Health(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.HealthStatus
}
func (m *MockSGROUPClient) GetStatuses(ctx context.Context) (chan *timestamppb.Timestamp, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.StatusesChan, nil
}
func (m *MockSGROUPClient) GetHostsByUUIDs(ctx context.Context, uuids []string) ([]*pb.Host, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	call := GetHostsByUUIDsCall{
		UUIDs: uuids,
	}
	if m.NextErrorToReturn != nil {
		call.Error = m.NextErrorToReturn
		m.GetHostsByUUIDsCalls = append(m.GetHostsByUUIDsCalls, call)
		err := m.NextErrorToReturn
		m.NextErrorToReturn = nil
		return nil, err
	}
	if m.GetHostsByUUIDsResponse != nil {
		hosts, err := m.GetHostsByUUIDsResponse(uuids)
		call.Error = err
		m.GetHostsByUUIDsCalls = append(m.GetHostsByUUIDsCalls, call)
		return hosts, err
	}
	m.GetHostsByUUIDsCalls = append(m.GetHostsByUUIDsCalls, call)
	return []*pb.Host{}, nil
}
func (m *MockSGROUPClient) ListAllHosts(ctx context.Context) ([]*pb.Host, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ListAllHostsCalls++
	if m.NextErrorToReturn != nil {
		err := m.NextErrorToReturn
		m.NextErrorToReturn = nil
		return nil, err
	}
	if m.ListAllHostsResponse != nil {
		return m.ListAllHostsResponse()
	}
	return []*pb.Host{}, nil
}
func (m *MockSGROUPClient) GetHostsInSecurityGroup(ctx context.Context, sgNames []string) ([]*pb.Host, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	call := GetHostsInSGCall{
		SGNames: sgNames,
	}
	if m.NextErrorToReturn != nil {
		call.Error = m.NextErrorToReturn
		m.GetHostsInSecurityGroupCalls = append(m.GetHostsInSecurityGroupCalls, call)
		err := m.NextErrorToReturn
		m.NextErrorToReturn = nil
		return nil, err
	}
	if m.GetHostsInSecurityGroupResponse != nil {
		hosts, err := m.GetHostsInSecurityGroupResponse(sgNames)
		call.Error = err
		m.GetHostsInSecurityGroupCalls = append(m.GetHostsInSecurityGroupCalls, call)
		return hosts, err
	}
	m.GetHostsInSecurityGroupCalls = append(m.GetHostsInSecurityGroupCalls, call)
	return []*pb.Host{}, nil
}
func (m *MockSGROUPClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.StatusesChan != nil {
		close(m.StatusesChan)
		m.StatusesChan = nil
	}
	return nil
}
func (m *MockSGROUPClient) GetSyncCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.SyncCalls)
}
func (m *MockSGROUPClient) GetSyncCallsForSubject(subject types.SyncSubjectType) []SyncCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	var filtered []SyncCall
	for _, call := range m.SyncCalls {
		if call.Subject == subject {
			filtered = append(filtered, call)
		}
	}
	return filtered
}
func (m *MockSGROUPClient) GetSyncCallsForOperation(operation types.SyncOperation) []SyncCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	var filtered []SyncCall
	for _, call := range m.SyncCalls {
		if call.Operation == operation {
			filtered = append(filtered, call)
		}
	}
	return filtered
}
func (m *MockSGROUPClient) GetLastSyncCall() *SyncCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.SyncCalls) == 0 {
		return nil
	}
	return &m.SyncCalls[len(m.SyncCalls)-1]
}
func (m *MockSGROUPClient) SimulateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NextErrorToReturn = err
}
func (m *MockSGROUPClient) SetHealthStatus(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HealthStatus = err
}
func (m *MockSGROUPClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SyncCalls = []SyncCall{}
	m.GetHostsByUUIDsCalls = []GetHostsByUUIDsCall{}
	m.GetHostsInSecurityGroupCalls = []GetHostsInSGCall{}
	m.ListAllHostsCalls = 0
	m.NextErrorToReturn = nil
	m.HealthStatus = nil
	if m.StatusesChan != nil {
		close(m.StatusesChan)
	}
	m.StatusesChan = make(chan *timestamppb.Timestamp, 100)
}
func (m *MockSGROUPClient) AssertSyncCallCount(expected int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	actual := len(m.SyncCalls)
	if actual != expected {
		return fmt.Errorf("expected %d Sync calls, got %d", expected, actual)
	}
	return nil
}
func (m *MockSGROUPClient) AssertSyncCalledWith(subject types.SyncSubjectType, operation types.SyncOperation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, call := range m.SyncCalls {
		if call.Subject == subject && call.Operation == operation {
			return nil
		}
	}
	return fmt.Errorf("Sync was not called with subject=%s, operation=%s", subject, operation)
}

var _ interfaces.SGroupGateway = (*MockSGROUPClient)(nil)
