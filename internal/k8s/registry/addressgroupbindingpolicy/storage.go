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

package addressgroupbindingpolicy

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

// AddressGroupBindingPolicyStorage implements REST storage for AddressGroupBindingPolicy resources
type AddressGroupBindingPolicyStorage struct {
	backendClient client.BackendClient
}

// NewAddressGroupBindingPolicyStorage creates a new AddressGroupBindingPolicy storage
func NewAddressGroupBindingPolicyStorage(backendClient client.BackendClient) *AddressGroupBindingPolicyStorage {
	return &AddressGroupBindingPolicyStorage{
		backendClient: backendClient,
	}
}

// New returns an empty AddressGroupBindingPolicy object
func (s *AddressGroupBindingPolicyStorage) New() runtime.Object {
	return &netguardv1beta1.AddressGroupBindingPolicy{}
}

// NewList returns an empty AddressGroupBindingPolicyList object
func (s *AddressGroupBindingPolicyStorage) NewList() runtime.Object {
	return &netguardv1beta1.AddressGroupBindingPolicyList{}
}

// NamespaceScoped returns true as AddressGroupBindingPolicies are namespaced
func (s *AddressGroupBindingPolicyStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name for the resource
func (s *AddressGroupBindingPolicyStorage) GetSingularName() string {
	return "addressgroupbindingpolicy"
}

// Destroy cleans up resources on shutdown
func (s *AddressGroupBindingPolicyStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves an AddressGroupBindingPolicy by name from backend
func (s *AddressGroupBindingPolicyStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace, ok := ctx.Value("namespace").(string)
	if !ok || namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	policy, err := s.backendClient.GetAddressGroupBindingPolicy(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AddressGroupBindingPolicy %s/%s: %w", namespace, name, err)
	}

	k8sPolicy := convertAddressGroupBindingPolicyToK8s(*policy)
	return k8sPolicy, nil
}

// List retrieves AddressGroupBindingPolicies from backend with filtering
func (s *AddressGroupBindingPolicyStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	var scope ports.Scope
	if options != nil && options.FieldSelector != nil {
		scope = ports.NewResourceIdentifierScope()
	}

	policies, err := s.backendClient.ListAddressGroupBindingPolicies(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list AddressGroupBindingPolicies: %w", err)
	}

	k8sPolicyList := &netguardv1beta1.AddressGroupBindingPolicyList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "AddressGroupBindingPolicyList",
		},
	}

	for _, policy := range policies {
		k8sPolicy := convertAddressGroupBindingPolicyToK8s(policy)
		k8sPolicyList.Items = append(k8sPolicyList.Items, *k8sPolicy)
	}

	return k8sPolicyList, nil
}

// Create creates a new AddressGroupBindingPolicy in backend
func (s *AddressGroupBindingPolicyStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sPolicy, ok := obj.(*netguardv1beta1.AddressGroupBindingPolicy)
	if !ok {
		return nil, fmt.Errorf("expected AddressGroupBindingPolicy, got %T", obj)
	}

	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	policy := convertAddressGroupBindingPolicyFromK8s(k8sPolicy)
	err := s.backendClient.CreateAddressGroupBindingPolicy(ctx, &policy)
	if err != nil {
		return nil, fmt.Errorf("failed to create AddressGroupBindingPolicy: %w", err)
	}

	setCondition(k8sPolicy, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroupBindingPolicy successfully created in backend")

	return k8sPolicy, nil
}

// Update updates an existing AddressGroupBindingPolicy in backend
func (s *AddressGroupBindingPolicyStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	currentObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if forceAllowCreate {
			newObj, err := objInfo.UpdatedObject(ctx, nil)
			if err != nil {
				return nil, false, err
			}
			createdObj, err := s.Create(ctx, newObj, createValidation, &metav1.CreateOptions{})
			return createdObj, true, err
		}
		return nil, false, err
	}

	updatedObj, err := objInfo.UpdatedObject(ctx, currentObj)
	if err != nil {
		return nil, false, err
	}

	k8sPolicy, ok := updatedObj.(*netguardv1beta1.AddressGroupBindingPolicy)
	if !ok {
		return nil, false, fmt.Errorf("expected AddressGroupBindingPolicy, got %T", updatedObj)
	}

	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, currentObj); err != nil {
			return nil, false, err
		}
	}

	policy := convertAddressGroupBindingPolicyFromK8s(k8sPolicy)
	err = s.backendClient.UpdateAddressGroupBindingPolicy(ctx, &policy)
	if err != nil {
		return nil, false, fmt.Errorf("failed to update AddressGroupBindingPolicy: %w", err)
	}

	setCondition(k8sPolicy, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroupBindingPolicy successfully updated in backend")

	return k8sPolicy, false, nil
}

// Delete removes an AddressGroupBindingPolicy from backend
func (s *AddressGroupBindingPolicyStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	obj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	if deleteValidation != nil {
		if err := deleteValidation(ctx, obj); err != nil {
			return nil, false, err
		}
	}

	namespace, ok := ctx.Value("namespace").(string)
	if !ok || namespace == "" {
		return nil, false, fmt.Errorf("namespace is required")
	}

	id := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	err = s.backendClient.DeleteAddressGroupBindingPolicy(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("failed to delete AddressGroupBindingPolicy: %w", err)
	}

	return obj, true, nil
}

// Watch implements watch functionality for AddressGroupBindingPolicies
func (s *AddressGroupBindingPolicyStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("addressgroupbindingpolicies")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// Helper functions for conversion
func convertAddressGroupBindingPolicyToK8s(policy models.AddressGroupBindingPolicy) *netguardv1beta1.AddressGroupBindingPolicy {
	k8sPolicy := &netguardv1beta1.AddressGroupBindingPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "AddressGroupBindingPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      policy.ResourceIdentifier.Name,
			Namespace: policy.ResourceIdentifier.Namespace,
		},
		Spec: netguardv1beta1.AddressGroupBindingPolicySpec{
			AddressGroupRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       policy.AddressGroupRef.Name,
				},
				Namespace: policy.AddressGroupRef.Namespace,
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       policy.ServiceRef.Name,
				},
				Namespace: policy.ServiceRef.Namespace,
			},
		},
	}

	return k8sPolicy
}

func convertAddressGroupBindingPolicyFromK8s(k8sPolicy *netguardv1beta1.AddressGroupBindingPolicy) models.AddressGroupBindingPolicy {
	policy := models.AddressGroupBindingPolicy{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sPolicy.Name,
				models.WithNamespace(k8sPolicy.Namespace),
			),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sPolicy.Spec.ServiceRef.Name,
				models.WithNamespace(k8sPolicy.Spec.ServiceRef.Namespace),
			),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sPolicy.Spec.AddressGroupRef.Name,
				models.WithNamespace(k8sPolicy.Spec.AddressGroupRef.Namespace),
			),
		},
	}

	return policy
}

// Status condition helpers
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
}
