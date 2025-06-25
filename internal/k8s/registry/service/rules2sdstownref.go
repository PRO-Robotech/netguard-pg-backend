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
	"netguard-pg-backend/internal/k8s/client"
)

// RuleS2SDstOwnRefREST implements the ruleS2SDstOwnRef subresource for Service
type RuleS2SDstOwnRefREST struct {
	backendClient client.BackendClient
}

// NewRuleS2SDstOwnRefREST creates a new ruleS2SDstOwnRef subresource handler
func NewRuleS2SDstOwnRefREST(backendClient client.BackendClient) *RuleS2SDstOwnRefREST {
	return &RuleS2SDstOwnRefREST{
		backendClient: backendClient,
	}
}

var _ rest.Getter = &RuleS2SDstOwnRefREST{}

// New returns a new RuleS2SDstOwnRefSpec object
func (r *RuleS2SDstOwnRefREST) New() runtime.Object {
	return &netguardv1beta1.RuleS2SDstOwnRefSpec{}
}

// Destroy cleans up resources
func (r *RuleS2SDstOwnRefREST) Destroy() {}

// Get retrieves the ruleS2SDstOwnRef for a Service
func (r *RuleS2SDstOwnRefREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Get the Service from backend
	serviceID := models.NewResourceIdentifier(name, models.WithNamespace(ctx.Value("namespace").(string)))
	service, err := r.backendClient.GetService(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	// Get all RuleS2S that reference this Service as destination from other namespaces
	allRules, err := r.backendClient.ListRuleS2S(ctx, nil) // Get all rules
	if err != nil {
		return nil, fmt.Errorf("failed to list rules2s: %w", err)
	}

	// Build RuleS2SDstOwnRefSpec from rules
	ruleS2SDstOwnRefSpec := &netguardv1beta1.RuleS2SDstOwnRefSpec{
		Items: []netguardv1beta1.NamespacedObjectReference{},
	}

	// Collect rules that reference this service as destination from other namespaces
	for _, rule := range allRules {
		// Check if this rule references our service as destination AND is from different namespace
		if rule.ServiceRef.Name == service.Name &&
			rule.ServiceRef.Namespace == service.Namespace &&
			rule.Namespace != service.Namespace {

			ref := netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "RuleS2S",
					Name:       rule.Name,
				},
				Namespace: rule.Namespace,
			}

			ruleS2SDstOwnRefSpec.Items = append(ruleS2SDstOwnRefSpec.Items, ref)
		}
	}

	return ruleS2SDstOwnRefSpec, nil
}
