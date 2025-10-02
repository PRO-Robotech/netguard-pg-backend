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
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// ✅ FIX: First get the full object like pre-refactoring implementation
	fullBinding, err := c.GetAddressGroupBinding(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get full binding for delete: %w", err)
	}


	// Now send the FULL object for deletion (like pre-refactoring)
	return c.syncAddressGroupBinding(ctx, models.SyncOpDelete, []*models.AddressGroupBinding{fullBinding})
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
	// ✅ FIX: First get the full object like pre-refactoring implementation
	fullRule, err := c.GetRuleS2S(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get full rule for delete: %w", err)
	}


	// Now send the FULL object for deletion (like pre-refactoring)
	return c.syncRuleS2S(ctx, models.SyncOpDelete, []*models.RuleS2S{fullRule})
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
	// ✅ FIX: First get the full object like pre-refactoring implementation
	fullAlias, err := c.GetServiceAlias(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get full alias for delete: %w", err)
	}


	// Now send the FULL object for deletion (like pre-refactoring)
	return c.syncServiceAlias(ctx, models.SyncOpDelete, []*models.ServiceAlias{fullAlias})
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
	// ✅ FIX: First get the full object like pre-refactoring implementation
	fullPolicy, err := c.GetAddressGroupBindingPolicy(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get full policy for delete: %w", err)
	}

	return c.syncAddressGroupBindingPolicy(ctx, models.SyncOpDelete, []*models.AddressGroupBindingPolicy{fullPolicy})
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
	rule := convertIEAgAgRuleFromProto(resp.IeagagRule)
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
		rules = append(rules, convertIEAgAgRuleFromProto(protoRule))
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

func (c *GRPCBackendClient) GetNetwork(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetNetworkReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetNetwork(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get network: %w", err)
	}
	network := convertNetworkFromProto(resp.Network)
	if network.BindingRef != nil {
	} else {
	}
	return &network, nil
}

func (c *GRPCBackendClient) ListNetworks(ctx context.Context, scope ports.Scope) ([]models.Network, error) {
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
	req := &netguardpb.ListNetworksReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListNetworks(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}
	networks := make([]models.Network, 0, len(resp.Items))
	for _, protoNet := range resp.Items {
		networks = append(networks, convertNetworkFromProto(protoNet))
	}
	return networks, nil
}

func (c *GRPCBackendClient) CreateNetwork(ctx context.Context, network *models.Network) error {
	// Use Sync API for creation
	networks := []models.Network{*network}
	return c.Sync(ctx, models.SyncOpUpsert, networks)
}

func (c *GRPCBackendClient) UpdateNetwork(ctx context.Context, network *models.Network) error {
	// Use Sync API for update
	networks := []models.Network{*network}
	return c.Sync(ctx, models.SyncOpUpsert, networks)
}

func (c *GRPCBackendClient) DeleteNetwork(ctx context.Context, id models.ResourceIdentifier) error {
	network, err := c.GetNetwork(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get network for deletion: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", id.Key())
	}

	networks := []models.Network{*network}
	return c.Sync(ctx, models.SyncOpDelete, networks)
}

func (c *GRPCBackendClient) GetNetworkBinding(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetNetworkBindingReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetNetworkBinding(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get network binding: %w", err)
	}
	binding := convertNetworkBindingFromProto(resp.NetworkBinding)
	return &binding, nil
}

func (c *GRPCBackendClient) ListNetworkBindings(ctx context.Context, scope ports.Scope) ([]models.NetworkBinding, error) {
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
	req := &netguardpb.ListNetworkBindingsReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListNetworkBindings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list network bindings: %w", err)
	}
	bindings := make([]models.NetworkBinding, 0, len(resp.Items))
	for _, protoBinding := range resp.Items {
		bindings = append(bindings, convertNetworkBindingFromProto(protoBinding))
	}
	return bindings, nil
}

func (c *GRPCBackendClient) CreateNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error {
	// Use Sync API for creation
	bindings := []models.NetworkBinding{*binding}
	return c.Sync(ctx, models.SyncOpUpsert, bindings)
}

func (c *GRPCBackendClient) UpdateNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error {
	// Use Sync API for update
	bindings := []models.NetworkBinding{*binding}
	return c.Sync(ctx, models.SyncOpUpsert, bindings)
}

func (c *GRPCBackendClient) DeleteNetworkBinding(ctx context.Context, id models.ResourceIdentifier) error {
	// Use Sync API for deletion
	// We need to get the binding first to delete it
	binding, err := c.GetNetworkBinding(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get network binding for deletion: %w", err)
	}
	if binding == nil {
		return fmt.Errorf("network binding not found: %s", id.Key())
	}

	bindings := []models.NetworkBinding{*binding}
	return c.Sync(ctx, models.SyncOpDelete, bindings)
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

func (c *GRPCBackendClient) syncNetwork(ctx context.Context, syncOp models.SyncOp, networks []*models.Network) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	protoNetworks := make([]*netguardpb.Network, 0, len(networks))
	for _, network := range networks {
		protoNetworks = append(protoNetworks, convertNetworkToPB(*network))
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
		Subject: &netguardpb.SyncReq_Networks{
			Networks: &netguardpb.SyncNetworks{
				Networks: protoNetworks,
			},
		},
	}
	_, err := c.client.Sync(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync networks: %w", err)
	}
	return nil
}

func (c *GRPCBackendClient) syncNetworkBinding(ctx context.Context, syncOp models.SyncOp, bindings []*models.NetworkBinding) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	protoBindings := make([]*netguardpb.NetworkBinding, 0, len(bindings))
	for _, binding := range bindings {
		protoBindings = append(protoBindings, convertNetworkBindingToPB(*binding))
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
		Subject: &netguardpb.SyncReq_NetworkBindings{
			NetworkBindings: &netguardpb.SyncNetworkBindings{
				NetworkBindings: protoBindings,
			},
		},
	}
	_, err := c.client.Sync(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync network bindings: %w", err)
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
	case []models.Network:
		ptrs := make([]*models.Network, 0, len(res))
		for i := range res {
			ptrs = append(ptrs, &res[i])
		}
		return c.syncNetwork(ctx, syncOp, ptrs)
	case []models.NetworkBinding:
		ptrs := make([]*models.NetworkBinding, 0, len(res))
		for i := range res {
			ptrs = append(ptrs, &res[i])
		}
		return c.syncNetworkBinding(ctx, syncOp, ptrs)
	case []models.Host:
		ptrs := make([]*models.Host, 0, len(res))
		for i := range res {
			ptrs = append(ptrs, &res[i])
		}
		return c.syncHost(ctx, syncOp, ptrs)
	case []models.HostBinding:
		ptrs := make([]*models.HostBinding, 0, len(res))
		for i := range res {
			ptrs = append(ptrs, &res[i])
		}
		return c.syncHostBinding(ctx, syncOp, ptrs)
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

func (c *GRPCBackendClient) UpdateNetworkMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	network, err := c.GetNetwork(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get network for meta update: %w", err)
	}
	network.Meta = meta
	return c.UpdateNetwork(ctx, network)
}

func (c *GRPCBackendClient) UpdateNetworkBindingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	binding, err := c.GetNetworkBinding(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get network binding for meta update: %w", err)
	}
	binding.Meta = meta
	return c.UpdateNetworkBinding(ctx, binding)
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
		if binding.ServiceRef.Name == serviceID.Name && binding.Namespace == serviceID.Namespace {
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

func (c *GRPCBackendClient) GetHost(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetHostReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetHost(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get host: %w", err)
	}
	host := convertHostFromProto(resp.Host)
	return &host, nil
}

func (c *GRPCBackendClient) ListHosts(ctx context.Context, scope ports.Scope) ([]models.Host, error) {
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
	req := &netguardpb.ListHostsReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListHosts(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}
	hosts := make([]models.Host, 0, len(resp.Items))
	for _, protoHost := range resp.Items {
		hosts = append(hosts, convertHostFromProto(protoHost))
	}
	return hosts, nil
}

func (c *GRPCBackendClient) CreateHost(ctx context.Context, host *models.Host) error {
	// Use Sync API for creation
	hosts := []models.Host{*host}
	return c.Sync(ctx, models.SyncOpUpsert, hosts)
}

func (c *GRPCBackendClient) UpdateHost(ctx context.Context, host *models.Host) error {
	// Use Sync API for update
	hosts := []models.Host{*host}
	return c.Sync(ctx, models.SyncOpUpsert, hosts)
}

func (c *GRPCBackendClient) DeleteHost(ctx context.Context, id models.ResourceIdentifier) error {
	host, err := c.GetHost(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get host for deletion: %w", err)
	}
	if host == nil {
		return fmt.Errorf("host not found: %s", id.Key())
	}

	hosts := []models.Host{*host}
	return c.Sync(ctx, models.SyncOpDelete, hosts)
}

func (c *GRPCBackendClient) GetHostBinding(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error) {
	if !c.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	req := &netguardpb.GetHostBindingReq{
		Identifier: &netguardpb.ResourceIdentifier{
			Namespace: id.Namespace,
			Name:      id.Name,
		},
	}
	resp, err := c.client.GetHostBinding(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get host binding: %w", err)
	}
	hostBinding := convertHostBindingFromProto(resp.HostBinding)
	return &hostBinding, nil
}

func (c *GRPCBackendClient) ListHostBindings(ctx context.Context, scope ports.Scope) ([]models.HostBinding, error) {
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
	req := &netguardpb.ListHostBindingsReq{
		Identifiers: identifiers,
	}
	resp, err := c.client.ListHostBindings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list host bindings: %w", err)
	}
	hostBindings := make([]models.HostBinding, 0, len(resp.Items))
	for _, protoBinding := range resp.Items {
		hostBindings = append(hostBindings, convertHostBindingFromProto(protoBinding))
	}
	return hostBindings, nil
}

func (c *GRPCBackendClient) CreateHostBinding(ctx context.Context, hostBinding *models.HostBinding) error {
	// Use Sync API for creation
	hostBindings := []models.HostBinding{*hostBinding}
	return c.Sync(ctx, models.SyncOpUpsert, hostBindings)
}

func (c *GRPCBackendClient) UpdateHostBinding(ctx context.Context, hostBinding *models.HostBinding) error {
	// Use Sync API for update
	hostBindings := []models.HostBinding{*hostBinding}
	return c.Sync(ctx, models.SyncOpUpsert, hostBindings)
}

func (c *GRPCBackendClient) DeleteHostBinding(ctx context.Context, id models.ResourceIdentifier) error {
	hostBinding, err := c.GetHostBinding(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get host binding for deletion: %w", err)
	}
	if hostBinding == nil {
		return fmt.Errorf("host binding not found: %s", id.Key())
	}

	hostBindings := []models.HostBinding{*hostBinding}
	return c.Sync(ctx, models.SyncOpDelete, hostBindings)
}

func (c *GRPCBackendClient) syncHost(ctx context.Context, syncOp models.SyncOp, hosts []*models.Host) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	var protoHosts []*netguardpb.Host
	for _, host := range hosts {
		protoHosts = append(protoHosts, convertHostToPB(*host))
	}

	req := &netguardpb.SyncReq{
		SyncOp: convertSyncOpToPB(syncOp),
		Subject: &netguardpb.SyncReq_Hosts{
			Hosts: &netguardpb.SyncHosts{
				Hosts: protoHosts,
			},
		},
	}

	_, err := c.client.Sync(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync hosts: %w", err)
	}
	return nil
}

func (c *GRPCBackendClient) syncHostBinding(ctx context.Context, syncOp models.SyncOp, hostBindings []*models.HostBinding) error {
	if !c.limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	var protoBindings []*netguardpb.HostBinding
	for _, hostBinding := range hostBindings {
		protoBindings = append(protoBindings, convertHostBindingToPB(*hostBinding))
	}

	req := &netguardpb.SyncReq{
		SyncOp: convertSyncOpToPB(syncOp),
		Subject: &netguardpb.SyncReq_HostBindings{
			HostBindings: &netguardpb.SyncHostBindings{
				HostBindings: protoBindings,
			},
		},
	}

	_, err := c.client.Sync(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync host bindings: %w", err)
	}
	return nil
}

func (c *GRPCBackendClient) UpdateHostMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	// TODO: Реализовать когда backend добавит UpdateMeta endpoints
	// На данный момент используем обычный Update через GetHost + UpdateHost
	host, err := c.GetHost(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get host for meta update: %w", err)
	}
	host.Meta = meta
	return c.UpdateHost(ctx, host)
}

func (c *GRPCBackendClient) UpdateHostBindingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	hostBinding, err := c.GetHostBinding(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get host binding for meta update: %w", err)
	}
	hostBinding.Meta = meta
	return c.UpdateHostBinding(ctx, hostBinding)
}

// Helper functions for Network conversions
func convertNetworkFromProto(protoNetwork *netguardpb.Network) models.Network {
	result := models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      protoNetwork.GetSelfRef().GetName(),
				Namespace: protoNetwork.GetSelfRef().GetNamespace(),
			},
		},
		CIDR: protoNetwork.Cidr,
		Meta: models.Meta{},
	}

	// Copy meta if provided
	if protoNetwork.Meta != nil {
		result.Meta = models.Meta{
			UID:             protoNetwork.Meta.Uid,
			ResourceVersion: protoNetwork.Meta.ResourceVersion,
			Generation:      protoNetwork.Meta.Generation,
			Labels:          protoNetwork.Meta.Labels,
			Annotations:     protoNetwork.Meta.Annotations,
			Conditions:      models.ProtoConditionsToK8s(protoNetwork.Meta.Conditions),
		}
		if protoNetwork.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(protoNetwork.Meta.CreationTs.AsTime())
		}
	}

	// Add status fields
	result.IsBound = protoNetwork.IsBound

	if protoNetwork.BindingRef != nil {
		result.BindingRef = &v1beta1.ObjectReference{
			APIVersion: protoNetwork.BindingRef.ApiVersion,
			Kind:       protoNetwork.BindingRef.Kind,
			Name:       protoNetwork.BindingRef.Name,
		}
	}

	if protoNetwork.AddressGroupRef != nil {
		result.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: protoNetwork.AddressGroupRef.ApiVersion,
			Kind:       protoNetwork.AddressGroupRef.Kind,
			Name:       protoNetwork.AddressGroupRef.Name,
		}
	}

	return result
}

func convertNetworkToPB(network models.Network) *netguardpb.Network {
	pbNetwork := &netguardpb.Network{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      network.Name,
			Namespace: network.Namespace,
		},
		Cidr: network.CIDR,
	}

	// Populate Meta information
	pbNetwork.Meta = &netguardpb.Meta{
		Uid:                network.Meta.UID,
		ResourceVersion:    network.Meta.ResourceVersion,
		Generation:         network.Meta.Generation,
		Labels:             network.Meta.Labels,
		Annotations:        network.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(network.Meta.Conditions),
		ObservedGeneration: network.Meta.ObservedGeneration,
	}
	if !network.Meta.CreationTS.IsZero() {
		pbNetwork.Meta.CreationTs = timestamppb.New(network.Meta.CreationTS.Time)
	}

	return pbNetwork
}

// Helper functions for NetworkBinding conversions
func convertNetworkBindingFromProto(protoBinding *netguardpb.NetworkBinding) models.NetworkBinding {
	result := models.NetworkBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      protoBinding.GetSelfRef().GetName(),
				Namespace: protoBinding.GetSelfRef().GetNamespace(),
			},
		},
		Meta: models.Meta{},
	}

	// Convert NetworkRef
	result.NetworkRef = v1beta1.ObjectReference{
		APIVersion: protoBinding.GetNetworkRef().GetApiVersion(),
		Kind:       protoBinding.GetNetworkRef().GetKind(),
		Name:       protoBinding.GetNetworkRef().GetName(),
	}

	// Convert AddressGroupRef
	result.AddressGroupRef = v1beta1.ObjectReference{
		APIVersion: protoBinding.GetAddressGroupRef().GetApiVersion(),
		Kind:       protoBinding.GetAddressGroupRef().GetKind(),
		Name:       protoBinding.GetAddressGroupRef().GetName(),
	}

	// Convert NetworkItem if present
	if protoBinding.NetworkItem != nil {
		result.NetworkItem = models.NetworkItem{
			Name: protoBinding.NetworkItem.Name,
			CIDR: protoBinding.NetworkItem.Cidr,
		}
	}

	// Copy Meta if presented
	if protoBinding.Meta != nil {
		result.Meta = models.Meta{
			UID:             protoBinding.Meta.Uid,
			ResourceVersion: protoBinding.Meta.ResourceVersion,
			Generation:      protoBinding.Meta.Generation,
			Labels:          protoBinding.Meta.Labels,
			Annotations:     protoBinding.Meta.Annotations,
			Conditions:      models.ProtoConditionsToK8s(protoBinding.Meta.Conditions),
		}
		if protoBinding.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(protoBinding.Meta.CreationTs.AsTime())
		}
	}

	return result
}

func convertNetworkBindingToPB(binding models.NetworkBinding) *netguardpb.NetworkBinding {
	pbBinding := &netguardpb.NetworkBinding{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      binding.Name,
			Namespace: binding.Namespace,
		},
		NetworkRef: &netguardpb.ObjectReference{
			ApiVersion: binding.NetworkRef.APIVersion,
			Kind:       binding.NetworkRef.Kind,
			Name:       binding.NetworkRef.Name,
		},
		AddressGroupRef: &netguardpb.ObjectReference{
			ApiVersion: binding.AddressGroupRef.APIVersion,
			Kind:       binding.AddressGroupRef.Kind,
			Name:       binding.AddressGroupRef.Name,
		},
	}

	// Convert NetworkItem
	pbBinding.NetworkItem = &netguardpb.NetworkItem{
		Name: binding.NetworkItem.Name,
		Cidr: binding.NetworkItem.CIDR,
	}

	// Populate Meta information
	pbBinding.Meta = &netguardpb.Meta{
		Uid:                binding.Meta.UID,
		ResourceVersion:    binding.Meta.ResourceVersion,
		Generation:         binding.Meta.Generation,
		Labels:             binding.Meta.Labels,
		Annotations:        binding.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(binding.Meta.Conditions),
		ObservedGeneration: binding.Meta.ObservedGeneration,
	}
	if !binding.Meta.CreationTS.IsZero() {
		pbBinding.Meta.CreationTs = timestamppb.New(binding.Meta.CreationTS.Time)
	}

	return pbBinding
}

// Helper functions for IEAgAgRule conversions
func convertIEAgAgRuleFromProto(protoRule *netguardpb.IEAgAgRule) models.IEAgAgRule {
	// Convert Transport
	var transport models.TransportProtocol
	switch protoRule.Transport {
	case netguardpb.Networks_NetIP_TCP:
		transport = models.TCP
	case netguardpb.Networks_NetIP_UDP:
		transport = models.UDP
	default:
		transport = models.TCP
	}

	// Convert Traffic
	var traffic models.Traffic
	switch protoRule.Traffic {
	case netguardpb.Traffic_Ingress:
		traffic = models.INGRESS
	case netguardpb.Traffic_Egress:
		traffic = models.EGRESS
	default:
		traffic = models.INGRESS
	}

	// Convert Action
	var action models.RuleAction
	switch protoRule.Action {
	case netguardpb.RuleAction_ACCEPT:
		action = models.ActionAccept
	case netguardpb.RuleAction_DROP:
		action = models.ActionDrop
	default:
		action = models.ActionAccept
	}

	result := models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      protoRule.GetSelfRef().GetName(),
				Namespace: protoRule.GetSelfRef().GetNamespace(),
			},
		},
		Transport: transport,
		Traffic:   traffic,
		Action:    action,
		Logs:      protoRule.Logs,
		Priority:  protoRule.Priority,
		Meta:      models.Meta{},
		Trace:     protoRule.Trace,
	}

	// Copy AddressGroups
	if protoRule.AddressGroupLocal != nil && protoRule.AddressGroupLocal.Identifier != nil {
		result.AddressGroupLocal = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       protoRule.AddressGroupLocal.Identifier.Name,
			},
			Namespace: protoRule.AddressGroupLocal.Identifier.Namespace,
		}
	}

	if protoRule.AddressGroup != nil && protoRule.AddressGroup.Identifier != nil {
		result.AddressGroup = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       protoRule.AddressGroup.Identifier.Name,
			},
			Namespace: protoRule.AddressGroup.Identifier.Namespace,
		}
	}

	// Copy Ports
	for _, protoPort := range protoRule.Ports {
		result.Ports = append(result.Ports, models.PortSpec{
			Source:      protoPort.Source,
			Destination: protoPort.Destination,
		})
	}

	// Copy Meta if provided
	if protoRule.Meta != nil {
		result.Meta = models.Meta{
			UID:             protoRule.Meta.Uid,
			ResourceVersion: protoRule.Meta.ResourceVersion,
			Generation:      protoRule.Meta.Generation,
			Labels:          protoRule.Meta.Labels,
			Annotations:     protoRule.Meta.Annotations,
		}
		if protoRule.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(protoRule.Meta.CreationTs.AsTime())
		}
		if protoRule.Meta.Conditions != nil {
			result.Meta.Conditions = models.ProtoConditionsToK8s(protoRule.Meta.Conditions)
		}
		result.Meta.ObservedGeneration = protoRule.Meta.ObservedGeneration
	}

	return result
}

// Helper functions for Host conversions
func convertHostFromProto(protoHost *netguardpb.Host) models.Host {
	result := models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      protoHost.GetSelfRef().GetName(),
				Namespace: protoHost.GetSelfRef().GetNamespace(),
			},
		},
		UUID: protoHost.GetUuid(),

		// Status fields
		HostName:         protoHost.GetHostNameSync(),
		AddressGroupName: protoHost.GetAddressGroupName(),
		IsBound:          protoHost.GetIsBound(),
	}

	// Set binding reference if present
	if protoHost.GetBindingRef() != nil {
		result.BindingRef = &v1beta1.ObjectReference{
			APIVersion: protoHost.GetBindingRef().GetApiVersion(),
			Kind:       protoHost.GetBindingRef().GetKind(),
			Name:       protoHost.GetBindingRef().GetName(),
		}
	}

	// Set address group reference if present
	if protoHost.GetAddressGroupRef() != nil {
		result.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: protoHost.GetAddressGroupRef().GetApiVersion(),
			Kind:       protoHost.GetAddressGroupRef().GetKind(),
			Name:       protoHost.GetAddressGroupRef().GetName(),
		}
	}

	// Copy Meta if provided
	if protoHost.Meta != nil {
		result.Meta = models.Meta{
			UID:             protoHost.Meta.Uid,
			ResourceVersion: protoHost.Meta.ResourceVersion,
			Generation:      protoHost.Meta.Generation,
			Labels:          protoHost.Meta.Labels,
			Annotations:     protoHost.Meta.Annotations,
		}
		if protoHost.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(protoHost.Meta.CreationTs.AsTime())
		}
		if protoHost.Meta.Conditions != nil {
			result.Meta.Conditions = models.ProtoConditionsToK8s(protoHost.Meta.Conditions)
		}
		result.Meta.ObservedGeneration = protoHost.Meta.ObservedGeneration
	}

	// Convert IP list if present
	if len(protoHost.GetIpList()) > 0 {
		result.IpList = make([]models.IPItem, len(protoHost.GetIpList()))
		for i, ipItem := range protoHost.GetIpList() {
			result.IpList[i] = models.IPItem{
				IP: ipItem.GetIp(),
			}
		}
	} else {
	}

	return result
}

func convertHostToPB(host models.Host) *netguardpb.Host {
	return &netguardpb.Host{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      host.Name,
			Namespace: host.Namespace,
		},
		Uuid: host.UUID,

		// Status fields
		HostNameSync:     host.HostName,
		AddressGroupName: host.AddressGroupName,
		IsBound:          host.IsBound,

		BindingRef: func() *netguardpb.ObjectReference {
			if host.BindingRef != nil {
				return &netguardpb.ObjectReference{
					ApiVersion: host.BindingRef.APIVersion,
					Kind:       host.BindingRef.Kind,
					Name:       host.BindingRef.Name,
				}
			}
			return nil
		}(),

		AddressGroupRef: func() *netguardpb.ObjectReference {
			if host.AddressGroupRef != nil {
				return &netguardpb.ObjectReference{
					ApiVersion: host.AddressGroupRef.APIVersion,
					Kind:       host.AddressGroupRef.Kind,
					Name:       host.AddressGroupRef.Name,
				}
			}
			return nil
		}(),

		Meta: &netguardpb.Meta{
			Uid:                host.Meta.UID,
			ResourceVersion:    host.Meta.ResourceVersion,
			Generation:         host.Meta.Generation,
			CreationTs:         timestamppb.New(host.Meta.CreationTS.Time),
			Labels:             host.Meta.Labels,
			Annotations:        host.Meta.Annotations,
			Conditions:         models.K8sConditionsToProto(host.Meta.Conditions),
			ObservedGeneration: host.Meta.ObservedGeneration,
		},
	}
}

// Helper functions for HostBinding conversions
func convertHostBindingFromProto(protoBinding *netguardpb.HostBinding) models.HostBinding {
	result := models.HostBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      protoBinding.GetSelfRef().GetName(),
				Namespace: protoBinding.GetSelfRef().GetNamespace(),
			},
		},
	}

	// Set host reference
	if protoBinding.GetHostRef() != nil {
		result.HostRef = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: protoBinding.GetHostRef().GetApiVersion(),
				Kind:       protoBinding.GetHostRef().GetKind(),
				Name:       protoBinding.GetHostRef().GetName(),
			},
			Namespace: protoBinding.GetHostRef().GetNamespace(),
		}
	}

	// Set address group reference
	if protoBinding.GetAddressGroupRef() != nil {
		result.AddressGroupRef = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: protoBinding.GetAddressGroupRef().GetApiVersion(),
				Kind:       protoBinding.GetAddressGroupRef().GetKind(),
				Name:       protoBinding.GetAddressGroupRef().GetName(),
			},
			Namespace: protoBinding.GetAddressGroupRef().GetNamespace(),
		}
	}

	// Copy Meta if provided
	if protoBinding.Meta != nil {
		result.Meta = models.Meta{
			UID:             protoBinding.Meta.Uid,
			ResourceVersion: protoBinding.Meta.ResourceVersion,
			Generation:      protoBinding.Meta.Generation,
			Labels:          protoBinding.Meta.Labels,
			Annotations:     protoBinding.Meta.Annotations,
		}
		if protoBinding.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(protoBinding.Meta.CreationTs.AsTime())
		}
		if protoBinding.Meta.Conditions != nil {
			result.Meta.Conditions = models.ProtoConditionsToK8s(protoBinding.Meta.Conditions)
		}
		result.Meta.ObservedGeneration = protoBinding.Meta.ObservedGeneration
	}

	return result
}

func convertHostBindingToPB(hostBinding models.HostBinding) *netguardpb.HostBinding {
	return &netguardpb.HostBinding{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      hostBinding.Name,
			Namespace: hostBinding.Namespace,
		},

		HostRef: &netguardpb.NamespacedObjectReference{
			ApiVersion: hostBinding.HostRef.APIVersion,
			Kind:       hostBinding.HostRef.Kind,
			Name:       hostBinding.HostRef.Name,
			Namespace:  hostBinding.HostRef.Namespace,
		},

		AddressGroupRef: &netguardpb.NamespacedObjectReference{
			ApiVersion: hostBinding.AddressGroupRef.APIVersion,
			Kind:       hostBinding.AddressGroupRef.Kind,
			Name:       hostBinding.AddressGroupRef.Name,
			Namespace:  hostBinding.AddressGroupRef.Namespace,
		},

		Meta: &netguardpb.Meta{
			Uid:                hostBinding.Meta.UID,
			ResourceVersion:    hostBinding.Meta.ResourceVersion,
			Generation:         hostBinding.Meta.Generation,
			CreationTs:         timestamppb.New(hostBinding.Meta.CreationTS.Time),
			Labels:             hostBinding.Meta.Labels,
			Annotations:        hostBinding.Meta.Annotations,
			Conditions:         models.K8sConditionsToProto(hostBinding.Meta.Conditions),
			ObservedGeneration: hostBinding.Meta.ObservedGeneration,
		},
	}
}

// convertSyncOpToPB converts domain SyncOp to protobuf SyncOp
func convertSyncOpToPB(syncOp models.SyncOp) netguardpb.SyncOp {
	return netguardpb.SyncOp(models.SyncOpToProto(syncOp))
}
