package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"google.golang.org/protobuf/types/known/timestamppb"

	"netguard-pg-backend/internal/infrastructure/repositories/pg"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/monitor"
	"netguard-pg-backend/internal/sync/syncers"
	"netguard-pg-backend/internal/sync/types"
	"netguard-pg-backend/internal/sync/worker"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"
)

// HTTPSGroupGateway implements SGroupGateway using HTTP client (for E2E tests with MockSGROUPServer)
type HTTPSGroupGateway struct {
	baseURL string
	client  *http.Client
	logger  *zap.Logger
}

// NewHTTPSGroupGateway creates a new HTTP-based SGROUP gateway for testing
func NewHTTPSGroupGateway(baseURL string, logger *zap.Logger) *HTTPSGroupGateway {
	return &HTTPSGroupGateway{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Sync sends a synchronization request to SGROUP via HTTP
func (g *HTTPSGroupGateway) Sync(ctx context.Context, req *types.SyncRequest) error {
	// Convert sync request to HTTP JSON payload
	payload := map[string]interface{}{
		"operation":    string(req.Operation),
		"subject_type": string(req.SubjectType),
		"resource":     convertProtoToMap(req.Data),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal sync request: %w", err)
	}

	// Send HTTP POST request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", g.baseURL+"/api/v1/sync", bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send sync request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sync request failed with status %d", resp.StatusCode)
	}

	g.logger.Debug("Sync request successful",
		zap.String("operation", string(req.Operation)),
		zap.String("subject_type", string(req.SubjectType)))

	return nil
}

// Health checks the health of SGROUP service
func (g *HTTPSGroupGateway) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", g.baseURL+"/health", nil)
	if err != nil {
		return err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}

// GetStatuses returns a channel of timestamp updates (not implemented for HTTP mock)
func (g *HTTPSGroupGateway) GetStatuses(ctx context.Context) (chan *timestamppb.Timestamp, error) {
	ch := make(chan *timestamppb.Timestamp)
	close(ch) // Close immediately - not needed for E2E tests
	return ch, nil
}

// GetHostsByUUIDs retrieves hosts by UUIDs (not implemented for HTTP mock)
func (g *HTTPSGroupGateway) GetHostsByUUIDs(ctx context.Context, uuids []string) ([]*pb.Host, error) {
	return []*pb.Host{}, nil
}

// ListAllHosts retrieves all hosts (not implemented for HTTP mock)
func (g *HTTPSGroupGateway) ListAllHosts(ctx context.Context) ([]*pb.Host, error) {
	return []*pb.Host{}, nil
}

// GetHostsInSecurityGroup retrieves hosts in security groups (not implemented for HTTP mock)
func (g *HTTPSGroupGateway) GetHostsInSecurityGroup(ctx context.Context, sgNames []string) ([]*pb.Host, error) {
	return []*pb.Host{}, nil
}

// Close closes the HTTP client (no-op)
func (g *HTTPSGroupGateway) Close() error {
	return nil
}

// convertProtoToMap converts protobuf data to map for JSON serialization
func convertProtoToMap(data interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	switch v := data.(type) {
	case *pb.SecGroup:
		result["name"] = v.Name
		// Add other fields as needed

	case *pb.SyncSecurityGroups:
		if len(v.Groups) > 0 {
			// Use first group's data for simplicity
			result["name"] = v.Groups[0].Name
		}

	case *pb.Host:
		result["name"] = v.Name
		// Add other fields as needed

	case *pb.SyncHosts:
		if len(v.Hosts) > 0 {
			result["name"] = v.Hosts[0].Name
		}

	case *pb.Network:
		result["name"] = v.Name
		// Add other fields as needed

	case *pb.SyncNetworks:
		if len(v.Networks) > 0 {
			result["name"] = v.Networks[0].Name
		}

	case *pb.Service:
		result["name"] = v.Name
		// Add other fields as needed

	case *pb.SyncServices:
		if len(v.Services) > 0 {
			result["name"] = v.Services[0].Name
		}

	default:
		// Fallback: empty resource
		result["name"] = "unknown"
	}

	return result
}

// Verify that HTTPSGroupGateway implements SGroupGateway interface
var _ interfaces.SGroupGateway = (*HTTPSGroupGateway)(nil)

// ==================== Test Worker Helper ====================

// TCCreateTestWorker creates an OutboxWorker configured for E2E testing
//
// Usage:
//
//	tc := SetupTestEnvironment(t)
//	worker := TCCreateTestWorker(t, tc)
//	err := worker.ProcessOnce(context.Background())
func TCCreateTestWorker(t *testing.T, tc *TestContainer) *worker.OutboxWorker {
	t.Helper()

	ctx := context.Background()

	// Create test logger
	logger := zaptest.NewLogger(t)

	// Create HTTP gateway that talks to MockSGROUPServer
	gateway := NewHTTPSGroupGateway(tc.MockSGROUP.URL(), logger)

	// Create REAL PgRegistry (not mock!) - this allows Worker to load entities from DB
	registry, err := pg.NewRegistryFromURI(ctx, tc.ConnectionString)
	require.NoError(t, err, "failed to create PgRegistry")

	// Get pool from registry for OutboxWorker
	pool := registry.Pool()
	require.NotNil(t, pool, "registry pool should not be nil")

	// Create logr.Logger for syncers (they use logr, not zap)
	logrLogger := zapr.NewLogger(logger)

	// Create all syncers
	hostSyncer := syncers.NewHostSyncer(gateway, logrLogger)
	addressGroupSyncer := syncers.NewAddressGroupSyncer(gateway, logrLogger)
	networkSyncer := syncers.NewNetworkSyncer(gateway, logrLogger)
	serviceSyncer := syncers.NewServiceSyncer(gateway, logrLogger)
	svcSvcRuleSyncer := syncers.NewSvcSvcRuleSyncer(gateway, logrLogger)
	connMonitor := monitor.NewSGroupConnectionMonitor(gateway, monitor.DefaultConfig(), logger)

	// Create worker configuration optimized for testing
	config := &worker.WorkerConfig{
		Enabled:               true,
		PollInterval:          1 * time.Second, // Short interval for tests
		BatchSize:             10,              // Small batch size
		SGROUPTimeout:         5 * time.Second, // Timeout for SGROUP requests
		MaxConcurrentBatches:  1,               // Sequential processing
		MaxAttemptsValidation: 3,
		MaxAttemptsTemporary:  5,
		MaxAttemptsNetwork:    10,
		HealthCheckInterval:   30 * time.Second,
		MetricsEnabled:        false, // Disable metrics for tests
	}

	// Create OutboxWorker
	outboxWorker := worker.NewOutboxWorker(
		pool,
		registry,
		hostSyncer,
		addressGroupSyncer,
		networkSyncer,
		serviceSyncer,
		svcSvcRuleSyncer,
		nil,
		logger,
		config,
		connMonitor,
	)

	t.Logf("  🏗️  Created OutboxWorker for E2E testing")

	// Register cleanup to close registry (which closes the pool)
	t.Cleanup(func() {
		registry.Close()
	})

	return outboxWorker
}

// TCWaitForWorkerProcessing waits for worker to process outbox entries and verifies SGROUP received requests
//
// Usage:
//
//	worker := TCCreateTestWorker(t, tc)
//	// Create some resources...
//	TCWaitForWorkerProcessing(t, tc, worker, 1, 5*time.Second)
func TCWaitForWorkerProcessing(
	t *testing.T,
	tc *TestContainer,
	worker *worker.OutboxWorker,
	expectedSGROUPRequests int,
	timeout time.Duration,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			// Timeout - dump state for debugging
			t.Logf("  ⏱️  Timeout after %v", time.Since(startTime))
			TCLogOutboxState(t, tc.DB)
			tc.MockSGROUP.DumpRequests()
			t.Fatalf("❌ Timeout waiting for worker processing (expected %d SGROUP requests, got %d)",
				expectedSGROUPRequests, tc.MockSGROUP.GetRequestCount())

		case <-ticker.C:
			// Process one batch
			err := worker.ProcessOnce(context.Background())
			if err != nil {
				t.Logf("  ⚠️  Worker ProcessOnce error: %v", err)
			}

			// Check if expected requests received
			actualRequests := tc.MockSGROUP.GetRequestCount()
			if actualRequests >= expectedSGROUPRequests {
				t.Logf("  ✅ Worker processed entries: %d SGROUP requests received (took %v)",
					actualRequests, time.Since(startTime))
				return
			}
		}
	}
}

// TCProcessWorkerOnce is a shorthand for calling worker.ProcessOnce with error checking
func TCProcessWorkerOnce(t *testing.T, worker *worker.OutboxWorker) {
	t.Helper()

	err := worker.ProcessOnce(context.Background())
	require.NoError(t, err, "worker.ProcessOnce failed")
}
