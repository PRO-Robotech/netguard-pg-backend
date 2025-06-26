package service

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// StatusREST implements the REST endpoint for Service status subresource
type StatusREST struct {
	store *ServiceStorage
}

// NewStatusREST creates a new StatusREST
func NewStatusREST(store *ServiceStorage) *StatusREST {
	return &StatusREST{store: store}
}

// New returns an empty Service object
func (r *StatusREST) New() runtime.Object {
	return &netguardv1beta1.Service{}
}

// Destroy cleans up resources
func (r *StatusREST) Destroy() {
	// Nothing to clean up
}

// Get retrieves the status of a Service
func (r *StatusREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Get the full object from the main store
	obj, err := r.store.Get(ctx, name, options)
	if err != nil {
		return nil, err
	}

	service, ok := obj.(*netguardv1beta1.Service)
	if !ok {
		return nil, fmt.Errorf("object is not a Service")
	}

	// Return only the status part
	return service, nil
}

// Update updates only the status of a Service
func (r *StatusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// Get current object
	currentObj, err := r.store.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	currentService, ok := currentObj.(*netguardv1beta1.Service)
	if !ok {
		return nil, false, fmt.Errorf("object is not a Service")
	}

	// Get updated object
	updatedObj, err := objInfo.UpdatedObject(ctx, currentObj)
	if err != nil {
		return nil, false, err
	}

	updatedService, ok := updatedObj.(*netguardv1beta1.Service)
	if !ok {
		return nil, false, fmt.Errorf("updated object is not a Service")
	}

	// Only update the status, preserve spec
	updatedService.Spec = currentService.Spec
	updatedService.ObjectMeta = currentService.ObjectMeta

	// Run validation if provided
	if updateValidation != nil {
		if err := updateValidation(ctx, updatedService, currentService); err != nil {
			return nil, false, err
		}
	}

	// For status updates, we don't need to call backend
	// Status is managed by controllers, not stored in backend

	return updatedService, false, nil
}

// GetResetFields returns the fields that should be reset during update
func (r *StatusREST) GetResetFields() map[string]interface{} {
	return map[string]interface{}{
		"spec": struct{}{},
	}
}
