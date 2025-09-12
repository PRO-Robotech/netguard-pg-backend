package host_binding

import (
	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// NewStatusREST returns a RESTStorage object that will work against HostBinding status subresource
func NewStatusREST(store *HostBindingStorage) *base.StatusREST[*netguardv1beta1.HostBinding, *models.HostBinding] {
	return base.NewStatusREST[*netguardv1beta1.HostBinding, *models.HostBinding](store.BaseStorage)
}
