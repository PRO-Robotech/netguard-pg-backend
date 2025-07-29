package network_binding

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

// REST implements the RESTStorage interface for NetworkBinding resources
type REST struct {
	backendClient client.BackendClient
	converter     *converters.NetworkBindingConverter
	validator     *validation.NetworkBindingValidator
}

// NewREST creates a new REST storage for NetworkBinding resources
func NewREST(backendClient client.BackendClient) *REST {
	return &REST{
		backendClient: backendClient,
		converter:     converters.NewNetworkBindingConverter(),
		validator:     validation.NewNetworkBindingValidator(),
	}
}

// New returns a new NetworkBinding object
func (r *REST) New() runtime.Object {
	return &v1beta1.NetworkBinding{}
}

// NewList returns a new NetworkBindingList object
func (r *REST) NewList() runtime.Object {
	return &v1beta1.NetworkBindingList{}
}

// NamespaceScoped returns true if the resource is namespaced
func (r *REST) NamespaceScoped() bool {
	return true
}

// Get retrieves a NetworkBinding by name
func (r *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Extract namespace from context
	namespace, ok := NamespaceFrom(ctx)
	if !ok {
		return nil, errors.NewBadRequest("namespace is required")
	}

	// Get from backend
	binding, err := r.backendClient.GetNetworkBinding(ctx, models.ResourceIdentifier{
		Name:      name,
		Namespace: namespace,
	})
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, errors.NewNotFound(schema.GroupResource{Group: "netguard.sgroups.io", Resource: "networkbindings"}, name)
	}

	// Convert to K8s object
	return r.converter.FromDomain(ctx, binding)
}

// List retrieves a list of NetworkBindings
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
	bindings, err := r.backendClient.ListNetworkBindings(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Convert slice of models.NetworkBinding to slice of *models.NetworkBinding
	bindingPtrs := make([]*models.NetworkBinding, len(bindings))
	for i := range bindings {
		bindingPtrs[i] = &bindings[i]
	}

	// Convert to K8s list
	return r.converter.ToList(ctx, bindingPtrs)
}

// Create creates a new NetworkBinding
func (r *REST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	binding, ok := obj.(*v1beta1.NetworkBinding)
	if !ok {
		return nil, errors.NewBadRequest(fmt.Sprintf("not a NetworkBinding: %T", obj))
	}

	// Validate
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	// Convert to domain
	domainBinding, err := r.converter.ToDomain(ctx, binding)
	if err != nil {
		return nil, err
	}

	// Create in backend
	err = r.backendClient.CreateNetworkBinding(ctx, domainBinding)
	if err != nil {
		return nil, err
	}

	// Get the created object with conditions from backend
	createdBinding, err := r.backendClient.GetNetworkBinding(ctx, models.ResourceIdentifier{
		Name:      domainBinding.Name,
		Namespace: domainBinding.Namespace,
	})
	if err != nil {
		return nil, err
	}

	// Convert back to K8s
	return r.converter.FromDomain(ctx, createdBinding)
}

// Update updates a NetworkBinding
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

	binding, ok := obj.(*v1beta1.NetworkBinding)
	if !ok {
		return nil, false, errors.NewBadRequest(fmt.Sprintf("not a NetworkBinding: %T", obj))
	}

	// Validate update
	if updateValidation != nil {
		if err := updateValidation(ctx, obj, existingObj); err != nil {
			return nil, false, err
		}
	}

	// Convert to domain
	domainBinding, err := r.converter.ToDomain(ctx, binding)
	if err != nil {
		return nil, false, err
	}

	// Update in backend
	err = r.backendClient.UpdateNetworkBinding(ctx, domainBinding)
	if err != nil {
		return nil, false, err
	}

	// Get the updated object with conditions from backend
	updatedBinding, err := r.backendClient.GetNetworkBinding(ctx, models.ResourceIdentifier{
		Name:      domainBinding.Name,
		Namespace: domainBinding.Namespace,
	})
	if err != nil {
		return nil, false, err
	}

	// Convert back to K8s
	result, err := r.converter.FromDomain(ctx, updatedBinding)
	return result, false, err
}

// Delete deletes a NetworkBinding
func (r *REST) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	// Extract namespace from context
	namespace, ok := NamespaceFrom(ctx)
	if !ok {
		return nil, false, errors.NewBadRequest("namespace is required")
	}

	// Delete from backend
	err := r.backendClient.DeleteNetworkBinding(ctx, models.ResourceIdentifier{
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
			{Name: "Network", Type: "string"},
			{Name: "Address Group", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(binding *v1beta1.NetworkBinding) {
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: binding},
			Cells:  []interface{}{binding.Name, binding.Spec.NetworkRef.Name, binding.Spec.AddressGroupRef.Name, translateTimestampSince(binding.CreationTimestamp)},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *v1beta1.NetworkBinding:
		addRow(v)
	case *v1beta1.NetworkBindingList:
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
	return watch.NewEmptyWatch(), nil
}

// NamespaceFrom extracts namespace from context
func NamespaceFrom(ctx context.Context) (string, bool) {
	// This is a simplified implementation
	// In a real implementation, you would extract namespace from the request context
	return "default", true
}
