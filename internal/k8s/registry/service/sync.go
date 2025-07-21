package service

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
)

// SyncREST implements the REST endpoint for Service sync subresource
type SyncREST struct {
	store         *ServiceStorage
	backendClient client.BackendClient
}

// NewSyncREST creates a new SyncREST
func NewSyncREST(store *ServiceStorage, backendClient client.BackendClient) *SyncREST {
	return &SyncREST{
		store:         store,
		backendClient: backendClient,
	}
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

	// Convert to backend format using BaseStorage converter
	backendService, err := r.store.BaseStorage.GetConverter().ToDomain(ctx, currentService)
	if err != nil {
		return nil, fmt.Errorf("failed to convert service to domain: %w", err)
	}

	// Trigger sync in backend using Sync API
	err = r.backendClient.Sync(ctx, models.SyncOpUpsert, []models.Service{*backendService})
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

// setServiceCondition sets or updates a condition on a Service
func setServiceCondition(service *netguardv1beta1.Service, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
		ObservedGeneration: service.Generation,
	}

	// Find existing condition and update or append new one
	for i, existingCondition := range service.Status.Conditions {
		if existingCondition.Type == conditionType {
			service.Status.Conditions[i] = condition
			return
		}
	}

	service.Status.Conditions = append(service.Status.Conditions, condition)
}
