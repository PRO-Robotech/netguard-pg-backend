package handlers

import (
	"context"

	"netguard-pg-backend/internal/api/netguard/converters"
	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"github.com/pkg/errors"
)

// ServiceHandler handles service-related operations
type ServiceHandler struct {
	service   *services.NetguardFacade
	validator *validation.SelectorValidator
}

// NewServiceHandler creates a new ServiceHandler
func NewServiceHandler(service *services.NetguardFacade, validator *validation.SelectorValidator) *ServiceHandler {
	return &ServiceHandler{
		service:   service,
		validator: validator,
	}
}

// ListServices gets list of services
func (h *ServiceHandler) ListServices(ctx context.Context, req *netguardpb.ListServicesReq) (*netguardpb.ListServicesResp, error) {
	// Validate field selectors and label selectors if provided
	if req.GetOptions() != nil {
		if err := h.validator.ValidateFieldSelectors("services", req.GetOptions().FieldSelectors); err != nil {
			return nil, errors.Wrap(err, "invalid field selectors")
		}
		if err := h.validator.ValidateLabelSelectors(req.GetOptions().LabelSelectors); err != nil {
			return nil, errors.Wrap(err, "invalid label selectors")
		}
	}

	scope := h.buildScopeWithFieldSelectors(req.Identifiers, req.GetOptions())

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

// ListServiceAliases gets list of service aliases
func (h *ServiceHandler) ListServiceAliases(ctx context.Context, req *netguardpb.ListServiceAliasesReq) (*netguardpb.ListServiceAliasesResp, error) {
	scope := h.buildScope(req.Identifiers)

	aliases, err := h.service.GetServiceAliases(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service aliases")
	}

	items := make([]*netguardpb.ServiceAlias, 0, len(aliases))
	for _, a := range aliases {
		items = append(items, converters.ConvertServiceAliasToPB(a))
	}

	return &netguardpb.ListServiceAliasesResp{Items: items}, nil
}

// GetServiceAlias gets a specific service alias by ID
func (h *ServiceHandler) GetServiceAlias(ctx context.Context, req *netguardpb.GetServiceAliasReq) (*netguardpb.GetServiceAliasResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	alias, err := h.service.GetServiceAliasByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service alias")
	}

	return &netguardpb.GetServiceAliasResp{
		ServiceAlias: converters.ConvertServiceAliasToPB(*alias),
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

// buildScopeWithFieldSelectors creates a scope from identifiers, field selectors, and label selectors
func (h *ServiceHandler) buildScopeWithFieldSelectors(
	identifiers []*netguardpb.ResourceIdentifier,
	options *netguardpb.ListOptions,
) ports.Scope {
	// If no options or no field/label selectors, fall back to legacy behavior
	if options == nil || (len(options.FieldSelectors) == 0 && len(options.LabelSelectors) == 0) {
		return h.buildScope(identifiers)
	}

	// Convert identifiers to domain models
	var ids []models.ResourceIdentifier
	if len(identifiers) > 0 {
		ids = make([]models.ResourceIdentifier, 0, len(identifiers))
		for _, id := range identifiers {
			ids = append(ids, converters.ResourceIdentifierFromPB(id))
		}
	}

	// Create FieldSelectorScope with identifiers, field selectors, and label selectors
	return ports.FieldSelectorScope{
		Identifiers:    ids,
		FieldSelectors: options.FieldSelectors,
		LabelSelectors: options.LabelSelectors,
	}
}
