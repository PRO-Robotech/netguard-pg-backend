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

// HostHandler handles host-related operations
type HostHandler struct {
	service   *services.NetguardFacade
	validator *validation.SelectorValidator
}

// NewHostHandler creates a new HostHandler
func NewHostHandler(service *services.NetguardFacade, validator *validation.SelectorValidator) *HostHandler {
	return &HostHandler{
		service:   service,
		validator: validator,
	}
}

// ListHosts gets list of hosts
func (h *HostHandler) ListHosts(ctx context.Context, req *netguardpb.ListHostsReq) (*netguardpb.ListHostsResp, error) {
	// Validate field selectors and label selectors if provided
	if req.Options != nil {
		if err := h.validator.ValidateFieldSelectors("hosts", req.Options.FieldSelectors); err != nil {
			return nil, errors.Wrap(err, "invalid field selectors")
		}
		if err := h.validator.ValidateLabelSelectors(req.Options.LabelSelectors); err != nil {
			return nil, errors.Wrap(err, "invalid label selectors")
		}
	}

	scope := h.buildScopeWithFieldSelectors(req.Identifiers, req.Options)

	hosts, err := h.service.GetHosts(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get hosts")
	}

	pbHosts := make([]*netguardpb.Host, 0, len(hosts))
	for _, host := range hosts {
		pbHosts = append(pbHosts, converters.ConvertHostToPB(host))
	}

	return &netguardpb.ListHostsResp{Items: pbHosts}, nil
}

// GetHost gets a host by identifier
func (h *HostHandler) GetHost(ctx context.Context, req *netguardpb.GetHostReq) (*netguardpb.GetHostResp, error) {
	id := models.NewResourceIdentifier(req.Identifier.Name, models.WithNamespace(req.Identifier.Namespace))

	host, err := h.service.GetHostByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get host")
	}

	return &netguardpb.GetHostResp{
		Host: converters.ConvertHostToPB(*host),
	}, nil
}

// ListHostBindings gets list of host bindings
func (h *HostHandler) ListHostBindings(ctx context.Context, req *netguardpb.ListHostBindingsReq) (*netguardpb.ListHostBindingsResp, error) {
	scope := h.buildScope(req.Identifiers)

	hostBindings, err := h.service.GetHostBindings(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get host bindings")
	}

	pbBindings := make([]*netguardpb.HostBinding, 0, len(hostBindings))
	for _, binding := range hostBindings {
		pbBindings = append(pbBindings, converters.ConvertHostBindingToPB(binding))
	}

	return &netguardpb.ListHostBindingsResp{Items: pbBindings}, nil
}

// GetHostBinding gets a host binding by identifier
func (h *HostHandler) GetHostBinding(ctx context.Context, req *netguardpb.GetHostBindingReq) (*netguardpb.GetHostBindingResp, error) {
	id := models.NewResourceIdentifier(req.Identifier.Name, models.WithNamespace(req.Identifier.Namespace))

	hostBinding, err := h.service.GetHostBindingByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get host binding")
	}

	return &netguardpb.GetHostBindingResp{
		HostBinding: converters.ConvertHostBindingToPB(*hostBinding),
	}, nil
}

// buildScope creates a scope from resource identifiers
func (h *HostHandler) buildScope(identifiers []*netguardpb.ResourceIdentifier) ports.Scope {
	if len(identifiers) == 0 {
		return ports.EmptyScope{}
	}

	ids := make([]models.ResourceIdentifier, 0, len(identifiers))
	for _, id := range identifiers {
		ids = append(ids, converters.ResourceIdentifierFromPB(id))
	}

	return ports.NewResourceIdentifierScope(ids...)
}

// buildScopeWithFieldSelectors creates a scope from resource identifiers, field selectors, and label selectors
func (h *HostHandler) buildScopeWithFieldSelectors(identifiers []*netguardpb.ResourceIdentifier, options *netguardpb.ListOptions) ports.Scope {
	// Extract field selectors and label selectors from options
	var fieldSelectors []*netguardpb.FieldSelector
	var labelSelectors []*netguardpb.LabelSelector
	if options != nil {
		if len(options.FieldSelectors) > 0 {
			fieldSelectors = options.FieldSelectors
		}
		if len(options.LabelSelectors) > 0 {
			labelSelectors = options.LabelSelectors
		}
	}

	// If we have no identifiers, field selectors, and label selectors, return empty scope
	if len(identifiers) == 0 && len(fieldSelectors) == 0 && len(labelSelectors) == 0 {
		return ports.EmptyScope{}
	}

	// If we only have identifiers (no field or label selectors), use ResourceIdentifierScope for backward compatibility
	if len(fieldSelectors) == 0 && len(labelSelectors) == 0 {
		ids := make([]models.ResourceIdentifier, 0, len(identifiers))
		for _, id := range identifiers {
			ids = append(ids, converters.ResourceIdentifierFromPB(id))
		}
		return ports.NewResourceIdentifierScope(ids...)
	}

	// If we have field or label selectors (with or without identifiers), use FieldSelectorScope
	ids := make([]models.ResourceIdentifier, 0, len(identifiers))
	for _, id := range identifiers {
		ids = append(ids, converters.ResourceIdentifierFromPB(id))
	}

	return ports.FieldSelectorScope{
		Identifiers:    ids,
		FieldSelectors: fieldSelectors,
		LabelSelectors: labelSelectors,
	}
}
