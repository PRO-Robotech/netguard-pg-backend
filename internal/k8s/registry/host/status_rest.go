package host

import (
	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// NewStatusREST returns a RESTStorage object that will work against Host status subresource
func NewStatusREST(store *HostStorage) *base.StatusREST[*netguardv1beta1.Host, *models.Host] {
	return base.NewStatusREST[*netguardv1beta1.Host, *models.Host](store.BaseStorage)
}
