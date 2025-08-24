package apiserver

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// SimpleRESTStorage provides a minimal REST storage implementation for aggregated API server
type SimpleRESTStorage struct{}

// Ensure SimpleRESTStorage implements all required interfaces
var _ rest.Storage = &SimpleRESTStorage{}
var _ rest.Scoper = &SimpleRESTStorage{}
var _ rest.KindProvider = &SimpleRESTStorage{}
var _ rest.Getter = &SimpleRESTStorage{}
var _ rest.Lister = &SimpleRESTStorage{}
var _ rest.Watcher = &SimpleRESTStorage{}
var _ rest.SingularNameProvider = &SimpleRESTStorage{}

// New returns a new empty object
func (r *SimpleRESTStorage) New() runtime.Object {
	return &netguardv1beta1.AddressGroup{}
}

// NewList returns a new empty list object
func (r *SimpleRESTStorage) NewList() runtime.Object {
	return &netguardv1beta1.AddressGroupList{}
}

// Destroy cleans up resources
func (r *SimpleRESTStorage) Destroy() {
	// Minimal implementation for aggregated API server
}

// NamespaceScoped returns true if the resource is namespace scoped
func (r *SimpleRESTStorage) NamespaceScoped() bool {
	return true
}

// Kind returns the resource kind
func (r *SimpleRESTStorage) Kind() string {
	return "AddressGroup"
}

// Get retrieves a single object
func (r *SimpleRESTStorage) Get(ctx context.Context, name string, opts *metav1.GetOptions) (runtime.Object, error) {
	// For aggregated API server, return not found - this is a minimal implementation
	return nil, errors.NewNotFound(netguardv1beta1.Resource("addressgroups"), name)
}

// List retrieves multiple objects
func (r *SimpleRESTStorage) List(ctx context.Context, opts *metainternalversion.ListOptions) (runtime.Object, error) {
	// Return empty list for minimal implementation
	return &netguardv1beta1.AddressGroupList{
		Items: []netguardv1beta1.AddressGroup{},
	}, nil
}

// Watch returns an empty watch.Interface; required so that installer can create watch endpoints even if we don't support them yet.
func (r *SimpleRESTStorage) Watch(ctx context.Context, opts *metainternalversion.ListOptions) (watch.Interface, error) {
	return watch.NewEmptyWatch(), nil
}

// ConvertToTable converts objects to table format (required for kubectl output)
func (r *SimpleRESTStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	// Minimal table implementation
	return &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Age", Type: "string"},
		},
		Rows: []metav1.TableRow{},
	}, nil
}

func (r *SimpleRESTStorage) GetSingularName() string { return "addressgroup" }
