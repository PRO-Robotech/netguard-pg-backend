package svcfqdn_rule

import (
	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// NewStatusREST creates a status subresource for SvcFqdnRule that supports condition updates.
func NewStatusREST(store *SvcFqdnRuleStorage) *base.StatusREST[*netguardv1beta1.SvcFqdnRule, *models.SvcFqdnRule] {
	return base.NewStatusREST[*netguardv1beta1.SvcFqdnRule, *models.SvcFqdnRule](
		store.BaseStorage,
	)
}
