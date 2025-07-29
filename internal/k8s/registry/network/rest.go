package network

import (
	"context"
	"fmt"
	"time"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/converters"
	"netguard-pg-backend/internal/k8s/registry/validation"

	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"
)

// REST implements the RESTStorage interface for Network resources
type REST struct {
	backendClient client.BackendClient
	converter     *converters.NetworkConverter
	validator     *validation.NetworkValidator
}

// NewREST creates a new REST storage for Network resources
func NewREST(backendClient client.BackendClient) *REST {
	return &REST{
		backendClient: backendClient,
		converter:     converters.NewNetworkConverter(),
		validator:     validation.NewNetworkValidator(),
	}
}

// New returns a new Network object
func (r *REST) New() runtime.Object {
	return &v1beta1.Network{}
}

// NewList returns a new NetworkList object
func (r *REST) NewList() runtime.Object {
	return &v1beta1.NetworkList{}
}

// NamespaceScoped returns true if the resource is namespaced
func (r *REST) NamespaceScoped() bool {
	return true
}

// Get retrieves a Network by name
func (r *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Extract namespace from context
	namespace, ok := NamespaceFrom(ctx)
	if !ok {
		return nil, errors.NewBadRequest("namespace is required")
	}

	// Get from backend
	network, err := r.backendClient.GetNetwork(ctx, models.ResourceIdentifier{
		Name:      name,
		Namespace: namespace,
	})
	if err != nil {
		return nil, err
	}
	if network == nil {
		return nil, errors.NewNotFound(schema.GroupResource{Group: "netguard.sgroups.io", Resource: "networks"}, name)
	}

	// Convert to K8s object
	return r.converter.FromDomain(ctx, network)
}

// List retrieves a list of Networks
func (r *REST) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	// Extract namespace from context
	namespace, ok := NamespaceFrom(ctx)
	if !ok {
		return nil, errors.NewBadRequest("namespace is required")
	}

	// Create scope for namespace
	scope := ports.ResourceIdentifierScope{
		Identifiers: []models.ResourceIdentifier{
			{Namespace: namespace},
		},
	}

	// List from backend
	networks, err := r.backendClient.ListNetworks(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Convert slice of models.Network to slice of *models.Network
	networkPtrs := make([]*models.Network, len(networks))
	for i := range networks {
		networkPtrs[i] = &networks[i]
	}

	// Convert to K8s list
	return r.converter.ToList(ctx, networkPtrs)
}

// Create creates a new Network
func (r *REST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	network, ok := obj.(*v1beta1.Network)
	if !ok {
		return nil, errors.NewBadRequest(fmt.Sprintf("not a Network: %T", obj))
	}

	// Validate
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	// Convert to domain
	domainNetwork, err := r.converter.ToDomain(ctx, network)
	if err != nil {
		return nil, err
	}

	// Create in backend
	err = r.backendClient.CreateNetwork(ctx, domainNetwork)
	if err != nil {
		return nil, err
	}

	// Get the created object with conditions from backend
	createdNetwork, err := r.backendClient.GetNetwork(ctx, models.ResourceIdentifier{
		Name:      domainNetwork.Name,
		Namespace: domainNetwork.Namespace,
	})
	if err != nil {
		return nil, err
	}

	// Convert back to K8s
	return r.converter.FromDomain(ctx, createdNetwork)
}

// Update updates a Network
func (r *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// Get existing object
	existingObj, err := r.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	// Get updated object
	obj, err := objInfo.UpdatedObject(ctx, existingObj)
	if err != nil {
		return nil, false, err
	}

	network, ok := obj.(*v1beta1.Network)
	if !ok {
		return nil, false, errors.NewBadRequest(fmt.Sprintf("not a Network: %T", obj))
	}

	// Validate update
	if updateValidation != nil {
		if err := updateValidation(ctx, obj, existingObj); err != nil {
			return nil, false, err
		}
	}

	// Convert to domain
	domainNetwork, err := r.converter.ToDomain(ctx, network)
	if err != nil {
		return nil, false, err
	}

	// Update in backend
	err = r.backendClient.UpdateNetwork(ctx, domainNetwork)
	if err != nil {
		return nil, false, err
	}

	// Get the updated object with conditions from backend
	updatedNetwork, err := r.backendClient.GetNetwork(ctx, models.ResourceIdentifier{
		Name:      domainNetwork.Name,
		Namespace: domainNetwork.Namespace,
	})
	if err != nil {
		return nil, false, err
	}

	// Convert back to K8s
	result, err := r.converter.FromDomain(ctx, updatedNetwork)
	return result, false, err
}

// Delete deletes a Network
func (r *REST) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	// Extract namespace from context
	namespace, ok := NamespaceFrom(ctx)
	if !ok {
		return nil, false, errors.NewBadRequest("namespace is required")
	}

	// Delete from backend
	err := r.backendClient.DeleteNetwork(ctx, models.ResourceIdentifier{
		Name:      name,
		Namespace: namespace,
	})
	if err != nil {
		return nil, false, err
	}

	return &metav1.Status{Status: metav1.StatusSuccess}, false, nil
}

// ConvertToTable implements minimal table output so kubectl can display resources.
func (r *REST) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "meta.k8s.io/v1",
			Kind:       "Table",
		},
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "CIDR", Type: "string"},
			{Name: "Bound", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(network *v1beta1.Network) {
		bound := "No"
		if network.Status.IsBound {
			bound = "Yes"
		}

		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: network},
			Cells:  []interface{}{network.Name, network.Spec.CIDR, bound, translateTimestampSince(network.CreationTimestamp)},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *v1beta1.Network:
		addRow(v)
	case *v1beta1.NetworkList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}
	return table, nil
}

// helper function to format duration
func translateTimestampSince(ts metav1.Time) string {
	if ts.IsZero() {
		return "<unknown>"
	}
	return durationShortHumanDuration(time.Since(ts.Time))
}

func durationShortHumanDuration(d time.Duration) string {
	if seconds := int(d.Seconds()); seconds < 90 {
		return fmt.Sprintf("%ds", seconds)
	}
	if minutes := int(d.Minutes()); minutes < 90 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := int(d.Round(time.Hour).Hours())
	if hours < 48 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd", days)
}

// Destroy cleans up resources
func (r *REST) Destroy() {
	// No cleanup needed for this implementation
}

// Watch returns a watch.Interface for the resource
func (r *REST) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	// For now, return a no-op watch implementation
	// In a real implementation, you would return a proper watch interface
	return watch.NewEmptyWatch(), nil
}

// NamespaceFrom extracts namespace from context
func NamespaceFrom(ctx context.Context) (string, bool) {
	// This is a simplified implementation
	// In a real implementation, you would extract namespace from the request context
	return "default", true
}
