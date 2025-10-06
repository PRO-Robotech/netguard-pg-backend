package addressgroup

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

// NetworksREST implements the Networks subresource for AddressGroup
type NetworksREST struct {
	backendClient client.BackendClient
}

// NewNetworksREST creates a new NetworksREST instance
func NewNetworksREST(backendClient client.BackendClient) *NetworksREST {
	return &NetworksREST{
		backendClient: backendClient,
	}
}

// Implement rest.Storage interface
var _ rest.Storage = &NetworksREST{}
var _ rest.Getter = &NetworksREST{}
var _ rest.Lister = &NetworksREST{}

// New returns a new empty Networks object
func (r *NetworksREST) New() runtime.Object {
	return &netguardv1beta1.NetworksSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "NetworksSpec",
		},
	}
}

// NewList returns a new empty Networks list object
func (r *NetworksREST) NewList() runtime.Object {
	return &netguardv1beta1.NetworksSpecList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "NetworksSpecList",
		},
	}
}

// Destroy cleans up resources
func (r *NetworksREST) Destroy() {
}

// Get retrieves the Networks subresource for a specific AddressGroup
func (r *NetworksREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Extract namespace from context
	namespace := utils.NamespaceFrom(ctx)

	// Get the parent AddressGroup
	addressGroupID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	addressGroup, err := r.backendClient.GetAddressGroup(ctx, addressGroupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get address group %s/%s: %w", namespace, name, err)
	}

	// Extract Networks from AddressGroup
	networks, err := r.getNetworksForAddressGroup(ctx, addressGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to get networks for address group %s/%s: %w", namespace, name, err)
	}

	// Set metadata for identification
	networks.ObjectMeta = metav1.ObjectMeta{
		Name:      addressGroup.Name,
		Namespace: addressGroup.Namespace,
	}

	return networks, nil
}

// List retrieves Networks for multiple AddressGroups (not typically used for subresources)
func (r *NetworksREST) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	// For subresources, List is typically not implemented or returns empty
	return &netguardv1beta1.NetworksSpecList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "NetworksSpecList",
		},
		Items: []netguardv1beta1.NetworksSpec{},
	}, nil
}

// ConvertToTable converts the Networks object to a table for kubectl output
func (r *NetworksREST) ConvertToTable(ctx context.Context, obj runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{
				Name:        "Name",
				Type:        "string",
				Format:      "name",
				Description: "Network name",
			},
			{
				Name:        "CIDR",
				Type:        "string",
				Description: "Network CIDR",
			},
			{
				Name:        "Kind",
				Type:        "string",
				Description: "Network resource kind",
			},
			{
				Name:        "Namespace",
				Type:        "string",
				Description: "Network resource namespace",
			},
		},
	}

	switch t := obj.(type) {
	case *netguardv1beta1.NetworksSpec:
		for _, network := range t.Items {
			table.Rows = append(table.Rows, metav1.TableRow{
				Cells: []interface{}{
					network.Name,
					network.CIDR,
					network.Kind,
					network.Namespace,
				},
			})
		}
	case *netguardv1beta1.NetworksSpecList:
		for _, networksSpec := range t.Items {
			for _, network := range networksSpec.Items {
				table.Rows = append(table.Rows, metav1.TableRow{
					Cells: []interface{}{
						network.Name,
						network.CIDR,
						network.Kind,
						network.Namespace,
					},
				})
			}
		}
	default:
		return nil, fmt.Errorf("unknown type %T", obj)
	}

	return table, nil
}

// getNetworksForAddressGroup extracts Networks from an AddressGroup
func (r *NetworksREST) getNetworksForAddressGroup(ctx context.Context, addressGroup *models.AddressGroup) (*netguardv1beta1.NetworksSpec, error) {
	if addressGroup == nil {
		return nil, fmt.Errorf("address group is nil")
	}

	// Convert domain NetworkItems to K8s NetworkItems
	networks := make([]netguardv1beta1.NetworkItem, len(addressGroup.Networks))
	for i, network := range addressGroup.Networks {
		networks[i] = netguardv1beta1.NetworkItem{
			Name:       network.Name,
			CIDR:       network.CIDR,
			ApiVersion: network.ApiVersion,
			Kind:       network.Kind,
			Namespace:  network.Namespace,
		}
	}

	return &netguardv1beta1.NetworksSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "NetworksSpec",
		},
		Items: networks,
	}, nil
}
