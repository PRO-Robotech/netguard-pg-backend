package handlers

import (
	"context"

	"netguard-pg-backend/internal/api/netguard/converters"
	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"github.com/pkg/errors"
)

// ServiceHandler handles service-related operations
type ServiceHandler struct {
	service *services.NetguardFacade
}

// NewServiceHandler creates a new ServiceHandler
func NewServiceHandler(service *services.NetguardFacade) *ServiceHandler {
	return &ServiceHandler{service: service}
}

// ListServices gets list of services
func (h *ServiceHandler) ListServices(ctx context.Context, req *netguardpb.ListServicesReq) (*netguardpb.ListServicesResp, error) {
	scope := h.buildScopeWithOptions(req.Identifiers, req.ListOptions)

	services, err := h.service.GetServices(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get services")
	}

	items := make([]*netguardpb.Service, 0, len(services))
	for _, svc := range services {
		items = append(items, converters.ConvertServiceToPB(svc))
	}

	return &netguardpb.ListServicesResp{Items: items}, nil
}

// GetService gets a specific service by ID
func (h *ServiceHandler) GetService(ctx context.Context, req *netguardpb.GetServiceReq) (*netguardpb.GetServiceResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	service, err := h.service.GetServiceByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service")
	}

	return &netguardpb.GetServiceResp{
		Service: converters.ConvertServiceToPB(*service),
	}, nil
}

// buildScope creates a scope from resource identifiers
func (h *ServiceHandler) buildScope(identifiers []*netguardpb.ResourceIdentifier) ports.Scope {
	if len(identifiers) == 0 {
		return ports.EmptyScope{}
	}

	ids := make([]models.ResourceIdentifier, 0, len(identifiers))
	for _, id := range identifiers {
		ids = append(ids, converters.ResourceIdentifierFromPB(id))
	}

	return ports.NewResourceIdentifierScope(ids...)
}

// buildScopeWithOptions creates a scope from resource identifiers and list options
func (h *ServiceHandler) buildScopeWithOptions(identifiers []*netguardpb.ResourceIdentifier, listOpts *netguardpb.ListOptions) ports.Scope {
	// Extract identifiers
	var ids []models.ResourceIdentifier
	if len(identifiers) > 0 {
		ids = make([]models.ResourceIdentifier, 0, len(identifiers))
		for _, id := range identifiers {
			ids = append(ids, converters.ResourceIdentifierFromPB(id))
		}
	}

	// Check if we have field or label selectors
	if listOpts != nil && (len(listOpts.FieldSelectors) > 0 || len(listOpts.LabelSelectors) > 0) {
		return ports.FieldSelectorScope{
			Identifiers:    ids,
			FieldSelectors: listOpts.FieldSelectors,
			LabelSelectors: listOpts.LabelSelectors,
		}
	}

	// Fall back to ResourceIdentifierScope or EmptyScope
	if len(ids) > 0 {
		return ports.NewResourceIdentifierScope(ids...)
	}
	return ports.EmptyScope{}
}
