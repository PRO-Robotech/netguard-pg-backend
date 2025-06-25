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
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
)

// AddressGroupsREST implements the addressGroups subresource for Service
type AddressGroupsREST struct {
	backendClient client.BackendClient
}

// NewAddressGroupsREST creates a new addressGroups subresource handler
func NewAddressGroupsREST(backendClient client.BackendClient) *AddressGroupsREST {
	return &AddressGroupsREST{
		backendClient: backendClient,
	}
}

var _ rest.Getter = &AddressGroupsREST{}

// New returns a new AddressGroupsSpec object
func (r *AddressGroupsREST) New() runtime.Object {
	return &netguardv1beta1.AddressGroupsSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupsSpec",
		},
	}
}

// Destroy cleans up resources
func (r *AddressGroupsREST) Destroy() {}

// Get retrieves the addressGroups for a Service
func (r *AddressGroupsREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Get the Service from backend
	serviceID := models.NewResourceIdentifier(name, models.WithNamespace(ctx.Value("namespace").(string)))
	service, err := r.backendClient.GetService(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	// Get all AddressGroupBindings that reference this Service
	scope := ports.NewResourceIdentifierScope(serviceID)
	bindings, err := r.backendClient.ListAddressGroupBindings(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list address group bindings: %w", err)
	}

	// Build AddressGroupsSpec from bindings
	addressGroupsSpec := &netguardv1beta1.AddressGroupsSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupsSpec",
		},
		Items: []netguardv1beta1.NamespacedObjectReference{},
	}

	// Collect unique address groups referenced by this service
	addressGroupMap := make(map[string]netguardv1beta1.NamespacedObjectReference)

	for _, binding := range bindings {
		// Check if this binding references our service
		if binding.ServiceRef.Name == service.Name && binding.ServiceRef.Namespace == service.Namespace {
			ref := netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       binding.AddressGroupRef.Name,
				},
				Namespace: binding.AddressGroupRef.Namespace,
			}

			// Use key to deduplicate
			key := fmt.Sprintf("%s/%s", ref.Namespace, ref.Name)
			addressGroupMap[key] = ref
		}
	}

	// Convert map to slice
	for _, ref := range addressGroupMap {
		addressGroupsSpec.Items = append(addressGroupsSpec.Items, ref)
	}

	return addressGroupsSpec, nil
}
