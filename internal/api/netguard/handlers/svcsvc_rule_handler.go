package handlers

import (
	"context"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/api/netguard/converters"
	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// SvcSvcRuleHandler handles SvcSvcRule-related operations
type SvcSvcRuleHandler struct {
	service *services.NetguardFacade
}

// NewSvcSvcRuleHandler creates a new SvcSvcRuleHandler
func NewSvcSvcRuleHandler(service *services.NetguardFacade) *SvcSvcRuleHandler {
	return &SvcSvcRuleHandler{service: service}
}

// ListSvcSvcRules gets list of SvcSvcRules
func (h *SvcSvcRuleHandler) ListSvcSvcRules(ctx context.Context, req *netguardpb.ListSvcSvcRulesReq) (*netguardpb.ListSvcSvcRulesResp, error) {
	scope := h.buildScope(req.Identifiers)

	rules, err := h.service.GetSvcSvcRules(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get SvcSvcRules")
	}

	items := make([]*netguardpb.SvcSvcRule, 0, len(rules))
	for _, r := range rules {
		items = append(items, converters.ConvertSvcSvcRuleToPB(r))
	}

	return &netguardpb.ListSvcSvcRulesResp{Items: items}, nil
}

// GetSvcSvcRule gets a specific SvcSvcRule by ID
func (h *SvcSvcRuleHandler) GetSvcSvcRule(ctx context.Context, req *netguardpb.GetSvcSvcRuleReq) (*netguardpb.GetSvcSvcRuleResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	rule, err := h.service.GetSvcSvcRuleByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get SvcSvcRule")
	}

	return &netguardpb.GetSvcSvcRuleResp{
		SvcsvcRule: converters.ConvertSvcSvcRuleToPB(*rule),
	}, nil
}

// buildScope creates a scope from resource identifiers
func (h *SvcSvcRuleHandler) buildScope(identifiers []*netguardpb.ResourceIdentifier) ports.Scope {
	if len(identifiers) == 0 {
		return ports.EmptyScope{}
	}

	ids := make([]models.ResourceIdentifier, 0, len(identifiers))
	for _, id := range identifiers {
		ids = append(ids, converters.ResourceIdentifierFromPB(id))
	}

	return ports.NewResourceIdentifierScope(ids...)
}
