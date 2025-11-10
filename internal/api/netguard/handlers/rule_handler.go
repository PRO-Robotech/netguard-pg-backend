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

// RuleHandler handles rule-related operations
type RuleHandler struct {
	service *services.NetguardFacade
}

// NewRuleHandler creates a new RuleHandler
func NewRuleHandler(service *services.NetguardFacade) *RuleHandler {
	return &RuleHandler{service: service}
}

// ListRuleS2S gets list of rule s2s
func (h *RuleHandler) ListRuleS2S(ctx context.Context, req *netguardpb.ListRuleS2SReq) (*netguardpb.ListRuleS2SResp, error) {
	scope := h.buildScope(req.Identifiers)

	rules, err := h.service.GetRuleS2S(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get rule s2s")
	}

	items := make([]*netguardpb.RuleS2S, 0, len(rules))
	for _, r := range rules {
		items = append(items, converters.ConvertRuleS2SToPB(r))
	}

	return &netguardpb.ListRuleS2SResp{Items: items}, nil
}

// GetRuleS2S gets a specific rule s2s by ID
func (h *RuleHandler) GetRuleS2S(ctx context.Context, req *netguardpb.GetRuleS2SReq) (*netguardpb.GetRuleS2SResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	rule, err := h.service.GetRuleS2SByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get rule s2s")
	}

	return &netguardpb.GetRuleS2SResp{
		RuleS2S: converters.ConvertRuleS2SToPB(*rule),
	}, nil
}

// ListIEAgAgRules gets list of IEAgAgRules
func (h *RuleHandler) ListIEAgAgRules(ctx context.Context, req *netguardpb.ListIEAgAgRulesReq) (*netguardpb.ListIEAgAgRulesResp, error) {
	scope := h.buildScope(req.Identifiers)

	rules, err := h.service.GetIEAgAgRules(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get IEAgAgRules")
	}

	items := make([]*netguardpb.IEAgAgRule, 0, len(rules))
	for _, rule := range rules {
		items = append(items, converters.ConvertIEAgAgRuleToPB(rule))
	}

	return &netguardpb.ListIEAgAgRulesResp{Items: items}, nil
}

// GetIEAgAgRule gets a specific IEAgAgRule by ID
func (h *RuleHandler) GetIEAgAgRule(ctx context.Context, req *netguardpb.GetIEAgAgRuleReq) (*netguardpb.GetIEAgAgRuleResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	rule, err := h.service.GetIEAgAgRuleByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get IEAgAgRule")
	}

	return &netguardpb.GetIEAgAgRuleResp{
		IeagagRule: converters.ConvertIEAgAgRuleToPB(*rule),
	}, nil
}

// buildScope creates a scope from resource identifiers
func (h *RuleHandler) buildScope(identifiers []*netguardpb.ResourceIdentifier) ports.Scope {
	if len(identifiers) == 0 {
		return ports.EmptyScope{}
	}

	ids := make([]models.ResourceIdentifier, 0, len(identifiers))
	for _, id := range identifiers {
		ids = append(ids, converters.ResourceIdentifierFromPB(id))
	}

	return ports.NewResourceIdentifierScope(ids...)
}
