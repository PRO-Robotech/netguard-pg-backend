package iecidrsvc_rule

import (
	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// NewStatusREST creates a status subresource for IECidrSvcRule that supports condition updates.
func NewStatusREST(store *IECidrSvcRuleStorage) *base.StatusREST[*netguardv1beta1.IECidrSvcRule, *models.IECidrSvcRule] {
	return base.NewStatusREST[*netguardv1beta1.IECidrSvcRule, *models.IECidrSvcRule](
		store.BaseStorage,
	)
}
