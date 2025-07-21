package base

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// Mock K8s object for testing
type MockK8sObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MockSpec   `json:"spec,omitempty"`
	Status            MockStatus `json:"status,omitempty"`
}

type MockSpec struct {
	Value string `json:"value,omitempty"`
}

type MockStatus struct {
	Phase string `json:"phase,omitempty"`
}

func (m *MockK8sObject) DeepCopyObject() runtime.Object {
	return &MockK8sObject{
		TypeMeta:   m.TypeMeta,
		ObjectMeta: *m.ObjectMeta.DeepCopy(),
		Spec:       m.Spec,
		Status:     m.Status,
	}
}

func (m *MockK8sObject) GetObjectKind() schema.ObjectKind {
	return &m.TypeMeta
}

// Mock K8s list object for testing
type MockK8sList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MockK8sObject `json:"items"`
}

func (m *MockK8sList) DeepCopyObject() runtime.Object {
	items := make([]MockK8sObject, len(m.Items))
	copy(items, m.Items)
	return &MockK8sList{
		TypeMeta: m.TypeMeta,
		ListMeta: *m.ListMeta.DeepCopy(),
		Items:    items,
	}
}

func (m *MockK8sList) GetObjectKind() schema.ObjectKind {
	return &m.TypeMeta
}

// Mock domain object for testing
type MockDomain struct {
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Value     string `json:"value"`
}

func (m *MockDomain) GetID() string {
	return m.ID
}

func (m *MockDomain) GetNamespace() string {
	return m.Namespace
}

func (m *MockDomain) GetName() string {
	return m.Name
}

// Mock converter for testing
type MockConverter struct {
	toDomainFunc   func(ctx context.Context, k8sObj *MockK8sObject) (*MockDomain, error)
	fromDomainFunc func(ctx context.Context, domainObj *MockDomain) (*MockK8sObject, error)
	toListFunc     func(ctx context.Context, domainObjs []*MockDomain) (runtime.Object, error)
}

func (m *MockConverter) ToDomain(ctx context.Context, k8sObj *MockK8sObject) (*MockDomain, error) {
	if m.toDomainFunc != nil {
		return m.toDomainFunc(ctx, k8sObj)
	}
	return &MockDomain{
		ID:        string(k8sObj.UID),
		Namespace: k8sObj.Namespace,
		Name:      k8sObj.Name,
		Value:     k8sObj.Spec.Value,
	}, nil
}

func (m *MockConverter) FromDomain(ctx context.Context, domainObj *MockDomain) (*MockK8sObject, error) {
	if m.fromDomainFunc != nil {
		return m.fromDomainFunc(ctx, domainObj)
	}
	return &MockK8sObject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "test.example.com/v1",
			Kind:       "MockObject",
		},
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID(domainObj.ID),
			Namespace: domainObj.Namespace,
			Name:      domainObj.Name,
		},
		Spec: MockSpec{
			Value: domainObj.Value,
		},
		Status: MockStatus{
			Phase: "Ready",
		},
	}, nil
}

func (m *MockConverter) ToList(ctx context.Context, domainObjs []*MockDomain) (runtime.Object, error) {
	if m.toListFunc != nil {
		return m.toListFunc(ctx, domainObjs)
	}

	items := make([]MockK8sObject, len(domainObjs))
	for i, domainObj := range domainObjs {
		k8sObj, err := m.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, err
		}
		items[i] = *k8sObj
	}

	return &MockK8sList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "test.example.com/v1",
			Kind:       "MockObjectList",
		},
		ListMeta: metav1.ListMeta{},
		Items:    items,
	}, nil
}

// Mock validator for testing
type MockValidator struct {
	validateCreateFunc func(ctx context.Context, obj *MockK8sObject) field.ErrorList
	validateUpdateFunc func(ctx context.Context, obj *MockK8sObject, old *MockK8sObject) field.ErrorList
	validateDeleteFunc func(ctx context.Context, obj *MockK8sObject) field.ErrorList
}

func (m *MockValidator) ValidateCreate(ctx context.Context, obj *MockK8sObject) field.ErrorList {
	if m.validateCreateFunc != nil {
		return m.validateCreateFunc(ctx, obj)
	}
	return field.ErrorList{}
}

func (m *MockValidator) ValidateUpdate(ctx context.Context, obj *MockK8sObject, old *MockK8sObject) field.ErrorList {
	if m.validateUpdateFunc != nil {
		return m.validateUpdateFunc(ctx, obj, old)
	}
	return field.ErrorList{}
}

func (m *MockValidator) ValidateDelete(ctx context.Context, obj *MockK8sObject) field.ErrorList {
	if m.validateDeleteFunc != nil {
		return m.validateDeleteFunc(ctx, obj)
	}
	return field.ErrorList{}
}

// MockBackendOperations implements BackendOperations for testing
type MockBackendOperations struct {
	getFunc    func(ctx context.Context, id models.ResourceIdentifier) (**MockDomain, error)
	listFunc   func(ctx context.Context, scope ports.Scope) ([]*MockDomain, error)
	createFunc func(ctx context.Context, obj **MockDomain) error
	updateFunc func(ctx context.Context, obj **MockDomain) error
	deleteFunc func(ctx context.Context, id models.ResourceIdentifier) error
}

func (m *MockBackendOperations) Get(ctx context.Context, id models.ResourceIdentifier) (**MockDomain, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	result := &MockDomain{
		ID:        "test-id",
		Namespace: id.Namespace,
		Name:      id.Name,
		Value:     "test-value",
	}
	return &result, nil
}

func (m *MockBackendOperations) List(ctx context.Context, scope ports.Scope) ([]*MockDomain, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, scope)
	}
	return []*MockDomain{}, nil
}

func (m *MockBackendOperations) Create(ctx context.Context, obj **MockDomain) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, obj)
	}
	return nil
}

func (m *MockBackendOperations) Update(ctx context.Context, obj **MockDomain) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, obj)
	}
	return nil
}

func (m *MockBackendOperations) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

// Helper function to create a test BaseStorage
func createTestStorage() *BaseStorage[*MockK8sObject, *MockDomain] {
	return &BaseStorage[*MockK8sObject, *MockDomain]{
		NewFunc: func() *MockK8sObject {
			return &MockK8sObject{}
		},
		NewListFunc: func() runtime.Object {
			return &MockK8sList{}
		},
		backendOps:   &MockBackendOperations{},
		converter:    &MockConverter{},
		validator:    &MockValidator{},
		watcher:      watch.NewBroadcaster(100, watch.DropIfChannelFull),
		resourceName: "mockobjects",
		kindName:     "MockObject",
		isNamespaced: true,
	}
}

// Helper function to create context with namespace
func createTestContext(namespace string) context.Context {
	ctx := context.Background()
	return context.WithValue(ctx, "namespace", namespace)
}

func TestBaseStorage_New(t *testing.T) {
	storage := createTestStorage()

	obj := storage.New()
	mockObj, ok := obj.(*MockK8sObject)
	if !ok {
		t.Errorf("Expected *MockK8sObject, got %T", obj)
	}

	if mockObj == nil {
		t.Error("Expected non-nil object")
	}
}

func TestBaseStorage_NewList(t *testing.T) {
	storage := createTestStorage()

	list := storage.NewList()
	mockList, ok := list.(*MockK8sList)
	if !ok {
		t.Errorf("Expected *MockK8sList, got %T", list)
	}

	if mockList == nil {
		t.Error("Expected non-nil list")
	}
}

func TestBaseStorage_ConvertToTable_SingleObject(t *testing.T) {
	storage := createTestStorage()

	obj := &MockK8sObject{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-object",
		},
	}

	table, err := storage.ConvertToTable(context.TODO(), obj, nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if table.Kind != "Table" {
		t.Errorf("Expected kind 'Table', got %s", table.Kind)
	}

	if len(table.Rows) != 1 {
		t.Errorf("Expected 1 row, got %d", len(table.Rows))
	}

	if table.Rows[0].Cells[0] != "test-object" {
		t.Errorf("Expected first cell to be 'test-object', got %v", table.Rows[0].Cells[0])
	}
}

func TestBaseStorage_ConvertToTable_List(t *testing.T) {
	storage := createTestStorage()

	list := &MockK8sList{
		Items: []MockK8sObject{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-object-1",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-object-2",
				},
			},
		},
	}

	table, err := storage.ConvertToTable(context.TODO(), list, nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(table.Rows) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(table.Rows))
	}

	if table.Rows[0].Cells[0] != "test-object-1" {
		t.Errorf("Expected first cell to be 'test-object-1', got %v", table.Rows[0].Cells[0])
	}

	if table.Rows[1].Cells[0] != "test-object-2" {
		t.Errorf("Expected second cell to be 'test-object-2', got %v", table.Rows[1].Cells[0])
	}
}

func TestBaseStorage_GetConverter(t *testing.T) {
	storage := createTestStorage()

	converter := storage.GetConverter()
	if converter == nil {
		t.Error("Expected non-nil converter")
	}

	if converter != storage.converter {
		t.Error("Expected converter to be the same instance")
	}
}

func TestBaseStorage_GetBackendOps(t *testing.T) {
	storage := createTestStorage()

	backendOps := storage.GetBackendOps()
	if backendOps == nil {
		t.Error("Expected non-nil backend operations")
	}

	if backendOps != storage.backendOps {
		t.Error("Expected backend operations to be the same instance")
	}
}

func TestBaseStorage_BroadcastWatchEvent(t *testing.T) {
	storage := createTestStorage()

	obj := &MockK8sObject{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-object",
		},
	}

	// Test that broadcasting doesn't panic
	storage.broadcastWatchEvent(watch.Added, obj)
	storage.broadcastWatchEvent(watch.Modified, obj)
	storage.broadcastWatchEvent(watch.Deleted, obj)

	// Test with nil watcher
	storage.watcher = nil
	storage.broadcastWatchEvent(watch.Added, obj)
}

func TestBaseStorage_ApplyPatch_UnsupportedType(t *testing.T) {
	storage := createTestStorage()

	obj := &MockK8sObject{}
	_, err := storage.applyPatch(obj, types.PatchType("unsupported"), []byte{})
	if err == nil {
		t.Error("Expected error for unsupported patch type")
	}

	expectedErr := "unsupported patch type: unsupported"
	if err.Error() != expectedErr {
		t.Errorf("Expected '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestBaseStorage_ApplyPatch_SupportedTypes(t *testing.T) {
	storage := createTestStorage()

	obj := &MockK8sObject{}

	// Test JSON patch
	_, err := storage.applyPatch(obj, types.JSONPatchType, []byte{})
	if err == nil {
		t.Error("Expected error for unimplemented JSON patch")
	}

	expectedErr := "JSON patch not implemented yet"
	if err.Error() != expectedErr {
		t.Errorf("Expected '%s', got '%s'", expectedErr, err.Error())
	}

	// Test merge patch
	_, err = storage.applyPatch(obj, types.MergePatchType, []byte{})
	if err == nil {
		t.Error("Expected error for unimplemented merge patch")
	}

	expectedErr = "merge patch not implemented yet"
	if err.Error() != expectedErr {
		t.Errorf("Expected '%s', got '%s'", expectedErr, err.Error())
	}

	// Test strategic merge patch
	_, err = storage.applyPatch(obj, types.StrategicMergePatchType, []byte{})
	if err == nil {
		t.Error("Expected error for unimplemented strategic merge patch")
	}

	expectedErr = "strategic merge patch not implemented yet"
	if err.Error() != expectedErr {
		t.Errorf("Expected '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestBaseStorage_GetObjectName(t *testing.T) {
	obj := &MockK8sObject{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-object",
		},
	}

	name := getObjectName(obj)
	if name != "test-object" {
		t.Errorf("Expected 'test-object', got '%s'", name)
	}

	// Test with object that doesn't implement meta.Accessor
	invalidObj := &struct{ runtime.Object }{}
	name = getObjectName(invalidObj)
	if name != "unknown" {
		t.Errorf("Expected 'unknown', got '%s'", name)
	}
}

func TestBaseStorage_BackendMethods_WithRealImplementation(t *testing.T) {
	storage := createTestStorage()
	ctx := context.TODO()

	// Test getFromBackend - should work now
	domain, err := storage.getFromBackend(ctx, "test-ns", "test-name")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if domain == nil {
		t.Error("Expected non-nil domain object")
	}

	// Test listFromBackend - should work now
	domains, err := storage.listFromBackend(ctx, nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if domains == nil {
		t.Error("Expected non-nil domain list")
	}

	// Test createInBackend - should work now
	mockDomain := &MockDomain{Name: "test"}
	_, err = storage.createInBackend(ctx, &mockDomain)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test updateInBackend - should work now
	_, err = storage.updateInBackend(ctx, &mockDomain)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test deleteFromBackend - should work now
	err = storage.deleteFromBackend(ctx, "test-ns", "test-name")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestBaseStorage_InterfaceAssertion(t *testing.T) {
	storage := createTestStorage()

	// Test that all interface assertions are satisfied
	var _ rest.Storage = storage
	var _ rest.Getter = storage
	var _ rest.Lister = storage
	var _ rest.Creater = storage
	var _ rest.Updater = storage
	var _ rest.Patcher = storage
	var _ rest.GracefulDeleter = storage
	var _ rest.Watcher = storage
}

func TestNewBaseStorage(t *testing.T) {
	newFunc := func() *MockK8sObject { return &MockK8sObject{} }
	newListFunc := func() runtime.Object { return &MockK8sList{} }
	backendOps := &MockBackendOperations{}
	converter := &MockConverter{}
	validator := &MockValidator{}
	watcher := watch.NewBroadcaster(100, watch.DropIfChannelFull)

	storage := NewBaseStorage(
		newFunc,
		newListFunc,
		backendOps,
		converter,
		validator,
		watcher,
		"testresources",
		"TestResource",
		true,
	)

	if storage == nil {
		t.Error("Expected non-nil storage")
	}

	if storage.resourceName != "testresources" {
		t.Errorf("Expected resourceName 'testresources', got '%s'", storage.resourceName)
	}

	if storage.kindName != "TestResource" {
		t.Errorf("Expected kindName 'TestResource', got '%s'", storage.kindName)
	}

	if !storage.isNamespaced {
		t.Error("Expected isNamespaced to be true")
	}

	if storage.backendOps != backendOps {
		t.Error("Expected backendOps to be set correctly")
	}

	if storage.converter != converter {
		t.Error("Expected converter to be set correctly")
	}

	if storage.validator != validator {
		t.Error("Expected validator to be set correctly")
	}

	if storage.watcher != watcher {
		t.Error("Expected watcher to be set correctly")
	}
}

func TestMockConverter_ToDomain(t *testing.T) {
	converter := &MockConverter{}

	k8sObj := &MockK8sObject{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "test-uid",
			Namespace: "test-namespace",
			Name:      "test-name",
		},
		Spec: MockSpec{
			Value: "test-value",
		},
	}

	domain, err := converter.ToDomain(context.TODO(), k8sObj)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if domain.ID != "test-uid" {
		t.Errorf("Expected ID 'test-uid', got %s", domain.ID)
	}

	if domain.Name != "test-name" {
		t.Errorf("Expected name 'test-name', got %s", domain.Name)
	}

	if domain.Namespace != "test-namespace" {
		t.Errorf("Expected namespace 'test-namespace', got %s", domain.Namespace)
	}

	if domain.Value != "test-value" {
		t.Errorf("Expected value 'test-value', got %s", domain.Value)
	}
}

func TestMockConverter_FromDomain(t *testing.T) {
	converter := &MockConverter{}

	domain := &MockDomain{
		ID:        "test-id",
		Namespace: "test-namespace",
		Name:      "test-name",
		Value:     "test-value",
	}

	k8sObj, err := converter.FromDomain(context.TODO(), domain)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if k8sObj.UID != "test-id" {
		t.Errorf("Expected UID 'test-id', got %s", k8sObj.UID)
	}

	if k8sObj.Name != "test-name" {
		t.Errorf("Expected name 'test-name', got %s", k8sObj.Name)
	}

	if k8sObj.Namespace != "test-namespace" {
		t.Errorf("Expected namespace 'test-namespace', got %s", k8sObj.Namespace)
	}

	if k8sObj.Spec.Value != "test-value" {
		t.Errorf("Expected spec value 'test-value', got %s", k8sObj.Spec.Value)
	}

	if k8sObj.Status.Phase != "Ready" {
		t.Errorf("Expected status phase 'Ready', got %s", k8sObj.Status.Phase)
	}
}

func TestMockConverter_ToList(t *testing.T) {
	converter := &MockConverter{}

	domains := []*MockDomain{
		{
			ID:        "test-id-1",
			Namespace: "test-namespace",
			Name:      "test-name-1",
			Value:     "test-value-1",
		},
		{
			ID:        "test-id-2",
			Namespace: "test-namespace",
			Name:      "test-name-2",
			Value:     "test-value-2",
		},
	}

	listObj, err := converter.ToList(context.TODO(), domains)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	list, ok := listObj.(*MockK8sList)
	if !ok {
		t.Errorf("Expected *MockK8sList, got %T", listObj)
	}

	if len(list.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(list.Items))
	}

	if list.Items[0].Name != "test-name-1" {
		t.Errorf("Expected first item name 'test-name-1', got %s", list.Items[0].Name)
	}

	if list.Items[1].Name != "test-name-2" {
		t.Errorf("Expected second item name 'test-name-2', got %s", list.Items[1].Name)
	}
}

func TestMockValidator_ValidateCreate(t *testing.T) {
	validator := &MockValidator{}

	obj := &MockK8sObject{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-object",
		},
	}

	errors := validator.ValidateCreate(context.TODO(), obj)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d", len(errors))
	}
}

func TestMockValidator_ValidateCreate_WithErrors(t *testing.T) {
	validator := &MockValidator{
		validateCreateFunc: func(ctx context.Context, obj *MockK8sObject) field.ErrorList {
			return field.ErrorList{
				field.Invalid(field.NewPath("spec", "value"), obj.Spec.Value, "invalid value"),
			}
		},
	}

	obj := &MockK8sObject{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-object",
		},
		Spec: MockSpec{
			Value: "invalid",
		},
	}

	errors := validator.ValidateCreate(context.TODO(), obj)
	if len(errors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(errors))
	}

	if errors[0].Field != "spec.value" {
		t.Errorf("Expected error field 'spec.value', got %s", errors[0].Field)
	}
}
