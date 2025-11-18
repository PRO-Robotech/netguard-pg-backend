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

// SvcFqdnRuleHandler handles SvcFqdnRule related operations
type SvcFqdnRuleHandler struct {
	service *services.NetguardFacade
}

// NewSvcFqdnRuleHandler creates a new SvcFqdnRuleHandler instance
func NewSvcFqdnRuleHandler(service *services.NetguardFacade) *SvcFqdnRuleHandler {
	return &SvcFqdnRuleHandler{service: service}
}

// ListSvcFqdnRules returns a list of SvcFqdnRules within the requested scope
func (h *SvcFqdnRuleHandler) ListSvcFqdnRules(ctx context.Context, req *netguardpb.ListSvcFqdnRulesReq) (*netguardpb.ListSvcFqdnRulesResp, error) {
	scope := h.buildScopeWithOptions(req.GetIdentifiers(), req.ListOptions)

	rules, err := h.service.GetSvcFqdnRules(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list SvcFqdnRules")
	}

	items := make([]*netguardpb.SvcFqdnRule, 0, len(rules))
	for _, rule := range rules {
		items = append(items, converters.ConvertSvcFqdnRuleToPB(rule))
	}

	return &netguardpb.ListSvcFqdnRulesResp{Items: items}, nil
}

// GetSvcFqdnRule returns a single SvcFqdnRule by identifier
func (h *SvcFqdnRuleHandler) GetSvcFqdnRule(ctx context.Context, req *netguardpb.GetSvcFqdnRuleReq) (*netguardpb.GetSvcFqdnRuleResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	rule, err := h.service.GetSvcFqdnRuleByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get SvcFqdnRule")
	}

	return &netguardpb.GetSvcFqdnRuleResp{SvcfqdnRule: converters.ConvertSvcFqdnRuleToPB(*rule)}, nil
}

func (h *SvcFqdnRuleHandler) buildScope(identifiers []*netguardpb.ResourceIdentifier) ports.Scope {
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
func (h *SvcFqdnRuleHandler) buildScopeWithOptions(identifiers []*netguardpb.ResourceIdentifier, listOpts *netguardpb.ListOptions) ports.Scope {
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
