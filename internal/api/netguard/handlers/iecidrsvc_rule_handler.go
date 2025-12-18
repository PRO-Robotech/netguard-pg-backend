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

// IECidrSvcRuleHandler handles IECidrSvcRule related operations
type IECidrSvcRuleHandler struct {
	service *services.NetguardFacade
}

// NewIECidrSvcRuleHandler creates a new IECidrSvcRuleHandler instance
func NewIECidrSvcRuleHandler(service *services.NetguardFacade) *IECidrSvcRuleHandler {
	return &IECidrSvcRuleHandler{service: service}
}

// ListIECidrSvcRules returns a list of IECidrSvcRules within the requested scope
func (h *IECidrSvcRuleHandler) ListIECidrSvcRules(ctx context.Context, req *netguardpb.ListIECidrSvcRulesReq) (*netguardpb.ListIECidrSvcRulesResp, error) {
	scope := h.buildScopeWithOptions(req.GetIdentifiers(), req.ListOptions)

	rules, err := h.service.GetIECidrSvcRules(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list IECidrSvcRules")
	}

	items := make([]*netguardpb.IECidrSvcRule, 0, len(rules))
	for _, rule := range rules {
		items = append(items, converters.ConvertIECidrSvcRuleToPB(rule))
	}

	return &netguardpb.ListIECidrSvcRulesResp{Items: items}, nil
}

// GetIECidrSvcRule returns a single IECidrSvcRule by identifier
func (h *IECidrSvcRuleHandler) GetIECidrSvcRule(ctx context.Context, req *netguardpb.GetIECidrSvcRuleReq) (*netguardpb.GetIECidrSvcRuleResp, error) {
	id := converters.ResourceIdentifierFromPB(req.GetIdentifier())

	rule, err := h.service.GetIECidrSvcRuleByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get IECidrSvcRule")
	}

	return &netguardpb.GetIECidrSvcRuleResp{IecidrsvcRule: converters.ConvertIECidrSvcRuleToPB(*rule)}, nil
}

func (h *IECidrSvcRuleHandler) buildScope(identifiers []*netguardpb.ResourceIdentifier) ports.Scope {
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
func (h *IECidrSvcRuleHandler) buildScopeWithOptions(identifiers []*netguardpb.ResourceIdentifier, listOpts *netguardpb.ListOptions) ports.Scope {
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
