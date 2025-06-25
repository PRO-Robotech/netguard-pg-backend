/*
Copyright 2024 The Netguard Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// SyncREST implements the REST endpoint for Service sync subresource
type SyncREST struct {
	store *ServiceStorage
}

// NewSyncREST creates a new SyncREST
func NewSyncREST(store *ServiceStorage) *SyncREST {
	return &SyncREST{store: store}
}

// New returns an empty Service object
func (r *SyncREST) New() runtime.Object {
	return &netguardv1beta1.Service{}
}

// Destroy cleans up resources
func (r *SyncREST) Destroy() {
	// Nothing to clean up
}

// Create triggers manual sync for a Service
// Usage: kubectl create -f - <<EOF
// apiVersion: netguard.sgroups.io/v1beta1
// kind: Service
// metadata:
//
//	name: my-service
//	namespace: default
//
// EOF
func (r *SyncREST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	service, ok := obj.(*netguardv1beta1.Service)
	if !ok {
		return nil, fmt.Errorf("not a Service object")
	}

	// Get current service from backend
	currentObj, err := r.store.Get(ctx, service.Name, &metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("service not found for sync: %w", err)
	}

	currentService, ok := currentObj.(*netguardv1beta1.Service)
	if !ok {
		return nil, fmt.Errorf("object is not a Service")
	}

	// Convert to backend format
	backendService := convertServiceFromK8s(*currentService)

	// Trigger sync in backend using Sync API
	err = r.store.backendClient.Sync(ctx, models.SyncOpUpsert, []*models.Service{&backendService})
	if err != nil {
		// Set error status
		setServiceCondition(currentService, "Ready", metav1.ConditionFalse, "SyncFailed", fmt.Sprintf("Manual sync failed: %v", err))
		return currentService, fmt.Errorf("sync failed: %w", err)
	}

	// Set success status
	setServiceCondition(currentService, "Ready", metav1.ConditionTrue, "SyncSucceeded", "Manual sync completed successfully")

	return currentService, nil
}

// ConnectMethods returns the list of HTTP methods supported by this subresource
func (r *SyncREST) ConnectMethods() []string {
	return []string{"POST"}
}
