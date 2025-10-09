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

// AddressGroupHandler handles address group-related operations
type AddressGroupHandler struct {
	service *services.NetguardFacade
}

// NewAddressGroupHandler creates a new AddressGroupHandler
func NewAddressGroupHandler(service *services.NetguardFacade) *AddressGroupHandler {
	return &AddressGroupHandler{service: service}
}

// ListAddressGroups gets list of address groups
func (h *AddressGroupHandler) ListAddressGroups(ctx context.Context, req *netguardpb.ListAddressGroupsReq) (*netguardpb.ListAddressGroupsResp, error) {
	scope := h.buildScope(req.GetIdentifiers())

	addressGroups, err := h.service.GetAddressGroups(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address groups")
	}

	items := make([]*netguardpb.AddressGroup, 0, len(addressGroups))
	for _, ag := range addressGroups {
		items = append(items, converters.ConvertAddressGroupToPB(ag))
	}

	return &netguardpb.ListAddressGroupsResp{Items: items}, nil
}

// GetAddressGroup gets a specific address group by ID
func (h *AddressGroupHandler) GetAddressGroup(ctx context.Context, req *netguardpb.GetAddressGroupReq) (*netguardpb.GetAddressGroupResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	addressGroup, err := h.service.GetAddressGroupByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group")
	}

	return &netguardpb.GetAddressGroupResp{
		AddressGroup: converters.ConvertAddressGroupToPB(*addressGroup),
	}, nil
}

// ListAddressGroupBindings gets list of address group bindings
func (h *AddressGroupHandler) ListAddressGroupBindings(ctx context.Context, req *netguardpb.ListAddressGroupBindingsReq) (*netguardpb.ListAddressGroupBindingsResp, error) {
	scope := h.buildScope(req.Identifiers)

	bindings, err := h.service.GetAddressGroupBindings(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group bindings")
	}

	items := make([]*netguardpb.AddressGroupBinding, 0, len(bindings))
	for _, b := range bindings {
		items = append(items, converters.ConvertAddressGroupBindingToPB(b))
	}

	return &netguardpb.ListAddressGroupBindingsResp{Items: items}, nil
}

// GetAddressGroupBinding gets a specific address group binding by ID
func (h *AddressGroupHandler) GetAddressGroupBinding(ctx context.Context, req *netguardpb.GetAddressGroupBindingReq) (*netguardpb.GetAddressGroupBindingResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	binding, err := h.service.GetAddressGroupBindingByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group binding")
	}

	return &netguardpb.GetAddressGroupBindingResp{
		AddressGroupBinding: converters.ConvertAddressGroupBindingToPB(*binding),
	}, nil
}

// ListAddressGroupPortMappings gets list of address group port mappings
func (h *AddressGroupHandler) ListAddressGroupPortMappings(ctx context.Context, req *netguardpb.ListAddressGroupPortMappingsReq) (*netguardpb.ListAddressGroupPortMappingsResp, error) {
	scope := h.buildScope(req.Identifiers)

	mappings, err := h.service.GetAddressGroupPortMappings(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group port mappings")
	}

	items := make([]*netguardpb.AddressGroupPortMapping, 0, len(mappings))
	for _, m := range mappings {
		items = append(items, converters.ConvertAddressGroupPortMappingToPB(m))
	}

	return &netguardpb.ListAddressGroupPortMappingsResp{Items: items}, nil
}

// GetAddressGroupPortMapping gets a specific address group port mapping by ID
func (h *AddressGroupHandler) GetAddressGroupPortMapping(ctx context.Context, req *netguardpb.GetAddressGroupPortMappingReq) (*netguardpb.GetAddressGroupPortMappingResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	mapping, err := h.service.GetAddressGroupPortMappingByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group port mapping")
	}

	return &netguardpb.GetAddressGroupPortMappingResp{
		AddressGroupPortMapping: converters.ConvertAddressGroupPortMappingToPB(*mapping),
	}, nil
}

// ListAddressGroupBindingPolicies gets list of address group binding policies
func (h *AddressGroupHandler) ListAddressGroupBindingPolicies(ctx context.Context, req *netguardpb.ListAddressGroupBindingPoliciesReq) (*netguardpb.ListAddressGroupBindingPoliciesResp, error) {
	scope := h.buildScope(req.Identifiers)

	policies, err := h.service.GetAddressGroupBindingPolicies(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group binding policies")
	}

	items := make([]*netguardpb.AddressGroupBindingPolicy, 0, len(policies))
	for _, policy := range policies {
		items = append(items, converters.ConvertAddressGroupBindingPolicyToPB(policy))
	}

	return &netguardpb.ListAddressGroupBindingPoliciesResp{Items: items}, nil
}

// GetAddressGroupBindingPolicy gets a specific address group binding policy by ID
func (h *AddressGroupHandler) GetAddressGroupBindingPolicy(ctx context.Context, req *netguardpb.GetAddressGroupBindingPolicyReq) (*netguardpb.GetAddressGroupBindingPolicyResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	policy, err := h.service.GetAddressGroupBindingPolicyByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group binding policy")
	}

	return &netguardpb.GetAddressGroupBindingPolicyResp{
		AddressGroupBindingPolicy: converters.ConvertAddressGroupBindingPolicyToPB(*policy),
	}, nil
}

// buildScope creates a scope from resource identifiers
func (h *AddressGroupHandler) buildScope(identifiers []*netguardpb.ResourceIdentifier) ports.Scope {
	if len(identifiers) == 0 {
		return ports.EmptyScope{}
	}

	ids := make([]models.ResourceIdentifier, 0, len(identifiers))
	for _, id := range identifiers {
		ids = append(ids, converters.ResourceIdentifierFromPB(id))
	}

	return ports.NewResourceIdentifierScope(ids...)
}
