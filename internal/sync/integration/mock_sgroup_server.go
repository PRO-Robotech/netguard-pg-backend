package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"netguard-pg-backend/internal/sync/types"
	"sync"
	"testing"
	"time"
)

type MockSGROUPServer struct {
	server    *httptest.Server
	mu        sync.RWMutex
	mode      SGROUPMode
	requests  []MockSGROUPRequest
	responses map[string]interface{}
	t         *testing.T
}
type SGROUPMode int

const (
	ModeHealthy SGROUPMode = iota
	ModeConnectionRefused
	ModeTimeout
	ModeServerError
	ModeRateLimited
)

type MockSGROUPRequest struct {
	Operation   types.SyncOperation
	SubjectType types.SyncSubjectType
	Resource    map[string]interface{}
	Timestamp   time.Time
}

func NewMockSGROUPServer(t *testing.T) *MockSGROUPServer {
	mock := &MockSGROUPServer{
		mode:      ModeHealthy,
		requests:  []MockSGROUPRequest{},
		responses: make(map[string]interface{}),
		t:         t,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/sync", mock.handleSync)
	mux.HandleFunc("/health", mock.handleHealth)
	mock.server = httptest.NewServer(mux)
	return mock
}
func (m *MockSGROUPServer) handleSync(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	mode := m.mode
	m.mu.RUnlock()
	switch mode {
	case ModeConnectionRefused:
		hijacker, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hijacker.Hijack()
			conn.Close()
		}
		return
	case ModeTimeout:
		m.t.Log("  🐌 Mock SGROUP: Simulating timeout...")
		time.Sleep(10 * time.Minute)
		return
	case ModeServerError:
		m.t.Log("  ❌ Mock SGROUP: Returning 500 error")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Internal server error",
		})
		return
	case ModeRateLimited:
		m.t.Log("  ⛔ Mock SGROUP: Returning 429 rate limited")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Too many requests",
		})
		return
	}
	var req struct {
		Operation   string                 `json:"operation"`
		SubjectType string                 `json:"subject_type"`
		Resource    map[string]interface{} `json:"resource"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.t.Logf("  ⚠️  Mock SGROUP: Failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid request body",
		})
		return
	}
	mockReq := MockSGROUPRequest{
		Operation:   types.SyncOperation(req.Operation),
		SubjectType: types.SyncSubjectType(req.SubjectType),
		Resource:    req.Resource,
		Timestamp:   time.Now(),
	}
	m.mu.Lock()
	m.requests = append(m.requests, mockReq)
	m.mu.Unlock()
	if req.Operation != "DELETE" {
		resourceKey := fmt.Sprintf("%s/%s", req.SubjectType, req.Resource["name"])
		m.mu.Lock()
		m.responses[resourceKey] = req.Resource
		m.mu.Unlock()
	}
	m.t.Logf("  ✅ Mock SGROUP: Received %s %s request", req.Operation, req.SubjectType)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
	})
}
func (m *MockSGROUPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	mode := m.mode
	m.mu.RUnlock()
	if mode == ModeConnectionRefused || mode == ModeTimeout || mode == ModeServerError {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "unhealthy",
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}
func (m *MockSGROUPServer) Start() {
	m.t.Logf("  🎭 Mock SGROUP server listening at: %s", m.server.URL)
}
func (m *MockSGROUPServer) Stop() {
	if m.server != nil {
		m.server.Close()
		m.t.Log("  🛑 Mock SGROUP server stopped")
	}
}
func (m *MockSGROUPServer) URL() string {
	return m.server.URL
}
func (m *MockSGROUPServer) SetMode(mode SGROUPMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mode = mode
	modeNames := map[SGROUPMode]string{
		ModeHealthy:           "Healthy",
		ModeConnectionRefused: "ConnectionRefused",
		ModeTimeout:           "Timeout",
		ModeServerError:       "ServerError",
		ModeRateLimited:       "RateLimited",
	}
	m.t.Logf("  🔧 Mock SGROUP mode set to: %s", modeNames[mode])
}
func (m *MockSGROUPServer) GetMode() SGROUPMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mode
}
func (m *MockSGROUPServer) GetRequests() []MockSGROUPRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]MockSGROUPRequest{}, m.requests...)
}
func (m *MockSGROUPServer) GetRequestsForOperation(op types.SyncOperation) []MockSGROUPRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var filtered []MockSGROUPRequest
	for _, req := range m.requests {
		if req.Operation == op {
			filtered = append(filtered, req)
		}
	}
	return filtered
}
func (m *MockSGROUPServer) GetRequestsForSubject(subject types.SyncSubjectType) []MockSGROUPRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var filtered []MockSGROUPRequest
	for _, req := range m.requests {
		if req.SubjectType == subject {
			filtered = append(filtered, req)
		}
	}
	return filtered
}
func (m *MockSGROUPServer) GetRequestCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.requests)
}
func (m *MockSGROUPServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = []MockSGROUPRequest{}
	m.responses = make(map[string]interface{})
	m.mode = ModeHealthy
	m.t.Log("  🔄 Mock SGROUP server reset")
}
func (m *MockSGROUPServer) AssertRequestCount(expected int) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	actual := len(m.requests)
	if actual != expected {
		return fmt.Errorf("expected %d requests, got %d", expected, actual)
	}
	return nil
}
func (m *MockSGROUPServer) AssertRequestReceived(operation types.SyncOperation, subjectType types.SyncSubjectType) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, req := range m.requests {
		if req.Operation == operation && req.SubjectType == subjectType {
			return nil
		}
	}
	return fmt.Errorf("no request found with operation=%s, subject_type=%s", operation, subjectType)
}
func (m *MockSGROUPServer) GetLastRequest() *MockSGROUPRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.requests) == 0 {
		return nil
	}
	return &m.requests[len(m.requests)-1]
}
func (m *MockSGROUPServer) GetLastRequestForOperation(op types.SyncOperation) *MockSGROUPRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := len(m.requests) - 1; i >= 0; i-- {
		if m.requests[i].Operation == op {
			return &m.requests[i]
		}
	}
	return nil
}
func (m *MockSGROUPServer) GetLastRequestForSubject(subject types.SyncSubjectType) *MockSGROUPRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := len(m.requests) - 1; i >= 0; i-- {
		if m.requests[i].SubjectType == subject {
			return &m.requests[i]
		}
	}
	return nil
}
func (m *MockSGROUPServer) GetRequestCountForOperation(op types.SyncOperation) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, req := range m.requests {
		if req.Operation == op {
			count++
		}
	}
	return count
}
func (m *MockSGROUPServer) GetRequestCountForSubject(subject types.SyncSubjectType) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, req := range m.requests {
		if req.SubjectType == subject {
			count++
		}
	}
	return count
}
func (m *MockSGROUPServer) WaitForRequest(op types.SyncOperation, subject types.SyncSubjectType, timeout time.Duration) (*MockSGROUPRequest, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m.mu.RLock()
		for i := len(m.requests) - 1; i >= 0; i-- {
			if m.requests[i].Operation == op && m.requests[i].SubjectType == subject {
				req := m.requests[i]
				m.mu.RUnlock()
				return &req, nil
			}
		}
		m.mu.RUnlock()
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for request: operation=%s, subject=%s", op, subject)
}
func (m *MockSGROUPServer) DumpRequests() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.t.Logf("  📋 Mock SGROUP Requests (%d total):", len(m.requests))
	for i, req := range m.requests {
		resourceName := "unknown"
		if name, ok := req.Resource["name"].(string); ok {
			resourceName = name
		}
		m.t.Logf("    [%d] %s %s (resource=%s) at %s",
			i+1, req.Operation, req.SubjectType, resourceName, req.Timestamp.Format("15:04:05.000"))
	}
}
