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

package rules2s

import (
	"context"
	"fmt"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	watchpkg "netguard-pg-backend/internal/k8s/registry/watch"
)

// RuleS2SStorage implements REST storage for RuleS2S resources
type RuleS2SStorage struct {
	backendClient client.BackendClient
}

// NewRuleS2SStorage creates a new RuleS2S storage
func NewRuleS2SStorage(backendClient client.BackendClient) *RuleS2SStorage {
	return &RuleS2SStorage{
		backendClient: backendClient,
	}
}

// New returns an empty RuleS2S object
func (s *RuleS2SStorage) New() runtime.Object {
	return &netguardv1beta1.RuleS2S{}
}

// NewList returns an empty RuleS2SList object
func (s *RuleS2SStorage) NewList() runtime.Object {
	return &netguardv1beta1.RuleS2SList{}
}

// NamespaceScoped returns true as RuleS2S are namespaced
func (s *RuleS2SStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name for the resource
func (s *RuleS2SStorage) GetSingularName() string {
	return "rules2s"
}

// Destroy cleans up resources on shutdown
func (s *RuleS2SStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves a RuleS2S by name from backend (READ-ONLY, no status changes)
func (s *RuleS2SStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Extract namespace from context
	namespace, ok := ctx.Value("namespace").(string)
	if !ok || namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	// Get from backend
	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	rule, err := s.backendClient.GetRuleS2S(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get RuleS2S %s/%s: %w", namespace, name, err)
	}

	// Convert to Kubernetes format
	k8sRule := convertRuleS2SToK8s(*rule)
	return k8sRule, nil
}

// List retrieves RuleS2S from backend with filtering (READ-ONLY, no status changes)
func (s *RuleS2SStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	// Create scope for filtering
	var scope ports.Scope
	if options != nil && options.FieldSelector != nil {
		// Extract namespace from field selector if present
		// For now, implement basic namespace filtering
		scope = ports.NewResourceIdentifierScope()
	}

	// Get from backend
	rules, err := s.backendClient.ListRuleS2S(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list RuleS2S: %w", err)
	}

	// Convert to Kubernetes format
	k8sRuleList := &netguardv1beta1.RuleS2SList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "RuleS2SList",
		},
	}

	for _, rule := range rules {
		k8sRule := convertRuleS2SToK8s(rule)
		k8sRuleList.Items = append(k8sRuleList.Items, *k8sRule)
	}

	return k8sRuleList, nil
}

// Create creates a new RuleS2S in backend via Sync API
func (s *RuleS2SStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sRule, ok := obj.(*netguardv1beta1.RuleS2S)
	if !ok {
		return nil, fmt.Errorf("expected RuleS2S, got %T", obj)
	}

	// Run validation if provided
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	// Convert to backend model
	rule := convertRuleS2SFromK8s(k8sRule)

	// Create via Sync API
	rules := []models.RuleS2S{rule}
	err := s.backendClient.Sync(ctx, models.SyncOpUpsert, rules)
	if err != nil {
		return nil, fmt.Errorf("failed to create RuleS2S: %w", err)
	}

	// Set successful status
	setCondition(k8sRule, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "RuleS2S successfully created in backend")

	return k8sRule, nil
}

// Update updates an existing RuleS2S in backend via Sync API
func (s *RuleS2SStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// Get current object
	currentObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if forceAllowCreate {
			// Convert to create operation
			newObj, err := objInfo.UpdatedObject(ctx, nil)
			if err != nil {
				return nil, false, err
			}
			createdObj, err := s.Create(ctx, newObj, createValidation, &metav1.CreateOptions{})
			return createdObj, true, err
		}
		return nil, false, err
	}

	// Get updated object
	updatedObj, err := objInfo.UpdatedObject(ctx, currentObj)
	if err != nil {
		return nil, false, err
	}

	k8sRule, ok := updatedObj.(*netguardv1beta1.RuleS2S)
	if !ok {
		return nil, false, fmt.Errorf("expected RuleS2S, got %T", updatedObj)
	}

	// Run validation if provided
	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, currentObj); err != nil {
			return nil, false, err
		}
	}

	// Convert to backend model
	rule := convertRuleS2SFromK8s(k8sRule)

	// Update via Sync API
	rules := []models.RuleS2S{rule}
	err = s.backendClient.Sync(ctx, models.SyncOpUpsert, rules)
	if err != nil {
		return nil, false, fmt.Errorf("failed to update RuleS2S: %w", err)
	}

	// Set successful status
	setCondition(k8sRule, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "RuleS2S successfully updated in backend")

	return k8sRule, false, nil
}

// Delete removes a RuleS2S from backend via Sync API
func (s *RuleS2SStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	// Get current object
	obj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	// Run validation if provided
	if deleteValidation != nil {
		if err := deleteValidation(ctx, obj); err != nil {
			return nil, false, err
		}
	}

	k8sRule, ok := obj.(*netguardv1beta1.RuleS2S)
	if !ok {
		return nil, false, fmt.Errorf("expected RuleS2S, got %T", obj)
	}

	// Convert to backend model
	rule := convertRuleS2SFromK8s(k8sRule)

	// Delete via Sync API
	rules := []models.RuleS2S{rule}
	err = s.backendClient.Sync(ctx, models.SyncOpDelete, rules)
	if err != nil {
		return nil, false, fmt.Errorf("failed to delete RuleS2S: %w", err)
	}

	return k8sRule, true, nil
}

// Watch implements watch functionality for RuleS2S
func (s *RuleS2SStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("rules2s")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// GetSupportedVerbs returns the supported verbs for this storage
func (s *RuleS2SStorage) GetSupportedVerbs() []string {
	return []string{"get", "list", "create", "update", "delete", "watch"}
}

// Helper functions for conversion

func convertRuleS2SToK8s(rule models.RuleS2S) *netguardv1beta1.RuleS2S {
	k8sRule := &netguardv1beta1.RuleS2S{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "RuleS2S",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rule.ResourceIdentifier.Name,
			Namespace: rule.ResourceIdentifier.Namespace,
		},
		Spec: netguardv1beta1.RuleS2SSpec{
			Traffic: string(rule.Traffic),
			ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       rule.ServiceLocalRef.Name,
				},
				Namespace: rule.ServiceLocalRef.Namespace,
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       rule.ServiceRef.Name,
				},
				Namespace: rule.ServiceRef.Namespace,
			},
		},
	}

	return k8sRule
}

func convertRuleS2SFromK8s(k8sRule *netguardv1beta1.RuleS2S) models.RuleS2S {
	rule := models.RuleS2S{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Name,
				models.WithNamespace(k8sRule.Namespace),
			),
		},
		Traffic: models.Traffic(k8sRule.Spec.Traffic),
		ServiceLocalRef: models.ServiceAliasRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Spec.ServiceLocalRef.Name,
				models.WithNamespace(k8sRule.Spec.ServiceLocalRef.Namespace),
			),
		},
		ServiceRef: models.ServiceAliasRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Spec.ServiceRef.Name,
				models.WithNamespace(k8sRule.Spec.ServiceRef.Namespace),
			),
		},
	}

	return rule
}

// Status condition helpers (same as in Service)
const (
	ConditionReady = "Ready"

	ReasonBindingCreated       = "BindingCreated"
	ReasonServiceNotFound      = "ServiceNotFound"
	ReasonAddressGroupNotFound = "AddressGroupNotFound"
	ReasonSyncFailed           = "SyncFailed"
	ReasonDeletionFailed       = "DeletionFailed"
)

func setCondition(obj runtime.Object, conditionType string, status metav1.ConditionStatus, reason, message string) {
	// TODO: Implement proper condition setting
	// This should be moved to a shared helper
}
