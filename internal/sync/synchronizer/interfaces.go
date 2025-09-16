package synchronizer

import (
	"context"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/sync/types"
)

// HostReader defines interface for reading host data from NETGUARD
type HostReader interface {
	// GetHostsWithoutIPSet returns hosts that don't have IPSet filled
	GetHostsWithoutIPSet(ctx context.Context, namespace string) ([]models.Host, error)

	// GetHostByUUID returns a host by its UUID
	GetHostByUUID(ctx context.Context, uuid string) (*models.Host, error)

	// ListHosts lists hosts by identifiers (namespace, name pairs)
	ListHosts(ctx context.Context, identifiers []HostIdentifier) ([]models.Host, error)
}

// HostWriter defines interface for updating host data in NETGUARD
type HostWriter interface {
	// UpdateHostIPSet updates the IPSet for a specific host
	UpdateHostIPSet(ctx context.Context, hostID string, ipSet []string) error

	// UpdateHostsIPSet updates IPSet for multiple hosts in batch
	UpdateHostsIPSet(ctx context.Context, updates []types.HostIPSetUpdate) error
}

// SGROUPHostReader defines interface for reading host data from SGROUP
type SGROUPHostReader interface {
	// GetHostsByUUIDs retrieves hosts from SGROUP by their UUIDs
	GetHostsByUUIDs(ctx context.Context, uuids []string) ([]*pb.Host, error)

	// ListAllHosts retrieves all hosts from SGROUP (for full sync)
	ListAllHosts(ctx context.Context) ([]*pb.Host, error)

	// GetHostsInSecurityGroup retrieves hosts belonging to specific security groups
	GetHostsInSecurityGroup(ctx context.Context, sgNames []string) ([]*pb.Host, error)
}

// HostSynchronizer defines interface for synchronizing hosts between NETGUARD and SGROUP
type HostSynchronizer interface {
	// SyncHosts synchronizes hosts for a specific namespace
	SyncHosts(ctx context.Context, namespace string) (*types.HostSyncResult, error)

	// SyncHostsByUUIDs synchronizes specific hosts by their UUIDs
	SyncHostsByUUIDs(ctx context.Context, uuids []string) (*types.HostSyncResult, error)

	// SyncAllHosts performs full synchronization of all hosts
	SyncAllHosts(ctx context.Context) (*types.HostSyncResult, error)
}

// HostIdentifier represents a host identifier (namespace/name pair)
type HostIdentifier struct {
	Namespace string
	Name      string
}

// HostSyncConfig holds configuration for host synchronization
type HostSyncConfig struct {
	// BatchSize is the number of hosts to process in each batch
	BatchSize int

	// MaxConcurrency is the maximum number of concurrent workers
	MaxConcurrency int

	// SyncTimeout is the timeout for synchronization operations
	SyncTimeout int // seconds

	// RetryAttempts is the number of retry attempts for failed operations
	RetryAttempts int

	// EnableIPSetValidation enables validation of IP addresses
	EnableIPSetValidation bool
}

// DefaultHostSyncConfig returns default configuration for host synchronization
func DefaultHostSyncConfig() HostSyncConfig {
	return HostSyncConfig{
		BatchSize:             50,
		MaxConcurrency:        5,
		SyncTimeout:           30, // 30 seconds
		RetryAttempts:         3,
		EnableIPSetValidation: true,
	}
}
