package addressgroupbindingpolicy

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// StatusREST implements the /status subresource for AddressGroupBindingPolicy.
// It supports GET; update/patch are disabled for now (status managed by backend).

type StatusREST struct {
	store *AddressGroupBindingPolicyStorage
}

// NewStatusREST returns a new StatusREST.
func NewStatusREST(store *AddressGroupBindingPolicyStorage) *StatusREST {
	return &StatusREST{store: store}
}

func (r *StatusREST) New() runtime.Object {
	return &netguardv1beta1.AddressGroupBindingPolicy{}
}

func (r *StatusREST) Get(ctx context.Context, name string, opts *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, opts)
}

func (r *StatusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, opts *metav1.UpdateOptions) (runtime.Object, bool, error) {
	return nil, false, fmt.Errorf("status updates are not supported for AddressGroupBindingPolicy")
}

func (r *StatusREST) Destroy() {}

var _ rest.Getter = &StatusREST{}
var _ rest.Updater = &StatusREST{}
