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

// NetworkHandler handles network-related operations
type NetworkHandler struct {
	service   *services.NetguardFacade
	validator *validation.SelectorValidator
}

// NewNetworkHandler creates a new NetworkHandler
func NewNetworkHandler(service *services.NetguardFacade, validator *validation.SelectorValidator) *NetworkHandler {
	return &NetworkHandler{
		service:   service,
		validator: validator,
	}
}

// ListNetworks gets list of networks
func (h *NetworkHandler) ListNetworks(ctx context.Context, req *netguardpb.ListNetworksReq) (*netguardpb.ListNetworksResp, error) {
	// Validate field selectors and label selectors if provided
	if req.GetOptions() != nil {
		if err := h.validator.ValidateFieldSelectors("networks", req.GetOptions().FieldSelectors); err != nil {
			return nil, errors.Wrap(err, "invalid field selectors")
		}
		if err := h.validator.ValidateLabelSelectors(req.GetOptions().LabelSelectors); err != nil {
			return nil, errors.Wrap(err, "invalid label selectors")
		}
	}

	scope := h.buildScopeWithFieldSelectors(req.Identifiers, req.GetOptions())

	networks, err := h.service.GetNetworks(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get networks")
	}

	items := make([]*netguardpb.Network, 0, len(networks))
	for _, network := range networks {
		items = append(items, converters.ConvertNetworkToPB(network))
	}

	return &netguardpb.ListNetworksResp{Items: items}, nil
}

// GetNetwork gets a specific network by ID
func (h *NetworkHandler) GetNetwork(ctx context.Context, req *netguardpb.GetNetworkReq) (*netguardpb.GetNetworkResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	network, err := h.service.GetNetworkByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network")
	}

	if network == nil {
		return nil, errors.New("network not found")
	}

	return &netguardpb.GetNetworkResp{
		Network: converters.ConvertNetworkToPB(*network),
	}, nil
}

// ListNetworkBindings gets list of network bindings
func (h *NetworkHandler) ListNetworkBindings(ctx context.Context, req *netguardpb.ListNetworkBindingsReq) (*netguardpb.ListNetworkBindingsResp, error) {
	scope := h.buildScope(req.Identifiers)

	bindings, err := h.service.GetNetworkBindings(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network bindings")
	}

	items := make([]*netguardpb.NetworkBinding, 0, len(bindings))
	for _, binding := range bindings {
		items = append(items, converters.ConvertNetworkBindingToPB(binding))
	}

	return &netguardpb.ListNetworkBindingsResp{Items: items}, nil
}

// GetNetworkBinding gets a specific network binding by ID
func (h *NetworkHandler) GetNetworkBinding(ctx context.Context, req *netguardpb.GetNetworkBindingReq) (*netguardpb.GetNetworkBindingResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	binding, err := h.service.GetNetworkBindingByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network binding")
	}

	if binding == nil {
		return nil, errors.New("network binding not found")
	}

	return &netguardpb.GetNetworkBindingResp{
		NetworkBinding: converters.ConvertNetworkBindingToPB(*binding),
	}, nil
}

// buildScope creates a scope from resource identifiers
func (h *NetworkHandler) buildScope(identifiers []*netguardpb.ResourceIdentifier) ports.Scope {
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
func (h *NetworkHandler) buildScopeWithFieldSelectors(
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
