package addressgroup

import (
	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// NewStatusREST creates a new StatusREST using BaseStorage
func NewStatusREST(store *AddressGroupStorage) *base.StatusREST[*netguardv1beta1.AddressGroup, *models.AddressGroup] {
	return base.NewStatusREST[*netguardv1beta1.AddressGroup, *models.AddressGroup](
		store.BaseStorage,
	)
}
