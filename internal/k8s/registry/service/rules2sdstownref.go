package service

import (
	"context"
	"fmt"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/utils"
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

// Compile-time interface assertions
var _ rest.Getter = &RuleS2SDstOwnRefREST{}
var _ rest.Lister = &RuleS2SDstOwnRefREST{}
var _ rest.TableConvertor = &RuleS2SDstOwnRefREST{}

// New returns a new RuleS2SDstOwnRefSpec object
func (r *RuleS2SDstOwnRefREST) New() runtime.Object {
	return &netguardv1beta1.RuleS2SDstOwnRefSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "RuleS2SDstOwnRefSpec",
		},
	}
}

// NewList returns a new RuleS2SDstOwnRefSpecList object
func (r *RuleS2SDstOwnRefREST) NewList() runtime.Object {
	return &netguardv1beta1.RuleS2SDstOwnRefSpecList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "RuleS2SDstOwnRefSpecList",
		},
	}
}

// Destroy cleans up resources
func (r *RuleS2SDstOwnRefREST) Destroy() {}

// Get retrieves the ruleS2SDstOwnRef for a specific Service
func (r *RuleS2SDstOwnRefREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)

	// Get the Service from backend
	serviceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	service, err := r.backendClient.GetService(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	// Get cross-namespace rules for this service
	ruleS2SDstOwnRefSpec, err := r.getCrossNamespaceRulesForService(ctx, service)
	if err != nil {
		return nil, err
	}

	// Set metadata for identification
	ruleS2SDstOwnRefSpec.ObjectMeta = metav1.ObjectMeta{
		Name:      service.Name,
		Namespace: service.Namespace,
	}

	return ruleS2SDstOwnRefSpec, nil
}

// List retrieves ruleS2SDstOwnRef for all Services in the namespace
func (r *RuleS2SDstOwnRefREST) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	scope := utils.ScopeFromContext(ctx)

	// Get all Services in scope
	services, err := r.backendClient.ListServices(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Build list response
	ruleS2SDstOwnRefSpecList := &netguardv1beta1.RuleS2SDstOwnRefSpecList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "RuleS2SDstOwnRefSpecList",
		},
		Items: []netguardv1beta1.RuleS2SDstOwnRefSpec{},
	}

	// Get cross-namespace rules for each service
	for _, service := range services {
		ruleS2SDstOwnRefSpec, err := r.getCrossNamespaceRulesForService(ctx, &service)
		if err != nil {
			// Log error but continue processing other services
			continue
		}

		// Set service name in metadata for identification
		ruleS2SDstOwnRefSpec.ObjectMeta = metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
		}

		ruleS2SDstOwnRefSpecList.Items = append(ruleS2SDstOwnRefSpecList.Items, *ruleS2SDstOwnRefSpec)
	}

	// Apply sorting (по умолчанию namespace + name, или через sortBy параметр)
	sortBy := utils.ExtractSortByFromContext(ctx)
	err = utils.ApplySorting(ruleS2SDstOwnRefSpecList.Items, sortBy,
		// idFn для извлечения ResourceIdentifier
		func(item netguardv1beta1.RuleS2SDstOwnRefSpec) models.ResourceIdentifier {
			return models.ResourceIdentifier{
				Name:      item.ObjectMeta.Name,
				Namespace: item.ObjectMeta.Namespace,
			}
		},
		// k8sObjectFn для конвертации в Kubernetes объект
		func(item netguardv1beta1.RuleS2SDstOwnRefSpec) runtime.Object {
			return &item
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sort rules: %w", err)
	}

	return ruleS2SDstOwnRefSpecList, nil
}

// ConvertToTable converts objects to tabular format for kubectl
func (r *RuleS2SDstOwnRefREST) ConvertToTable(ctx context.Context, obj runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Service", Type: "string", Format: "name", Description: "Service name"},
			{Name: "Namespace", Type: "string", Description: "Service namespace"},
			{Name: "CrossNamespaceRules", Type: "integer", Description: "Number of cross-namespace rules"},
		},
	}

	switch t := obj.(type) {
	case *netguardv1beta1.RuleS2SDstOwnRefSpec:
		table.Rows = []metav1.TableRow{
			{
				Cells: []interface{}{
					t.ObjectMeta.Name,
					t.ObjectMeta.Namespace,
					len(t.Items),
				},
				Object: runtime.RawExtension{Object: t},
			},
		}
	case *netguardv1beta1.RuleS2SDstOwnRefSpecList:
		for _, item := range t.Items {
			table.Rows = append(table.Rows, metav1.TableRow{
				Cells: []interface{}{
					item.ObjectMeta.Name,
					item.ObjectMeta.Namespace,
					len(item.Items),
				},
				Object: runtime.RawExtension{Object: &item},
			})
		}
	default:
		return nil, fmt.Errorf("unsupported object type: %T", obj)
	}

	return table, nil
}

// getCrossNamespaceRulesForService is a helper function to get cross-namespace rules for a service
func (r *RuleS2SDstOwnRefREST) getCrossNamespaceRulesForService(ctx context.Context, service *models.Service) (*netguardv1beta1.RuleS2SDstOwnRefSpec, error) {
	allRules, err := r.backendClient.ListRuleS2S(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list rules2s: %w", err)
	}

	// Build RuleS2SDstOwnRefSpec from rules
	ruleS2SDstOwnRefSpec := &netguardv1beta1.RuleS2SDstOwnRefSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "RuleS2SDstOwnRefSpec",
		},
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
