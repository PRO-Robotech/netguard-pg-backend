package client

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/emptypb"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"k8s.io/klog/v2"
)

// GRPCBackendClient базовый gRPC клиент
// Реализует BackendClient
// Все методы CRUD для Service, AddressGroup, AddressGroupBinding и заглушки для остальных

type GRPCBackendClient struct {
	client  netguardpb.NetguardServiceClient
	conn    *grpc.ClientConn
	limiter *rate.Limiter
	config  BackendClientConfig

	dependencyValidator *validation.DependencyValidator
	reader              ports.Reader
}

func NewGRPCBackendClient(config BackendClientConfig) (*GRPCBackendClient, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                5 * time.Minute,
			Timeout:             3 * time.Second,
			PermitWithoutStream: false,
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, config.Endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to backend: %w", err)
	}

	client := netguardpb.NewNetguardServiceClient(conn)
	limiter := rate.NewLimiter(rate.Limit(config.RateLimit), config.RateBurst)

	grpcClient := &GRPCBackendClient{
		client:  client,
		conn:    conn,
		limiter: limiter,
		config:  config,
	}

	// Создаем reader и validator
	grpcClient.reader = NewGRPCReader(grpcClient)
	grpcClient.dependencyValidator = validation.NewDependencyValidator(grpcClient.reader)

	return grpcClient, nil
}

// --- Service ---
func (c *GRPCBackendClient) GetService(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetServiceReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetService(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	service := convertServiceFromProto(resp.Service)
	return &service, nil
}

func (c *GRPCBackendClient) ListServices(ctx context.Context, scope ports.Scope) ([]models.Service, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	var identifiers []*netguardpb.ResourceIdentifier
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			for _, id := range ris.Identifiers {
				identifiers = append(identifiers, &netguardpb.ResourceIdentifier{
					Namespace: id.Namespace,
					Name:      id.Name,
				})
			}
		}
	}
	req := &netguardpb.ListServicesReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListServices(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	services := make([]models.Service, 0, len(resp.Items))
	for _, protoSvc := range resp.Items {
		services = append(services, convertServiceFromProto(protoSvc))
	}
	return services, nil
}

func (c *GRPCBackendClient) CreateService(ctx context.Context, service *models.Service) error {
	return c.syncService(ctx, models.SyncOpUpsert, []*models.Service{service})
}

func (c *GRPCBackendClient) UpdateService(ctx context.Context, service *models.Service) error {
	return c.syncService(ctx, models.SyncOpUpsert, []*models.Service{service})
}

func (c *GRPCBackendClient) DeleteService(ctx context.Context, id models.ResourceIdentifier) error {
	service := &models.Service{
		SelfRef: models.SelfRef{ResourceIdentifier: id},
	}
	return c.syncService(ctx, models.SyncOpDelete, []*models.Service{service})
}

func (c *GRPCBackendClient) syncService(ctx context.Context, syncOp models.SyncOp, services []*models.Service) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	// DEBUG: логируем содержимое ingressPorts, чтобы отследить пустые порты
	for _, s := range services {
		var parts []string
		for _, p := range s.IngressPorts {
			parts = append(parts, fmt.Sprintf("%s:%s", p.Protocol, p.Port))
		}
		klog.V(2).Infof("syncService op=%s ns=%q name=%q ports=%v", syncOp.String(), s.Namespace, s.Name, parts)
	}

	protoServices := make([]*netguardpb.Service, 0, len(services))
	for _, svc := range services {
		protoServices = append(protoServices, convertServiceToProto(*svc))
	}
	var protoSyncOp netguardpb.SyncOp
	switch syncOp {
	case models.SyncOpUpsert:
		protoSyncOp = netguardpb.SyncOp_Upsert
	case models.SyncOpDelete:
		protoSyncOp = netguardpb.SyncOp_Delete
	case models.SyncOpFullSync:
		protoSyncOp = netguardpb.SyncOp_FullSync
	default:
		protoSyncOp = netguardpb.SyncOp_NoOp
	}
	req := &netguardpb.SyncReq{
		SyncOp: protoSyncOp,
		Subject: &netguardpb.SyncReq_Services{
			Services: &netguardpb.SyncServices{
				Services: protoServices,
			},
		},
	}
	klog.V(2).Infof("GRPCBackendClient.syncService sending Sync len=%d op=%s", len(services), syncOp.String())
	_, err := c.client.Sync(ctx, req)
	if err != nil {
		klog.V(2).Infof("GRPCBackendClient.syncService error: %v", err)
		return fmt.Errorf("failed to sync services: %w", err)
	}
	klog.V(2).Infof("GRPCBackendClient.syncService OK op=%s", syncOp.String())
	return nil
}

// --- AddressGroup ---
func (c *GRPCBackendClient) GetAddressGroup(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	klog.V(4).Infof("GRPCBackendClient.GetAddressGroup ns=%q name=%q", id.Namespace, id.Name)
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetAddressGroupReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetAddressGroup(ctx, req)
	if err != nil {
		klog.V(2).Infof("GRPC GetAddressGroup returned error: %v", err)
		return nil, fmt.Errorf("failed to get address group: %w", err)
	}
	addressGroup := convertAddressGroupFromProto(resp.AddressGroup)
	return &addressGroup, nil
}

func (c *GRPCBackendClient) ListAddressGroups(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	var identifiers []*netguardpb.ResourceIdentifier
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			for _, id := range ris.Identifiers {
				identifiers = append(identifiers, &netguardpb.ResourceIdentifier{
					Namespace: id.Namespace,
					Name:      id.Name,
				})
			}
		}
	}
	req := &netguardpb.ListAddressGroupsReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListAddressGroups(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list address groups: %w", err)
	}
	addressGroups := make([]models.AddressGroup, 0, len(resp.Items))
	for _, protoAG := range resp.Items {
		addressGroups = append(addressGroups, convertAddressGroupFromProto(protoAG))
	}
	return addressGroups, nil
}

func (c *GRPCBackendClient) CreateAddressGroup(ctx context.Context, group *models.AddressGroup) error {
	return c.syncAddressGroup(ctx, models.SyncOpUpsert, []*models.AddressGroup{group})
}

func (c *GRPCBackendClient) UpdateAddressGroup(ctx context.Context, group *models.AddressGroup) error {
	return c.syncAddressGroup(ctx, models.SyncOpUpsert, []*models.AddressGroup{group})
}

func (c *GRPCBackendClient) DeleteAddressGroup(ctx context.Context, id models.ResourceIdentifier) error {
	group := &models.AddressGroup{
		SelfRef: models.SelfRef{ResourceIdentifier: id},
	}
	return c.syncAddressGroup(ctx, models.SyncOpDelete, []*models.AddressGroup{group})
}

func (c *GRPCBackendClient) syncAddressGroup(ctx context.Context, syncOp models.SyncOp, groups []*models.AddressGroup) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	protoGroups := make([]*netguardpb.AddressGroup, 0, len(groups))
	for _, group := range groups {
		protoGroups = append(protoGroups, convertAddressGroupToProto(*group))
	}
	var protoSyncOp netguardpb.SyncOp
	switch syncOp {
	case models.SyncOpUpsert:
		protoSyncOp = netguardpb.SyncOp_Upsert
	case models.SyncOpDelete:
		protoSyncOp = netguardpb.SyncOp_Delete
	case models.SyncOpFullSync:
		protoSyncOp = netguardpb.SyncOp_FullSync
	default:
		protoSyncOp = netguardpb.SyncOp_NoOp
	}
	req := &netguardpb.SyncReq{
		SyncOp: protoSyncOp,
		Subject: &netguardpb.SyncReq_AddressGroups{
			AddressGroups: &netguardpb.SyncAddressGroups{
				AddressGroups: protoGroups,
			},
		},
	}
	_, err := c.client.Sync(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync address groups: %w", err)
	}
	return nil
}

// --- AddressGroupBinding ---
func (c *GRPCBackendClient) GetAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetAddressGroupBindingReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetAddressGroupBinding(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get address group binding: %w", err)
	}
	binding := convertAddressGroupBindingFromProto(resp.AddressGroupBinding)
	return &binding, nil
}

func (c *GRPCBackendClient) ListAddressGroupBindings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBinding, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	var identifiers []*netguardpb.ResourceIdentifier
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			for _, id := range ris.Identifiers {
				identifiers = append(identifiers, &netguardpb.ResourceIdentifier{
					Namespace: id.Namespace,
					Name:      id.Name,
				})
			}
		}
	}
	req := &netguardpb.ListAddressGroupBindingsReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListAddressGroupBindings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list address group bindings: %w", err)
	}
	bindings := make([]models.AddressGroupBinding, 0, len(resp.Items))
	for _, protoBinding := range resp.Items {
		bindings = append(bindings, convertAddressGroupBindingFromProto(protoBinding))
	}
	return bindings, nil
}

func (c *GRPCBackendClient) CreateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error {
	return c.syncAddressGroupBinding(ctx, models.SyncOpUpsert, []*models.AddressGroupBinding{binding})
}

func (c *GRPCBackendClient) UpdateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error {
	return c.syncAddressGroupBinding(ctx, models.SyncOpUpsert, []*models.AddressGroupBinding{binding})
}

func (c *GRPCBackendClient) DeleteAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) error {
	binding := &models.AddressGroupBinding{
		SelfRef: models.SelfRef{ResourceIdentifier: id},
	}
	return c.syncAddressGroupBinding(ctx, models.SyncOpDelete, []*models.AddressGroupBinding{binding})
}

func (c *GRPCBackendClient) syncAddressGroupBinding(ctx context.Context, syncOp models.SyncOp, bindings []*models.AddressGroupBinding) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	protoBindings := make([]*netguardpb.AddressGroupBinding, 0, len(bindings))
	for _, binding := range bindings {
		protoBindings = append(protoBindings, convertAddressGroupBindingToProto(*binding))
	}
	var protoSyncOp netguardpb.SyncOp
	switch syncOp {
	case models.SyncOpUpsert:
		protoSyncOp = netguardpb.SyncOp_Upsert
	case models.SyncOpDelete:
		protoSyncOp = netguardpb.SyncOp_Delete
	case models.SyncOpFullSync:
		protoSyncOp = netguardpb.SyncOp_FullSync
	default:
		protoSyncOp = netguardpb.SyncOp_NoOp
	}
	req := &netguardpb.SyncReq{
		SyncOp: protoSyncOp,
		Subject: &netguardpb.SyncReq_AddressGroupBindings{
			AddressGroupBindings: &netguardpb.SyncAddressGroupBindings{
				AddressGroupBindings: protoBindings,
			},
		},
	}
	_, err := c.client.Sync(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync address group bindings: %w", err)
	}
	return nil
}

// --- AddressGroupPortMapping ---
func (c *GRPCBackendClient) GetAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetAddressGroupPortMappingReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetAddressGroupPortMapping(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get address group port mapping: %w", err)
	}
	mapping := convertAddressGroupPortMappingFromProto(resp.AddressGroupPortMapping)
	return &mapping, nil
}

func (c *GRPCBackendClient) ListAddressGroupPortMappings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupPortMapping, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	var identifiers []*netguardpb.ResourceIdentifier
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			for _, id := range ris.Identifiers {
				identifiers = append(identifiers, &netguardpb.ResourceIdentifier{
					Namespace: id.Namespace,
					Name:      id.Name,
				})
			}
		}
	}
	req := &netguardpb.ListAddressGroupPortMappingsReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListAddressGroupPortMappings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list address group port mappings: %w", err)
	}
	mappings := make([]models.AddressGroupPortMapping, 0, len(resp.Items))
	for _, protoMapping := range resp.Items {
		mappings = append(mappings, convertAddressGroupPortMappingFromProto(protoMapping))
	}
	return mappings, nil
}

func (c *GRPCBackendClient) CreateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	return c.syncAddressGroupPortMapping(ctx, models.SyncOpUpsert, []*models.AddressGroupPortMapping{mapping})
}

func (c *GRPCBackendClient) UpdateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	return c.syncAddressGroupPortMapping(ctx, models.SyncOpUpsert, []*models.AddressGroupPortMapping{mapping})
}

func (c *GRPCBackendClient) DeleteAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) error {
	mapping := &models.AddressGroupPortMapping{
		SelfRef: models.SelfRef{ResourceIdentifier: id},
	}
	return c.syncAddressGroupPortMapping(ctx, models.SyncOpDelete, []*models.AddressGroupPortMapping{mapping})
}

func (c *GRPCBackendClient) syncAddressGroupPortMapping(ctx context.Context, syncOp models.SyncOp, mappings []*models.AddressGroupPortMapping) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	protoMappings := make([]*netguardpb.AddressGroupPortMapping, 0, len(mappings))
	for _, mapping := range mappings {
		protoMappings = append(protoMappings, convertAddressGroupPortMappingToProto(*mapping))
	}
	var protoSyncOp netguardpb.SyncOp
	switch syncOp {
	case models.SyncOpUpsert:
		protoSyncOp = netguardpb.SyncOp_Upsert
	case models.SyncOpDelete:
		protoSyncOp = netguardpb.SyncOp_Delete
	case models.SyncOpFullSync:
		protoSyncOp = netguardpb.SyncOp_FullSync
	default:
		protoSyncOp = netguardpb.SyncOp_NoOp
	}
	req := &netguardpb.SyncReq{
		SyncOp: protoSyncOp,
		Subject: &netguardpb.SyncReq_AddressGroupPortMappings{
			AddressGroupPortMappings: &netguardpb.SyncAddressGroupPortMappings{
				AddressGroupPortMappings: protoMappings,
			},
		},
	}
	_, err := c.client.Sync(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync address group port mappings: %w", err)
	}
	return nil
}

// --- RuleS2S ---
func (c *GRPCBackendClient) GetRuleS2S(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetRuleS2SReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetRuleS2S(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get ruleS2S: %w", err)
	}
	rule := convertRuleS2SFromProto(resp.RuleS2S)
	return &rule, nil
}

func (c *GRPCBackendClient) ListRuleS2S(ctx context.Context, scope ports.Scope) ([]models.RuleS2S, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	var identifiers []*netguardpb.ResourceIdentifier
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			for _, id := range ris.Identifiers {
				identifiers = append(identifiers, &netguardpb.ResourceIdentifier{
					Namespace: id.Namespace,
					Name:      id.Name,
				})
			}
		}
	}
	req := &netguardpb.ListRuleS2SReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListRuleS2S(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list ruleS2S: %w", err)
	}
	rules := make([]models.RuleS2S, 0, len(resp.Items))
	for _, protoRule := range resp.Items {
		rules = append(rules, convertRuleS2SFromProto(protoRule))
	}
	return rules, nil
}

func (c *GRPCBackendClient) CreateRuleS2S(ctx context.Context, rule *models.RuleS2S) error {
	return c.syncRuleS2S(ctx, models.SyncOpUpsert, []*models.RuleS2S{rule})
}

func (c *GRPCBackendClient) UpdateRuleS2S(ctx context.Context, rule *models.RuleS2S) error {
	return c.syncRuleS2S(ctx, models.SyncOpUpsert, []*models.RuleS2S{rule})
}

func (c *GRPCBackendClient) DeleteRuleS2S(ctx context.Context, id models.ResourceIdentifier) error {
	rule := &models.RuleS2S{
		SelfRef: models.SelfRef{ResourceIdentifier: id},
	}
	return c.syncRuleS2S(ctx, models.SyncOpDelete, []*models.RuleS2S{rule})
}

func (c *GRPCBackendClient) syncRuleS2S(ctx context.Context, syncOp models.SyncOp, rules []*models.RuleS2S) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	protoRules := make([]*netguardpb.RuleS2S, 0, len(rules))
	for _, rule := range rules {
		protoRules = append(protoRules, convertRuleS2SToProto(*rule))
	}
	var protoSyncOp netguardpb.SyncOp
	switch syncOp {
	case models.SyncOpUpsert:
		protoSyncOp = netguardpb.SyncOp_Upsert
	case models.SyncOpDelete:
		protoSyncOp = netguardpb.SyncOp_Delete
	case models.SyncOpFullSync:
		protoSyncOp = netguardpb.SyncOp_FullSync
	default:
		protoSyncOp = netguardpb.SyncOp_NoOp
	}
	req := &netguardpb.SyncReq{
		SyncOp: protoSyncOp,
		Subject: &netguardpb.SyncReq_RuleS2S{
			RuleS2S: &netguardpb.SyncRuleS2S{
				RuleS2S: protoRules,
			},
		},
	}
	_, err := c.client.Sync(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync ruleS2S: %w", err)
	}
	return nil
}

// --- ServiceAlias ---
func (c *GRPCBackendClient) GetServiceAlias(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetServiceAliasReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetServiceAlias(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get service alias: %w", err)
	}
	alias := convertServiceAliasFromProto(resp.ServiceAlias)
	return &alias, nil
}

func (c *GRPCBackendClient) ListServiceAliases(ctx context.Context, scope ports.Scope) ([]models.ServiceAlias, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	var identifiers []*netguardpb.ResourceIdentifier
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			for _, id := range ris.Identifiers {
				identifiers = append(identifiers, &netguardpb.ResourceIdentifier{
					Namespace: id.Namespace,
					Name:      id.Name,
				})
			}
		}
	}
	req := &netguardpb.ListServiceAliasesReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListServiceAliases(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list service aliases: %w", err)
	}
	aliases := make([]models.ServiceAlias, 0, len(resp.Items))
	for _, protoAlias := range resp.Items {
		aliases = append(aliases, convertServiceAliasFromProto(protoAlias))
	}
	return aliases, nil
}

func (c *GRPCBackendClient) CreateServiceAlias(ctx context.Context, alias *models.ServiceAlias) error {
	return c.syncServiceAlias(ctx, models.SyncOpUpsert, []*models.ServiceAlias{alias})
}

func (c *GRPCBackendClient) UpdateServiceAlias(ctx context.Context, alias *models.ServiceAlias) error {
	return c.syncServiceAlias(ctx, models.SyncOpUpsert, []*models.ServiceAlias{alias})
}

func (c *GRPCBackendClient) DeleteServiceAlias(ctx context.Context, id models.ResourceIdentifier) error {
	alias := &models.ServiceAlias{
		SelfRef: models.SelfRef{ResourceIdentifier: id},
	}
	return c.syncServiceAlias(ctx, models.SyncOpDelete, []*models.ServiceAlias{alias})
}

func (c *GRPCBackendClient) syncServiceAlias(ctx context.Context, syncOp models.SyncOp, aliases []*models.ServiceAlias) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	protoAliases := make([]*netguardpb.ServiceAlias, 0, len(aliases))
	for _, alias := range aliases {
		protoAliases = append(protoAliases, convertServiceAliasToProto(*alias))
	}
	var protoSyncOp netguardpb.SyncOp
	switch syncOp {
	case models.SyncOpUpsert:
		protoSyncOp = netguardpb.SyncOp_Upsert
	case models.SyncOpDelete:
		protoSyncOp = netguardpb.SyncOp_Delete
	case models.SyncOpFullSync:
		protoSyncOp = netguardpb.SyncOp_FullSync
	default:
		protoSyncOp = netguardpb.SyncOp_NoOp
	}
	req := &netguardpb.SyncReq{
		SyncOp: protoSyncOp,
		Subject: &netguardpb.SyncReq_ServiceAliases{
			ServiceAliases: &netguardpb.SyncServiceAliases{
				ServiceAliases: protoAliases,
			},
		},
	}
	klog.V(2).Infof("GRPCBackendClient.syncServiceAlias sending Sync len=%d op=%s", len(aliases), syncOp.String())
	_, err := c.client.Sync(ctx, req)
	if err != nil {
		klog.V(2).Infof("GRPCBackendClient.syncServiceAlias error: %v", err)
	} else {
		klog.V(2).Infof("GRPCBackendClient.syncServiceAlias OK op=%s", syncOp.String())
	}
	return nil
}

// --- AddressGroupBindingPolicy ---
func (c *GRPCBackendClient) GetAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetAddressGroupBindingPolicyReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetAddressGroupBindingPolicy(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get address group binding policy: %w", err)
	}
	policy := convertAddressGroupBindingPolicyFromProto(resp.AddressGroupBindingPolicy)
	return &policy, nil
}

func (c *GRPCBackendClient) ListAddressGroupBindingPolicies(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBindingPolicy, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	var identifiers []*netguardpb.ResourceIdentifier
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			for _, id := range ris.Identifiers {
				identifiers = append(identifiers, &netguardpb.ResourceIdentifier{
					Namespace: id.Namespace,
					Name:      id.Name,
				})
			}
		}
	}
	req := &netguardpb.ListAddressGroupBindingPoliciesReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListAddressGroupBindingPolicies(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list address group binding policies: %w", err)
	}
	policies := make([]models.AddressGroupBindingPolicy, 0, len(resp.Items))
	for _, protoPolicy := range resp.Items {
		policies = append(policies, convertAddressGroupBindingPolicyFromProto(protoPolicy))
	}
	return policies, nil
}

func (c *GRPCBackendClient) CreateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	return c.syncAddressGroupBindingPolicy(ctx, models.SyncOpUpsert, []*models.AddressGroupBindingPolicy{policy})
}

func (c *GRPCBackendClient) UpdateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	return c.syncAddressGroupBindingPolicy(ctx, models.SyncOpUpsert, []*models.AddressGroupBindingPolicy{policy})
}

func (c *GRPCBackendClient) DeleteAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) error {
	policy := &models.AddressGroupBindingPolicy{
		SelfRef: models.SelfRef{ResourceIdentifier: id},
	}
	return c.syncAddressGroupBindingPolicy(ctx, models.SyncOpDelete, []*models.AddressGroupBindingPolicy{policy})
}

func (c *GRPCBackendClient) syncAddressGroupBindingPolicy(ctx context.Context, syncOp models.SyncOp, policies []*models.AddressGroupBindingPolicy) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	protoPolicies := make([]*netguardpb.AddressGroupBindingPolicy, 0, len(policies))
	for _, policy := range policies {
		protoPolicies = append(protoPolicies, convertAddressGroupBindingPolicyToProto(*policy))
	}
	var protoSyncOp netguardpb.SyncOp
	switch syncOp {
	case models.SyncOpUpsert:
		protoSyncOp = netguardpb.SyncOp_Upsert
	case models.SyncOpDelete:
		protoSyncOp = netguardpb.SyncOp_Delete
	case models.SyncOpFullSync:
		protoSyncOp = netguardpb.SyncOp_FullSync
	default:
		protoSyncOp = netguardpb.SyncOp_NoOp
	}
	req := &netguardpb.SyncReq{
		SyncOp: protoSyncOp,
		Subject: &netguardpb.SyncReq_AddressGroupBindingPolicies{
			AddressGroupBindingPolicies: &netguardpb.SyncAddressGroupBindingPolicies{
				AddressGroupBindingPolicies: protoPolicies,
			},
		},
	}
	_, err := c.client.Sync(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync address group binding policies: %w", err)
	}
	return nil
}

// --- IEAgAgRule ---
func (c *GRPCBackendClient) GetIEAgAgRule(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetIEAgAgRuleReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetIEAgAgRule(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get IEAgAgRule: %w", err)
	}
	rule := ConvertIEAgAgRuleFromProto(resp.IeagagRule)
	return &rule, nil
}

func (c *GRPCBackendClient) ListIEAgAgRules(ctx context.Context, scope ports.Scope) ([]models.IEAgAgRule, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	var identifiers []*netguardpb.ResourceIdentifier
	if scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && len(ris.Identifiers) > 0 {
			for _, id := range ris.Identifiers {
				identifiers = append(identifiers, &netguardpb.ResourceIdentifier{
					Namespace: id.Namespace,
					Name:      id.Name,
				})
			}
		}
	}
	req := &netguardpb.ListIEAgAgRulesReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListIEAgAgRules(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list IEAgAgRules: %w", err)
	}
	rules := make([]models.IEAgAgRule, 0, len(resp.Items))
	for _, protoRule := range resp.Items {
		rules = append(rules, ConvertIEAgAgRuleFromProto(protoRule))
	}
	return rules, nil
}

func (c *GRPCBackendClient) CreateIEAgAgRule(ctx context.Context, rule *models.IEAgAgRule) error {
	return c.syncIEAgAgRule(ctx, models.SyncOpUpsert, []*models.IEAgAgRule{rule})
}

func (c *GRPCBackendClient) UpdateIEAgAgRule(ctx context.Context, rule *models.IEAgAgRule) error {
	return c.syncIEAgAgRule(ctx, models.SyncOpUpsert, []*models.IEAgAgRule{rule})
}

func (c *GRPCBackendClient) DeleteIEAgAgRule(ctx context.Context, id models.ResourceIdentifier) error {
	rule := &models.IEAgAgRule{
		SelfRef: models.SelfRef{ResourceIdentifier: id},
	}
	return c.syncIEAgAgRule(ctx, models.SyncOpDelete, []*models.IEAgAgRule{rule})
}

func (c *GRPCBackendClient) syncIEAgAgRule(ctx context.Context, syncOp models.SyncOp, rules []*models.IEAgAgRule) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	protoRules := make([]*netguardpb.IEAgAgRule, 0, len(rules))
	for _, rule := range rules {
		protoRules = append(protoRules, convertIEAgAgRuleToProto(*rule))
	}
	var protoSyncOp netguardpb.SyncOp
	switch syncOp {
	case models.SyncOpUpsert:
		protoSyncOp = netguardpb.SyncOp_Upsert
	case models.SyncOpDelete:
		protoSyncOp = netguardpb.SyncOp_Delete
	case models.SyncOpFullSync:
		protoSyncOp = netguardpb.SyncOp_FullSync
	default:
		protoSyncOp = netguardpb.SyncOp_NoOp
	}
	req := &netguardpb.SyncReq{
		SyncOp: protoSyncOp,
		Subject: &netguardpb.SyncReq_IeagagRules{
			IeagagRules: &netguardpb.SyncIEAgAgRules{
				IeagagRules: protoRules,
			},
		},
	}
	_, err := c.client.Sync(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync IEAgAgRules: %w", err)
	}
	return nil
}

// Sync implements generic sync for known slice types. Currently supports AddressGroup.
func (c *GRPCBackendClient) Sync(ctx context.Context, syncOp models.SyncOp, resources interface{}) error {
	switch res := resources.(type) {
	case []models.AddressGroup:
		ptrs := make([]*models.AddressGroup, 0, len(res))
		for i := range res {
			ptrs = append(ptrs, &res[i])
		}
		return c.syncAddressGroup(ctx, syncOp, ptrs)
	case []models.Service:
		ptrs := make([]*models.Service, 0, len(res))
		for i := range res {
			ptrs = append(ptrs, &res[i])
		}
		return c.syncService(ctx, syncOp, ptrs)
	case []models.ServiceAlias:
		ptrs := make([]*models.ServiceAlias, 0, len(res))
		for i := range res {
			ptrs = append(ptrs, &res[i])
		}
		return c.syncServiceAlias(ctx, syncOp, ptrs)
	case []models.AddressGroupBindingPolicy:
		ptrs := make([]*models.AddressGroupBindingPolicy, 0, len(res))
		for i := range res {
			ptrs = append(ptrs, &res[i])
		}
		return c.syncAddressGroupBindingPolicy(ctx, syncOp, ptrs)
	default:
		return fmt.Errorf("generic sync not implemented for %T", resources)
	}
}

func (c *GRPCBackendClient) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	resp, err := c.client.SyncStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed to get sync status: %w", err)
	}
	return &models.SyncStatus{
		UpdatedAt: resp.UpdatedAt.AsTime(),
	}, nil
}

func (c *GRPCBackendClient) GetDependencyValidator() *validation.DependencyValidator {
	return c.dependencyValidator
}

func (c *GRPCBackendClient) GetReader(ctx context.Context) (ports.Reader, error) {
	return c.reader, nil
}

func (c *GRPCBackendClient) HealthCheck(ctx context.Context) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	_, err := c.client.SyncStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("backend health check failed: %w", err)
	}
	return nil
}

// Ping - простая проверка соединения с backend (быстрее чем HealthCheck)
func (c *GRPCBackendClient) Ping(ctx context.Context) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	// Используем короткий timeout для ping
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Используем SyncStatus как простой ping endpoint
	_, err := c.client.SyncStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("backend ping failed: %w", err)
	}
	return nil
}

// UpdateMeta методы для всех ресурсов
func (c *GRPCBackendClient) UpdateServiceMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	// TODO: Реализовать когда backend добавит UpdateMeta endpoints
	// На данный момент используем обычный Update через GetService + UpdateService
	service, err := c.GetService(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get service for meta update: %w", err)
	}
	service.Meta = meta
	return c.UpdateService(ctx, service)
}

func (c *GRPCBackendClient) UpdateAddressGroupMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	addressGroup, err := c.GetAddressGroup(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get address group for meta update: %w", err)
	}
	addressGroup.Meta = meta
	return c.UpdateAddressGroup(ctx, addressGroup)
}

func (c *GRPCBackendClient) UpdateAddressGroupBindingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	binding, err := c.GetAddressGroupBinding(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get address group binding for meta update: %w", err)
	}
	binding.Meta = meta
	return c.UpdateAddressGroupBinding(ctx, binding)
}

func (c *GRPCBackendClient) UpdateAddressGroupPortMappingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	mapping, err := c.GetAddressGroupPortMapping(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get address group port mapping for meta update: %w", err)
	}
	mapping.Meta = meta
	return c.UpdateAddressGroupPortMapping(ctx, mapping)
}

func (c *GRPCBackendClient) UpdateRuleS2SMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	rule, err := c.GetRuleS2S(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get ruleS2S for meta update: %w", err)
	}
	rule.Meta = meta
	return c.UpdateRuleS2S(ctx, rule)
}

func (c *GRPCBackendClient) UpdateServiceAliasMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	alias, err := c.GetServiceAlias(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get service alias for meta update: %w", err)
	}
	alias.Meta = meta
	return c.UpdateServiceAlias(ctx, alias)
}

func (c *GRPCBackendClient) UpdateAddressGroupBindingPolicyMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	policy, err := c.GetAddressGroupBindingPolicy(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get address group binding policy for meta update: %w", err)
	}
	policy.Meta = meta
	return c.UpdateAddressGroupBindingPolicy(ctx, policy)
}

func (c *GRPCBackendClient) UpdateIEAgAgRuleMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	rule, err := c.GetIEAgAgRule(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get IEAgAgRule for meta update: %w", err)
	}
	rule.Meta = meta
	return c.UpdateIEAgAgRule(ctx, rule)
}

// Helper методы для subresources (оптимизированные запросы)
func (c *GRPCBackendClient) ListAddressGroupsForService(ctx context.Context, serviceID models.ResourceIdentifier) ([]models.AddressGroup, error) {
	klog.V(4).Infof("GRPCBackendClient.ListAddressGroupsForService serviceID=%s/%s", serviceID.Namespace, serviceID.Name)

	// Получаем все bindings, которые ссылаются на этот Service
	scope := ports.NewResourceIdentifierScope(serviceID)
	bindings, err := c.ListAddressGroupBindings(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list address group bindings for service: %w", err)
	}

	// Собираем уникальные AddressGroup идентификаторы
	addressGroupIDs := make(map[string]models.ResourceIdentifier)
	for _, binding := range bindings {
		if binding.ServiceRef.Name == serviceID.Name && binding.ServiceRef.Namespace == serviceID.Namespace {
			key := fmt.Sprintf("%s/%s", binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name)
			addressGroupIDs[key] = models.NewResourceIdentifier(
				binding.AddressGroupRef.Name,
				models.WithNamespace(binding.AddressGroupRef.Namespace),
			)
		}
	}

	// Получаем все AddressGroups по найденным идентификаторам
	var addressGroups []models.AddressGroup
	for _, agID := range addressGroupIDs {
		ag, err := c.GetAddressGroup(ctx, agID)
		if err != nil {
			klog.V(2).Infof("Failed to get address group %s/%s: %v", agID.Namespace, agID.Name, err)
			continue // Пропускаем недоступные, но продолжаем
		}
		addressGroups = append(addressGroups, *ag)
	}

	klog.V(4).Infof("GRPCBackendClient.ListAddressGroupsForService found %d address groups for service %s/%s",
		len(addressGroups), serviceID.Namespace, serviceID.Name)
	return addressGroups, nil
}

func (c *GRPCBackendClient) ListRuleS2SDstOwnRef(ctx context.Context, serviceID models.ResourceIdentifier) ([]models.RuleS2S, error) {
	klog.V(4).Infof("GRPCBackendClient.ListRuleS2SDstOwnRef serviceID=%s/%s", serviceID.Namespace, serviceID.Name)

	// Получаем все RuleS2S правила
	allRules, err := c.ListRuleS2S(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list all ruleS2S: %w", err)
	}

	// Фильтруем правила, которые ссылаются на этот Service как destination из других namespaces
	var crossNamespaceRules []models.RuleS2S
	for _, rule := range allRules {
		// Проверяем, что это правило ссылается на наш service как destination И из другого namespace
		if rule.ServiceRef.Name == serviceID.Name &&
			rule.ServiceRef.Namespace == serviceID.Namespace &&
			rule.Namespace != serviceID.Namespace {
			crossNamespaceRules = append(crossNamespaceRules, rule)
		}
	}

	klog.V(4).Infof("GRPCBackendClient.ListRuleS2SDstOwnRef found %d cross-namespace rules for service %s/%s",
		len(crossNamespaceRules), serviceID.Namespace, serviceID.Name)
	return crossNamespaceRules, nil
}

func (c *GRPCBackendClient) ListAccessPorts(ctx context.Context, mappingID models.ResourceIdentifier) ([]models.ServicePortsRef, error) {
	klog.V(4).Infof("GRPCBackendClient.ListAccessPorts mappingID=%s/%s", mappingID.Namespace, mappingID.Name)

	// Получаем AddressGroupPortMapping
	mapping, err := c.GetAddressGroupPortMapping(ctx, mappingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get address group port mapping: %w", err)
	}

	// Конвертируем AccessPorts map в slice ServicePortsRef
	var servicePortsRefs []models.ServicePortsRef
	for serviceRef, servicePorts := range mapping.AccessPorts {
		servicePortsRef := models.ServicePortsRef{
			ServiceRef: serviceRef,
			Ports:      servicePorts,
		}
		servicePortsRefs = append(servicePortsRefs, servicePortsRef)
	}

	klog.V(4).Infof("GRPCBackendClient.ListAccessPorts found %d service ports refs for mapping %s/%s",
		len(servicePortsRefs), mappingID.Namespace, mappingID.Name)
	return servicePortsRefs, nil
}

func (c *GRPCBackendClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
