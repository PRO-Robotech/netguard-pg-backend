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

package ieagagrule

import (
	"context"
	"fmt"
	"strings"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	watchpkg "netguard-pg-backend/internal/k8s/registry/watch"
)

// IEAgAgRuleStorage implements REST storage for IEAgAgRule resources
type IEAgAgRuleStorage struct {
	backendClient client.BackendClient
}

// NewIEAgAgRuleStorage creates a new IEAgAgRule storage
func NewIEAgAgRuleStorage(backendClient client.BackendClient) *IEAgAgRuleStorage {
	return &IEAgAgRuleStorage{
		backendClient: backendClient,
	}
}

// New returns an empty IEAgAgRule object
func (s *IEAgAgRuleStorage) New() runtime.Object {
	return &netguardv1beta1.IEAgAgRule{}
}

// NewList returns an empty IEAgAgRuleList object
func (s *IEAgAgRuleStorage) NewList() runtime.Object {
	return &netguardv1beta1.IEAgAgRuleList{}
}

// NamespaceScoped returns true as IEAgAgRules are namespaced
func (s *IEAgAgRuleStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name for the resource
func (s *IEAgAgRuleStorage) GetSingularName() string {
	return "ieagagrule"
}

// Destroy cleans up resources on shutdown
func (s *IEAgAgRuleStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves an IEAgAgRule by name from backend
func (s *IEAgAgRuleStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace, ok := ctx.Value("namespace").(string)
	if !ok || namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	rule, err := s.backendClient.GetIEAgAgRule(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get IEAgAgRule %s/%s: %w", namespace, name, err)
	}

	k8sRule := convertIEAgAgRuleToK8s(*rule)
	return k8sRule, nil
}

// List retrieves IEAgAgRules from backend with filtering
func (s *IEAgAgRuleStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	var scope ports.Scope
	if options != nil && options.FieldSelector != nil {
		scope = ports.NewResourceIdentifierScope()
	}

	rules, err := s.backendClient.ListIEAgAgRules(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list IEAgAgRules: %w", err)
	}

	k8sRuleList := &netguardv1beta1.IEAgAgRuleList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "IEAgAgRuleList",
		},
	}

	for _, rule := range rules {
		k8sRule := convertIEAgAgRuleToK8s(rule)
		k8sRuleList.Items = append(k8sRuleList.Items, *k8sRule)
	}

	return k8sRuleList, nil
}

// Create creates a new IEAgAgRule in backend via Sync API
func (s *IEAgAgRuleStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sRule, ok := obj.(*netguardv1beta1.IEAgAgRule)
	if !ok {
		return nil, fmt.Errorf("expected IEAgAgRule, got %T", obj)
	}

	// Convert to backend model
	rule := convertIEAgAgRuleFromK8s(*k8sRule)

	// Create via Sync API
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.IEAgAgRule{rule}); err != nil {
		return nil, fmt.Errorf("failed to create IEAgAgRule via sync: %w", err)
	}

	return k8sRule, nil
}

// Update updates an IEAgAgRule in backend via Sync API
func (s *IEAgAgRuleStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	existing, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	updated, err := objInfo.UpdatedObject(ctx, existing)
	if err != nil {
		return nil, false, err
	}

	k8sRule, ok := updated.(*netguardv1beta1.IEAgAgRule)
	if !ok {
		return nil, false, fmt.Errorf("expected IEAgAgRule, got %T", updated)
	}

	// Convert to backend model
	rule := convertIEAgAgRuleFromK8s(*k8sRule)

	// Update via Sync API
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.IEAgAgRule{rule}); err != nil {
		return nil, false, fmt.Errorf("failed to update IEAgAgRule via sync: %w", err)
	}

	return k8sRule, false, nil
}

// Delete deletes an IEAgAgRule from backend via Sync API
func (s *IEAgAgRuleStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	existing, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	k8sRule := existing.(*netguardv1beta1.IEAgAgRule)

	// Convert to backend model
	rule := convertIEAgAgRuleFromK8s(*k8sRule)

	// Delete via Sync API
	if err := s.backendClient.Sync(ctx, models.SyncOpDelete, []models.IEAgAgRule{rule}); err != nil {
		return nil, false, fmt.Errorf("failed to delete IEAgAgRule via sync: %w", err)
	}

	return k8sRule, true, nil
}

// Watch implements watch functionality
func (s *IEAgAgRuleStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("ieagagrules")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// convertIEAgAgRuleToK8s converts from backend model to K8s API
func convertIEAgAgRuleToK8s(rule models.IEAgAgRule) *netguardv1beta1.IEAgAgRule {
	return &netguardv1beta1.IEAgAgRule{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "IEAgAgRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rule.SelfRef.Name,
			Namespace: rule.SelfRef.Namespace,
		},
		Spec: netguardv1beta1.IEAgAgRuleSpec{
			Transport: string(rule.Transport),
			Traffic:   string(rule.Traffic),
			AddressGroupLocal: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       rule.AddressGroupLocal.Name,
			},
			AddressGroup: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       rule.AddressGroup.Name,
			},
			Ports:    convertPortSpecsToK8s(rule.Ports),
			Action:   string(rule.Action),
			Priority: rule.Priority,
		},
	}
}

// convertIEAgAgRuleFromK8s converts from K8s API to backend model
func convertIEAgAgRuleFromK8s(k8sRule netguardv1beta1.IEAgAgRule) models.IEAgAgRule {
	return models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Name,
				models.WithNamespace(k8sRule.Namespace),
			),
		},
		Transport: models.TransportProtocol(k8sRule.Spec.Transport),
		Traffic:   models.Traffic(k8sRule.Spec.Traffic),
		AddressGroupLocal: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Spec.AddressGroupLocal.Name,
				models.WithNamespace(k8sRule.Namespace), // Same namespace as rule
			),
		},
		AddressGroup: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Spec.AddressGroup.Name,
				models.WithNamespace(k8sRule.Namespace), // Same namespace as rule
			),
		},
		Ports:    convertPortSpecsFromK8s(k8sRule.Spec.Ports),
		Action:   models.RuleAction(k8sRule.Spec.Action),
		Priority: k8sRule.Spec.Priority,
	}
}

// Helper functions for port conversion
func convertPortSpecsToK8s(portSpecs []models.PortSpec) []netguardv1beta1.PortSpec {
	var k8sPortSpecs []netguardv1beta1.PortSpec
	for _, portSpec := range portSpecs {
		k8sPortSpec := netguardv1beta1.PortSpec{}

		// Convert destination port string to PortRange
		if portSpec.Destination != "" {
			// Parse port string using existing validation function
			portRanges, err := validation.ParsePortRanges(portSpec.Destination)
			if err == nil && len(portRanges) > 0 {
				// Use first port range
				portRange := portRanges[0]
				if portRange.Start == portRange.End {
					// Single port
					k8sPortSpec.Port = int32(portRange.Start)
				} else {
					// Port range
					k8sPortSpec.PortRange = &netguardv1beta1.PortRange{
						From: int32(portRange.Start),
						To:   int32(portRange.End),
					}
				}
			}
		}

		k8sPortSpecs = append(k8sPortSpecs, k8sPortSpec)
	}
	return k8sPortSpecs
}

func convertPortSpecsFromK8s(k8sPortSpecs []netguardv1beta1.PortSpec) []models.PortSpec {
	var portSpecs []models.PortSpec
	for _, k8sPortSpec := range k8sPortSpecs {
		portSpec := models.PortSpec{}

		if k8sPortSpec.Port != 0 {
			portSpec.Destination = fmt.Sprintf("%d", k8sPortSpec.Port)
		} else if k8sPortSpec.PortRange != nil {
			portSpec.Destination = fmt.Sprintf("%d-%d", k8sPortSpec.PortRange.From, k8sPortSpec.PortRange.To)
		}

		portSpecs = append(portSpecs, portSpec)
	}
	return portSpecs
}

// formatPortRangesToString converts []models.PortRange to comma-separated string like "80,443,8080-9090"
func formatPortRangesToString(ranges []models.PortRange) string {
	var parts []string
	for _, portRange := range ranges {
		if portRange.Start == portRange.End {
			// Single port
			parts = append(parts, fmt.Sprintf("%d", portRange.Start))
		} else {
			// Port range
			parts = append(parts, fmt.Sprintf("%d-%d", portRange.Start, portRange.End))
		}
	}
	return strings.Join(parts, ",")
}
