package service

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// REST implements a RESTStorage for Service
type REST struct {
	*genericregistry.Store
}

// NewREST returns a RESTStorage object that will work against API services.
func NewREST(scheme *runtime.Scheme, optsGetter generic.RESTOptionsGetter) (*REST, error) {
	strategy := NewStrategy(scheme)

	store := &genericregistry.Store{
		NewFunc:                  func() runtime.Object { return &netguardv1beta1.Service{} },
		NewListFunc:              func() runtime.Object { return &netguardv1beta1.ServiceList{} },
		DefaultQualifiedResource: netguardv1beta1.Resource("services"),

		CreateStrategy: strategy,
		UpdateStrategy: strategy,
		DeleteStrategy: strategy,

		TableConvertor: rest.NewDefaultTableConvertor(netguardv1beta1.Resource("services")),
	}
	options := &generic.StoreOptions{RESTOptions: optsGetter}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, err
	}

	return &REST{store}, nil
}

// New creates a new Service object
func (r *REST) New() runtime.Object {
	return &netguardv1beta1.Service{}
}

// Destroy cleans up resources on shutdown.
func (r *REST) Destroy() {
	// Given that underlying store is shared with REST,
	// we don't destroy it here explicitly.
}
