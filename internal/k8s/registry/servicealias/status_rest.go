package servicealias

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// StatusREST implements the /status subresource for ServiceAlias.
// It allows read-only GET; Update/Patch are no-ops for now (only metadata).
// This aligns behaviour with Service/AddressGroup.

type StatusREST struct {
	store *ServiceAliasStorage
}

func NewStatusREST(store *ServiceAliasStorage) *StatusREST {
	return &StatusREST{store: store}
}

func (r *StatusREST) New() runtime.Object {
	return &netguardv1beta1.ServiceAlias{}
}

// Get returns the current object including status.
func (r *StatusREST) Get(ctx context.Context, name string, opts *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, opts)
}

// Update is disabled – status is managed by backend on Sync.
func (r *StatusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, opts *metav1.UpdateOptions) (runtime.Object, bool, error) {
	return nil, false, fmt.Errorf("status updates are not supported for ServiceAlias")
}

// Destroy no-op.
func (r *StatusREST) Destroy() {}

// Needed to satisfy interfaces
var _ rest.Getter = &StatusREST{}
var _ rest.Updater = &StatusREST{}
