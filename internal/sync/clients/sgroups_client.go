package clients

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/H-BF/protos/pkg/api/common"
	pb "github.com/H-BF/protos/pkg/api/sgroups"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/emptypb"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// SGroupsConfig holds configuration for sgroups client
type SGroupsConfig struct {
	GRPCAddress    string          `yaml:"grpc_address" env:"SGROUPS_GRPC_ADDRESS"`
	RequestTimeout time.Duration   `yaml:"request_timeout" env:"SGROUPS_REQUEST_TIMEOUT"`
	KeepAlive      KeepAliveConfig `yaml:"keep_alive"`
	TLS            TLSConfig       `yaml:"tls"`
}

// KeepAliveConfig holds gRPC keep-alive configuration
type KeepAliveConfig struct {
	Time                time.Duration `yaml:"time"`
	Timeout             time.Duration `yaml:"timeout"`
	PermitWithoutStream bool          `yaml:"permit_without_stream"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	CertFile           string `yaml:"cert_file"`
	KeyFile            string `yaml:"key_file"`
	CAFile             string `yaml:"ca_file"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

// sgroupsClient implements SGroupGateway interface
type sgroupsClient struct {
	conn   *grpc.ClientConn
	client pb.SecGroupServiceClient
	config SGroupsConfig
}

// NewSGroupsClient creates a new sgroups client
func NewSGroupsClient(config SGroupsConfig) (interfaces.SGroupGateway, error) {
	// Set default values
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.KeepAlive.Time == 0 {
		config.KeepAlive.Time = 30 * time.Second
	}
	if config.KeepAlive.Timeout == 0 {
		config.KeepAlive.Timeout = 5 * time.Second
	}

	// Create gRPC connection options
	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                config.KeepAlive.Time,
			Timeout:             config.KeepAlive.Timeout,
			PermitWithoutStream: config.KeepAlive.PermitWithoutStream,
		}),
	}

	// Configure TLS or insecure connection
	if config.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: config.TLS.InsecureSkipVerify,
		}

		// Load client certificate if provided
		if config.TLS.CertFile != "" && config.TLS.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(config.TLS.CertFile, config.TLS.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to load client certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		// Load CA certificate if provided
		if config.TLS.CAFile != "" {
			caCert, err := os.ReadFile(config.TLS.CAFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA certificate: %w", err)
			}

			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("failed to add CA certificate to pool")
			}
			tlsConfig.RootCAs = caCertPool
		}

		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Establish connection
	conn, err := grpc.Dial(config.GRPCAddress, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to sgroups service at %s: %w", config.GRPCAddress, err)
	}

	return &sgroupsClient{
		conn:   conn,
		client: pb.NewSecGroupServiceClient(conn),
		config: config,
	}, nil
}

// Sync sends a synchronization request to sgroups
func (c *sgroupsClient) Sync(ctx context.Context, req *types.SyncRequest) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	// Convert sync request to protobuf format
	pbReq, err := c.convertSyncRequestToProto(req)
	if err != nil {
		return fmt.Errorf("failed to convert sync request to proto: %w", err)
	}

	// Real GRPC call to sgroups service
	_, err = c.client.Sync(ctx, pbReq)
	return err
}

// Health checks the health of sgroups service
func (c *sgroupsClient) Health(ctx context.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	// Real health check using SyncStatuses
	_, err := c.client.SyncStatuses(ctx, &emptypb.Empty{})
	return err
}

// Close closes the gRPC connection
func (c *sgroupsClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// convertSyncRequestToProto converts sync request to protobuf format
func (c *sgroupsClient) convertSyncRequestToProto(req *types.SyncRequest) (*pb.SyncReq, error) {
	pbReq := &pb.SyncReq{}

	// Convert SyncOperation to pb.SyncReq_SyncOp
	switch req.Operation {
	case types.SyncOperationNoOp:
		pbReq.SyncOp = pb.SyncReq_NoOp
	case types.SyncOperationFullSync:
		pbReq.SyncOp = pb.SyncReq_FullSync
	case types.SyncOperationUpsert:
		pbReq.SyncOp = pb.SyncReq_Upsert
	case types.SyncOperationDelete:
		pbReq.SyncOp = pb.SyncReq_Delete
	default:
		return nil, fmt.Errorf("unknown sync operation: %s", req.Operation)
	}

	// Convert subject based on SubjectType
	switch req.SubjectType {
	case types.SyncSubjectTypeGroups:
		if groups, ok := req.Data.(*pb.SyncSecurityGroups); ok {
			pbReq.Subject = &pb.SyncReq_Groups{Groups: groups}
		} else {
			return nil, fmt.Errorf("invalid data type for Groups subject")
		}
	case types.SyncSubjectTypeNetworks:
		if networks, ok := req.Data.(*pb.SyncNetworks); ok {
			pbReq.Subject = &pb.SyncReq_Networks{Networks: networks}
		} else {
			return nil, fmt.Errorf("invalid data type for Networks subject")
		}
	case types.SyncSubjectTypeIEAgAgRules:
		// Convert domain model IEAgAgRule to sgroups IESgSgRule protobuf
		if rule, ok := req.Data.(*models.IEAgAgRule); ok {
			pbRule := convertIEAgAgRuleToSGroupsProto(*rule)
			pbReq.Subject = &pb.SyncReq_IeSgSgRules{
				IeSgSgRules: &pb.SyncIESgSgRules{
					Rules: []*pb.IESgSgRule{pbRule},
				},
			}
		} else {
			return nil, fmt.Errorf("invalid data type for IEAgAgRules subject, expected *models.IEAgAgRule, got %T", req.Data)
		}
	default:
		return nil, fmt.Errorf("unknown subject type: %s", req.SubjectType)
	}

	return pbReq, nil
}

// convertIEAgAgRuleToSGroupsProto converts domain IEAgAgRule to sgroups IESgSgRule protobuf
func convertIEAgAgRuleToSGroupsProto(rule models.IEAgAgRule) *pb.IESgSgRule {
	// Convert Transport (using common package)
	var transport common.Networks_NetIP_Transport
	switch rule.Transport {
	case models.TCP:
		transport = common.Networks_NetIP_TCP
	case models.UDP:
		transport = common.Networks_NetIP_UDP
	default:
		transport = common.Networks_NetIP_TCP // default to TCP
	}

	// Convert Traffic (using common package)
	var traffic common.Traffic
	switch rule.Traffic {
	case models.INGRESS:
		traffic = common.Traffic_Ingress
	case models.EGRESS:
		traffic = common.Traffic_Egress
	default:
		traffic = common.Traffic_Ingress // default to Ingress
	}

	// Convert Action (using sgroups package)
	var action pb.RuleAction
	switch rule.Action {
	case models.ActionAccept:
		action = pb.RuleAction_ACCEPT
	case models.ActionDrop:
		action = pb.RuleAction_DROP
	default:
		action = pb.RuleAction_ACCEPT // default to ACCEPT
	}

	// Convert Ports
	var ports []*pb.AccPorts
	for _, port := range rule.Ports {
		if port.Destination != "" {
			ports = append(ports, &pb.AccPorts{
				S: port.Source,      // Source port (can be empty)
				D: port.Destination, // Destination port
			})
		}
	}

	return &pb.IESgSgRule{
		Transport: transport,
		SG:        fmt.Sprintf("%s/%s", rule.AddressGroup.Namespace, rule.AddressGroup.Name),           // Remote AddressGroup (namespace/name)
		SgLocal:   fmt.Sprintf("%s/%s", rule.AddressGroupLocal.Namespace, rule.AddressGroupLocal.Name), // Local AddressGroup (namespace/name)
		Traffic:   traffic,
		Ports:     ports,
		Logs:      rule.Logs,
		Action:    action,
	}
}

// DefaultSGroupsConfig returns default configuration for sgroups client
func DefaultSGroupsConfig() SGroupsConfig {
	return SGroupsConfig{
		GRPCAddress:    "localhost:9090",
		RequestTimeout: 30 * time.Second,
		KeepAlive: KeepAliveConfig{
			Time:                30 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		},
		TLS: TLSConfig{
			Enabled: false,
		},
	}
}
